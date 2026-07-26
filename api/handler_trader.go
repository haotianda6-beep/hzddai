package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nofx/crypto"
	"nofx/logger"
	"nofx/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AI trader management related structures
type CreateTraderRequest struct {
	Name                string  `json:"name" binding:"required"`
	AIModelID           string  `json:"ai_model_id" binding:"required"`
	ExchangeID          string  `json:"exchange_id" binding:"required"`
	StrategyID          string  `json:"strategy_id"` // Strategy ID (new version)
	InitialBalance      float64 `json:"initial_balance"`
	ScanIntervalMinutes int     `json:"scan_interval_minutes"`
	IsCrossMargin       *bool   `json:"is_cross_margin"`     // Pointer type, nil means use default value true
	ShowInCompetition   *bool   `json:"show_in_competition"` // Pointer type, nil means use default value true
	// The following fields are kept for backward compatibility, new version uses strategy config
	BTCETHLeverage       int    `json:"btc_eth_leverage"`
	AltcoinLeverage      int    `json:"altcoin_leverage"`
	TradingSymbols       string `json:"trading_symbols"`
	CustomPrompt         string `json:"custom_prompt"`
	OverrideBasePrompt   bool   `json:"override_base_prompt"`
	SystemPromptTemplate string `json:"system_prompt_template"` // System prompt template name
	UseAI500             bool   `json:"use_ai500"`
	UseOITop             bool   `json:"use_oi_top"`
}

// UpdateTraderRequest Update trader request
type UpdateTraderRequest struct {
	Name                string  `json:"name" binding:"required"`
	AIModelID           string  `json:"ai_model_id" binding:"required"`
	ExchangeID          string  `json:"exchange_id" binding:"required"`
	StrategyID          string  `json:"strategy_id"` // Strategy ID (new version)
	InitialBalance      float64 `json:"initial_balance"`
	ScanIntervalMinutes int     `json:"scan_interval_minutes"`
	IsCrossMargin       *bool   `json:"is_cross_margin"`
	ShowInCompetition   *bool   `json:"show_in_competition"`
	// The following fields are kept for backward compatibility, new version uses strategy config
	BTCETHLeverage       int    `json:"btc_eth_leverage"`
	AltcoinLeverage      int    `json:"altcoin_leverage"`
	TradingSymbols       string `json:"trading_symbols"`
	CustomPrompt         string `json:"custom_prompt"`
	OverrideBasePrompt   bool   `json:"override_base_prompt"`
	SystemPromptTemplate string `json:"system_prompt_template"`
}

func formatTraderCreationError(reason, nextStep string) string {
	if nextStep == "" {
		return fmt.Sprintf("这次未能创建机器人：%s。", reason)
	}
	return fmt.Sprintf("这次未能创建机器人：%s。%s。", reason, nextStep)
}

func traderCreationRequestError(reason string) string {
	return formatTraderCreationError(reason, "请检查你刚刚填写的内容后，再重新提交")
}

func exchangeDisplayName(exchange *store.Exchange) string {
	if exchange == nil {
		return "所选交易所账户"
	}
	if exchange.AccountName != "" {
		return fmt.Sprintf("%s（%s）", exchange.Name, exchange.AccountName)
	}
	if exchange.Name != "" {
		return exchange.Name
	}
	return "所选交易所账户"
}

func (s *Server) findTraderUsingStrategyExchange(userID, exchangeID, strategyID, excludeTraderID string, onlyRunning bool) (*store.Trader, error) {
	if strings.TrimSpace(exchangeID) == "" || strings.TrimSpace(strategyID) == "" {
		return nil, nil
	}
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		return nil, err
	}
	for _, trader := range traders {
		if trader == nil || trader.ID == excludeTraderID {
			continue
		}
		if trader.ExchangeID != exchangeID || trader.StrategyID != strategyID {
			continue
		}
		if onlyRunning && !trader.IsRunning {
			continue
		}
		return trader, nil
	}
	return nil, nil
}

func missingExchangeFields(exchange *store.Exchange) []string {
	if exchange == nil {
		return nil
	}

	var missing []string
	switch exchange.ExchangeType {
	case "binance", "bybit", "gate", "indodax", "hz":
		if exchange.APIKey == "" {
			missing = append(missing, "API Key")
		}
		if exchange.SecretKey == "" {
			missing = append(missing, "Secret Key")
		}
		if exchange.ExchangeType == "hz" && strings.TrimSpace(exchange.APIURL) == "" {
			missing = append(missing, "API URL")
		}
	case "okx", "bitget", "kucoin":
		if exchange.APIKey == "" {
			missing = append(missing, "API Key")
		}
		if exchange.SecretKey == "" {
			missing = append(missing, "Secret Key")
		}
		if exchange.Passphrase == "" {
			missing = append(missing, "Passphrase")
		}
	case "hyperliquid":
		if exchange.APIKey == "" {
			missing = append(missing, "私钥")
		}
		if strings.TrimSpace(exchange.HyperliquidWalletAddr) == "" {
			missing = append(missing, "钱包地址")
		}
	case "aster":
		if strings.TrimSpace(exchange.AsterUser) == "" {
			missing = append(missing, "Aster User")
		}
		if strings.TrimSpace(exchange.AsterSigner) == "" {
			missing = append(missing, "Aster Signer")
		}
		if exchange.AsterPrivateKey == "" {
			missing = append(missing, "Aster Private Key")
		}
	case "lighter":
		if strings.TrimSpace(exchange.LighterWalletAddr) == "" {
			missing = append(missing, "钱包地址")
		}
		if exchange.LighterAPIKeyPrivateKey == "" {
			missing = append(missing, "API Key Private Key")
		}
	}

	return missing
}

func mapStringPairs(kv ...string) map[string]string {
	if len(kv) == 0 {
		return nil
	}

	params := make(map[string]string, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		params[kv[i]] = kv[i+1]
	}
	return params
}

