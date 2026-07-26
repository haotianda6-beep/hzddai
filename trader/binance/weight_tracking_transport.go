package binance

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nofx/logger"
)

// 币安 U 本位合约 REST：响应头 X-MBX-USED-WEIGHT-1m 表示当前 IP（或连接维度）近 1 分钟已消耗权重。
// 同机多账号若共用同一 HTTP(s)/SOCKS5 出口，权重在该出口上累加，易触发 -1003；此处按「代理 URL 字符串」聚类记录并软限速。
// 文档口径随 Binance 调整，可通过环境变量覆盖阈值。

var (
	weightSoftThreshold   int = 2000 // 接近上限前轻微礼让
	weightHardThreshold   int = 2350 // 触发较长冷却（默认低于常见 2400 IP 上限）
	weightMaxReference    int = 2400 // 仅用于日志百分比
	weightLogMinInterval      = 45 * time.Second
	directRESTMinInterval time.Duration
	blockDirectREST       bool
)

func init() {
	parseWeightEnvInt("BINANCE_FUTURES_WEIGHT_SOFT", 100, 2399, &weightSoftThreshold)
	parseWeightEnvInt("BINANCE_FUTURES_WEIGHT_HARD", weightSoftThreshold, 2400, &weightHardThreshold)
	parseWeightEnvInt("BINANCE_FUTURES_WEIGHT_REF_MAX", 1000, 10000, &weightMaxReference)
	if v := strings.TrimSpace(os.Getenv("BINANCE_WEIGHT_LOG_INTERVAL_SEC")); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec >= 5 && sec <= 600 {
			weightLogMinInterval = time.Duration(sec) * time.Second
		}
	}
	if v := strings.TrimSpace(os.Getenv("BINANCE_DIRECT_REST_MIN_INTERVAL_MS")); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms >= 0 && ms <= 10000 {
			directRESTMinInterval = time.Duration(ms) * time.Millisecond
			if directRESTMinInterval > 0 {
				logger.Infof("binance weight: BINANCE_DIRECT_REST_MIN_INTERVAL_MS=%d", ms)
			}
		} else {
			logger.Warnf("binance weight: BINANCE_DIRECT_REST_MIN_INTERVAL_MS=%q 无效（需 0–10000），保持关闭", v)
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BINANCE_BLOCK_DIRECT_REST"))) {
	case "1", "true", "yes", "on":
		blockDirectREST = true
		logger.Warnf("binance weight: BINANCE_BLOCK_DIRECT_REST enabled; Binance REST requires outbound_proxy_url")
	}
}

func parseWeightEnvInt(key string, min, max int, target *int) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		logger.Warnf("binance weight: %s=%q 无效（需 %d–%d），保持 %d", key, raw, min, max, *target)
		return
	}
	*target = n
	logger.Infof("binance weight: %s=%d", key, n)
}

type weightGate struct {
	mu            sync.Mutex
	lastUsed      int
	lastAt        time.Time
	chillUntil    time.Time
	nextAllowedAt time.Time
	lastLogAt     time.Time
	totalRespSeen int64
}

var weightGateByKey sync.Map // string -> *weightGate

var (
	binanceBanUntilMS int64
	binanceBanUntilRe = regexp.MustCompile(`banned until ([0-9]{10,})`)
)

func gateForKey(key string) *weightGate {
	if key == "" {
		key = "direct"
	}
	if v, ok := weightGateByKey.Load(key); ok {
		return v.(*weightGate)
	}
	g := &weightGate{}
	actual, _ := weightGateByKey.LoadOrStore(key, g)
	return actual.(*weightGate)
}

// WeightProxyKey 将 outbound_proxy_url 规范为权重统计桶键；相同字符串共用同一 IP 权重池（你的代理模式）。
func WeightProxyKey(outboundProxyURL string) string {
	s := strings.TrimSpace(outboundProxyURL)
	if s == "" {
		return "direct"
	}
	return s
}

type weightTrackingRoundTripper struct {
	base     http.RoundTripper
	proxyKey string
	fault    *ProxyFaultMeta
}

func newWeightTrackingTransport(base http.RoundTripper, proxyKey string, fault *ProxyFaultMeta) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &weightTrackingRoundTripper{base: base, proxyKey: WeightProxyKey(proxyKey), fault: fault}
}

func (w *weightTrackingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := waitBeforeBinanceREST(req.Context(), w.proxyKey, binanceRESTWaitBudget(req)); err != nil {
		return nil, err
	}

	resp, err := w.base.RoundTrip(req)
	if err != nil {
		recordProxyTransportError(w.fault, err)
	}
	if resp != nil {
		recordBinanceBanResponse(resp)
		recordBinanceWeightHeaders(w.proxyKey, resp.Header, resp.StatusCode)
	}
	return resp, err
}

func recordBinanceBanResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil || (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusTeapot) {
		return
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))

	m := binanceBanUntilRe.FindSubmatch(body)
	if len(m) < 2 {
		return
	}
	ms, err := strconv.ParseInt(string(m[1]), 10, 64)
	if err != nil || ms <= time.Now().UnixMilli() {
		return
	}
	for {
		old := atomic.LoadInt64(&binanceBanUntilMS)
		if ms <= old || atomic.CompareAndSwapInt64(&binanceBanUntilMS, old, ms) {
			break
		}
	}
	logger.Warnf("⚠️ Binance REST 本机出口已被临时限制，冷却至 %s，期间本地停止 REST 请求",
		time.UnixMilli(ms).Format(time.RFC3339))
}

