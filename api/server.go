package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"nofx/auth"
	"nofx/crypto"
	"nofx/logger"
	"nofx/manager"
	"nofx/store"
	"nofx/trader/binance"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Server HTTP API server
type Server struct {
	router                    *gin.Engine
	traderManager             *manager.TraderManager
	store                     *store.Store
	cryptoHandler             *CryptoHandler
	exchangeAccountStateCache *ExchangeAccountStateCache
	httpServer                *http.Server
	port                      int
	telegramReloadCh          chan<- struct{} // signal Telegram bot to reload
	hzMasterPollCancel        context.CancelFunc
	partnerRebateCancel       context.CancelFunc
}

// NewServer Creates API server
func NewServer(traderManager *manager.TraderManager, st *store.Store, cryptoService *crypto.CryptoService, port int) *Server {
	// Set to Release mode (reduce log output)
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()

	// Enable CORS
	router.Use(corsMiddleware())

	// Create crypto handler
	cryptoHandler := NewCryptoHandler(cryptoService)

	s := &Server{
		router:                    router,
		traderManager:             traderManager,
		store:                     st,
		cryptoHandler:             cryptoHandler,
		exchangeAccountStateCache: NewExchangeAccountStateCache(),
		port:                      port,
	}

	binance.SetProxyFaultRecorder(func(in store.RecordFaultInput) {
		if err := st.ProxyFault().RecordFault(in); err != nil {
			logger.Warnf("record outbound proxy fault: %v", err)
		}
	})

	// Setup routes
	s.setupRoutes()
	s.startHZMasterEventPoller()
	s.startPartnerRebateOutboxWorker()

	return s
}

