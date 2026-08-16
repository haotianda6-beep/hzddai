package payment

import (
	"crypto/ecdsa"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/crypto"

	"nofx/mcp"
	"nofx/mcp/provider"
	"nofx/store"

	"gorm.io/gorm"
)

const (
	DefaultClaw402URL   = "https://claw402.ai"
	DefaultClaw402Model = "glm-5"
)

// claw402ModelEndpoints maps user-friendly model names to claw402 API paths.
var claw402ModelEndpoints = map[string]string{
	// OpenAI
	"gpt-5.4":     "/api/v1/ai/openai/chat/5.4",
	"gpt-5.4-pro": "/api/v1/ai/openai/chat/5.4-pro",
	"gpt-5.3":     "/api/v1/ai/openai/chat/5.3",
	"gpt-5-mini":  "/api/v1/ai/openai/chat/5-mini",
	// Anthropic
	"claude-opus":   "/api/v1/ai/anthropic/messages/opus",
	"claude-sonnet": "/api/v1/ai/anthropic/messages/sonnet",
	"claude-haiku":  "/api/v1/ai/anthropic/messages/haiku",
	// DeepSeek
	"deepseek":          "/api/v1/ai/deepseek/chat",
	"deepseek-v4-pro":   "/api/v1/ai/deepseek/v4-pro",
	"deepseek-v4-flash": "/api/v1/ai/deepseek/v4-flash",
	"deepseek-reasoner": "/api/v1/ai/deepseek/chat/reasoner",
	// Qwen
	"qwen-max":   "/api/v1/ai/qwen/chat/max",
	"qwen-plus":  "/api/v1/ai/qwen/chat/plus",
	"qwen-turbo": "/api/v1/ai/qwen/chat/turbo",
	"qwen-flash": "/api/v1/ai/qwen/chat/flash",
	"qwen-coder": "/api/v1/ai/qwen/chat/coder",
	// Grok
	"grok-4.1":      "/api/v1/ai/grok/chat/4.1",
	"grok-4.1-fast": "/api/v1/ai/grok/chat/4.1-fast",
	"grok-4":        "/api/v1/ai/grok/chat/4",
	"grok-3-mini":   "/api/v1/ai/grok/chat/3-mini",
	// Gemini
	"gemini-3.1-pro":        "/api/v1/ai/gemini/chat/3.1-pro",
	"gemini-3-flash":        "/api/v1/ai/gemini/chat/3-flash",
	"gemini-3.1-flash-lite": "/api/v1/ai/gemini/chat/3.1-flash-lite",
	"gemini-2.5-pro":        "/api/v1/ai/gemini/chat/2.5-pro",
	"gemini-2.5-flash":      "/api/v1/ai/gemini/chat/2.5-flash",
	"gemini-2.5-flash-lite": "/api/v1/ai/gemini/chat/2.5-flash-lite",
	// Kimi
	"kimi-k2.5": "/api/v1/ai/kimi/chat/k2.5",
	"kimi-k2":   "/api/v1/ai/kimi/chat/k2",
	// Z.AI (智谱)
	"glm-5":       "/api/v1/ai/zhipu/chat",
	"glm-5-turbo": "/api/v1/ai/zhipu/chat/turbo",
}

func init() {
	mcp.RegisterProvider(mcp.ProviderClaw402, func(opts ...mcp.ClientOption) mcp.AIClient {
		return NewClaw402ClientWithOptions(opts...)
	})
	mcp.RegisterProvider(mcp.ProviderComkunProxy, func(opts ...mcp.ClientOption) mcp.AIClient {
		return NewClaw402ClientWithOptions(opts...)
	})
}

// Claw402Client implements AIClient using claw402.ai's x402 v2 USDC payment gateway.
// When the selected model routes to an Anthropic endpoint, it automatically uses
// the Anthropic wire format for requests and responses (via an internal ClaudeClient).
type Claw402Client struct {
	*mcp.Client
	privateKey  *ecdsa.PrivateKey
	claudeProxy *provider.ClaudeClient // non-nil when endpoint is /anthropic/
	billing     *PlatformBillingConfig
}

type PlatformBillingConfig struct {
	Store            *store.Store
	UserID           string
	TraderID         string
	Provider         string
	Model            string
	MarkupMultiplier float64
	MinChargeUSDT    float64
}

type platformBillingSession struct {
	cfg            *PlatformBillingConfig
	mu             sync.Mutex
	usageID        string
	charged        bool
	refunded       bool
	chargedUSDT    float64
	actualCostUSDC float64
}

func (c *Claw402Client) BaseClient() *mcp.Client { return c.Client }

func (c *Claw402Client) ConfigurePlatformBilling(cfg PlatformBillingConfig) {
	if cfg.MarkupMultiplier <= 0 {
		cfg.MarkupMultiplier = platformAIMarkupMultiplier()
	}
	if cfg.MinChargeUSDT <= 0 {
		cfg.MinChargeUSDT = platformAIMinChargeUSDT()
	}
	c.billing = &cfg
}

