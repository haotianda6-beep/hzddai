package trader

import (
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	"nofx/mcp/payment"
	_ "nofx/mcp/provider"
	"nofx/store"
	"nofx/trader/aster"
	"nofx/trader/binance"
	"nofx/trader/bitget"
	"nofx/trader/bybit"
	"nofx/trader/gate"
	"nofx/trader/hyperliquid"
	"nofx/trader/hz"
	"nofx/trader/indodax"
	"nofx/trader/kucoin"
	"nofx/trader/lighter"
	"nofx/trader/okx"
	"nofx/wallet"
	"os"
	"strings"
	"sync"
	"time"
)

// AutoTraderConfig auto trading configuration (simplified version - AI makes all decisions)
type AutoTraderConfig struct {
	// Trader identification
	ID      string // Trader unique identifier (for log directory, etc.)
	Name    string // Trader display name
	AIModel string // AI model: "qwen" or "deepseek"

	// Trading platform selection
	Exchange         string // Exchange type: "binance", "bybit", "okx", "bitget", "gate", "hyperliquid", "aster" or "lighter"
	ExchangeID       string // Exchange account UUID (for multi-account support)
	OutboundProxyURL string // 可选：CEX REST 出口代理（http(s)/socks5）

	// Binance API configuration
	BinanceAPIKey           string
	BinanceSecretKey        string
	BinanceTestnet          bool   // 币安合约虚拟盘（demo-fapi）
	BinanceOutboundProxyURL string // 可选：HTTP(s)/SOCKS5，REST 走独立出口 IP

	// Bybit API configuration
	BybitAPIKey    string
	BybitSecretKey string

	// OKX API configuration
	OKXAPIKey     string
	OKXSecretKey  string
	OKXPassphrase string

	// Bitget API configuration
	BitgetAPIKey     string
	BitgetSecretKey  string
	BitgetPassphrase string

	// Gate API configuration
	GateAPIKey    string
	GateSecretKey string
	GateTestnet   bool

	// KuCoin API configuration
	KuCoinAPIKey     string
	KuCoinSecretKey  string
	KuCoinPassphrase string

	// Indodax API configuration
	IndodaxAPIKey    string
	IndodaxSecretKey string

	// HZ trading account configuration
	HZAPIURL    string
	HZAPIKey    string
	HZSecretKey string

	// Hyperliquid configuration
	HyperliquidPrivateKey  string
	HyperliquidWalletAddr  string
	HyperliquidTestnet     bool
	HyperliquidUnifiedAcct bool // Unified Account mode: Spot USDC as Perp collateral

	// Aster configuration
	AsterUser       string // Aster main wallet address
	AsterSigner     string // Aster API wallet address
	AsterPrivateKey string // Aster API wallet private key

	// LIGHTER configuration
	LighterWalletAddr       string // LIGHTER wallet address (L1 wallet)
	LighterPrivateKey       string // LIGHTER L1 private key (for account identification)
	LighterAPIKeyPrivateKey string // LIGHTER API Key private key (40 bytes, for transaction signing)
	LighterAPIKeyIndex      int    // LIGHTER API Key index (0-255)
	LighterTestnet          bool   // Whether to use testnet

	// AI configuration
	UseQwen     bool
	DeepSeekKey string
	QwenKey     string

	// Custom AI API configuration
	CustomAPIURL    string
	CustomAPIKey    string
	CustomModelName string

	// Scan configuration
	ScanInterval time.Duration // Scan interval (recommended 3 minutes)

	// Account configuration
	InitialBalance float64 // Initial balance (for P&L calculation, must be set manually)

	// Risk control (only as hints, AI can make autonomous decisions)
	MaxDailyLoss    float64       // Maximum daily loss percentage (hint)
	MaxDrawdown     float64       // Maximum drawdown percentage (hint)
	StopTradingTime time.Duration // Pause duration after risk control triggers

	// Position mode
	IsCrossMargin bool // true=cross margin mode, false=isolated margin mode

	// Competition visibility
	ShowInCompetition bool // Whether to show in competition page

	// Strategy configuration (use complete strategy config)
	StrategyConfig *store.StrategyConfig // Strategy configuration (includes coin sources, indicators, risk control, prompts, etc.)

	// StrategyID 数据库 strategies.id（与 comkun 广播 source_strategy_id 一致，用于主控自动发布跟单广播）
	StrategyID string
}

