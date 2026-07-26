package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nofx/config"
	"nofx/crypto"
	"nofx/logger"
	"nofx/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ExchangeConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // "cex" or "dex"
	Enabled   bool   `json:"enabled"`
	APIKey    string `json:"apiKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
	Testnet   bool   `json:"testnet,omitempty"`
}

// SafeExchangeConfig Safe exchange configuration structure (does not contain sensitive information)
type SafeExchangeConfig struct {
	ID                      string `json:"id"`            // UUID
	ExchangeType            string `json:"exchange_type"` // "binance", "bybit", "okx", "hyperliquid", "aster", "lighter"
	AccountName             string `json:"account_name"`  // User-defined account name
	Name                    string `json:"name"`          // Display name
	Type                    string `json:"type"`          // "cex" or "dex"
	Enabled                 bool   `json:"enabled"`
	Testnet                 bool   `json:"testnet,omitempty"`
	APIURL                  string `json:"apiUrl,omitempty"`
	HyperliquidWalletAddr   string `json:"hyperliquidWalletAddr"`     // Hyperliquid wallet address (not sensitive)
	AsterUser               string `json:"asterUser"`                 // Aster username (not sensitive)
	AsterSigner             string `json:"asterSigner"`               // Aster signer (not sensitive)
	LighterWalletAddr       string `json:"lighterWalletAddr"`         // LIGHTER wallet address (not sensitive)
	OutboundProxyConfigured bool   `json:"outbound_proxy_configured"` // CEX：是否配置了 REST 出口代理（不返回具体地址）
	/** 当前出口是否来自管理员代理池分配 */
	OutboundProxyFromPool bool `json:"outbound_proxy_from_pool"`
	/** 池条目到期时间（RFC3339），仅 from_pool 时可能有 */
	OutboundProxyPoolExpiresAt *string `json:"outbound_proxy_pool_expires_at,omitempty"`
	/** API 白名单应填写的出口地址（与代理池 display_host 一致，可为 IP 或域名） */
	OutboundProxyWhitelistHost string `json:"outbound_proxy_whitelist_host,omitempty"`
}

type UpdateExchangeConfigRequest struct {
	Exchanges map[string]struct {
		Enabled                 bool    `json:"enabled"`
		APIKey                  string  `json:"api_key"`
		SecretKey               string  `json:"secret_key"`
		Passphrase              string  `json:"passphrase"` // OKX specific
		Testnet                 bool    `json:"testnet"`
		APIURL                  string  `json:"api_url"`
		HyperliquidWalletAddr   string  `json:"hyperliquid_wallet_addr"`
		HyperliquidUnifiedAcct  bool    `json:"hyperliquid_unified_account"` // Unified Account mode
		AsterUser               string  `json:"aster_user"`
		AsterSigner             string  `json:"aster_signer"`
		AsterPrivateKey         string  `json:"aster_private_key"`
		LighterWalletAddr       string  `json:"lighter_wallet_addr"`
		LighterPrivateKey       string  `json:"lighter_private_key"`
		LighterAPIKeyPrivateKey string  `json:"lighter_api_key_private_key"`
		LighterAPIKeyIndex      int     `json:"lighter_api_key_index"`
		OutboundProxyURL        *string `json:"outbound_proxy_url,omitempty"`         // CEX REST 出口代理；不设表示不改
		OutboundProxyClear      bool    `json:"outbound_proxy_clear,omitempty"`       // 为 true 时清除已保存代理
		OutboundProxyAutoAssign bool    `json:"outbound_proxy_auto_assign,omitempty"` // CEX：从管理员代理池重新分配一条（须不与手动 URL 同用）
	} `json:"exchanges"`
}

// CreateExchangeRequest request structure for creating a new exchange account
type CreateExchangeRequest struct {
	ExchangeType            string `json:"exchange_type" binding:"required"` // "binance", "bybit", "okx", "hyperliquid", "aster", "lighter"
	AccountName             string `json:"account_name"`                     // User-defined account name
	Enabled                 bool   `json:"enabled"`
	APIKey                  string `json:"api_key"`
	SecretKey               string `json:"secret_key"`
	Passphrase              string `json:"passphrase"`
	Testnet                 bool   `json:"testnet"`
	APIURL                  string `json:"api_url"`
	HyperliquidWalletAddr   string `json:"hyperliquid_wallet_addr"`
	HyperliquidUnifiedAcct  bool   `json:"hyperliquid_unified_account"` // Unified Account mode: Spot as Perp collateral
	AsterUser               string `json:"aster_user"`
	AsterSigner             string `json:"aster_signer"`
	AsterPrivateKey         string `json:"aster_private_key"`
	LighterWalletAddr       string `json:"lighter_wallet_addr"`
	LighterPrivateKey       string `json:"lighter_private_key"`
	LighterAPIKeyPrivateKey string `json:"lighter_api_key_private_key"`
	LighterAPIKeyIndex      int    `json:"lighter_api_key_index"`
	OutboundProxyURL        string `json:"outbound_proxy_url"` // 可选：CEX REST 独立出口（http(s)/socks5）
	/** CEX：未填 outbound_proxy_url 时是否从管理员代理池自动分配（默认 true） */
	AutoAssignOutboundProxy *bool `json:"auto_assign_outbound_proxy"`
}

func exchangeTypeUsesOutboundProxy(exchangeType string) bool {
	switch strings.ToLower(strings.TrimSpace(exchangeType)) {
	case "binance", "bybit", "okx", "bitget", "gate":
		return true
	default:
		return false
	}
}

func shouldAutoAssignCEXProxyCreate(req *CreateExchangeRequest) bool {
	if !exchangeTypeUsesOutboundProxy(req.ExchangeType) {
		return false
	}
	if strings.TrimSpace(req.OutboundProxyURL) != "" {
		return false
	}
	if req.AutoAssignOutboundProxy != nil && !*req.AutoAssignOutboundProxy {
		return false
	}
	return true
}

func normalizeHZAPIURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("HZ API URL must be a valid HTTPS address")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// handleGetExchangeConfigs Get exchange configurations
func (s *Server) handleGetExchangeConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	logger.Infof("🔍 Querying exchange configs for user %s", userID)
	exchanges, err := s.store.Exchange().List(userID)
	if err != nil {
		SafeInternalError(c, "Failed to get exchange configs", err)
		return
	}

	// If no exchanges in database, return empty array (user needs to create accounts)
	if len(exchanges) == 0 {
		logger.Infof("⚠️ No exchanges in database for user %s", userID)
		c.JSON(http.StatusOK, []SafeExchangeConfig{})
		return
	}

	logger.Infof("✅ Found %d exchange configs", len(exchanges))

	// Convert to safe response structure, remove sensitive information
	safeExchanges := make([]SafeExchangeConfig, len(exchanges))
	for i, exchange := range exchanges {
		safeExchanges[i] = SafeExchangeConfig{
			ID:                      exchange.ID,
			ExchangeType:            exchange.ExchangeType,
			AccountName:             exchange.AccountName,
			Name:                    exchange.Name,
			Type:                    exchange.Type,
			Enabled:                 exchange.Enabled,
			Testnet:                 exchange.Testnet,
			APIURL:                  exchange.APIURL,
			HyperliquidWalletAddr:   exchange.HyperliquidWalletAddr,
			AsterUser:               exchange.AsterUser,
			AsterSigner:             exchange.AsterSigner,
			LighterWalletAddr:       exchange.LighterWalletAddr,
			OutboundProxyConfigured: strings.TrimSpace(string(exchange.OutboundProxyURL)) != "",
		}
		if exchangeTypeUsesOutboundProxy(exchange.ExchangeType) {
			if ok, exp, host, qerr := s.store.ProxyPool().ProxyBindingForExchange(exchange.ID); qerr == nil && ok {
				safeExchanges[i].OutboundProxyFromPool = true
				if exp != nil {
					ts := exp.UTC().Format(time.RFC3339)
					safeExchanges[i].OutboundProxyPoolExpiresAt = &ts
				}
				if strings.TrimSpace(host) != "" {
					safeExchanges[i].OutboundProxyWhitelistHost = strings.TrimSpace(host)
				}
			}
		}
	}

	c.JSON(http.StatusOK, safeExchanges)
}

// handleUpdateExchangeConfigs Update exchange configurations (supports both encrypted and plain text based on config)
func (s *Server) handleUpdateExchangeConfigs(c *gin.Context) {
	userID := c.GetString("user_id")
	cfg := config.Get()

	// Read raw request body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var req UpdateExchangeConfigRequest

	// Check if transport encryption is enabled
	if !cfg.TransportEncryption {
		// Transport encryption disabled, accept plain JSON
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			logger.Infof("❌ Failed to parse plain JSON request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}
		logger.Infof("📝 Received plain text exchange config (UserID: %s)", userID)
	} else {
		// Transport encryption enabled, require encrypted payload
		var encryptedPayload crypto.EncryptedPayload
		if err := json.Unmarshal(bodyBytes, &encryptedPayload); err != nil {
			logger.Infof("❌ Failed to parse encrypted payload: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format, encrypted transmission required"})
			return
		}

		// Verify encrypted data
		if encryptedPayload.WrappedKey == "" {
			logger.Infof("❌ Detected unencrypted request (UserID: %s)", userID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "This endpoint only supports encrypted transmission, please use encrypted client",
				"code":    "ENCRYPTION_REQUIRED",
				"message": "Encrypted transmission is required for security reasons",
			})
			return
		}

		// Decrypt data
		decrypted, err := s.cryptoHandler.cryptoService.DecryptSensitiveData(&encryptedPayload)
		if err != nil {
			logger.Infof("❌ Failed to decrypt exchange config (UserID: %s): %v", userID, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decrypt data"})
			return
		}

		// Parse decrypted data
		if err := json.Unmarshal([]byte(decrypted), &req); err != nil {
			logger.Infof("❌ Failed to parse decrypted data: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse decrypted data"})
			return
		}
		logger.Infof("🔓 Decrypted exchange config data (UserID: %s)", userID)
	}

	// Update each exchange's configuration and track traders that need reload
	tradersToReload := make(map[string]bool)
	for exchangeID, exchangeData := range req.Exchanges {
		// Find traders using this exchange BEFORE updating
		traders, _ := s.store.Trader().ListByExchangeID(userID, exchangeID)
		for _, t := range traders {
			tradersToReload[t.ID] = true
		}

		exRow, exErr := s.store.Exchange().GetByID(userID, exchangeID)
		if exErr != nil {
			SafeInternalError(c, fmt.Sprintf("Get exchange %s", exchangeID), exErr)
			return
		}
		if exRow.ExchangeType == "hz" {
			exchangeData.APIURL, err = normalizeHZAPIURL(exchangeData.APIURL)
			if err != nil {
				SafeBadRequest(c, err.Error())
				return
			}
		}
		if exchangeTypeUsesOutboundProxy(exRow.ExchangeType) {
			manual := ""
			if exchangeData.OutboundProxyURL != nil {
				manual = strings.TrimSpace(*exchangeData.OutboundProxyURL)
			}
			if manual != "" {
				_ = s.store.ProxyPool().ReleaseByExchangeID(exchangeID)
			} else if exchangeData.OutboundProxyClear {
				_ = s.store.ProxyPool().ReleaseByExchangeID(exchangeID)
			}
			if exchangeData.OutboundProxyAutoAssign && manual == "" {
				_ = s.store.ProxyPool().ReleaseByExchangeID(exchangeID)
				terr := s.store.GormDB().Transaction(func(tx *gorm.DB) error {
					_, url, aerr := s.store.ProxyPool().ClaimProxyForExchange(tx, userID, exchangeID, exRow.ExchangeType)
					if aerr != nil {
						return aerr
					}
					u := url
					exchangeData.OutboundProxyURL = &u
					exchangeData.OutboundProxyClear = false
					return nil
				})
				if terr != nil {
					if errors.Is(terr, store.ErrProxyPoolExhausted) {
						c.JSON(http.StatusBadRequest, gin.H{"error": "暂无可用出口代理，请联系管理员补充代理池"})
						return
					}
					SafeInternalError(c, "Allocate proxy from pool", terr)
					return
				}
			}
		}

		err := s.store.Exchange().Update(userID, exchangeID, exchangeData.Enabled, exchangeData.APIKey, exchangeData.SecretKey, exchangeData.Passphrase, exchangeData.Testnet, exchangeData.APIURL, exchangeData.HyperliquidWalletAddr, exchangeData.HyperliquidUnifiedAcct, exchangeData.AsterUser, exchangeData.AsterSigner, exchangeData.AsterPrivateKey, exchangeData.LighterWalletAddr, exchangeData.LighterPrivateKey, exchangeData.LighterAPIKeyPrivateKey, exchangeData.LighterAPIKeyIndex, exchangeData.OutboundProxyURL, exchangeData.OutboundProxyClear)
		if err != nil {
			SafeInternalError(c, fmt.Sprintf("Update exchange %s", exchangeID), err)
			return
		}
	}

	s.exchangeAccountStateCache.Invalidate(userID)

	// Remove affected traders from memory BEFORE reloading to pick up new config
	for traderID := range tradersToReload {
		logger.Infof("🔄 Removing trader %s from memory to reload with new exchange config", traderID)
		s.traderManager.RemoveTrader(traderID)
	}

	// Reload all traders for this user to make new config take effect immediately
	err = s.traderManager.LoadUserTradersFromStore(s.store, userID)
	if err != nil {
		logger.Infof("⚠️ Failed to reload user traders into memory: %v", err)
		// Don't return error here since exchange config was successfully updated to database
	}

	logger.Infof("✓ Updated %d exchange configs for user %s", len(req.Exchanges), userID)
	c.JSON(http.StatusOK, gin.H{"message": "Exchange configuration updated"})
}

// handleCreateExchange Create a new exchange account
func (s *Server) handleCreateExchange(c *gin.Context) {
	userID := c.GetString("user_id")
	cfg := config.Get()

	// Read raw request body
	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	var req CreateExchangeRequest

	// Check if transport encryption is enabled
	if !cfg.TransportEncryption {
		// Transport encryption disabled, accept plain JSON
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			logger.Infof("❌ Failed to parse plain JSON request: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
			return
		}
	} else {
		// Transport encryption enabled, require encrypted payload
		var encryptedPayload crypto.EncryptedPayload
		if err := json.Unmarshal(bodyBytes, &encryptedPayload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format, encrypted transmission required"})
			return
		}

		if encryptedPayload.WrappedKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "This endpoint only supports encrypted transmission",
				"code":    "ENCRYPTION_REQUIRED",
				"message": "Encrypted transmission is required for security reasons",
			})
			return
		}

		decrypted, err := s.cryptoHandler.cryptoService.DecryptSensitiveData(&encryptedPayload)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decrypt data"})
			return
		}

		if err := json.Unmarshal([]byte(decrypted), &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse decrypted data"})
			return
		}
	}

	// Validate exchange type
	validTypes := map[string]bool{
		"binance": true, "bybit": true, "okx": true, "bitget": true,
		"hyperliquid": true, "aster": true, "lighter": true, "gate": true, "kucoin": true, "indodax": true,
		"hz": true,
	}
	if !validTypes[req.ExchangeType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid exchange type: %s", req.ExchangeType)})
		return
	}
	if req.ExchangeType == "hz" {
		if strings.TrimSpace(req.APIKey) == "" || strings.TrimSpace(req.SecretKey) == "" {
			SafeBadRequest(c, "HZ 交易账户需要 API Key 和 Secret Key")
			return
		}
		req.APIURL, err = normalizeHZAPIURL(req.APIURL)
		if err != nil {
			SafeBadRequest(c, err.Error())
			return
		}
	}

	outbound := strings.TrimSpace(req.OutboundProxyURL)

	if shouldAutoAssignCEXProxyCreate(&req) {
		exID := uuid.New().String()
		err := s.store.GormDB().Transaction(func(tx *gorm.DB) error {
			_, proxyPlain, err := s.store.ProxyPool().ClaimProxyForExchange(tx, userID, exID, req.ExchangeType)
			if err != nil {
				return err
			}
			_, err = s.store.Exchange().CreateInTx(tx, exID, userID, req.ExchangeType, req.AccountName, req.Enabled,
				req.APIKey, req.SecretKey, req.Passphrase, req.Testnet, req.APIURL,
				req.HyperliquidWalletAddr, req.HyperliquidUnifiedAcct,
				req.AsterUser, req.AsterSigner, req.AsterPrivateKey,
				req.LighterWalletAddr, req.LighterPrivateKey, req.LighterAPIKeyPrivateKey, req.LighterAPIKeyIndex,
				proxyPlain)
			return err
		})
		if err != nil {
			if errors.Is(err, store.ErrProxyPoolExhausted) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "暂无可用出口代理，请联系管理员在代理池中导入 SOCKS5"})
				return
			}
			logger.Infof("❌ Failed to create exchange with proxy pool: %v", err)
			SafeInternalError(c, "Failed to create exchange account", err)
			return
		}
		s.exchangeAccountStateCache.Invalidate(userID)
		logger.Infof("✓ Created exchange account (proxy pool): type=%s, name=%s, id=%s", req.ExchangeType, req.AccountName, exID)
		c.JSON(http.StatusOK, gin.H{"message": "Exchange account created", "id": exID})
		return
	}

	id, err := s.store.Exchange().Create(
		userID, req.ExchangeType, req.AccountName, req.Enabled,
		req.APIKey, req.SecretKey, req.Passphrase, req.Testnet, req.APIURL,
		req.HyperliquidWalletAddr, req.HyperliquidUnifiedAcct,
		req.AsterUser, req.AsterSigner, req.AsterPrivateKey,
		req.LighterWalletAddr, req.LighterPrivateKey, req.LighterAPIKeyPrivateKey, req.LighterAPIKeyIndex,
		outbound,
	)
	if err != nil {
		logger.Infof("❌ Failed to create exchange account: %v", err)
		SafeInternalError(c, "Failed to create exchange account", err)
		return
	}

	s.exchangeAccountStateCache.Invalidate(userID)

	logger.Infof("✓ Created exchange account: type=%s, name=%s, id=%s", req.ExchangeType, req.AccountName, id)
	c.JSON(http.StatusOK, gin.H{
		"message": "Exchange account created",
		"id":      id,
	})
}

// handleDeleteExchange Delete an exchange account
func (s *Server) handleDeleteExchange(c *gin.Context) {
	userID := c.GetString("user_id")
	exchangeID := c.Param("id")

	if exchangeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Exchange ID is required"})
		return
	}

	// Check if any traders are using this exchange
	traders, err := s.store.Trader().List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check traders"})
		return
	}

	for _, trader := range traders {
		if trader.ExchangeID == exchangeID {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"无法删除：仍有交易员「%s」正在使用该交易所账户，请先到「交易员」中删除或更换该交易员绑定的交易所后再删账户",
					trader.Name,
				),
				"trader_id":   trader.ID,
				"trader_name": trader.Name,
			})
			return
		}
	}

	// Delete exchange account
	err = s.store.Exchange().Delete(userID, exchangeID)
	if err != nil {
		logger.Infof("❌ Failed to delete exchange account: %v", err)
		SafeInternalError(c, "Failed to delete exchange account", err)
		return
	}
	_ = s.store.ProxyPool().ReleaseByExchangeID(exchangeID)

	s.exchangeAccountStateCache.Invalidate(userID)

	logger.Infof("✓ Deleted exchange account: id=%s", exchangeID)
	c.JSON(http.StatusOK, gin.H{"message": "Exchange account deleted"})
}

// handleGetSupportedExchanges Get list of exchanges supported by the system
func (s *Server) handleGetSupportedExchanges(c *gin.Context) {
	// Return static list of supported exchange types
	// Note: ID is empty for supported exchanges (they are templates, not actual accounts)
	supportedExchanges := []SafeExchangeConfig{
		{ExchangeType: "binance", Name: "Binance Futures", Type: "cex"},
		{ExchangeType: "bybit", Name: "Bybit Futures", Type: "cex"},
		{ExchangeType: "okx", Name: "OKX Futures", Type: "cex"},
		{ExchangeType: "gate", Name: "Gate.io Futures", Type: "cex"},
		{ExchangeType: "kucoin", Name: "KuCoin Futures", Type: "cex"},
		{ExchangeType: "hz", Name: "HZ 交易账户", Type: "cex"},
		{ExchangeType: "hyperliquid", Name: "Hyperliquid", Type: "dex"},
		{ExchangeType: "aster", Name: "Aster DEX", Type: "dex"},
		{ExchangeType: "lighter", Name: "LIGHTER DEX", Type: "dex"},
		{ExchangeType: "alpaca", Name: "Alpaca (US Stocks)", Type: "stock"},
		{ExchangeType: "forex", Name: "Forex (TwelveData)", Type: "forex"},
		{ExchangeType: "metals", Name: "Metals (TwelveData)", Type: "metals"},
	}

	c.JSON(http.StatusOK, supportedExchanges)
}