// NewClaw402Client creates a claw402 client (backward compatible).
func NewClaw402Client() mcp.AIClient {
	return NewClaw402ClientWithOptions()
}

// NewClaw402ClientWithOptions creates a claw402 client with options.
func NewClaw402ClientWithOptions(opts ...mcp.ClientOption) mcp.AIClient {
	baseOpts := []mcp.ClientOption{
		mcp.WithProvider(mcp.ProviderClaw402),
		mcp.WithModel(DefaultClaw402Model),
		mcp.WithBaseURL(DefaultClaw402URL),
		mcp.WithTimeout(X402Timeout),
		mcp.WithMaxRetries(1), // disable outer retry — inner x402 loop handles retries; outer retry causes duplicate payments
	}
	allOpts := append(baseOpts, opts...)
	baseClient := mcp.NewClient(allOpts...).(*mcp.Client)
	baseClient.UseFullURL = true
	baseClient.BaseURL = DefaultClaw402URL + claw402ModelEndpoints[DefaultClaw402Model]

	c := &Claw402Client{Client: baseClient}
	baseClient.Hooks = c
	return c
}

// SetAPIKey stores the EVM private key and selects the model endpoint.
func (c *Claw402Client) SetAPIKey(apiKey string, _ string, customModel string) {
	if strings.TrimSpace(apiKey) == "" {
		apiKey = os.Getenv("PLATFORM_CLAW402_WALLET_KEY")
	}
	hexKey := strings.TrimPrefix(apiKey, "0x")
	privKey, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		c.Log.Warnf("⚠️  [MCP] Claw402: invalid private key: %v", err)
	} else {
		c.privateKey = privKey
		c.APIKey = apiKey
		addr := crypto.PubkeyToAddress(privKey.PublicKey).Hex()
		c.Log.Infof("🔧 [MCP] Claw402 wallet: %s", addr)
	}
	if customModel != "" {
		c.Model = customModel
	}
	endpoint := c.resolveEndpoint()
	c.BaseURL = DefaultClaw402URL + endpoint

	// Anthropic endpoints need different wire format (Messages API)
	if strings.Contains(endpoint, "/anthropic/") {
		c.claudeProxy = &provider.ClaudeClient{Client: c.Client}
		c.Log.Infof("🔧 [MCP] Claw402 model: %s → %s (Anthropic format)", c.Model, endpoint)
	} else {
		c.claudeProxy = nil
		c.Log.Infof("🔧 [MCP] Claw402 model: %s → %s", c.Model, endpoint)
	}
}

// resolveEndpoint returns the API path for the configured model.
func (c *Claw402Client) resolveEndpoint() string {
	if ep, ok := claw402ModelEndpoints[c.Model]; ok {
		return ep
	}
	// Allow raw path override (e.g. "/api/v1/ai/openai/chat/5.4")
	if strings.HasPrefix(c.Model, "/api/") {
		return c.Model
	}
	return claw402ModelEndpoints[DefaultClaw402Model]
}

func (c *Claw402Client) SetAuthHeader(h http.Header) { X402SetAuthHeader(h) }

func (c *Claw402Client) Call(systemPrompt, userPrompt string) (string, error) {
	session := c.newPlatformBillingSession()
	out, err := X402CallStream(c.Client, session.signPayment(c.signPayment), "Claw402", systemPrompt, userPrompt, nil)
	if err != nil {
		session.refund(err)
		return "", err
	}
	session.success("")
	return out, nil
}

func (c *Claw402Client) CallWithRequestFull(req *mcp.Request) (*mcp.LLMResponse, error) {
	session := c.newPlatformBillingSession()
	out, err := X402CallFull(c.Client, session.signPayment(c.signPayment), "Claw402", req)
	if err != nil {
		session.refund(err)
		return nil, err
	}
	session.success("")
	return out, nil
}

// signPayment signs x402 v2 EIP-712 payment on Base chain + USDC.
func (c *Claw402Client) signPayment(paymentHeaderB64 string) (string, error) {
	return SignBasePaymentHeader(c.privateKey, paymentHeaderB64, "Claw402")
}

func (c *Claw402Client) newPlatformBillingSession() *platformBillingSession {
	if c.billing == nil || c.billing.Store == nil || strings.TrimSpace(c.billing.UserID) == "" {
		return &platformBillingSession{}
	}
	cfg := *c.billing
	if cfg.Model == "" {
		cfg.Model = c.Model
	}
	return &platformBillingSession{cfg: &cfg}
}

func (s *platformBillingSession) signPayment(next X402SignFunc) X402SignFunc {
	return func(paymentHeaderB64 string) (string, error) {
		if err := s.chargeOnce(paymentHeaderB64); err != nil {
			return "", err
		}
		return next(paymentHeaderB64)
	}
}