// AutoTrader automatic trader
type AutoTrader struct {
	id                    string // Trader unique identifier
	name                  string // Trader display name
	aiModel               string // AI model name
	exchange              string // Trading platform type (binance/bybit/etc)
	exchangeID            string // Exchange account UUID
	showInCompetition     bool   // Whether to show in competition page
	config                AutoTraderConfig
	trader                Trader // Use Trader interface (supports multiple platforms)
	mcpClient             mcp.AIClient
	store                 *store.Store           // Data storage (decision records, etc.)
	strategyEngine        *kernel.StrategyEngine // Strategy engine (uses strategy configuration)
	cycleNumber           int                    // Current cycle number
	initialBalance        float64
	dailyPnL              float64
	customPrompt          string // Custom trading strategy prompt
	overrideBasePrompt    bool   // Whether to override base prompt
	lastResetTime         time.Time
	stopUntil             time.Time
	isRunning             bool
	isRunningMutex        sync.RWMutex       // Mutex to protect isRunning flag
	startTime             time.Time          // System start time
	callCount             int                // AI call count
	positionFirstSeenTime map[string]int64   // Position first seen time (symbol_side -> timestamp in milliseconds)
	stopMonitorCh         chan struct{}      // Used to stop monitoring goroutine
	stopMonitorClosed     bool               // 防止重复 close panic
	stopChMu              sync.Mutex         // 保护 stopMonitorCh 的创建与关闭
	monitorWg             sync.WaitGroup     // Used to wait for monitoring goroutine to finish
	peakPnLCache          map[string]float64 // Peak profit cache (symbol -> peak P&L percentage)
	peakPnLCacheMutex     sync.RWMutex       // Cache read-write lock
	lastBalanceSyncTime   time.Time          // Last balance sync time
	userID                string             // User ID
	gridState             *GridState         // Grid trading state (only used when StrategyType == "grid_trading")
	claw402WalletAddr     string             // Claw402 wallet address (derived from private key at start)
	consecutiveAIFailures int                // Consecutive AI call failures
	safeMode              bool               // Safe mode: no new positions, protect existing ones
	safeModeReason        string             // Why safe mode was activated

	// 程序化马丁：首仓锚定价与方向（无仓时清零；落盘到 data/martingale_state）
	martingaleMu             sync.Mutex
	martingaleAnchorPrice    float64
	martingaleSide           string
	martingaleDayStartEquity float64
	martingaleDayReset       time.Time
	martingaleDailyPaused    bool

	// Comkun 跟单：已完整同步过的主控广播 id；有新 id 才拉交易所/扣费（被控随主控扫描记录驱动，不空转）
	lastComkunConsumedBroadcastMu sync.Mutex
	lastComkunConsumedBroadcastID uint64
	// 镜像限价「名片」去重：与上一轮完全相同的限价集合指纹时不重复写 decision_json 卡片
	lastComkunMirrorLimitFPMu sync.Mutex
	lastComkunMirrorLimitFP   string
	// 主控空快照二次确认：避免交易所 API 偶发返回空持仓/空挂单时把被控误清仓
	comkunEmptySnapshotPending map[string]bool
	// 主控（币安）：用户数据流 ACCOUNT_UPDATE 后唤醒快照广播，与定时 ticker 并行以降低跟单延迟
	comkunMasterPokeCh          chan struct{}
	comkunMasterLastRESTRefresh time.Time // 用户流在线时周期性 REST 对账，避免每 5s Invalidate 打穿 IP 限频
	// 跟单被控：进程内主控广播落库唤醒（与 comkun_follow_wake 配合），避免干等轮询间隔
	comkunFollowWakeCh chan struct{}
	// COMKUN 展示节奏：执行可 10 秒轮询，但前端输出按用户配置的扫描间隔节流。
	lastComkunDisplayAt           time.Time
	lastComkunOfflineChargeAt     time.Time
	pendingComkunDisplayMu        sync.Mutex
	pendingComkunDisplayDecisions []kernel.Decision
	pendingComkunDisplayFP        string
	pendingComkunDisplayCoT       string
	shownComkunDisplayFP          map[string]bool

	// 镜像跟单「安全第一」：主控无仓须连续多轮确认才市价全平；市价操作按合约冷却，避免快照抖动反复打脸。
	mirrorSafetyMu           sync.Mutex
	mirrorMasterFlatStreak   map[string]int       // posKey -> 主控已连续多少轮不再持有该腿
	mirrorLastMirrorMarketAt map[string]time.Time // symbol -> 上次镜像市价全平成功时间（仅用于全平冷却；加仓不再更新，避免挡跟平）
	// mirrorSchemeAPostSeedCyclesRemaining：restore 后若为全新被控（水位线从 0 提到最新），首轮镜像跳过 TP/SL（方案A）；成功执行一轮后归零。
	mirrorSchemeAPostSeedCyclesRemaining int

	// mirrorSeedBaselineQty: API 被控启动水位线快照中的缩放目标仓位；仅防冷启动追主控已有仓，不拦运行中主控新开/补仓。
	mirrorSeedBaselineQty map[string]float64
	// mirrorWebStartupAlignPending: bn-screen-mirror 被控启动后首轮 Phase4 按目标 mq 全仓对齐（放宽差额最小名义与 5% 阈值）
	mirrorWebStartupAlignPending bool
	// mirrorWebHighPnLSkipKeys: 网页镜像被控启动时主控浮盈的腿跳过市价进场，等主控平掉或新开其它仓
	mirrorWebHighPnLSkipKeys map[string]bool
}