func validateExchangeForTraderCreation(exchange *store.Exchange) (string, string, map[string]string) {
	if exchange == nil {
		return formatTraderCreationError("还没有找到你选择的交易所账户", "请前往「设置 > 交易所配置」先添加一个可用账户，再回来创建机器人"),
			"trader.create.exchange_not_found", nil
	}
	if !exchange.Enabled {
		return formatTraderCreationError(
			fmt.Sprintf("交易所账户「%s」目前处于未启用状态", exchangeDisplayName(exchange)),
			"请前往「设置 > 交易所配置」启用该账户后，再重新创建机器人",
		), "trader.create.exchange_disabled", mapStringPairs("exchange_name", exchangeDisplayName(exchange))
	}

	missing := missingExchangeFields(exchange)
	if len(missing) > 0 {
		return formatTraderCreationError(
				fmt.Sprintf("交易所账户「%s」的配置还不完整，缺少 %s", exchangeDisplayName(exchange), strings.Join(missing, "、")),
				"请前往「设置 > 交易所配置」补全该账户的必填信息后，再重新创建机器人",
			), "trader.create.exchange_missing_fields", mapStringPairs(
				"exchange_name", exchangeDisplayName(exchange),
				"missing_fields", strings.Join(missing, ", "),
			)
	}

	switch exchange.ExchangeType {
	case "binance", "bybit", "okx", "bitget", "gate", "kucoin", "hyperliquid", "aster", "lighter", "indodax", "hz":
		return "", "", nil
	default:
		return formatTraderCreationError(
				fmt.Sprintf("交易所账户「%s」使用了当前版本暂不支持的类型 %s", exchangeDisplayName(exchange), exchange.ExchangeType),
				"请改用当前版本支持的交易所账户后，再重新创建机器人",
			), "trader.create.exchange_unsupported", mapStringPairs(
				"exchange_name", exchangeDisplayName(exchange),
				"exchange_type", exchange.ExchangeType,
			)
	}
}

func classifyTraderSetupReason(reason string) (string, string) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "", ""
	}

	lower := strings.ToLower(trimmed)

	switch {
	case strings.Contains(lower, "failed to parse strategy config"),
		strings.Contains(lower, "failed to parse strategy configuration"):
		return "trader.reason.strategy_config_invalid", "当前策略配置内容已损坏，系统暂时无法解析"
	case strings.Contains(lower, "has no strategy configured"):
		return "trader.reason.strategy_missing", "当前机器人缺少有效的交易策略配置"
	case strings.Contains(lower, "failed to parse private key"),
		(strings.Contains(lower, "invalid hex character") && strings.Contains(lower, "private key")):
		return "trader.reason.private_key_invalid", "私钥格式不正确，系统无法识别"
	case strings.Contains(lower, "failed to initialize hyperliquid trader"):
		return "trader.reason.hyperliquid_init_failed", "Hyperliquid 账户初始化失败，请确认私钥、主钱包地址和 Agent Wallet 配置是否正确"
	case strings.Contains(lower, "failed to initialize aster trader"):
		return "trader.reason.aster_init_failed", "Aster 账户初始化失败，请确认 Aster User、Signer 和私钥是否正确"
	case strings.Contains(lower, "failed to get meta information"):
		return "trader.reason.exchange_meta_unavailable", "系统暂时无法从交易所读取账户元信息"
	case strings.Contains(lower, "security check failed") && strings.Contains(lower, "agent wallet balance too high"):
		return "trader.reason.hyperliquid_agent_balance_too_high", "Hyperliquid Agent Wallet 余额过高，不符合当前安全要求"
	case strings.Contains(lower, "failed to initialize account"):
		return "trader.reason.exchange_account_init_failed", "交易所账户初始化失败，请确认钱包地址和 API Key 是否匹配"
	case strings.Contains(lower, "unsupported trading platform"):
		return "trader.reason.exchange_unsupported", "当前交易所类型暂不支持机器人初始化"
	case strings.Contains(lower, "initial balance not set and unable to fetch balance from exchange"):
		return "trader.reason.exchange_balance_unavailable", "系统暂时无法从交易所读取账户余额（请检查 API 权限、IP 白名单、代理出口与密钥是否正确）"
	case strings.Contains(lower, "initial balance must be greater than 0"):
		return "trader.reason.exchange_zero_balance", "合约侧可用余额为 0 或无法获取；请向币安合约账户转入资金，并确认 API 可读取余额"
	case strings.Contains(lower, "trader id") && strings.Contains(lower, "does not exist"):
		return "trader.reason.runtime_not_loaded", "运行实例未加载（常见于 AI 模型被停用、交易所账户被禁用，或初始化报错）。请刷新页面；仍不行则检查模型启用状态与交易所配置"
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "no such host"), strings.Contains(lower, "connection refused"):
		return "trader.reason.exchange_service_unreachable", "系统暂时无法连接交易所服务"
	default:
		return "trader.reason.unknown", trimmed
	}
}

func humanizeTraderSetupReason(reason string) string {
	_, message := classifyTraderSetupReason(reason)
	return message
}

// explainTraderSetupFailure 将加载/初始化错误转成用户可读中文（避免 SanitizeError 清空后只剩笼统提示）
func explainTraderSetupFailure(err error) string {
	if err == nil {
		return ""
	}
	sanitized := SanitizeError(err, "")
	reason := humanizeTraderSetupReason(sanitized)
	if reason != "" {
		return reason
	}
	if IsSensitiveError(err) {
		return "连接交易所或校验 API 失败（详细错误已隐藏，多为网络或权限问题）。请重点检查：API Key 与合约权限、交易所 IP 白名单（只加专属代理出口，不要加平台服务器 IP）、账户是否在合约侧有余额。"
	}
	if sanitized != "" {
		return sanitized
	}
	return strings.TrimSpace(err.Error())
}

func traderSetupReasonParams(err error, fallback string, kv ...string) map[string]string {
	params := mapStringPairs(kv...)
	rawReason := SanitizeError(err, fallback)
	reasonKey, reasonMessage := classifyTraderSetupReason(rawReason)
	if reasonMessage == "" && fallback != "" {
		reasonMessage = fallback
	}
	if reasonMessage != "" {
		if params == nil {
			params = map[string]string{}
		}
		params["reason"] = reasonMessage
	}
	if reasonKey != "" {
		if params == nil {
			params = map[string]string{}
		}
		params["reason_key"] = reasonKey
	}
	return params
}

func describeTraderLoadError(traderName string, err error) string {
	if err == nil {
		return formatTraderCreationError("机器人配置虽然保存了，但运行实例没有成功初始化", "请检查模型、策略和交易所配置是否完整，然后再试一次")
	}

	reason := humanizeTraderSetupReason(SanitizeError(err, ""))
	if reason == "" {
		return formatTraderCreationError(
			fmt.Sprintf("机器人「%s」在初始化运行实例时没有成功启动", traderName),
			"请检查模型、策略和交易所配置是否完整，然后再试一次",
		)
	}

	return formatTraderCreationError(
		fmt.Sprintf("机器人「%s」在初始化运行实例时没有成功启动，原因是：%s", traderName, reason),
		"请检查模型、策略和交易所配置是否完整，然后再试一次",
	)
}