func binanceRESTWaitBudget(req *http.Request) time.Duration {
	if req == nil || req.URL == nil {
		return 0
	}
	q := req.URL.Query()
	if q.Get("timestamp") == "" || q.Get("signature") == "" {
		return 0
	}
	return 45 * time.Second
}

func waitBeforeBinanceREST(ctx context.Context, proxyKey string, maxWait time.Duration) error {
	if untilMS := atomic.LoadInt64(&binanceBanUntilMS); untilMS > 0 {
		if until := time.UnixMilli(untilMS); time.Now().Before(until) {
			return fmt.Errorf("binance REST temporarily blocked until %s", until.Format(time.RFC3339))
		}
	}
	key := WeightProxyKey(proxyKey)
	if key == "direct" && blockDirectREST {
		return fmt.Errorf("binance REST direct outbound disabled; configure outbound_proxy_url")
	}
	g := gateForKey(key)
	started := time.Now()
	for {
		g.mu.Lock()
		now := time.Now()
		waitUntil := g.chillUntil
		if key == "direct" && directRESTMinInterval > 0 && g.nextAllowedAt.After(waitUntil) {
			waitUntil = g.nextAllowedAt
		}
		wait := waitUntil.Sub(now)
		if wait <= 0 {
			if key == "direct" && directRESTMinInterval > 0 {
				jitter := time.Duration(rand.Intn(80)) * time.Millisecond
				g.nextAllowedAt = now.Add(directRESTMinInterval + jitter)
			}
			g.mu.Unlock()
			return nil
		}
		if maxWait > 0 {
			remaining := maxWait - now.Sub(started)
			if remaining <= 0 {
				g.mu.Unlock()
				return nil
			}
			if wait > remaining {
				wait = remaining
			}
		}
		g.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func recordBinanceWeightHeaders(proxyKey string, h http.Header, statusCode int) {
	if h == nil {
		return
	}
	used := parseMBXUsedWeight1m(h)
	if used < 0 {
		return
	}
	g := gateForKey(proxyKey)
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.totalRespSeen++
	g.lastUsed = used
	g.lastAt = now

	pct := 0
	if weightMaxReference > 0 {
		pct = used * 100 / weightMaxReference
		if pct > 100 {
			pct = 100
		}
	}

	shouldLog := now.Sub(g.lastLogAt) >= weightLogMinInterval
	if used >= weightSoftThreshold {
		shouldLog = true
	}

	if shouldLog {
		g.lastLogAt = now
		tag := proxyTagForLog(proxyKey)
		logger.Infof("📊 Binance REST 权重 [%s] used_weight_1m=%d (~%d%% ref_max=%d) http=%d",
			tag, used, pct, weightMaxReference, statusCode)
	}

	chill := binanceWeightChillDuration(used)
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusTeapot {
		chill = 50 * time.Second
	}
	if chill > 0 {
		until := now.Add(chill)
		if until.After(g.chillUntil) {
			g.chillUntil = until
		}
	}

	switch {
	case used >= 2300:
		logger.Warnf("⚠️ Binance REST 权重[%s] 极高 %d/%d，已进入约 %.1fs 串行退避，避免 IP 被限流",
			proxyTagForLog(proxyKey), used, weightMaxReference, chill.Seconds())
	case used >= weightHardThreshold:
		logger.Warnf("⚠️ Binance REST 权重[%s] 已达硬阈值 %d≥%d，已进入约 %.1fs 串行退避，避免 -1003",
			proxyTagForLog(proxyKey), used, weightHardThreshold, chill.Seconds())
	}
}

func binanceWeightChillDuration(used int) time.Duration {
	switch {
	case used >= 2300:
		return time.Duration(25000+rand.Intn(5000)) * time.Millisecond
	case used >= 2200:
		return time.Duration(12000+rand.Intn(4000)) * time.Millisecond
	case used >= 2000:
		return time.Duration(6000+rand.Intn(2000)) * time.Millisecond
	case used >= weightHardThreshold:
		return time.Duration(1500+rand.Intn(1000)) * time.Millisecond
	case used >= weightSoftThreshold:
		return time.Duration(300+rand.Intn(300)) * time.Millisecond
	default:
		return 0
	}
}

func parseMBXUsedWeight1m(h http.Header) int {
	if h == nil {
		return -1
	}
	best := -1
	for k, vals := range h {
		if !strings.EqualFold(k, "X-MBX-USED-WEIGHT-1m") || len(vals) == 0 {
			continue
		}
		for _, s := range vals {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				continue
			}
			if n > best {
				best = n
			}
		}
	}
	return best
}

func proxyTagForLog(proxyKey string) string {
	if proxyKey == "" || proxyKey == "direct" {
		return "direct(本机出口)"
	}
	if len(proxyKey) <= 24 {
		return proxyKey
	}
	return proxyKey[:12] + "…" + proxyKey[len(proxyKey)-8:]
}

// ensureWeightTrackingOnHTTPClient 为 go-binance Client 包装 Transport，无代理时也要统计「本机出口」权重。
func ensureWeightTrackingOnHTTPClient(httpClient *http.Client, outboundProxyURL string, fault *ProxyFaultMeta) {
	if httpClient == nil {
		return
	}
	base := httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if _, ok := base.(*weightTrackingRoundTripper); ok {
		return
	}
	httpClient.Transport = newWeightTrackingTransport(base, outboundProxyURL, fault)
}