// NewAutoTrader creates an automatic trader
// st parameter is used to store decision records to database
func NewAutoTrader(config AutoTraderConfig, st *store.Store, userID string) (*AutoTrader, error) {
	// Set default values
	if config.ID == "" {
		config.ID = "default_trader"
	}
	if config.Name == "" {
		config.Name = "Default Trader"
	}
	if config.AIModel == "" {
		if config.UseQwen {
			config.AIModel = "qwen"
		} else {
			config.AIModel = "deepseek"
		}
	}

	// Initialize AI client based on provider
	var mcpClient mcp.AIClient
	aiModel := config.AIModel
	if config.UseQwen && aiModel == "" {
		aiModel = "qwen"
	}

	// Resolve API key (provider-specific overrides)
	apiKey := config.CustomAPIKey
	customURL := config.CustomAPIURL
	switch aiModel {
	case "qwen":
		if config.QwenKey != "" {
			apiKey = config.QwenKey
		}
	case "deepseek", "":
		if config.DeepSeekKey != "" {
			apiKey = config.DeepSeekKey
		}
	case mcp.ProviderComkunProxy:
		apiKey = os.Getenv("PLATFORM_CLAW402_WALLET_KEY")
	}

	// Create client via registry (covers all registered providers)
	if aiModel == "custom" {
		mcpClient = mcp.New()
	} else if aiModel == "" {
		aiModel = "deepseek"
		mcpClient = mcp.NewAIClientByProvider(aiModel)
	} else {
		mcpClient = mcp.NewAIClientByProvider(aiModel)
	}
	if mcpClient == nil {
		mcpClient = mcp.New()
	}

	// Payment providers (claw402) ignore customURL
	switch aiModel {
	case "claw402", mcp.ProviderComkunProxy:
		mcpClient.SetAPIKey(apiKey, "", config.CustomModelName)
	default:
		mcpClient.SetAPIKey(apiKey, customURL, config.CustomModelName)
	}
	if billingClient, ok := mcpClient.(interface {
		ConfigurePlatformBilling(payment.PlatformBillingConfig)
	}); ok && aiModel == mcp.ProviderComkunProxy {
		billingClient.ConfigurePlatformBilling(payment.PlatformBillingConfig{
			Store:    st,
			UserID:   userID,
			TraderID: config.ID,
			Provider: mcp.ProviderComkunProxy,
			Model:    config.CustomModelName,
		})
	}
	logger.Infof("🤖 [%s] Using %s AI", config.Name, aiModel)

	if config.CustomAPIURL != "" || config.CustomModelName != "" {
		logger.Infof("🔧 [%s] Custom config - URL: %s, Model: %s", config.Name, config.CustomAPIURL, config.CustomModelName)
	}

	// Set default trading platform
	if config.Exchange == "" {
		config.Exchange = "binance"
	}

	// Create corresponding trader based on configuration
	var trader Trader
	var err error
	outboundProxyURL := strings.TrimSpace(config.OutboundProxyURL)
	if outboundProxyURL == "" {
		outboundProxyURL = strings.TrimSpace(config.BinanceOutboundProxyURL)
	}

	// Record position mode (general)
	marginModeStr := "Cross Margin"
	if !config.IsCrossMargin {
		marginModeStr = "Isolated Margin"
	}
	logger.Infof("📊 [%s] Position mode: %s", config.Name, marginModeStr)

	switch config.Exchange {
	case "binance":
		if config.BinanceTestnet {
			logger.Infof("🏦 [%s] Using Binance Futures demo trading (virtual)", config.Name)
		} else {
			logger.Infof("🏦 [%s] Using Binance Futures trading", config.Name)
		}
		trader = binance.NewFuturesTrader(config.BinanceAPIKey, config.BinanceSecretKey, userID, outboundProxyURL, config.BinanceTestnet, binance.ProxyFaultMeta{
			UserID:     userID,
			TraderID:   config.ID,
			ExchangeID: config.ExchangeID,
		})
	case "bybit":
		logger.Infof("🏦 [%s] Using Bybit Futures trading", config.Name)
		trader = bybit.NewBybitTrader(config.BybitAPIKey, config.BybitSecretKey, outboundProxyURL)
	case "okx":
		logger.Infof("🏦 [%s] Using OKX Futures trading", config.Name)
		trader = okx.NewOKXTrader(config.OKXAPIKey, config.OKXSecretKey, config.OKXPassphrase, outboundProxyURL)
	case "bitget":
		logger.Infof("🏦 [%s] Using Bitget Futures trading", config.Name)
		trader = bitget.NewBitgetTrader(config.BitgetAPIKey, config.BitgetSecretKey, config.BitgetPassphrase, outboundProxyURL)
	case "gate":
		if config.GateTestnet {
			logger.Infof("🏦 [%s] Using Gate.io Futures testnet (virtual)", config.Name)
		} else {
			logger.Infof("🏦 [%s] Using Gate.io Futures trading", config.Name)
		}
		trader = gate.NewGateTraderWithTestnet(config.GateAPIKey, config.GateSecretKey, config.GateTestnet, outboundProxyURL)
	case "kucoin":
		logger.Infof("🏦 [%s] Using KuCoin Futures trading", config.Name)
		trader = kucoin.NewKuCoinTrader(config.KuCoinAPIKey, config.KuCoinSecretKey, config.KuCoinPassphrase)
	case "hyperliquid":
		logger.Infof("🏦 [%s] Using Hyperliquid trading", config.Name)
		trader, err = hyperliquid.NewHyperliquidTrader(config.HyperliquidPrivateKey, config.HyperliquidWalletAddr, config.HyperliquidTestnet, config.HyperliquidUnifiedAcct)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Hyperliquid trader: %w", err)
		}
	case "aster":
		logger.Infof("🏦 [%s] Using Aster trading", config.Name)
		trader, err = aster.NewAsterTrader(config.AsterUser, config.AsterSigner, config.AsterPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Aster trader: %w", err)
		}
	case "lighter":
		logger.Infof("🏦 [%s] Using LIGHTER trading", config.Name)

		if config.LighterWalletAddr == "" || config.LighterAPIKeyPrivateKey == "" {
			return nil, fmt.Errorf("Lighter requires wallet address and API Key private key")
		}

		// Lighter only supports mainnet (testnet disabled)
		trader, err = lighter.NewLighterTraderV2(
			config.LighterWalletAddr,
			config.LighterAPIKeyPrivateKey,
			config.LighterAPIKeyIndex,
			false, // Always use mainnet for Lighter
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize LIGHTER trader: %w", err)
		}
		logger.Infof("✓ LIGHTER trader initialized successfully")
	case "indodax":
		logger.Infof("🏦 [%s] Using Indodax Spot trading", config.Name)
		trader = indodax.NewIndodaxTrader(config.IndodaxAPIKey, config.IndodaxSecretKey)
	case "hz":
		var hzTrader *hz.Trader
		hzTrader, err = hz.NewTrader(config.HZAPIURL, config.HZAPIKey, config.HZSecretKey, config.IsCrossMargin)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize HZ trader: %w", err)
		}
		if err = market.ConfigureHZ(config.HZAPIURL); err != nil {
			hzTrader.Close()
			return nil, fmt.Errorf("failed to configure HZ market data: %w", err)
		}
		market.SetHZInstruments(hzTrader.InstrumentNames())
		trader = hzTrader
	default:
		return nil, fmt.Errorf("unsupported trading platform: %s", config.Exchange)
	}

	// Validate initial balance configuration, auto-fetch from exchange if 0
	if config.InitialBalance <= 0 {
		logger.Infof("📊 [%s] Initial balance not set, attempting to fetch current balance from exchange...", config.Name)
		account, err := trader.GetBalance()
		if err != nil {
			return nil, fmt.Errorf("initial balance not set and unable to fetch balance from exchange: %w", err)
		}
		// Try multiple balance field names (different exchanges return different formats)
		balanceKeys := []string{"total_equity", "totalWalletBalance", "wallet_balance", "totalEq", "balance"}
		var foundBalance float64
		for _, key := range balanceKeys {
			if balance, ok := account[key].(float64); ok && balance > 0 {
				foundBalance = balance
				break
			}
		}
		if foundBalance > 0 {
			config.InitialBalance = foundBalance
			logger.Infof("✓ [%s] Auto-fetched initial balance: %.2f USDT", config.Name, foundBalance)
			// Save to database so it persists across restarts
			if st != nil {
				if err := st.Trader().UpdateInitialBalance(userID, config.ID, foundBalance); err != nil {
					logger.Infof("⚠️  [%s] Failed to save initial balance to database: %v", config.Name, err)
				} else {
					logger.Infof("✓ [%s] Initial balance saved to database", config.Name)
				}
			}
		} else {
			return nil, fmt.Errorf("initial balance must be greater than 0, please set InitialBalance in config or ensure exchange account has balance")
		}
	}

	// Get last cycle number (for recovery)
	var cycleNumber int
	if st != nil {
		cycleNumber, _ = st.Decision().GetLastCycleNumber(config.ID)
		logger.Infof("📊 [%s] Decision records will be stored to database", config.Name)
	}

	// Create strategy engine (must have strategy config)
	if config.StrategyConfig == nil {
		return nil, fmt.Errorf("[%s] strategy not configured", config.Name)
	}
	// Pass claw402 wallet key to strategy engine so nofxos data requests
	// are routed through claw402 (reuses the same wallet as AI calls)
	var claw402Key string
	if config.AIModel == "claw402" && config.CustomAPIKey != "" {
		claw402Key = config.CustomAPIKey
	} else if config.AIModel == mcp.ProviderComkunProxy {
		claw402Key = os.Getenv("PLATFORM_CLAW402_WALLET_KEY")
	}
	strategyEngine := kernel.NewStrategyEngine(config.StrategyConfig, claw402Key)
	logger.Infof("✓ [%s] Using strategy engine (strategy configuration loaded)", config.Name)

	return &AutoTrader{
		id:                         config.ID,
		name:                       config.Name,
		aiModel:                    config.AIModel,
		exchange:                   config.Exchange,
		exchangeID:                 config.ExchangeID,
		showInCompetition:          config.ShowInCompetition,
		config:                     config,
		trader:                     trader,
		mcpClient:                  mcpClient,
		store:                      st,
		strategyEngine:             strategyEngine,
		cycleNumber:                cycleNumber,
		initialBalance:             config.InitialBalance,
		lastResetTime:              time.Now(),
		startTime:                  time.Now(),
		callCount:                  0,
		isRunning:                  false,
		positionFirstSeenTime:      make(map[string]int64),
		stopMonitorCh:              make(chan struct{}),
		monitorWg:                  sync.WaitGroup{},
		peakPnLCache:               make(map[string]float64),
		peakPnLCacheMutex:          sync.RWMutex{},
		lastBalanceSyncTime:        time.Now(),
		userID:                     userID,
		comkunEmptySnapshotPending: make(map[string]bool),
		shownComkunDisplayFP:       make(map[string]bool),
	}, nil
}