func describeTraderCreationWarning(traderName string, err error) string {
	if err == nil {
		return fmt.Sprintf("机器人「%s」已经保存，但当前还没有通过启动前校验。请先检查模型、策略和交易所配置，修正后再点击启动。", traderName)
	}

	reason := explainTraderSetupFailure(err)
	if reason == "" {
		return fmt.Sprintf("机器人「%s」已经保存，但当前暂时还不能启动。请先检查模型、策略和交易所配置，修正后再点击启动。", traderName)
	}

	return fmt.Sprintf("机器人「%s」已经保存，但当前暂时还不能启动，原因是：%s。请先检查模型、策略和交易所配置，修正后再点击启动。", traderName, reason)
}

func describeTraderStartError(traderName string, err error) string {
	if err == nil {
		return fmt.Sprintf("这次未能启动机器人：机器人「%s」暂时还不能启动。请检查模型、策略和交易所配置后，再重新点击启动。", traderName)
	}

	reason := explainTraderSetupFailure(err)
	if reason == "" {
		return fmt.Sprintf("这次未能启动机器人：机器人「%s」暂时还不能启动。请检查模型、策略和交易所配置后，再重新点击启动。", traderName)
	}

	return fmt.Sprintf("这次未能启动机器人：机器人「%s」暂时还不能启动，原因是：%s。请检查模型、策略和交易所配置后，再重新点击启动。", traderName, reason)
}

func formatTraderStartError(reason, nextStep string) string {
	if nextStep == "" {
		return fmt.Sprintf("这次未能启动机器人：%s。", reason)
	}
	return fmt.Sprintf("这次未能启动机器人：%s。%s。", reason, nextStep)
}

func traderStartPlatformBalanceRequirement(fullCfg *store.TraderFullConfig, billing *store.BillingStore, userID string) (required float64, ok bool) {
	if fullCfg == nil {
		return 0, false
	}
	if fullCfg.Strategy != nil {
		if cfg, err := fullCfg.Strategy.ParseConfig(); err == nil && store.IsComkunMarketFollowStrategy(cfg) {
			return store.ComkunFollowScanFeeMaxUSDTOrDefault(), true
		}
	}
	if fullCfg.AIModel != nil {
		provider := strings.TrimSpace(fullCfg.AIModel.Provider)
		if provider == "claw402" || provider == "comkun_proxy" || provider == "comkun_ai" {
			return 0.00000001, true
		}
	}
	return 0, false
}

func isPlatformBilledAIProvider(provider string) bool {
	switch strings.TrimSpace(provider) {
	case "comkun_ai", "comkun_proxy":
		return true
	default:
		return false
	}
}

func traderStartBalanceError(balance, required float64) string {
	if required >= 0.01 {
		return fmt.Sprintf("平台余额不足（当前平台余额 %.4f USDT，不足启动所需 %.4f USDT）。请先充值平台余额后再启动智能体。", balance, required)
	}
	return fmt.Sprintf("平台余额不足（当前平台余额 %.4f USDT）。请先充值平台余额后再启动智能体。", balance)
}

// ErrTraderMarketSourceAlreadyBound 已购策略市场源（entitlement）下已存在绑定该源副本的交易员。
var ErrTraderMarketSourceAlreadyBound = errors.New("market purchased source already has a trader")

const multipleMarketSourceTraderExemptionEmail = "2497937010@qq.com"

func allowsMultipleMarketSourceTradersForEmail(email string) bool {
	return strings.EqualFold(strings.TrimSpace(email), multipleMarketSourceTraderExemptionEmail)
}

func (s *Server) assertSingleTraderForMarketPurchasedSource(userID string, st *store.Strategy, excludeTraderID string) error {
	if st == nil {
		return nil
	}
	user, err := s.store.User().GetByID(userID)
	if err != nil {
		return err
	}
	if allowsMultipleMarketSourceTradersForEmail(user.Email) {
		return nil
	}
	src := strings.TrimSpace(st.SourceStrategyID)
	if src == "" {
		return nil
	}
	has, err := s.store.Billing().HasEntitlement(userID, src)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	n, err := s.store.Trader().CountTradersLinkedToMarketSource(userID, src, excludeTraderID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrTraderMarketSourceAlreadyBound
	}
	return nil
}