// corsMiddleware CORS middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// setupRoutes Setup routes
func (s *Server) setupRoutes() {
	// API route group
	api := s.router.Group("/api")
	{
		// Health check
		api.Any("/health", s.handleHealth)

		// Admin login (used in admin mode, public)

		// System supported models and exchanges (no authentication required)
		s.route(api, "GET", "/supported-models", "List supported AI model providers", s.handleGetSupportedModels)
		s.route(api, "GET", "/supported-exchanges", "List supported exchange types", s.handleGetSupportedExchanges)

		// System config (no authentication required, for frontend to determine admin mode/registration status)
		s.route(api, "GET", "/config", "Get system configuration", s.handleGetSystemConfig)

		// Wallet validation (no authentication required — used by frontend config form)
		api.POST("/wallet/validate", s.handleWalletValidate)
		api.POST("/wallet/generate", s.handleWalletGenerate)

		// Crypto related endpoints (no authentication required, not exposed to bot)
		api.GET("/crypto/config", s.cryptoHandler.HandleGetCryptoConfig)
		api.GET("/crypto/public-key", s.cryptoHandler.HandleGetPublicKey)
		api.POST("/crypto/decrypt", s.cryptoHandler.HandleDecryptSensitiveData)

		// Public competition data (no authentication required)
		s.route(api, "GET", "/traders", "Public trader list", s.handlePublicTraderList)
		s.route(api, "GET", "/competition", "Public competition data", s.handlePublicCompetition)
		s.route(api, "GET", "/top-traders", "Top traders leaderboard", s.handleTopTraders)
		s.route(api, "GET", "/equity-history", "Equity history for a trader", s.handleEquityHistory)
		s.route(api, "POST", "/equity-history-batch", "Batch equity history for multiple traders", s.handleEquityHistoryBatch)
		s.route(api, "GET", "/traders/:id/public-config", "Public trader configuration", s.handleGetPublicTraderConfig)

		// Market data (no authentication required)
		s.route(api, "GET", "/klines", "Candlestick data (?symbol=&interval=&limit=)", s.handleKlines)
		s.route(api, "GET", "/symbols", "Available trading symbols", s.handleSymbols)
		s.route(api, "GET", "/market/board", "Aggregated free market board (crypto, forex, delayed equities)", s.handleMarketBoard)

		// Public strategy market (no authentication required)
		s.route(api, "GET", "/strategies/public", "Public strategy market", s.handlePublicStrategies)
		s.route(api, "GET", "/strategies/public/:id", "Public strategy market detail", s.handlePublicStrategyDetail)
		s.route(api, "POST", "/strategies/estimate-tokens", "Estimate token usage for a strategy config", s.handleEstimateTokens)

		// Authentication related routes (no authentication required)
		s.routeWithSchema(api, "POST", "/integrations/hz/v1/master-events", "HZ AI master event ingestion",
			`Headers: X-HZ-Event-Id, X-HZ-Event-Timestamp (13-digit milliseconds), X-HZ-Event-Nonce (UUID), X-HZ-Event-Signature (HMAC-SHA256 hex of timestamp+"\n"+nonce+"\n"+rawBody)
Body: {"eventId":"<string>","sequence":"<positive integer string>","eventType":"OPEN|INCREASE|REDUCE|CLOSE|PROTECTION|LIQUIDATION|RECONCILE","occurredAt":"<RFC3339>","masterAccountId":"<string>","account":{"accountId":"<string>","accountScope":"AI","walletId":"ai:<id>","positionBookId":"ai:<id>","currency":"<string>","balance":"<decimal>","equity":"<positive decimal>","availableMargin":"<decimal>","usedMargin":"<decimal>","marginRatio":"<decimal>","unrealizedPnl":"<decimal>","tradable":<bool>},"positions":[<HZ API v1 position>]}
200: accepted or duplicate. 409 SEQUENCE_GAP: {"expected_next_sequence":<integer>}`,
			s.handleHZMasterEvent)
		s.route(api, "POST", "/auth/send-register-code", "Send email OTP for first-time registration", s.handleSendRegisterEmailCode)
		s.route(api, "POST", "/auth/send-reset-password-code", "Send email OTP for password reset", s.handleSendResetPasswordEmailCode)
		s.route(api, "POST", "/register", "Register new user", s.handleRegister)
		s.route(api, "POST", "/login", "User login, returns JWT token", s.handleLogin)
		s.route(api, "POST", "/reset-password", "Reset password", s.handleResetPassword)
		s.route(api, "POST", "/reset-account", "Clear all users and reset system to allow re-registration", s.handleResetAccount)

		// Routes requiring authentication
		s.route(api, "POST", "/send_signal_strategy", "MT4 EA strategy signal webhook", s.handleMT4StrategySignal)
		s.route(api, "POST", "/news/gold-market-snapshot", "MT4 gold and dollar index market snapshot", s.handleGoldMarketSnapshot)
		s.route(api, "POST", "/comkun/screen-broadcast", "Screen monitor: publish master state from OCR detection", s.handleScreenMonitorBroadcast)
		s.route(api, "POST", "/okx-scraper-relay", "OKX Tampermonkey → local okx_dom_scraper relay", s.handleOkxScraperRelay)
		s.route(api, "OPTIONS", "/okx-scraper-relay", "CORS preflight for OKX scraper relay", s.handleOkxScraperRelay)
		protected := api.Group("/", s.authMiddleware(), s.accountLockMiddleware())
		{
			// Logout (add to blacklist)
			s.route(protected, "POST", "/logout", "Logout (blacklist token)", s.handleLogout)
			s.route(protected, "POST", "/onboarding/beginner", "Prepare beginner claw402 wallet and default model", s.handleBeginnerOnboarding)
			s.route(protected, "GET", "/onboarding/beginner/current", "Get current beginner claw402 wallet", s.handleCurrentBeginnerWallet)

			// User account management
			s.routeWithSchema(protected, "PUT", "/user/password", "Change current user password",
				`Body: {"new_password":"<string, min 8 chars>"}`,
				s.handleChangePassword)
			s.route(protected, "GET", "/user/me", "Current user profile (display name, avatar)", s.handleGetMe)
			s.route(protected, "GET", "/news/crypto", "Internal news intelligence monitor feed", s.handleCryptoNews)
			s.routeWithSchema(protected, "PUT", "/user/profile", "Update display name",
				`Body: {"display_name":"<string, 1-32 chars>"}`,
				s.handleUpdateProfile)

			// 平台站内余额（策略市场消费；用户自助演示充值已停用）
			s.route(protected, "GET", "/wallet", "Platform wallet balance and ledger", s.handleGetWallet)
			s.route(protected, "GET", "/notifications", "User in-app notifications (recharge, admin credit)", s.handleListUserNotifications)
			s.route(protected, "GET", "/invite/me", "Current user's invite code, link and invited users", s.handleInviteMe)
			s.route(protected, "GET", "/partner/dashboard", "Partner rebate dashboard and umbrella ledger", s.handlePartnerDashboard)
			s.route(protected, "POST", "/partner/withdrawals", "Submit TRC20 partner rebate withdrawal", s.handlePartnerWithdrawal)
			s.route(protected, "POST", "/partner/studio-requests", "Branch nominates an umbrella user as studio", s.handlePartnerStudioRequest)

			// Server IP query (requires authentication, for whitelist configuration)
			s.route(protected, "GET", "/server-ip", "Get server public IP (for exchange whitelist)", s.handleGetServerIP)

			// AI trader management
			s.routeWithSchema(protected, "GET", "/my-traders", "List user's traders with status",
				`Returns: [{"trader_id":"<EXACT id — use this as trader_id in all ?trader_id= queries and POST /traders/:id/start|stop>","trader_name":"<string>","is_running":<bool>}]
NOTE: The id field is "trader_id" (NOT "id"). Always read trader_id from this endpoint before querying data.`,
				s.handleTraderList)
			s.routeWithSchema(protected, "GET", "/traders/:id/config", "Get full trader configuration",
				`:id = trader_id from GET /api/my-traders`,
				s.handleGetTraderConfig)
			s.routeWithSchema(protected, "POST", "/traders", "Create a new AI trader",
				`Body: {"name":"<string, required>","ai_model_id":"<EXACT id field from GET /api/models — e.g. 'abc123_deepseek', NOT the provider name 'deepseek'>","exchange_id":"<EXACT id field from GET /api/exchanges — e.g. '05785d3b-841e-...', NOT the type name>","strategy_id":"<EXACT id field from GET /api/strategies>","scan_interval_minutes":<int, default 3, minimum 3>}
IMPORTANT: ai_model_id and exchange_id must be the full "id" value from the Account State, not the provider/type name.`,
				s.handleCreateTrader)
			s.routeWithSchema(protected, "PUT", "/traders/:id", "Update trader configuration",
				`:id = trader_id from GET /api/my-traders
Body: {"name":"<string>","ai_model_id":"<EXACT id from GET /api/models>","exchange_id":"<EXACT id from GET /api/exchanges>","strategy_id":"<EXACT id from GET /api/strategies>","scan_interval_minutes":<int, min 3>,"is_cross_margin":<bool>}
Only include fields you want to change.`,
				s.handleUpdateTrader)
			s.routeWithSchema(protected, "DELETE", "/traders/:id", "Delete trader",
				`:id = trader_id from GET /api/my-traders. Stops and permanently removes the trader and all its data.`,
				s.handleDeleteTrader)
			s.routeWithSchema(protected, "POST", "/traders/:id/start", "Start trader — begins live trading",
				`:id = trader_id from GET /api/my-traders. No request body needed. The trader must have a valid exchange and AI model configured.`,
				s.handleStartTrader)
			s.routeWithSchema(protected, "POST", "/traders/:id/stop", "Stop trader — halts live trading",
				`:id = trader_id from GET /api/my-traders. No request body needed. Gracefully stops the trading loop.`,
				s.handleStopTrader)
			s.routeWithSchema(protected, "PUT", "/traders/:id/prompt", "Override the trader's AI system prompt",
				`Body: {"prompt":"<string — the full custom prompt text>"}`,
				s.handleUpdateTraderPrompt)
			s.routeWithSchema(protected, "POST", "/traders/:id/sync-balance", "Sync account balance from exchange",
				`:id = trader_id from GET /api/my-traders. No request body needed. Refreshes initial_balance from the exchange.`,
				s.handleSyncBalance)
			s.routeWithSchema(protected, "POST", "/traders/:id/close-position", "Force-close an open position",
				`:id = trader_id from GET /api/my-traders.
Body: {"symbol":"<string, e.g. BTCUSDT — must match an open position symbol from GET /api/positions>"}`,
				s.handleClosePosition)
			s.routeWithSchema(protected, "PUT", "/traders/:id/competition", "Toggle competition leaderboard visibility",
				`:id = trader_id from GET /api/my-traders.
Body: {"show_in_competition":<bool>}`,
				s.handleToggleCompetition)
			s.routeWithSchema(protected, "GET", "/traders/:id/grid-risk", "Get grid trading risk info",
				`:id = trader_id from GET /api/my-traders.`,
				s.handleGetGridRiskInfo)
			// AI cost tracking
			s.route(protected, "GET", "/ai-costs", "Get AI call costs for a trader (?trader_id=xxx&period=today)", s.handleGetAICosts)
			s.route(protected, "GET", "/ai-costs/summary", "Get AI cost summary (?period=today)", s.handleGetAICostsSummary)
			s.route(protected, "GET", "/ai-platform-usage", "Get current user's AI platform billing ledger", s.handleGetAIPlatformUsage)

			// AI model configuration
			s.routeWithSchema(protected, "GET", "/models", "List AI model configs",
				`Returns: [{"id":"<EXACT id — use this as ai_model_id when creating/updating a trader>","name":"<display name>","provider":"<short provider name — NOT a valid id>","enabled":<bool>}]
CRITICAL: The "id" field (e.g. "abc123_deepseek") is what you must use for ai_model_id. The "provider" field ("deepseek") is NOT valid as an id.`,
				s.handleGetModelConfigs)
			s.routeWithSchema(protected, "PUT", "/models", "Configure an AI model provider",
				`Body: {"models":{"<model_id>":{"enabled":<bool>,"api_key":"<string>","custom_api_url":"<string, leave empty to use provider default>","custom_model_name":"<string, leave empty to use provider default>"}}}
model_id values: "openai","deepseek","qwen","kimi","grok","gemini","claude","minimax"
Defaults when custom fields empty: openai→api.openai.com/v1 (gpt-5.5), deepseek→api.deepseek.com (deepseek-v4-flash), qwen→dashscope.aliyuncs.com/compatible-mode/v1 (qwen3.7-max), kimi→api.moonshot.ai/v1 (kimi-k2.6), grok→api.x.ai/v1 (grok-4.3-latest), gemini→generativelanguage.googleapis.com/v1beta/openai (gemini-flash-latest), claude→api.anthropic.com/v1 (claude-opus-4-8), minimax→api.minimax.io/v1 (MiniMax-M3)`,
				s.handleUpdateModelConfigs)

			// Exchange configuration
			s.routeWithSchema(protected, "GET", "/exchanges", "List exchange accounts",
				`Returns: [{"id":"<EXACT id — use this as exchange_id when creating/updating a trader>","exchange_type":"<e.g. okx, binance, hz>","account_name":"<user label>","enabled":<bool>,"api_url":"<HZ API base URL, hz only>"}]
CRITICAL: Always use the "id" field for exchange_id. Do not use "exchange_type" as an id.`,
				s.handleGetExchangeConfigs)
			s.routeWithSchema(protected, "GET", "/exchanges/account-state", "Get connection and balance state for each exchange account",
				`Returns: {"states":{"<exchange_id>":{"status":"ok|disabled|missing_credentials|invalid_credentials|permission_denied|unavailable","display_balance":"<string>","total_equity":<number>,"available_balance":<number>,"asset":"USD|USDT|USDC","checked_at":"<RFC3339>","error_code":"<string>","error_message":"<string>"}}}
Use this endpoint to show balance and health in the exchange list without depending on traders.`,
				s.handleGetExchangeAccountStates)
			s.routeWithSchema(protected, "POST", "/exchanges", "Create a new exchange account",
				`Body: {"exchange_type":"<string>","account_name":"<string, user label>","enabled":true,"api_url":"<HTTPS HZ API base URL, required for hz>","api_key":"<string>","secret_key":"<string>","passphrase":"<string, required for okx/gate/kucoin>"}
exchange_type values: "binance","bybit","okx","bitget","gate","kucoin","indodax","hz" (CEX/platform account) | "hyperliquid","aster","lighter" (DEX)
Required fields by exchange:
  binance/bybit/bitget/indodax: api_key + secret_key
  okx/gate/kucoin: api_key + secret_key + passphrase
  hz: api_url + api_key + secret_key
  hyperliquid: hyperliquid_wallet_addr
  aster: aster_user + aster_signer + aster_private_key
  lighter: lighter_wallet_addr + lighter_private_key + lighter_api_key_private_key + lighter_api_key_index`,
				s.handleCreateExchange)
			s.routeWithSchema(protected, "PUT", "/exchanges", "Update an existing exchange account configuration",
				`Body: {"id":"<EXACT id from GET /api/exchanges>","exchange_type":"<string>","account_name":"<string>","enabled":<bool>,"api_url":"<HTTPS HZ API base URL, required for hz>","api_key":"<string>","secret_key":"<string>","passphrase":"<string, for okx/gate/kucoin>"}
Use this to enable/disable an exchange or update API credentials. The "id" field is required to identify which exchange to update.`,
				s.handleUpdateExchangeConfigs)
			s.routeWithSchema(protected, "DELETE", "/exchanges/:id", "Delete exchange account",
				`:id = EXACT id from GET /api/exchanges. Permanently removes the exchange account and disconnects any traders using it.`,
				s.handleDeleteExchange)

			// ED25519 key pair generation for exchange API authentication
			s.route(protected, "POST", "/crypto/generate-ed25519-keypair", "Generate ED25519 key pair for exchange API", s.cryptoHandler.HandleGenerateED25519KeyPair)

			// Telegram bot configuration
			s.routeWithSchema(protected, "GET", "/telegram", "Get Telegram bot configuration",
				`Returns: {"bot_token":"<string>","model_id":"<EXACT id of configured AI model>","chat_id":"<bound Telegram chat id, empty if not bound>"}`,
				s.handleGetTelegramConfig)
			s.routeWithSchema(protected, "POST", "/telegram", "Set Telegram bot token and AI model",
				`Body: {"bot_token":"<string — Telegram BotFather token>","model_id":"<EXACT id from GET /api/models>"}
Both fields are required. After saving, the user must send /start in Telegram to bind their account.`,
				s.handleUpdateTelegramConfig)
			s.routeWithSchema(protected, "POST", "/telegram/model", "Update Telegram bot AI model only",
				`Body: {"model_id":"<EXACT id from GET /api/models>"}`,
				s.handleUpdateTelegramModel)
			s.routeWithSchema(protected, "DELETE", "/telegram/binding", "Unbind Telegram account",
				`No body needed. Clears the Telegram chat_id binding so the user can re-bind with /start.`,
				s.handleUnbindTelegram)

			// Strategy management
			s.routeWithSchema(protected, "GET", "/strategies", "List user's strategies",
				`Returns: [{"id":"<EXACT id — use as strategy_id when creating/updating a trader>","name":"<string>","is_active":<bool>,"is_default":<bool>}]
CRITICAL: Always use the "id" field for strategy_id.`,
				s.handleGetStrategies)
			s.routeWithSchema(protected, "GET", "/strategies/active", "Get the currently active strategy",
				`Returns the strategy marked is_active=true for this user, or the system default. Use this to find which strategy is currently in use.`,
				s.handleGetActiveStrategy)
			s.routeWithSchema(protected, "GET", "/strategies/default-config", "Get default strategy config with all fields and sensible values — use as reference for building configs",
				`No parameters needed. Returns a complete StrategyConfig object with all fields populated with recommended defaults. Read this before building a custom config.`,
				s.handleGetDefaultStrategyConfig)
			s.route(protected, "POST", "/strategies/preview-prompt", "Preview the AI prompt that will be generated from a config", s.handlePreviewPrompt)
			s.route(protected, "POST", "/strategies/test-run", "Test-run strategy AI analysis", s.handleStrategyTestRun)
			s.route(protected, "GET", "/strategies/market/owned/:id", "Get purchased or self-owned market strategy full config", s.handleMarketOwnedStrategy)
			s.route(protected, "GET", "/strategies/market/entitlements", "List strategy market purchase entitlements", s.handleMarketEntitlements)
			s.route(protected, "POST", "/strategies/market/purchase", "Purchase market strategy with platform balance", s.handleMarketStrategyPurchase)
			s.route(protected, "GET", "/strategies/:id/comkun-master-board", "Comkun listing template: master broadcasts + bound trader orders for studio board", s.handleGetStrategyComkunMasterBoard)
			s.route(protected, "GET", "/strategies/:id", "Get strategy by ID", s.handleGetStrategy)
			s.routeWithSchema(protected, "POST", "/strategies", "Create a new trading strategy",
				`Body: {"name":"<string, required>","description":"<string, optional>","lang":"zh|en","config":<StrategyConfig object, OPTIONAL — if omitted the system applies complete working defaults automatically (ai500 top coins, all standard indicators, standard risk control)>}
IMPORTANT: For most use cases just POST {"name":"<name>"} — the backend fills everything in. Only include "config" when the user explicitly requests custom settings (specific coins, custom leverage, custom timeframes).

StrategyConfig fields:
  coin_source.source_type: "static"(fixed coin list) | "ai500"(AI top500 ranking) | "oi_top"(OI increasing, suited for long) | "oi_low"(OI decreasing, suited for short) | "mixed"
  coin_source.static_coins: ["BTCUSDT","ETHUSDT"] — only when source_type="static"
  coin_source.use_ai500, ai500_limit: number of coins from AI500 pool (default 10)
  coin_source.use_oi_top/use_oi_low, oi_top_limit/oi_low_limit: OI-based coin selection
  indicators.klines.primary_timeframe: "1m"|"3m"|"5m"|"15m"|"1h"|"4h" — scalping→"5m", trend/swing→"1h"/"4h"
  indicators.klines.primary_count: number of candles (20-100)
  indicators.klines.enable_multi_timeframe: true for trend/swing analysis
  indicators.klines.selected_timeframes: e.g. ["5m","15m","1h","4h"]
  indicators.enable_raw_klines: ALWAYS true (raw OHLCV required)
  indicators.enable_ema: true for trend-following (EMA crossover signals)
  indicators.enable_macd: true for trend + momentum confirmation
  indicators.enable_rsi: true for overbought/oversold, divergence detection
  indicators.enable_boll: true for volatility, range trading, breakout strategies
  indicators.enable_atr: true for volatility measurement and stop-loss sizing
  indicators.enable_volume: ALWAYS true
  indicators.enable_oi: ALWAYS true (open interest data)
  indicators.enable_funding_rate: ALWAYS true
  indicators.ema_periods: [20,50] default, [9,21] for faster signals
  indicators.rsi_periods: [7,14] default
  indicators.atr_periods: [14] default
  indicators.boll_periods: [20] default
  indicators.nofxos_api_key: ALWAYS "cm_568c67eae410d912c54c"
  indicators.enable_quant_data: ALWAYS true
  indicators.enable_quant_oi: ALWAYS true
  indicators.enable_quant_netflow: ALWAYS true
  indicators.enable_oi_ranking: ALWAYS true, oi_ranking_duration:"1h", oi_ranking_limit:10
  indicators.enable_netflow_ranking: ALWAYS true, netflow_ranking_duration:"1h", netflow_ranking_limit:10
  indicators.enable_price_ranking: ALWAYS true, price_ranking_duration:"1h,4h,24h", price_ranking_limit:10
  risk_control.max_positions: max simultaneous positions (1=single coin, 3=diversified, 5=wide)
  risk_control.btc_eth_max_leverage: BTC/ETH leverage (conservative:3-5, moderate:5-10, aggressive:10-20)
  risk_control.altcoin_max_leverage: altcoin leverage (usually lower than BTC leverage)
  risk_control.btc_eth_max_position_value_ratio: max position size as multiple of equity (default 5)
  risk_control.altcoin_max_position_value_ratio: default 1
  risk_control.max_margin_usage: 0.5-0.95 (default 0.9 = use up to 90% margin)
  risk_control.min_position_size: minimum USDT per trade (default 12)
  risk_control.min_risk_reward_ratio: minimum profit/loss ratio required (default 3 = 3:1)
  risk_control.min_confidence: minimum AI confidence to open position (default 75, range 60-90)
  strategy_prompt: single string — full user-authored strategy narrative injected into the system prompt (preferred)
  prompt_sections: legacy four-field split; used only when strategy_prompt is empty`,
				s.handleCreateStrategy)
			s.routeWithSchema(protected, "PUT", "/strategies/:id", "Update an existing strategy — WORKFLOW: 1) GET /api/strategies/:id first to read current config 2) Merge your changes into the full config 3) PUT with complete merged config 4) GET again to verify saved values",
				`Body: {"name":"<string>","description":"<string>","config":<complete StrategyConfig — same structure as POST /api/strategies>}
IMPORTANT: config is merged with existing values server-side, but always send the complete section you are modifying.
After updating, always GET /api/strategies/:id to verify and show the user actual saved values.`,
				s.handleUpdateStrategy)
			s.routeWithSchema(protected, "POST", "/strategies/:id/market-update", "Publish strategy content to the market and bump market_revision (for subscribers)",
				`Body same as PUT /strategies/:id. Requires is_public=true on the strategy. Keeps listing public and increments market_revision.`,
				s.handlePublishMarketUpdate)
			s.routeWithSchema(protected, "DELETE", "/strategies/:id", "Delete strategy",
				`:id = EXACT id from GET /api/strategies. Cannot delete a strategy that is currently assigned to a running trader.`,
				s.handleDeleteStrategy)
			s.routeWithSchema(protected, "POST", "/strategies/:id/activate", "Mark a strategy as the active strategy for this user",
				`:id = EXACT id from GET /api/strategies.
No request body needed. Sets this strategy as is_active=true (and deactivates the previous active strategy).
After activating, create or update a trader with this strategy_id to apply it.`,
				s.handleActivateStrategy)
			s.routeWithSchema(protected, "POST", "/strategies/:id/duplicate", "Duplicate an existing strategy",
				`:id = EXACT id from GET /api/strategies. Creates a copy with " (copy)" appended to the name.`,
				s.handleDuplicateStrategy)

			// Data for specified trader (using query parameter ?trader_id=xxx)
			// IMPORTANT: All ?trader_id= values must be the EXACT "trader_id" field from GET /api/my-traders
			s.routeWithSchema(protected, "GET", "/status", "Trader running status",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>
Returns: {"is_running":<bool>,"trader_id":"<string>"}`,
				s.handleStatus)
			s.routeWithSchema(protected, "GET", "/account", "Account balance and equity",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>
Returns: {"balance":<float>,"equity":<float>,"unrealized_pnl":<float>,"initial_balance":<float>,"total_return_pct":<float>}`,
				s.handleAccount)
			s.routeWithSchema(protected, "GET", "/positions", "Current open positions",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>
Returns: [{"symbol":"<string>","side":"long|short","size":<float>,"entry_price":<float>,"mark_price":<float>,"last_price":<float>,"last_price_time":<unix_ms>,"unrealized_pnl":<float>,"leverage":<int>}]`,
				s.handlePositions)
			s.routeWithSchema(protected, "GET", "/positions/history", "Closed position history",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>&limit=<int, default 20>`,
				s.handlePositionHistory)
			s.routeWithSchema(protected, "GET", "/trades", "Trade records",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>&limit=<int, default 20>`,
				s.handleTrades)
			s.routeWithSchema(protected, "GET", "/orders", "All order records",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>&limit=<int, default 20>`,
				s.handleOrders)
			s.routeWithSchema(protected, "GET", "/orders/:id/fills", "Order fill details",
				`:id = order id from GET /api/orders`,
				s.handleOrderFills)
			s.routeWithSchema(protected, "GET", "/open-orders", "Open orders currently on exchange",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>&symbol=<optional, e.g. BTCUSDT>
Omit symbol to fetch all symbols' pending orders (limits, TP/SL, etc.).`,
				s.handleOpenOrders)
			s.routeWithSchema(protected, "GET", "/decisions", "AI trading decisions (decision records)",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>&limit=<int, default 20>
Returns: [{"id":"<string>","symbol":"<string>","action":"open_long|open_short|close_long|close_short|hold","confidence":<int>,"reasoning":"<string>","created_at":"<timestamp>"}]`,
				s.handleDecisions)
			s.routeWithSchema(protected, "GET", "/decisions/latest", "Latest AI decisions (most recent scan results)",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>
Returns the most recent AI decision for each symbol analyzed in the last scan cycle.`,
				s.handleLatestDecisions)
			s.routeWithSchema(protected, "GET", "/statistics", "Trading performance statistics",
				`Query: ?trader_id=<EXACT trader_id from GET /api/my-traders>
Returns: {"total_trades":<int>,"winning_trades":<int>,"win_rate":<float>,"total_pnl":<float>,"sharpe_ratio":<float>,"max_drawdown":<float>}`,
				s.handleStatistics)

			s.route(protected, "GET", "/comkun/follow-balance", "Comkun compliant follow: platform wallet balance", s.handleComkunFollowBalance)

			admin := protected.Group("/admin", s.adminMiddleware())
			{
				s.route(admin, "GET", "/users-overview", "Admin: list users with balance and Binance positions", s.handleAdminUsersOverview)
				s.route(admin, "GET", "/partner/dashboard", "Admin: complete partner deposit and commission ledger", s.handleAdminPartnerDashboard)
				s.route(admin, "GET", "/ai-platform-usage", "Admin: AI platform usage billing ledger", s.handleAdminAIPlatformUsage)
				s.route(admin, "GET", "/users/:id", "Admin: user detail and wallet ledger", s.handleAdminUserDetail)
				s.route(admin, "POST", "/users/:id/wallet-adjust", "Admin: adjust user platform balance", s.handleAdminUserWalletAdjust)
				s.route(admin, "POST", "/traders/:id/start", "Admin: start trader on behalf of owner", s.handleAdminStartTrader)
				s.route(admin, "POST", "/traders/:id/stop", "Admin: stop trader on behalf of owner", s.handleAdminStopTrader)
				s.route(admin, "POST", "/traders/:id/sync-positions-from-exchange", "Admin: rebuild OPEN positions from exchange (fix stale DB)", s.handleAdminSyncTraderPositionsFromExchange)
				s.route(admin, "POST", "/comkun/master-broadcast", "Admin: publish master analysis+decisions for comkun follow subscribers", s.handleAdminComkunMasterBroadcast)
				s.route(admin, "POST", "/comkun/trader-token-credit", "Admin: credit comkun virtual follow tokens to a trader", s.handleAdminComkunTraderTokenCredit)
				s.route(admin, "POST", "/notifications/broadcast", "Admin: publish in-app broadcast visible to all logged-in users", s.handleAdminBroadcastNotification)
				s.route(admin, "POST", "/strategies/:id/market-review", "Admin: approve or reject a customer strategy market listing request", s.handleAdminStrategyMarketReview)
				s.route(admin, "POST", "/partner/roles", "Admin: assign partner role", s.handleAdminPartnerRole)
				s.route(admin, "POST", "/partner/studio-requests/:id/review", "Admin: review studio nomination", s.handleAdminPartnerStudioReview)
				s.route(admin, "POST", "/partner/withdrawals/:id/review", "Admin: review partner withdrawal", s.handleAdminPartnerWithdrawalReview)
				s.route(admin, "GET", "/outbound-proxy-pool", "Admin: SOCKS5 outbound proxy pool + assignments", s.handleAdminOutboundProxyPoolList)
				s.route(admin, "POST", "/outbound-proxy-pool/import", "Admin: bulk import SOCKS5 proxy lines", s.handleAdminOutboundProxyPoolImport)
				s.route(admin, "DELETE", "/outbound-proxy-pool/:id", "Admin: delete unassigned pool entry", s.handleAdminOutboundProxyPoolDelete)
				s.route(admin, "POST", "/outbound-proxy-pool/:id/release", "Admin: force release pool assignment and clear exchange proxy", s.handleAdminOutboundProxyPoolRelease)
				s.route(admin, "POST", "/outbound-proxy-pool/:id/assign", "Admin: bind unassigned pool entry to exchange + set outbound proxy", s.handleAdminOutboundProxyPoolAssign)
			}

			finance := protected.Group("/finance", s.financeMiddleware())
			{
				s.route(finance, "GET", "/users-lite", "Finance: list users id/email/balance for crediting customers", s.handleFinanceUsersLite)
				s.route(finance, "POST", "/users/:id/wallet-adjust", "Finance: positive balance credit for a customer (same rebate path as admin positive adjust)", s.handleFinanceUserWalletAdjust)
			}

		}
	}
}