func (at *AutoTrader) isComkunListingMaster() bool {
	return at.config.StrategyConfig != nil &&
		at.config.StrategyConfig.ComkunFollowListingTemplate &&
		!store.IsComkunMarketFollowStrategy(at.config.StrategyConfig)
}

// comkunFollowPollWaitAfterCycle 主循环休眠：被控跟单固定短轮询等主控；非跟单仍用 ScanInterval。
func (at *AutoTrader) comkunFollowPollWaitAfterCycle(scanWait time.Duration) time.Duration {
	if at.config.StrategyConfig != nil && store.IsComkunMarketFollowStrategy(at.config.StrategyConfig) {
		return comkunFollowFollowMasterPollInterval
	}
	if scanWait <= 0 {
		return 3 * time.Minute
	}
	return scanWait
}

func (at *AutoTrader) startComkunMasterSnapshotMonitor() {
	if at.store == nil || !at.isComkunListingMaster() {
		return
	}
	logger.Infof("📡 [%s] comkun 主控轻量快照监控已启动：每 %v 检查一次持仓/挂单变化；币安主控另在用户数据流 ACCOUNT_UPDATE 时唤醒（与跟单广播非同一 WS）", at.name, comkunMasterSnapshotPollInterval)
	at.comkunMasterPokeCh = nil
	if ft, ok := at.trader.(*binance.FuturesTrader); ok {
		ch := make(chan struct{}, 1)
		at.comkunMasterPokeCh = ch
		ft.SetComkunMasterMirrorSnapshotNotify(func() {
			select {
			case ch <- struct{}{}:
			default:
			}
		})
	}
	at.monitorWg.Add(1)
	go func() {
		defer at.monitorWg.Done()
		if ft, ok := at.trader.(*binance.FuturesTrader); ok {
			defer ft.ClearComkunMasterMirrorSnapshotNotify()
		}
		ticker := time.NewTicker(comkunMasterSnapshotPollInterval)
		defer ticker.Stop()
		poke := at.comkunMasterPokeCh
		for {
			if poke != nil {
				select {
				case <-ticker.C:
					at.publishComkunMasterSnapshotIfChanged(false)
				case <-poke:
					// 用户数据流已写入内存持仓缓存；勿再 Invalidate 否则 REST 常滞后数秒～十余秒，快照仍空 → 广播判重跳过 → 跟单极慢
					at.publishComkunMasterSnapshotIfChanged(true)
				case <-at.stopMonitorCh:
					logger.Infof("📡 [%s] comkun 主控轻量快照监控已停止", at.name)
					return
				}
			} else {
				select {
				case <-ticker.C:
					at.publishComkunMasterSnapshotIfChanged(false)
				case <-at.stopMonitorCh:
					logger.Infof("📡 [%s] comkun 主控轻量快照监控已停止", at.name)
					return
				}
			}
		}
	}()
}