// handleCreateTrader Create new AI trader
func (s *Server) handleCreateTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	var req CreateTraderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequestWithDetails(c, traderCreationRequestError("提交的信息不完整，或者格式不正确"), "trader.create.invalid_request", nil)
		return
	}

	// Validate leverage values
	if req.BTCETHLeverage < 0 || req.BTCETHLeverage > 50 {
		SafeBadRequestWithDetails(c, traderCreationRequestError("BTC/ETH 杠杆倍数需要在 1 到 50 倍之间"), "trader.create.invalid_btc_eth_leverage", nil)
		return
	}
	if req.AltcoinLeverage < 0 || req.AltcoinLeverage > 20 {
		SafeBadRequestWithDetails(c, traderCreationRequestError("山寨币杠杆倍数需要在 1 到 20 倍之间"), "trader.create.invalid_altcoin_leverage", nil)
		return
	}

	// Validate trading symbol format
	if req.TradingSymbols != "" {
		symbols := strings.Split(req.TradingSymbols, ",")
		for _, symbol := range symbols {
			symbol = strings.TrimSpace(symbol)
			if symbol != "" && !strings.HasSuffix(strings.ToUpper(symbol), "USDT") {
				SafeBadRequestWithDetails(c, traderCreationRequestError(
					fmt.Sprintf("交易对 %s 的格式不正确，目前只支持以 USDT 结尾的合约交易对", symbol),
				), "trader.create.invalid_symbol", mapStringPairs("symbol", symbol))
				return
			}
		}
	}

	model, err := s.store.AIModel().Get(userID, req.AIModelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			SafeBadRequestWithDetails(c, formatTraderCreationError("还没有找到你选择的 AI 模型", "请前往「设置 > 模型配置」先添加并启用一个可用模型，再回来创建机器人"), "trader.create.model_not_found", nil)
			return
		}
		SafeError(c, http.StatusInternalServerError,
			formatTraderCreationError("暂时无法读取你的 AI 模型配置", "请稍后重试；如果问题持续，再检查本地服务是否正常"),
			err,
		)
		return
	}
	if !model.Enabled {
		SafeBadRequestWithDetails(c, formatTraderCreationError(
			fmt.Sprintf("AI 模型「%s」目前还没有启用", model.Name),
			"请前往「设置 > 模型配置」启用它后，再重新创建机器人",
		), "trader.create.model_disabled", mapStringPairs("model_name", model.Name))
		return
	}
	if !isPlatformBilledAIProvider(model.Provider) && model.APIKey == "" {
		SafeBadRequestWithDetails(c, formatTraderCreationError(
			fmt.Sprintf("AI 模型「%s」缺少 API Key 或支付凭证", model.Name),
			"请前往「设置 > 模型配置」补全模型凭证后，再重新创建机器人",
		), "trader.create.model_missing_credentials", mapStringPairs("model_name", model.Name))
		return
	}

	if req.StrategyID == "" {
		SafeBadRequestWithDetails(c, formatTraderCreationError("你还没有选择交易策略", "请先选择一个策略，再继续创建机器人"), "trader.create.strategy_required", nil)
		return
	}

	var strategyConfig *store.StrategyConfig
	if req.StrategyID != "" {
		stObj, errSt := s.store.Strategy().Get(userID, req.StrategyID)
		if errSt != nil {
			if errors.Is(errSt, gorm.ErrRecordNotFound) {
				SafeBadRequestWithDetails(c, formatTraderCreationError("你选择的策略不存在，或者已经被删除了", "请重新选择一个可用策略后，再继续创建机器人"), "trader.create.strategy_not_found", nil)
				return
			}
			SafeError(c, http.StatusInternalServerError,
				formatTraderCreationError("暂时无法读取你选择的策略配置", "请稍后重试；如果问题持续，再检查本地服务是否正常"),
				errSt,
			)
			return
		}
		cfg, perr := stObj.ParseConfig()
		if perr != nil {
			SafeBadRequestWithDetails(c, formatTraderCreationError("策略配置无效，无法解析", "请在策略实验室检查该策略的配置"), "trader.create.strategy_config_invalid", nil)
			return
		}
		if err := store.ValidateComkunAIStrategyBinding(model, cfg); err != nil {
			SafeBadRequestWithDetails(c, formatTraderCreationError(err.Error(), "请更换 AI 模型或更换策略后再创建"), "trader.create.ai_strategy_mismatch", nil)
			return
		}
		strategyConfig = cfg
		if err := s.assertSingleTraderForMarketPurchasedSource(userID, stObj, ""); err != nil {
			if errors.Is(err, ErrTraderMarketSourceAlreadyBound) {
				SafeBadRequestWithDetails(
					c,
					formatTraderCreationError(
						"该策略来自你已购买的策略市场订阅/解锁包，同一市场源只能绑定一个交易员",
						"请先为其他机器人更换策略，或删除、停用已占用该策略包的交易员后再创建",
					),
					"trader.create.market_source_already_bound",
					nil,
				)
				return
			}
			SafeInternalError(c, formatTraderCreationError("暂时无法校验策略市场绑定规则", "请稍后重试"), err)
			return
		}
	}

	// Generate trader ID (use short UUID prefix for readability)
	exchangeIDShort := req.ExchangeID
	if len(exchangeIDShort) > 8 {
		exchangeIDShort = exchangeIDShort[:8]
	}
	traderID := fmt.Sprintf("%s_%s_%d", exchangeIDShort, req.AIModelID, time.Now().Unix())

	// Set default values
	isCrossMargin := true // Default to cross margin mode
	if req.IsCrossMargin != nil {
		isCrossMargin = *req.IsCrossMargin
	}

	showInCompetition := true // Default to show in competition
	if req.ShowInCompetition != nil {
		showInCompetition = *req.ShowInCompetition
	}

	// Set leverage default values
	btcEthLeverage := 10 // Default value
	altcoinLeverage := 5 // Default value
	if req.BTCETHLeverage > 0 {
		btcEthLeverage = req.BTCETHLeverage
	}
	if req.AltcoinLeverage > 0 {
		altcoinLeverage = req.AltcoinLeverage
	}

	// Set system prompt template default value
	systemPromptTemplate := "default"
	if req.SystemPromptTemplate != "" {
		systemPromptTemplate = req.SystemPromptTemplate
	}

	// Set scan interval default value
	scanIntervalMinutes := req.ScanIntervalMinutes
	if scanIntervalMinutes < 3 {
		scanIntervalMinutes = 3 // Default 3 minutes, not allowed to be less than 3
	}

	// Query exchange actual balance, override user input
	actualBalance := req.InitialBalance // Default to use user input
	exchanges, err := s.store.Exchange().List(userID)
	if err != nil {
		SafeError(c, http.StatusInternalServerError,
			formatTraderCreationError("暂时无法读取你的交易所配置", "请稍后重试；如果问题持续，再检查本地服务是否正常"),
			err,
		)
		return
	}

	// Find matching exchange configuration
	var exchangeCfg *store.Exchange
	for _, ex := range exchanges {
		if ex.ID == req.ExchangeID {
			exchangeCfg = ex
			break
		}
	}

	if exchangeMsg, exchangeErrorKey, exchangeErrorParams := validateExchangeForTraderCreation(exchangeCfg); exchangeMsg != "" {
		SafeBadRequestWithDetails(c, exchangeMsg, exchangeErrorKey, exchangeErrorParams)
		return
	}
	if err := store.ValidateStrategyExchange(exchangeCfg.ExchangeType, strategyConfig); err != nil {
		SafeBadRequestWithDetails(c, formatTraderCreationError(
			err.Error(), "请重新选择匹配的策略或交易账户",
		), "trader.create.strategy_exchange_mismatch", nil)
		return
	}

	if existing, dupErr := s.findTraderUsingStrategyExchange(userID, req.ExchangeID, req.StrategyID, "", false); dupErr != nil {
		SafeInternalError(c, formatTraderCreationError("暂时无法检查重复机器人", "请稍后重试"), dupErr)
		return
	} else if existing != nil {
		SafeBadRequestWithDetails(
			c,
			formatTraderCreationError(
				fmt.Sprintf("交易所账户「%s」已经绑定过这个策略机器人「%s」", exchangeDisplayName(exchangeCfg), existing.Name),
				"请直接编辑或启动现有机器人，不要重复创建同一套交易",
			),
			"trader.create.duplicate_strategy_exchange",
			mapStringPairs("trader_id", existing.ID, "trader_name", existing.Name),
		)
		return
	}

	{
		tempTrader, createErr := buildExchangeProbeTrader(exchangeCfg, userID)
		if createErr != nil {
			SafeBadRequestWithDetails(c, formatTraderCreationError(
				fmt.Sprintf("交易所账户「%s」没有通过初始化校验，原因是：%s", exchangeDisplayName(exchangeCfg), humanizeTraderSetupReason(SanitizeError(createErr, "配置校验未通过"))),
				"请前往「设置 > 交易所配置」检查这个账户的密钥、地址和账户信息是否填写正确",
			), "trader.create.exchange_probe_failed", traderSetupReasonParams(createErr, "配置校验未通过",
				"exchange_name", exchangeDisplayName(exchangeCfg),
			))
			return
		} else if tempTrader != nil {
			if closer, ok := tempTrader.(interface{ Close() }); ok {
				defer closer.Close()
			}
			// Query actual balance
			balanceInfo, balanceErr := tempTrader.GetBalance()
			if balanceErr != nil {
				logger.Infof("⚠️ Failed to query exchange balance, using user input for initial balance: %v", balanceErr)
			} else {
				if extractedBalance, found := extractExchangeTotalEquity(balanceInfo); found {
					actualBalance = extractedBalance
					logger.Infof("✓ Queried exchange total equity: %.2f %s (user input: %.2f)",
						actualBalance, accountAssetForExchange(exchangeCfg.ExchangeType), req.InitialBalance)
				} else {
					logger.Infof("⚠️ Unable to extract total equity from balance info, balanceInfo=%v, using user input for initial balance", balanceInfo)
				}
			}
		}
	}

	// Create trader configuration (database entity)
	logger.Infof("🔧 DEBUG: Starting to create trader config, ID=%s, Name=%s, AIModel=%s, Exchange=%s, StrategyID=%s", traderID, req.Name, req.AIModelID, req.ExchangeID, req.StrategyID)
	traderRecord := &store.Trader{
		ID:                   traderID,
		UserID:               userID,
		Name:                 req.Name,
		AIModelID:            req.AIModelID,
		ExchangeID:           req.ExchangeID,
		StrategyID:           req.StrategyID, // Associated strategy ID (new version)
		InitialBalance:       actualBalance,  // Use actual queried balance
		BTCETHLeverage:       btcEthLeverage,
		AltcoinLeverage:      altcoinLeverage,
		TradingSymbols:       req.TradingSymbols,
		UseAI500:             req.UseAI500,
		UseOITop:             req.UseOITop,
		CustomPrompt:         req.CustomPrompt,
		OverrideBasePrompt:   req.OverrideBasePrompt,
		SystemPromptTemplate: systemPromptTemplate,
		IsCrossMargin:        isCrossMargin,
		ShowInCompetition:    showInCompetition,
		ScanIntervalMinutes:  scanIntervalMinutes,
		IsRunning:            false,
	}

	// Save to database
	logger.Infof("🔧 DEBUG: Preparing to call CreateTrader")
	err = s.store.Trader().Create(traderRecord)
	if err != nil {
		logger.Infof("❌ Failed to create trader: %v", err)
		publicMsg := SanitizeError(err, formatTraderCreationError("机器人配置没有保存成功", "请检查名称、模型、策略和交易所配置后，再试一次"))
		statusCode := http.StatusBadRequest
		if publicMsg == formatTraderCreationError("机器人配置没有保存成功", "请检查名称、模型、策略和交易所配置后，再试一次") {
			statusCode = http.StatusInternalServerError
		}
		SafeError(c, statusCode, publicMsg, err)
		return
	}
	logger.Infof("🔧 DEBUG: CreateTrader succeeded")

	// Immediately load new trader into TraderManager
	logger.Infof("🔧 DEBUG: Preparing to call LoadUserTraders")
	startupWarning := ""
	err = s.traderManager.LoadUserTradersFromStore(s.store, userID)
	if err != nil {
		logger.Infof("⚠️ Failed to load user traders into memory: %v", err)
		startupWarning = describeTraderCreationWarning(req.Name, err)
	}
	logger.Infof("🔧 DEBUG: LoadUserTraders completed")

	if startupWarning == "" {
		if loadErr := s.traderManager.GetLoadError(traderID); loadErr != nil {
			logger.Infof("⚠️ Trader %s failed to load after creation: %v", traderID, loadErr)
			startupWarning = describeTraderCreationWarning(req.Name, loadErr)
		}
	}

	if startupWarning == "" {
		if _, getErr := s.traderManager.GetTrader(traderID); getErr != nil {
			logger.Infof("⚠️ Trader %s not found in memory after creation: %v", traderID, getErr)
			startupWarning = describeTraderCreationWarning(req.Name, getErr)
		}
	}

	logger.Infof("✓ Trader created successfully: %s (model: %s, exchange: %s)", req.Name, req.AIModelID, req.ExchangeID)

	c.JSON(http.StatusCreated, gin.H{
		"trader_id":       traderID,
		"trader_name":     req.Name,
		"ai_model":        req.AIModelID,
		"is_running":      false,
		"startup_warning": startupWarning,
	})
}

