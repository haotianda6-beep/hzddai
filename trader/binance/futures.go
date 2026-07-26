package binance

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"nofx/hook"
	"nofx/logger"
	"nofx/trader/types"
	"strings"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/common"
	"github.com/adshao/go-binance/v2/futures"
	"golang.org/x/net/proxy"
)

const (
	// BaseApiDemoURL 币安 U 本位合约虚拟盘（官方 Demo Trading REST）
	BaseApiDemoURL = "https://demo-fapi.binance.com"
	// Binance signed REST requests can queue behind the shared direct-IP limiter during startup bursts.
	binanceSignedRecvWindowMS int64 = 60000
)

// getBrOrderID generates unique order ID (for futures contracts)
// Format: x-{BR_ID}{TIMESTAMP}{RANDOM}
// Futures limit is 32 characters, use this limit consistently
// Uses nanosecond timestamp + random number to ensure global uniqueness (collision probability < 10^-20)
func getBrOrderID() string {
	brID := futuresBrokerOrderTag()

	// 前缀 "x-" + 8 位经纪商标识 = 10 字符；剩余最多 22 字符给时间戳与随机段（本实现 13+8=21）
	timestamp := time.Now().UnixNano() % 10000000000000 // 13-digit nanosecond timestamp

	// Generate 4-byte random number (8 hex digits)
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)

	// Example: x-6UajqqpR{13-digit timestamp}{8 hex random}
	orderID := fmt.Sprintf("x-%s%d%s", brID, timestamp, randomHex)

	// Ensure not exceeding 32-character limit (theoretically exactly 31 characters)
	if len(orderID) > 32 {
		orderID = orderID[:32]
	}

	return orderID
}

// FuturesTrader Binance futures trader
type FuturesTrader struct {
	client               *futures.Client
	settingsMu           sync.RWMutex
	leverageBySymbol     map[string]int
	marginModeBySymbol   map[string]bool
	leverageBracketCache map[string]leverageBracketCacheEntry

	// Balance cache
	cachedBalance     map[string]interface{}
	balanceCacheTime  time.Time
	balanceCacheMutex sync.RWMutex

	// Position cache
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex

	// Open orders cache (primarily maintained by Binance user data websocket)
	cachedOpenOrders     []types.OpenOrder
	openOrdersCacheTime  time.Time
	openOrdersCacheMutex sync.RWMutex

	// Binance rate-limit protection
	rateLimitUntil time.Time
	rateLimitMutex sync.RWMutex

	// User data stream lifecycle
	userDataStreamMutex   sync.Mutex
	userDataStreamStarted bool
	userDataStreamStopC   chan struct{}

	// U 本位 WebSocket API（order.place），供镜像跟单在 COMKUN_MIRROR_V2_WS_ORDER=1 时走市价/平仓等（与主控 JSON v1/v2 引擎字段独立）
	wsAPIClient *FuturesWSAPIClient
	wsAPIMu     sync.Mutex

	// COMKUN 主控：用户数据流 ACCOUNT_UPDATE 后回调（非阻塞），用于尽快发持仓快照广播
	comkunMasterSnapshotNotifyMu sync.Mutex
	comkunMasterSnapshotNotify   func()

	// useDemoTrading 为 true 时 REST 走 demo-fapi（虚拟盘）；与主网账户隔离
	useDemoTrading bool

	// Cache validity — 余额类 REST（可与持仓分开降频）
	cacheDuration time.Duration
	// 持仓 REST（/fapi/v2/positionRisk）单独更长 TTL：同 IP 多账号时显著减少 -1003
	positionsCacheDuration time.Duration
}