func (at *AutoTrader) publishComkunMasterSnapshotIfChanged(skipPositionCacheInvalidate bool) {
	at.isRunningMutex.RLock()
	running := at.isRunning
	at.isRunningMutex.RUnlock()
	if !running || at.store == nil || !at.isComkunListingMaster() {
		return
	}
	// 定时轮询：用户流未连接时仍每轮 REST 强刷；已连接时仅 ~90s 对账一次，其余靠 WS 缓存（防 -1003）。
	// WS 唤醒：保留 ACCOUNT_UPDATE 刚写入的内存缓存，避免 REST 短暂滞后导致整轮不发广播。
	if !skipPositionCacheInvalidate {
		at.maybeInvalidateComkunMasterPositionsForREST()
	}
	ctx, err := at.buildComkunMasterSnapshotContext()
	if err != nil {
		logger.Warnf("comkun master snapshot monitor: 轻量交易所快照失败: %v", err)
		return
	}
	at.maybePublishComkunMasterBroadcast(ctx, nil, nil)
}

// Run runs the automatic trading main loop
func (at *AutoTrader) Run() error {
	at.isRunningMutex.Lock()
	at.isRunning = true
	at.isRunningMutex.Unlock()

	at.stopChMu.Lock()
	at.stopMonitorClosed = false
	at.stopMonitorCh = make(chan struct{})
	at.stopChMu.Unlock()
	at.startTime = time.Now()

	logger.Info("🚀 AI-driven automatic trading system started")
	logger.Infof("💰 Initial balance: %.2f USDT", at.initialBalance)
	logger.Infof("⚙️  Scan interval: %v", at.config.ScanInterval)
	logger.Info("🤖 AI will make full decisions on leverage, position size, stop loss/take profit, etc.")

	// 合规跟单被控：从 DB 恢复「已消费主控广播」，避免容器/进程重启后重复同一条广播的思维链与扣费
	at.restoreComkunFollowConsumedBroadcastID()
	at.seedComkunFollowBroadcastWatermarkOnStart()

	// 同一进程内：主控广播落库后立即唤醒本被控（见 store.NotifyComkunFollowersOfBroadcast）；多实例时仍靠轮询兜底。
	at.comkunFollowWakeCh = nil
	if at.config.StrategyConfig != nil && store.IsComkunMarketFollowStrategy(at.config.StrategyConfig) && at.store != nil {
		src := store.ResolveComkunFollowSourceStrategyID(at.config.StrategyConfig)
		if src != "" {
			ch := make(chan struct{}, 1)
			unreg := store.RegisterComkunFollowWake(src, ch)
			defer unreg()
			at.comkunFollowWakeCh = ch
			logger.Infof("📡 [%s] comkun 跟单被控：已注册主广播进程内唤醒（source_strategy_id=%s）", at.name, src)
		}
	}

	// Pre-launch checks for claw402 users
	at.runPreLaunchChecks()
	at.monitorWg.Add(1)
	defer at.monitorWg.Done()

	// 主控上架模板与被控跟单都不能有后台自动平仓，否则会绕开「主控人工、被控跟快照」的规则。
	if at.config.StrategyConfig != nil &&
		(at.config.StrategyConfig.ComkunFollowListingTemplate || store.IsComkunMarketFollowStrategy(at.config.StrategyConfig)) {
		logger.Infof("🛡️ [%s] comkun 跟单链路：已禁用后台回撤自动平仓监控", at.name)
	} else {
		at.startDrawdownMonitor()
	}
	at.startComkunMasterSnapshotMonitor()

	// Start Lighter order sync if using Lighter exchange
	if at.exchange == "lighter" {
		if lighterTrader, ok := at.trader.(*lighter.LighterTraderV2); ok && at.store != nil {
			lighterTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] Lighter order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Hyperliquid order sync if using Hyperliquid exchange
	if at.exchange == "hyperliquid" {
		if hyperliquidTrader, ok := at.trader.(*hyperliquid.HyperliquidTrader); ok && at.store != nil {
			hyperliquidTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] Hyperliquid order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Bybit order sync if using Bybit exchange
	if at.exchange == "bybit" {
		if bybitTrader, ok := at.trader.(*bybit.BybitTrader); ok && at.store != nil {
			bybitTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] Bybit order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start OKX order sync if using OKX exchange
	if at.exchange == "okx" {
		if okxTrader, ok := at.trader.(*okx.OKXTrader); ok && at.store != nil {
			okxTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] OKX order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Bitget order sync if using Bitget exchange
	if at.exchange == "bitget" {
		if bitgetTrader, ok := at.trader.(*bitget.BitgetTrader); ok && at.store != nil {
			bitgetTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] Bitget order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Aster order sync if using Aster exchange
	if at.exchange == "aster" {
		if asterTrader, ok := at.trader.(*aster.AsterTrader); ok && at.store != nil {
			asterTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] Aster order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start Binance order sync if using Binance exchange
	if at.exchange == "binance" {
		if binanceTrader, ok := at.trader.(*binance.FuturesTrader); ok && at.store != nil {
			binanceTrader.StartUserDataStream(at.id)
			binanceTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 15*time.Minute)
			logger.Infof("🔄 [%s] Binance user stream enabled; REST 成交回补同步每 15m（降权重；实时持仓/挂单依赖 WS）", at.name)
		}
	}

	// Start Gate order sync if using Gate exchange
	if at.exchange == "gate" {
		if gateTrader, ok := at.trader.(*gate.GateTrader); ok && at.store != nil {
			gateTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] Gate order+position sync enabled (every 30s)", at.name)
		}
	}

	// Start KuCoin order sync if using KuCoin exchange
	if at.exchange == "kucoin" {
		if kucoinTrader, ok := at.trader.(*kucoin.KuCoinTrader); ok && at.store != nil {
			kucoinTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second)
			logger.Infof("🔄 [%s] KuCoin order+position sync enabled (every 30s)", at.name)
		}
	}

	// 扫描间隔：非跟单交易员按“轮次开始时间”对齐。
	// 例如配置 3m：若本轮耗时 40s，则等 2m20s；若本轮耗时超过 3m，则下一轮立即开始。
	// 这样不会因为长耗时 AI/数据源重试后又额外等待 3m，导致实际间隔越拖越长。
	scanWait := at.config.ScanInterval
	if scanWait <= 0 {
		scanWait = 3 * time.Minute
		logger.Warnf("[%s] ScanInterval 无效，已回退为 3m", at.name)
	}
	if at.config.StrategyConfig != nil && store.IsComkunMarketFollowStrategy(at.config.StrategyConfig) {
		logger.Infof("⏱️ [%s] comkun 跟单被控：主控新广播落库后会进程内唤醒；无唤醒时每 %v 轮询查库兜底（与界面「扫描间隔」无关）", at.name, comkunFollowFollowMasterPollInterval)
	} else {
		logger.Infof("⏱️ [%s] 扫描节奏：目标每 %v 开始一轮；若上一轮超时则立即补下一轮", at.name, scanWait)
	}

	// Check if this is a grid trading strategy
	isGridStrategy := at.IsGridStrategy()
	if isGridStrategy {
		logger.Infof("🔲 [%s] Grid trading strategy detected, initializing grid...", at.name)
		if err := at.InitializeGrid(); err != nil {
			logger.Errorf("❌ [%s] Failed to initialize grid: %v", at.name, err)
			return fmt.Errorf("grid initialization failed: %w", err)
		}
	}

	// Execute immediately on first run
	lastCycleStartedAt := time.Now()
	if isGridStrategy {
		if err := at.RunGridCycle(); err != nil {
			logger.Infof("❌ Grid execution failed: %v", err)
		}
	} else {
		if err := at.runCycle(); err != nil {
			logger.Infof("❌ Execution failed: %v", err)
		}
	}

	for {
		at.isRunningMutex.RLock()
		running := at.isRunning
		at.isRunningMutex.RUnlock()

		if !running {
			break
		}

		waitNext := at.comkunFollowPollWaitAfterCycle(scanWait)
		if at.config.StrategyConfig == nil || !store.IsComkunMarketFollowStrategy(at.config.StrategyConfig) {
			elapsed := time.Since(lastCycleStartedAt)
			if elapsed < scanWait {
				waitNext = scanWait - elapsed
			} else {
				waitNext = 0
			}
		}
		if waitNext > 0 {
			timer := time.NewTimer(waitNext)
			wakeCh := at.comkunFollowWakeCh
			if wakeCh != nil {
				select {
				case <-timer.C:
				case <-wakeCh:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
				case <-at.stopMonitorCh:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					logger.Infof("[%s] ⏹ Stop signal received, exiting automatic trading main loop", at.name)
					return nil
				}
			} else {
				select {
				case <-timer.C:
				case <-at.stopMonitorCh:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					logger.Infof("[%s] ⏹ Stop signal received, exiting automatic trading main loop", at.name)
					return nil
				}
			}
		} else {
			wakeCh := at.comkunFollowWakeCh
			if wakeCh != nil {
				select {
				case <-wakeCh:
				case <-at.stopMonitorCh:
					logger.Infof("[%s] ⏹ Stop signal received, exiting automatic trading main loop", at.name)
					return nil
				default:
				}
			} else {
				select {
				case <-at.stopMonitorCh:
					logger.Infof("[%s] ⏹ Stop signal received, exiting automatic trading main loop", at.name)
					return nil
				default:
				}
			}
		}

		at.isRunningMutex.RLock()
		running = at.isRunning
		at.isRunningMutex.RUnlock()
		if !running {
			break
		}

		if isGridStrategy {
			lastCycleStartedAt = time.Now()
			if err := at.RunGridCycle(); err != nil {
				logger.Infof("❌ Grid execution failed: %v", err)
			}
		} else {
			lastCycleStartedAt = time.Now()
			if err := at.runCycle(); err != nil {
				logger.Infof("❌ Execution failed: %v", err)
			}
		}
	}

	return nil
}

// invalidateCachedExchangePositionsForComkunBroadcast 主控发广播前强刷 REST 持仓（AI 周期结束、手动对账等）。
// 由币安用户数据流唤醒的快照路径应跳过，以免清掉 WS 刚写入的持仓后 REST 仍短暂滞后。
func (at *AutoTrader) invalidateCachedExchangePositionsForComkunBroadcast() {
	if ft, ok := at.trader.(*binance.FuturesTrader); ok {
		ft.InvalidatePositionsCache()
	}
	at.comkunMasterLastRESTRefresh = time.Now()
}

// maybeInvalidateComkunMasterPositionsForREST 主控定时 ticker：用户流在线时降低 REST 强刷频率。
func (at *AutoTrader) maybeInvalidateComkunMasterPositionsForREST() {
	ft, ok := at.trader.(*binance.FuturesTrader)
	if !ok {
		at.invalidateCachedExchangePositionsForComkunBroadcast()
		return
	}
	if !ft.IsUserDataStreamActive() {
		at.invalidateCachedExchangePositionsForComkunBroadcast()
		return
	}
	if time.Since(at.comkunMasterLastRESTRefresh) < 90*time.Second {
		return
	}
	at.invalidateCachedExchangePositionsForComkunBroadcast()
}

// signalStop marks not running and closes stop channel once (idempotent).
func (at *AutoTrader) signalStop() {
	at.isRunningMutex.Lock()
	if !at.isRunning {
		at.isRunningMutex.Unlock()
		return
	}
	at.isRunning = false
	at.isRunningMutex.Unlock()

	at.stopChMu.Lock()
	if !at.stopMonitorClosed {
		close(at.stopMonitorCh)
		at.stopMonitorClosed = true
	}
	at.stopChMu.Unlock()
}

// StopAsync requests stop and returns immediately so HTTP API is not blocked
// on a long in-flight AI cycle. The Run goroutine exits after the current work.
func (at *AutoTrader) StopAsync() {
	at.signalStop()
	go at.Close()
	logger.Info("⏹ Stop requested (async); loop will exit after current step")
}

// Stop blocks until the trading monitor goroutine has fully exited.
func (at *AutoTrader) Stop() {
	at.signalStop()
	at.monitorWg.Wait()
	logger.Info("⏹ Automatic trading system stopped")
}

// Close permanently stops the automatic trader and releases exchange resources.
func (at *AutoTrader) Close() {
	at.Stop()
	if closer, ok := at.trader.(interface{ Close() }); ok {
		closer.Close()
	}
}

// GetID gets trader ID
func (at *AutoTrader) GetID() string {
	return at.id
}

// GetUnderlyingTrader returns the underlying Trader interface implementation
// This is used by grid trading and other components that need direct exchange access
func (at *AutoTrader) GetUnderlyingTrader() Trader {
	return at.trader
}

// GetName gets trader name
func (at *AutoTrader) GetName() string {
	return at.name
}

// GetAIModel gets AI model
func (at *AutoTrader) GetAIModel() string {
	return at.aiModel
}

// GetExchange gets exchange
func (at *AutoTrader) GetExchange() string {
	return at.exchange
}

// GetShowInCompetition returns whether trader should be shown in competition
func (at *AutoTrader) GetShowInCompetition() bool {
	return at.showInCompetition
}

// SetShowInCompetition sets whether trader should be shown in competition
func (at *AutoTrader) SetShowInCompetition(show bool) {
	at.showInCompetition = show
}

// SetCustomPrompt sets custom trading strategy prompt
func (at *AutoTrader) SetCustomPrompt(prompt string) {
	at.customPrompt = prompt
}

// SetOverrideBasePrompt sets whether to override base prompt
func (at *AutoTrader) SetOverrideBasePrompt(override bool) {
	at.overrideBasePrompt = override
}

// GetSystemPromptTemplate gets current system prompt template name (from strategy config)
func (at *AutoTrader) GetSystemPromptTemplate() string {
	if at.strategyEngine != nil {
		config := at.strategyEngine.GetConfig()
		if config.CustomPrompt != "" {
			return "custom"
		}
	}
	return "strategy"
}

// GetStore gets data store (for external access to decision records, etc.)
func (at *AutoTrader) GetStore() *store.Store {
	return at.store
}

// calculatePnLPercentage calculates P&L percentage (based on margin, automatically considers leverage)
// Return rate = Unrealized P&L / Margin x 100%
func calculatePnLPercentage(unrealizedPnl, marginUsed float64) float64 {
	if marginUsed > 0 {
		return (unrealizedPnl / marginUsed) * 100
	}
	return 0.0
}

// runPreLaunchChecks performs pre-launch checks for claw402 users (wallet balance, runway estimate)
func (at *AutoTrader) runPreLaunchChecks() {
	if !store.IsClaw402Config(at.config.AIModel) {
		return
	}

	logger.Info("🔍 Running pre-launch checks (claw402)...")

	// Derive wallet address from CustomAPIKey (which is the private key for claw402)
	if at.config.CustomAPIKey != "" {
		// Try to derive address using go-ethereum
		addr := deriveWalletAddress(at.config.CustomAPIKey)
		if addr != "" {
			at.claw402WalletAddr = addr
			logger.Infof("💳 [%s] Claw402 wallet: %s", at.name, addr)

			// Query USDC balance
			balance, err := wallet.QueryUSDCBalance(addr)
			if err != nil {
				logger.Warnf("⚠️ [%s] Could not query USDC balance: %v", at.name, err)
			} else {
				// Estimate runway
				scanMinutes := int(at.config.ScanInterval.Minutes())
				modelName := at.config.CustomModelName
				if modelName == "" {
					modelName = "deepseek"
				}
				dailyCost, runway := store.EstimateRunway(balance, modelName, scanMinutes)
				logger.Infof("💰 [%s] USDC Balance: $%.2f | Daily AI cost: ~$%.2f | Runway: ~%.1f days",
					at.name, balance, dailyCost, runway)

				if balance < 1.0 {
					logger.Warnf("⚠️ [%s] Low USDC balance! Consider topping up.", at.name)
				}
				if balance <= 0 {
					logger.Errorf("🚨 [%s] USDC balance is ZERO — AI calls will fail!", at.name)
				}
			}
		}
	}

	logger.Info("✅ Pre-launch checks complete")
}

// deriveWalletAddress derives an Ethereum address from a hex private key
func deriveWalletAddress(privateKeyHex string) string {
	// Remove 0x prefix if present
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return ""
	}

	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	return address.Hex()
}