// handleUpdateTrader Update trader configuration
func (s *Server) handleUpdateTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	var req UpdateTraderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	// Check if trader exists and belongs to current user
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get trader list"})
		return
	}

	var existingTrader *store.Trader
	for _, t := range traders {
		if t.ID == traderID {
			existingTrader = t
			break
		}
	}

	if existingTrader == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist"})
		return
	}

	// Set default values
	isCrossMargin := existingTrader.IsCrossMargin // Keep original value
	if req.IsCrossMargin != nil {
		isCrossMargin = *req.IsCrossMargin
	}

	showInCompetition := existingTrader.ShowInCompetition // Keep original value
	if req.ShowInCompetition != nil {
		showInCompetition = *req.ShowInCompetition
	}

	// Set leverage default values
	btcEthLeverage := req.BTCETHLeverage
	altcoinLeverage := req.AltcoinLeverage
	if btcEthLeverage <= 0 {
		btcEthLeverage = existingTrader.BTCETHLeverage // Keep original value
	}
	if altcoinLeverage <= 0 {
		altcoinLeverage = existingTrader.AltcoinLeverage // Keep original value
	}

	// Set scan interval, allow updates
	scanIntervalMinutes := req.ScanIntervalMinutes
	logger.Infof("📊 Update trader scan_interval: req=%d, existing=%d", req.ScanIntervalMinutes, existingTrader.ScanIntervalMinutes)
	if scanIntervalMinutes <= 0 {
		scanIntervalMinutes = existingTrader.ScanIntervalMinutes // Keep original value
	} else if scanIntervalMinutes < 3 {
		scanIntervalMinutes = 3
	}
	logger.Infof("📊 Final scan_interval_minutes: %d", scanIntervalMinutes)

	// Set system prompt template
	systemPromptTemplate := req.SystemPromptTemplate
	if systemPromptTemplate == "" {
		systemPromptTemplate = existingTrader.SystemPromptTemplate // Keep original value
	}

	// Handle strategy ID (if not provided, keep original value)
	strategyID := req.StrategyID
	if strategyID == "" {
		strategyID = existingTrader.StrategyID
	}
	exchangeID := req.ExchangeID
	if exchangeID == "" {
		exchangeID = existingTrader.ExchangeID
	}

	modelRow, errModel := s.store.AIModel().Get(userID, req.AIModelID)
	if errModel != nil {
		if errors.Is(errModel, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "AI model not found"})
			return
		}
		SafeInternalError(c, "Failed to load AI model", errModel)
		return
	}
	stObj, errSt := s.store.Strategy().Get(userID, strategyID)
	if errSt != nil {
		if errors.Is(errSt, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Strategy not found"})
			return
		}
		SafeInternalError(c, "Failed to load strategy", errSt)
		return
	}
	cfgUp, perr := stObj.ParseConfig()
	if perr != nil {
		SafeBadRequestWithDetails(c, fmt.Sprintf("策略配置无效：%v", perr), "trader.update.strategy_config_invalid", nil)
		return
	}
	if err := store.ValidateComkunAIStrategyBinding(modelRow, cfgUp); err != nil {
		SafeBadRequestWithDetails(c, err.Error(), "trader.update.ai_strategy_mismatch", nil)
		return
	}
	exchanges, err := s.store.Exchange().List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to load exchange configs", err)
		return
	}
	var exchangeCfg *store.Exchange
	for _, exchange := range exchanges {
		if exchange.ID == exchangeID {
			exchangeCfg = exchange
			break
		}
	}
	if exchangeMsg, exchangeErrorKey, exchangeErrorParams := validateExchangeForTraderCreation(exchangeCfg); exchangeMsg != "" {
		SafeBadRequestWithDetails(c, exchangeMsg, exchangeErrorKey, exchangeErrorParams)
		return
	}
	if err := store.ValidateStrategyExchange(exchangeCfg.ExchangeType, cfgUp); err != nil {
		SafeBadRequestWithDetails(c, err.Error(), "trader.update.strategy_exchange_mismatch", nil)
		return
	}
	if err := s.assertSingleTraderForMarketPurchasedSource(userID, stObj, traderID); err != nil {
		if errors.Is(err, ErrTraderMarketSourceAlreadyBound) {
			SafeBadRequestWithDetails(
				c,
				"该策略来自你已购买的策略市场订阅/解锁包，同一市场源只能绑定一个交易员。请先为其他机器人更换策略，或停用已占用该包的交易员。",
				"trader.update.market_source_already_bound",
				nil,
			)
			return
		}
		SafeInternalError(c, "检查策略市场绑定规则失败", err)
		return
	}

	exchangeChanged := exchangeID != existingTrader.ExchangeID
	resetInitialBalance := exchangeChanged && req.InitialBalance <= 0

	initialBalance := existingTrader.InitialBalance
	if req.InitialBalance > 0 {
		initialBalance = req.InitialBalance
	}
	if resetInitialBalance {
		initialBalance = 0
	}

	// Update trader configuration
	traderRecord := &store.Trader{
		ID:                   traderID,
		UserID:               userID,
		Name:                 req.Name,
		AIModelID:            req.AIModelID,
		ExchangeID:           exchangeID,
		StrategyID:           strategyID, // Associated strategy ID
		InitialBalance:       initialBalance,
		BTCETHLeverage:       btcEthLeverage,
		AltcoinLeverage:      altcoinLeverage,
		TradingSymbols:       req.TradingSymbols,
		CustomPrompt:         req.CustomPrompt,
		OverrideBasePrompt:   req.OverrideBasePrompt,
		SystemPromptTemplate: systemPromptTemplate,
		IsCrossMargin:        isCrossMargin,
		ShowInCompetition:    showInCompetition,
		ScanIntervalMinutes:  scanIntervalMinutes,
		IsRunning:            existingTrader.IsRunning, // Keep original value
	}

	// Check if trader was running before update (we'll restart it after)
	wasRunning := false
	if existingMemTrader, memErr := s.traderManager.GetTrader(traderID); memErr == nil {
		status := existingMemTrader.GetStatus()
		if running, ok := status["is_running"].(bool); ok && running {
			wasRunning = true
			logger.Infof("🔄 Trader %s was running, will restart with new config after update", traderID)
		}
	}

	// Update database
	logger.Infof("🔄 Updating trader: ID=%s, Name=%s, AIModelID=%s, StrategyID=%s, ScanInterval=%d min",
		traderRecord.ID, traderRecord.Name, traderRecord.AIModelID, traderRecord.StrategyID, scanIntervalMinutes)
	err = s.store.Trader().Update(traderRecord)
	if err != nil {
		SafeInternalError(c, "Failed to update trader", err)
		return
	}

	if resetInitialBalance {
		logger.Infof("🔄 Exchange changed for trader %s, resetting stale initial_balance to 0", traderID)
		if err := s.store.Trader().UpdateInitialBalance(userID, traderID, 0); err != nil {
			SafeInternalError(c, "Failed to reset trader initial balance", err)
			return
		}
	}

	// Remove old trader from memory first (this also stops if running)
	s.traderManager.RemoveTrader(traderID)

	// Reload traders into memory with fresh config
	err = s.traderManager.LoadUserTradersFromStore(s.store, userID)
	if err != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", err)
	}

	// If trader was running before, restart it with new config
	if wasRunning {
		if reloadedTrader, getErr := s.traderManager.GetTrader(traderID); getErr == nil {
			go func() {
				logger.Infof("▶️ Restarting trader %s with new config...", traderID)
				if runErr := reloadedTrader.Run(); runErr != nil {
					logger.Infof("❌ Trader %s runtime error: %v", traderID, runErr)
				}
			}()
		}
	}

	logger.Infof("✓ Trader updated successfully: %s (model: %s, exchange: %s, strategy: %s)", req.Name, req.AIModelID, exchangeID, strategyID)

	c.JSON(http.StatusOK, gin.H{
		"trader_id":   traderID,
		"trader_name": req.Name,
		"ai_model":    req.AIModelID,
		"message":     "Trader updated successfully",
	})
}