// applyOutboundProxyToFuturesClient 让币安 REST 走独立出口 IP（缓解同服务器多账号共享出口触发 -1003）。
// 支持 http(s):// 与 socks5://（含账号密码）。
// 说明：listenKey 创建/续期、下单、撤单、REST 拉仓位等均走此 HTTPClient → 与账户绑定的出口代理一致；
// User Stream 的 **行情 WS** 连接默认直连（go-binance）；若 WS 也须固定出口，需在库层替换 Dialer（当前未接）。
// 降频要点：FuturesTrader 已拉长持仓 REST TTL、依赖 WS 推送刷新缓存；跟单侧 mirrorCancelSymbolSleep / 限价下单间隔见 comkun_env_timing.go。
func applyOutboundProxyToFuturesClient(client *futures.Client, proxyRaw string) {
	proxyRaw = strings.TrimSpace(proxyRaw)
	if proxyRaw == "" {
		return
	}
	u, err := url.Parse(proxyRaw)
	if err != nil {
		logger.Warnf("Binance 出口代理 URL 无效，忽略: %v", err)
		return
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https":
		tr := &http.Transport{
			Proxy:           http.ProxyURL(u),
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}
		client.HTTPClient = &http.Client{Transport: tr, Timeout: 60 * time.Second}
		logger.Infof("📡 Binance REST 使用 HTTP(s) 出口代理")
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(u, proxy.Direct)
		if err != nil {
			logger.Warnf("Binance SOCKS5 代理不可用，忽略: %v", err)
			return
		}
		cd, ok := dialer.(proxy.ContextDialer)
		if !ok {
			logger.Warnf("Binance SOCKS5 dialer 不支持 ContextDialer，忽略代理")
			return
		}
		tr := &http.Transport{
			DialContext:     cd.DialContext,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}
		client.HTTPClient = &http.Client{Transport: tr, Timeout: 60 * time.Second}
		logger.Infof("📡 Binance REST 使用 SOCKS5 出口代理")
	default:
		logger.Warnf("Binance 出口代理 scheme %q 不支持（请用 http、https、socks5）", u.Scheme)
	}
}