// handleHealth Health check
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   c.Request.Context().Value("time"),
	})
}

// handleGetSystemConfig Get system configuration (configuration that client needs to know)
func (s *Server) handleGetSystemConfig(c *gin.Context) {
	initialized := true
	c.JSON(http.StatusOK, gin.H{
		"initialized":      initialized,
		"btc_eth_leverage": 10,
		"altcoin_leverage": 5,
	})
}

// handleGetServerIP Get server IP address (for whitelist configuration)
func (s *Server) handleGetServerIP(c *gin.Context) {
	// Try to get public IP via third-party API
	publicIP := getPublicIPFromAPI()

	// If third-party API fails, get first public IP from network interface
	if publicIP == "" {
		publicIP = getPublicIPFromInterface()
	}

	// If still cannot get it, return error
	if publicIP == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to get public IP address"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"public_ip": publicIP,
		"message":   "Please add this IP address to the whitelist",
	})
}

// getPublicIPFromAPI Get public IP via third-party API (IPv4 only)
func getPublicIPFromAPI() string {
	// Try multiple public IP query services (IPv4-only endpoints)
	services := []string{
		"https://api4.ipify.org?format=text", // IPv4 only
		"https://ipv4.icanhazip.com",         // IPv4 only
		"https://v4.ident.me",                // IPv4 only
		"https://api.ipify.org?format=text",  // May return IPv4 or IPv6
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body := make([]byte, 128)
			n, err := resp.Body.Read(body)
			if err != nil && err.Error() != "EOF" {
				continue
			}

			ip := strings.TrimSpace(string(body[:n]))
			parsedIP := net.ParseIP(ip)
			// Verify if it's a valid IPv4 address (not containing ":")
			if parsedIP != nil && parsedIP.To4() != nil {
				return ip
			}
		}
	}

	return ""
}