// handleDeleteTrader Delete trader
func (s *Server) handleDeleteTrader(c *gin.Context) {
	userID := c.GetString("user_id")
	traderID := c.Param("id")

	// Delete from database
	err := s.store.Trader().Delete(userID, traderID)
	if err != nil {
		SafeInternalError(c, "Failed to delete trader", err)
		return
	}

	// 从内存移除（内部会阻塞 Stop，等循环退出后再删，避免野指针）
	s.traderManager.RemoveTrader(traderID)

	logger.Infof("✓ Trader deleted: %s", traderID)
	c.JSON(http.StatusOK, gin.H{"message": "Trader deleted"})
}

// handleStartTrader Start trader（仅当前登录用户自己的交易员）
func (s *Server) handleStartTrader(c *gin.Context) {
	s.handleStartTraderForUser(c, c.GetString("user_id"), c.Param("id"))
}

func (s *Server) expiredCEXProxyHost(fullCfg *store.TraderFullConfig) (bool, string, error) {
	if fullCfg == nil || fullCfg.Exchange == nil || !exchangeTypeUsesOutboundProxy(fullCfg.Exchange.ExchangeType) {
		return false, "", nil
	}
	bound, expiresAt, displayHost, err := s.store.ProxyPool().ProxyBindingForExchange(fullCfg.Exchange.ID)
	if err != nil {
		return false, "", err
	}
	if !bound || expiresAt == nil || expiresAt.After(time.Now().UTC()) {
		return false, "", nil
	}
	return true, strings.TrimSpace(displayHost), nil
}