// ensureFuturesClientWeightTracking 无代理时也为默认 HTTPClient 挂上权重记录 Transport（与有代理时一致）。
func ensureFuturesClientWeightTracking(client *futures.Client, outboundProxyURL string, fault *ProxyFaultMeta) {
	if client == nil {
		return
	}
	if client.HTTPClient == nil {
		client.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	ensureWeightTrackingOnHTTPClient(client.HTTPClient, outboundProxyURL, fault)
}

// NewFuturesTrader creates futures trader
// testnet=true 时使用币安合约虚拟盘 REST（demo-fapi.binance.com）。
// outboundProxyURL 可选；非空时仅作用于 REST（见 applyOutboundProxyToFuturesClient）。
// identity 可选：用于出口代理故障时写入管理端告警（trader_id / exchange_id）。
func NewFuturesTrader(apiKey, secretKey string, userId string, outboundProxyURL string, testnet bool, identity ...ProxyFaultMeta) *FuturesTrader {
	var fault *ProxyFaultMeta
	if len(identity) > 0 {
		m := identity[0]
		fault = &m
	}
	if fault != nil && strings.TrimSpace(fault.ProxyURL) == "" {
		fault.ProxyURL = outboundProxyURL
	}
	client := futures.NewClient(apiKey, secretKey)
	// Auto-detect Ed25519 PEM key and override KeyType (SDK defaults to HMAC)
	// Normalize PEM (fixes single-line format from web form that strips newlines)
	if isEd25519PEM(secretKey) {
		secretKey = normalizePEM(secretKey)
		client.SecretKey = secretKey
		client.KeyType = common.KeyTypeEd25519
	}

	hookRes := hook.HookExec[hook.NewBinanceTraderResult](hook.NEW_BINANCE_TRADER, userId, client)
	if hookRes != nil && hookRes.GetResult() != nil {
		client = hookRes.GetResult()
	}

	if testnet {
		client.BaseURL = BaseApiDemoURL
		logger.Infof("📡 Binance Futures 虚拟盘 REST: %s", BaseApiDemoURL)
	}

	applyOutboundProxyToFuturesClient(client, outboundProxyURL)

	// Sync time to avoid "Timestamp ahead" error
	syncBinanceServerTime(client)
	ensureFuturesClientWeightTracking(client, outboundProxyURL, fault)
	trader := &FuturesTrader{
		client:                 client,
		useDemoTrading:         testnet,
		cacheDuration:          35 * time.Second,  // 余额 / account REST
		positionsCacheDuration: 120 * time.Second, // 持仓 REST：限 IP 时拉长避免轮询打爆（WS ACCOUNT_UPDATE 仍会刷新缓存）
	}

	// Set dual-side position mode (Hedge Mode)
	// This is required because the code uses PositionSide (LONG/SHORT)
	if err := trader.setDualSidePosition(); err != nil {
		logger.Infof("⚠️ Failed to set dual-side position mode: %v (ignore this warning if already in dual-side mode)", err)
	}

	return trader
}

// setDualSidePosition sets dual-side position mode (called during initialization)
func (t *FuturesTrader) setDualSidePosition() error {
	// Try to set dual-side position mode
	err := t.client.NewChangePositionModeService().
		DualSide(true). // true = dual-side position (Hedge Mode)
		Do(context.Background(), futures.WithRecvWindow(binanceSignedRecvWindowMS))

	if err != nil {
		// If error message contains "No need to change", it means already in dual-side position mode
		if strings.Contains(err.Error(), "No need to change position side") {
			logger.Infof("  ✓ Account is already in dual-side position mode (Hedge Mode)")
			return nil
		}
		// Other errors are returned (but won't interrupt initialization in the caller)
		return err
	}

	logger.Infof("  ✓ Account has been switched to dual-side position mode (Hedge Mode)")
	logger.Infof("  ℹ️  Dual-side position mode allows holding both long and short positions simultaneously")
	return nil
}

// syncBinanceServerTime syncs Binance server time to ensure request timestamps are valid
func syncBinanceServerTime(client *futures.Client) {
	serverTime, err := client.NewServerTimeService().Do(context.Background())
	if err != nil {
		logger.Infof("⚠️ Failed to sync Binance server time: %v", err)
		return
	}

	now := time.Now().UnixMilli()
	offset := now - serverTime
	client.TimeOffset = offset
	logger.Infof("⏱ Binance server time synced, offset %dms", offset)
}

func (t *FuturesTrader) markRateLimited(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.Contains(msg, "-1003") && !strings.Contains(strings.ToLower(msg), "too many requests") {
		return
	}
	t.rateLimitMutex.Lock()
	t.rateLimitUntil = time.Now().Add(120 * time.Second)
	t.rateLimitMutex.Unlock()
	logger.Warnf("⚠️ Binance rate limit detected; pausing REST reads until %s", t.rateLimitUntil.Format(time.RFC3339))
}

func (t *FuturesTrader) isRateLimited() bool {
	t.rateLimitMutex.RLock()
	until := t.rateLimitUntil
	t.rateLimitMutex.RUnlock()
	return !until.IsZero() && time.Now().Before(until)
}

func (t *FuturesTrader) invalidateRealtimeCaches() {
	t.balanceCacheMutex.Lock()
	t.cachedBalance = nil
	t.balanceCacheTime = time.Time{}
	t.balanceCacheMutex.Unlock()

	t.positionsCacheMutex.Lock()
	t.cachedPositions = nil
	t.positionsCacheTime = time.Time{}
	t.positionsCacheMutex.Unlock()
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// calculatePrecision calculates precision from stepSize
func calculatePrecision(stepSize string) int {
	// Remove trailing zeros
	stepSize = trimTrailingZeros(stepSize)

	// Find decimal point
	dotIndex := -1
	for i := 0; i < len(stepSize); i++ {
		if stepSize[i] == '.' {
			dotIndex = i
			break
		}
	}

	// If no decimal point or decimal point is at the end, precision is 0
	if dotIndex == -1 || dotIndex == len(stepSize)-1 {
		return 0
	}

	// Return number of digits after decimal point
	return len(stepSize) - dotIndex - 1
}

// trimTrailingZeros removes trailing zeros
func trimTrailingZeros(s string) string {
	// If no decimal point, return directly
	if !stringContains(s, ".") {
		return s
	}

	// Iterate backwards to remove trailing zeros
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}

	// If last character is decimal point, remove it too
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}

	return s
}