func (s *platformBillingSession) chargeOnce(paymentHeaderB64 string) error {
	if s == nil || s.cfg == nil || s.cfg.Store == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.charged {
		return nil
	}
	actual, err := X402CostUSDCFromHeader(paymentHeaderB64)
	if err != nil {
		return err
	}
	charged := math.Max(actual*s.cfg.MarkupMultiplier, s.cfg.MinChargeUSDT)
	charged = math.Ceil(charged*1_000_000) / 1_000_000
	var before, after float64
	var usageID string
	var walletLedgerID uint64
	err = s.cfg.Store.Transaction(func(tx *gorm.DB) error {
		u, err := s.cfg.Store.User().GetByID(s.cfg.UserID)
		if err != nil {
			return err
		}
		before = u.BalanceUSDT
		bal, ok, err := s.cfg.Store.User().AddBalanceDelta(tx, s.cfg.UserID, -charged)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("平台余额不足，请先充值")
		}
		after = bal
		walletLedgerID, err = s.cfg.Store.Billing().AppendLedger(tx, s.cfg.UserID, -charged, after, "ai_platform_call", s.cfg.TraderID)
		if err != nil {
			return err
		}
		id, err := s.cfg.Store.AIPlatformUsage().CreatePendingTx(
			tx,
			s.cfg.UserID,
			s.cfg.TraderID,
			s.cfg.Provider,
			s.cfg.Model,
			actual,
			charged,
			s.cfg.MarkupMultiplier,
			s.cfg.MinChargeUSDT,
			before,
			after,
		)
		if err != nil {
			return err
		}
		usageID = id
		return nil
	})
	if err != nil {
		return err
	}
	store.DispatchAgentRebateSpendIfEligible(s.cfg.UserID, charged, walletLedgerID, "ai_platform_call")
	s.usageID = usageID
	s.actualCostUSDC = actual
	s.chargedUSDT = charged
	s.charged = true
	return nil
}

func (s *platformBillingSession) success(txHash string) {
	if s == nil || s.cfg == nil || s.cfg.Store == nil || !s.charged || s.usageID == "" {
		return
	}
	_ = s.cfg.Store.AIPlatformUsage().MarkSuccess(s.usageID, txHash)
}

func (s *platformBillingSession) refund(callErr error) {
	if s == nil || s.cfg == nil || s.cfg.Store == nil || !s.charged || s.refunded {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refunded {
		return
	}
	errMsg := ""
	if callErr != nil {
		errMsg = callErr.Error()
	}
	var after float64
	err := s.cfg.Store.Transaction(func(tx *gorm.DB) error {
		bal, _, err := s.cfg.Store.User().AddBalanceDelta(tx, s.cfg.UserID, s.chargedUSDT)
		if err != nil {
			return err
		}
		after = bal
		if _, err2 := s.cfg.Store.Billing().AppendLedger(tx, s.cfg.UserID, s.chargedUSDT, after, "ai_platform_refund", s.cfg.TraderID); err2 != nil {
			return err2
		}
		return s.cfg.Store.AIPlatformUsage().MarkRefundedTx(tx, s.usageID, errMsg, after)
	})
	if err == nil {
		s.refunded = true
	}
}

func platformAIMarkupMultiplier() float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("PLATFORM_AI_MARKUP_MULTIPLIER")), 64); err == nil && v > 0 {
		return v
	}
	return 3.0
}

func platformAIMinChargeUSDT() float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("PLATFORM_AI_MIN_CHARGE_USDT")), 64); err == nil && v > 0 {
		return v
	}
	return 0.01
}

// ── Format overrides for Anthropic endpoints ─────────────────────────────────

func (c *Claw402Client) BuildMCPRequestBody(systemPrompt, userPrompt string) map[string]any {
	if c.claudeProxy != nil {
		return c.claudeProxy.BuildMCPRequestBody(systemPrompt, userPrompt)
	}
	return c.Client.BuildMCPRequestBody(systemPrompt, userPrompt)
}

func (c *Claw402Client) BuildRequestBodyFromRequest(req *mcp.Request) map[string]any {
	if c.claudeProxy != nil {
		return c.claudeProxy.BuildRequestBodyFromRequest(req)
	}
	return c.Client.BuildRequestBodyFromRequest(req)
}

func (c *Claw402Client) ParseMCPResponse(body []byte) (string, error) {
	if c.claudeProxy != nil {
		return c.claudeProxy.ParseMCPResponse(body)
	}
	return c.Client.ParseMCPResponse(body)
}

func (c *Claw402Client) ParseMCPResponseFull(body []byte) (*mcp.LLMResponse, error) {
	if c.claudeProxy != nil {
		return c.claudeProxy.ParseMCPResponseFull(body)
	}
	return c.Client.ParseMCPResponseFull(body)
}

// BuildUrl returns the full claw402 endpoint URL.
func (c *Claw402Client) BuildUrl() string {
	return c.BaseURL
}

func (c *Claw402Client) BuildRequest(url string, jsonData []byte) (*http.Request, error) {
	return X402BuildRequest(url, jsonData)
}