// getPublicIPFromInterface Get first public IP from network interface
func getPublicIPFromInterface() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		// Skip disabled interfaces and loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Only consider IPv4 addresses
			if ip.To4() != nil {
				ipStr := ip.String()
				// Exclude private IP address ranges
				if !isPrivateIP(ip) {
					return ipStr
				}
			}
		}
	}

	return ""
}

// isPrivateIP Determine if it's a private IP address
func isPrivateIP(ip net.IP) bool {
	// Private IP address ranges:
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// getTraderFromQuery Get trader from query parameter
func (s *Server) getTraderFromQuery(c *gin.Context) (*manager.TraderManager, string, error) {
	userID := c.GetString("user_id")
	traderID := c.Query("trader_id")

	if traderID == "" {
		// 仅使用当前登录用户自己的交易员；禁止回退到全局内存里的第一个 id（否则会串号、看到别人 AI 动态）
		userTraders, err := s.store.Trader().List(userID)
		if err != nil || len(userTraders) == 0 {
			return nil, "", fmt.Errorf("trader_id is required (no traders for this account)")
		}
		traderID = userTraders[0].ID
	}
	if _, err := s.traderManager.GetTrader(traderID); err != nil {
		// 只有内存里缺这个交易员时才加载，避免数据看板每个接口都反复重载交易员造成卡顿。
		if loadErr := s.traderManager.LoadUserTradersFromStore(s.store, userID); loadErr != nil {
			logger.Infof("⚠️ Failed to load traders for user %s: %v", userID, loadErr)
		}
	}

	return s.traderManager, traderID, nil
}

// authMiddleware JWT authentication middleware
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
			c.Abort()
			return
		}

		// Check Bearer token format
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization format"})
			c.Abort()
			return
		}

		tokenString := tokenParts[1]

		// Blacklist check
		if auth.IsTokenBlacklisted(tokenString) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token expired, please login again"})
			c.Abort()
			return
		}

		// Validate JWT token
		claims, err := auth.ValidateJWT(tokenString)
		if err != nil {
			logger.Errorf("[Auth] Invalid token: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Store user information in context
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}

// Start Start server
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	logger.Infof("🌐 API server starting at http://localhost%s", addr)
	logger.Infof("📊 API Documentation:")
	logger.Infof("  • GET  /api/health           - Health check")
	logger.Infof("  • GET  /api/traders          - Public AI trader leaderboard top 50 (no auth required)")
	logger.Infof("  • GET  /api/competition      - Public competition data (no auth required)")
	logger.Infof("  • GET  /api/top-traders      - Top 5 trader data (no auth required, for performance comparison)")
	logger.Infof("  • GET  /api/equity-history?trader_id=xxx - Public return rate historical data (no auth required, for competition)")
	logger.Infof("  • GET  /api/equity-history-batch?trader_ids=a,b,c - Batch get historical data (no auth required, performance comparison optimization)")
	logger.Infof("  • GET  /api/traders/:id/public-config - Public trader config (no auth required, no sensitive info)")
	logger.Infof("  • POST /api/traders          - Create new AI trader")
	logger.Infof("  • DELETE /api/traders/:id    - Delete AI trader")
	logger.Infof("  • POST /api/traders/:id/start - Start AI trader")
	logger.Infof("  • POST /api/traders/:id/stop  - Stop AI trader")
	logger.Infof("  • GET  /api/models           - Get AI model config")
	logger.Infof("  • PUT  /api/models           - Update AI model config")
	logger.Infof("  • GET  /api/exchanges        - Get exchange config")
	logger.Infof("  • PUT  /api/exchanges        - Update exchange config")
	logger.Infof("  • GET  /api/status?trader_id=xxx     - Specified trader's system status")
	logger.Infof("  • GET  /api/account?trader_id=xxx    - Specified trader's account info")
	logger.Infof("  • GET  /api/positions?trader_id=xxx  - Specified trader's position list")
	logger.Infof("  • GET  /api/decisions?trader_id=xxx  - Specified trader's decision log")
	logger.Infof("  • GET  /api/decisions/latest?trader_id=xxx - Specified trader's latest decisions")
	logger.Infof("  • GET  /api/statistics?trader_id=xxx - Specified trader's statistics")
	logger.Infof("  • GET  /api/performance?trader_id=xxx - Specified trader's AI learning performance analysis")
	logger.Info()

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown Gracefully shutdown server
func (s *Server) Shutdown() error {
	if s.hzMasterPollCancel != nil {
		s.hzMasterPollCancel()
	}
	if s.partnerRebateCancel != nil {
		s.partnerRebateCancel()
	}
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// SetTelegramReloadCh sets the channel used to signal the Telegram bot to reload
func (s *Server) SetTelegramReloadCh(ch chan<- struct{}) {
	s.telegramReloadCh = ch
}