func (s *Server) ensureCEXOutboundProxyBeforeTraderStart(userID string, fullCfg *store.TraderFullConfig) error {
	if fullCfg == nil || fullCfg.Exchange == nil {
		return nil
	}
	ex := fullCfg.Exchange
	if !exchangeTypeUsesOutboundProxy(ex.ExchangeType) || strings.TrimSpace(string(ex.OutboundProxyURL)) != "" {
		return nil
	}
	return s.store.GormDB().Transaction(func(tx *gorm.DB) error {
		var current store.Exchange
		if err := tx.Where("id = ? AND user_id = ?", ex.ID, userID).First(&current).Error; err != nil {
			return err
		}
		if !exchangeTypeUsesOutboundProxy(current.ExchangeType) || strings.TrimSpace(string(current.OutboundProxyURL)) != "" {
			return nil
		}
		_, proxyPlain, err := s.store.ProxyPool().ClaimProxyForExchange(tx, userID, current.ID, current.ExchangeType)
		if err != nil {
			return err
		}
		logger.Infof("📌 启动交易员前自动分配 CEX 出口代理 exchange_id=%s type=%s", current.ID, current.ExchangeType)
		return tx.Model(&store.Exchange{}).
			Where("id = ? AND user_id = ?", current.ID, userID).
			Updates(map[string]interface{}{
				"outbound_proxy_url": crypto.EncryptedString(proxyPlain),
				"updated_at":         time.Now().UTC(),
			}).Error
	})
}

// handleStartTraderForUser 启动交易员；userID 为交易员所属用户（管理员可代客启动）
func (s *Server) handleStartTraderForUser(c *gin.Context, userID, traderID string) {
	// Verify trader belongs to userID
	fullCfg, err := s.store.Trader().GetFullConfig(userID, traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist or no access permission"})
		return
	}
	traderName := traderID
	if fullCfg != nil && fullCfg.Trader != nil && fullCfg.Trader.Name != "" {
		traderName = fullCfg.Trader.Name
	}

	if fullCfg != nil {
		var stratCfg *store.StrategyConfig
		if fullCfg.Strategy != nil {
			var perr error
			stratCfg, perr = fullCfg.Strategy.ParseConfig()
			if perr != nil {
				SafeBadRequestWithDetails(c, fmt.Sprintf("策略配置无效：%v", perr), "trader.start.strategy_config_invalid", mapStringPairs("trader_name", traderName))
				return
			}
		}
		if err := store.ValidateComkunAIStrategyBinding(fullCfg.AIModel, stratCfg); err != nil {
			SafeBadRequestWithDetails(c, err.Error(), "trader.start.ai_strategy_mismatch", mapStringPairs("trader_name", traderName))
			return
		}
	}

	if proxyExpired, expiredHost, proxyErr := s.expiredCEXProxyHost(fullCfg); proxyErr != nil {
		SafeInternalError(c, "读取代理有效期失败", proxyErr)
		return
	} else if proxyExpired {
		message := "IP已过期，请联系管理员更换代理后再启动"
		if expiredHost != "" {
			message = fmt.Sprintf("IP已过期（%s），请联系管理员更换代理后再启动", expiredHost)
		}
		SafeBadRequestWithDetails(c, message, "trader.start.proxy_expired", mapStringPairs("trader_name", traderName))
		return
	}

	if fullCfg != nil && fullCfg.Trader != nil {
		if existing, dupErr := s.findTraderUsingStrategyExchange(userID, fullCfg.Trader.ExchangeID, fullCfg.Trader.StrategyID, traderID, true); dupErr != nil {
			SafeInternalError(c, "检查重复运行机器人失败", dupErr)
			return
		} else if existing != nil {
			SafeBadRequestWithDetails(
				c,
				formatTraderStartError(
					fmt.Sprintf("同一个交易所账户和策略已有正在运行的机器人「%s」", existing.Name),
					"请先停止已有机器人，再启动当前机器人，避免重复扣费和重复下单",
				),
				"trader.start.duplicate_strategy_exchange_running",
				mapStringPairs("trader_id", existing.ID, "trader_name", existing.Name),
			)
			return
		}
	}

	if requiredBalance, needsPlatformBalance := traderStartPlatformBalanceRequirement(fullCfg, s.store.Billing(), userID); needsPlatformBalance {
		u, uerr := s.store.User().GetByID(userID)
		if uerr != nil {
			SafeInternalError(c, "balance read", uerr)
			return
		}
		if u.BalanceUSDT+1e-9 < requiredBalance {
			SafeBadRequestWithDetails(c, traderStartBalanceError(u.BalanceUSDT, requiredBalance), "trader.start.platform_balance_insufficient", mapStringPairs("trader_name", traderName))
			return
		}
	}

	// Check if trader exists in memory and if it's running
	existingTrader, _ := s.traderManager.GetTrader(traderID)
	if existingTrader != nil {
		status := existingTrader.GetStatus()
		if isRunning, ok := status["is_running"].(bool); ok && isRunning {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Trader is already running"})
			return
		}
		// Trader exists but is stopped - remove from memory to reload fresh config
		logger.Infof("🔄 Removing stopped trader %s from memory to reload config...", traderID)
		s.traderManager.RemoveTrader(traderID)
	}

	if err := s.ensureCEXOutboundProxyBeforeTraderStart(userID, fullCfg); err != nil {
		if errors.Is(err, store.ErrProxyPoolExhausted) {
			SafeBadRequestWithDetails(c, "暂无可用出口代理，请联系管理员补充代理池后再启动", "trader.start.proxy_pool_exhausted", mapStringPairs("trader_name", traderName))
			return
		}
		SafeInternalError(c, "启动前分配出口代理失败", err)
		return
	}

	// Load trader from database (always reload to get latest config)
	logger.Infof("🔄 Loading trader %s from database...", traderID)
	if loadErr := s.traderManager.LoadUserTradersFromStore(s.store, userID); loadErr != nil {
		logger.Infof("❌ Failed to load user traders: %v", loadErr)
		SafeErrorWithDetails(c, http.StatusInternalServerError, describeTraderStartError(traderName, loadErr), "trader.start.load_failed", traderSetupReasonParams(loadErr, "", "trader_name", traderName), loadErr)
		return
	}

	trader, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		if fullCfg != nil && fullCfg.Trader != nil {
			// Check strategy
			if fullCfg.Strategy == nil {
				SafeBadRequestWithDetails(c, describeTraderStartError(traderName, fmt.Errorf("trader has no strategy configured")), "trader.start.strategy_missing", mapStringPairs("trader_name", traderName))
				return
			}
			// Check AI model
			if fullCfg.AIModel == nil {
				SafeBadRequestWithDetails(c, formatTraderStartError("这个机器人关联的 AI 模型不存在", "请前往「设置 > 模型配置」检查后，再重新点击启动"), "trader.start.model_not_found", mapStringPairs("trader_name", traderName))
				return
			}
			if !fullCfg.AIModel.Enabled {
				SafeBadRequestWithDetails(c, formatTraderStartError(
					fmt.Sprintf("机器人「%s」关联的 AI 模型「%s」目前还没有启用", traderName, fullCfg.AIModel.Name),
					"请前往「设置 > 模型配置」启用它后，再重新点击启动",
				), "trader.start.model_disabled", mapStringPairs("trader_name", traderName, "model_name", fullCfg.AIModel.Name))
				return
			}
			// Check exchange
			if fullCfg.Exchange == nil {
				SafeBadRequestWithDetails(c, formatTraderStartError("这个机器人关联的交易所账户不存在", "请前往「设置 > 交易所配置」检查后，再重新点击启动"), "trader.start.exchange_not_found", mapStringPairs("trader_name", traderName))
				return
			}
			if !fullCfg.Exchange.Enabled {
				SafeBadRequestWithDetails(c, formatTraderStartError(
					fmt.Sprintf("机器人「%s」关联的交易所账户「%s」目前还没有启用", traderName, exchangeDisplayName(fullCfg.Exchange)),
					"请前往「设置 > 交易所配置」启用它后，再重新点击启动",
				), "trader.start.exchange_disabled", mapStringPairs("trader_name", traderName, "exchange_name", exchangeDisplayName(fullCfg.Exchange)))
				return
			}
		}
		// Check if there's a specific load error
		if loadErr := s.traderManager.GetLoadError(traderID); loadErr != nil {
			SafeBadRequestWithDetails(c, describeTraderStartError(traderName, loadErr), "trader.start.load_failed", traderSetupReasonParams(loadErr, "", "trader_name", traderName))
			return
		}
		SafeBadRequestWithDetails(c, describeTraderStartError(traderName, err), "trader.start.setup_invalid", traderSetupReasonParams(err, "", "trader_name", traderName))
		return
	}

	// Start trader
	go func() {
		logger.Infof("▶️  Starting trader %s (%s)", traderID, trader.GetName())
		if err := trader.Run(); err != nil {
			logger.Infof("❌ Trader %s runtime error: %v", trader.GetName(), err)
		}
	}()

	// Update running status in database
	err = s.store.Trader().UpdateStatus(userID, traderID, true)
	if err != nil {
		logger.Infof("⚠️  Failed to update trader status: %v", err)
	}

	logger.Infof("✓ Trader %s started", trader.GetName())
	c.JSON(http.StatusOK, gin.H{"message": "Trader started"})
}

// handleStopTrader Stop trader（仅当前登录用户自己的交易员）
func (s *Server) handleStopTrader(c *gin.Context) {
	s.executeStopTrader(c, c.GetString("user_id"), c.Param("id"), false)
}

// executeStopTrader 停止交易员。adminForceDB：管理员在内存找不到实例时仍将数据库标为已停止（修复异常状态）
func (s *Server) executeStopTrader(c *gin.Context, userID, traderID string, adminForceDB bool) {
	_, err := s.store.Trader().GetFullConfig(userID, traderID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist or no access permission"})
		return
	}

	at, err := s.traderManager.GetTrader(traderID)
	if err != nil {
		_ = s.traderManager.LoadUserTradersFromStore(s.store, userID)
		at, err = s.traderManager.GetTrader(traderID)
	}
	if err != nil {
		if adminForceDB {
			_ = s.store.Trader().UpdateStatus(userID, traderID, false)
			logger.Infof("⏹ Admin stop: trader %s not in memory, DB marked stopped", traderID)
			c.JSON(http.StatusOK, gin.H{
				"message": "已将数据库标为停止（本机未加载该交易员进程，可能从未启动或已卸载）",
				"warning": "memory_instance_not_found",
			})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Trader does not exist"})
		return
	}

	status := at.GetStatus()
	if isRunning, ok := status["is_running"].(bool); ok && !isRunning {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Trader is already stopped"})
		return
	}

	at.StopAsync()

	err = s.store.Trader().UpdateStatus(userID, traderID, false)
	if err != nil {
		logger.Infof("⚠️  Failed to update trader status: %v", err)
	}

	logger.Infof("⏹  Trader %s stopped", at.GetName())
	c.JSON(http.StatusOK, gin.H{"message": "Trader stopped"})
}
