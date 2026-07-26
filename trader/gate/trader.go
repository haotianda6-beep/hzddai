package gate

import (
	"context"
	"fmt"
	"nofx/logger"
	"nofx/trader/proxyhttp"
	"nofx/trader/types"
	"strings"
	"sync"
	"time"

	"github.com/gateio/gateapi-go/v6"
)

// GateTrader implements types.Trader interface for Gate.io Futures
type GateTrader struct {
	apiKey    string
	secretKey string
	basePath  string
	client    *gateapi.APIClient
	ctx       context.Context

	// Cache fields
	cachedBalance       map[string]interface{}
	balanceCacheTime    time.Time
	balanceCacheMutex   sync.RWMutex
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex
	contractsCache      map[string]*gateapi.Contract
	contractsCacheMutex sync.RWMutex
	marginModeBySymbol  map[string]bool
	marginModeMutex     sync.RWMutex
	cacheDuration       time.Duration
}

const gateFuturesTestnetBasePath = "https://api-testnet.gateapi.io/api/v4"

// NewGateTrader creates a new Gate trader instance
func NewGateTrader(apiKey, secretKey string, outboundProxyURL ...string) *GateTrader {
	return newGateTrader(apiKey, secretKey, false, outboundProxyURL...)
}

// NewGateTraderWithTestnet creates a Gate futures trader against the selected environment.
func NewGateTraderWithTestnet(apiKey, secretKey string, testnet bool, outboundProxyURL ...string) *GateTrader {
	return newGateTrader(apiKey, secretKey, testnet, outboundProxyURL...)
}

func newGateTrader(apiKey, secretKey string, testnet bool, outboundProxyURL ...string) *GateTrader {
	config := gateapi.NewConfiguration()
	if testnet {
		config.BasePath = gateFuturesTestnetBasePath
	}
	basePath := config.BasePath
	proxyURL := ""
	if len(outboundProxyURL) > 0 {
		proxyURL = strings.TrimSpace(outboundProxyURL[0])
	}
	if httpClient, err := proxyhttp.Client(proxyURL, 30*time.Second); err == nil {
		config.HTTPClient = httpClient
	} else {
		logger.Warnf("Gate 出口代理 URL 无效，忽略: %v", err)
	}
	config.AddDefaultHeader("X-Gate-Channel-Id", "nofx")
	client := gateapi.NewAPIClient(config)

	ctx := context.WithValue(context.Background(),
		gateapi.ContextGateAPIV4,
		gateapi.GateAPIV4{
			Key:    apiKey,
			Secret: secretKey,
		},
	)

	return &GateTrader{
		apiKey:             apiKey,
		secretKey:          secretKey,
		basePath:           basePath,
		client:             client,
		ctx:                ctx,
		contractsCache:     make(map[string]*gateapi.Contract),
		marginModeBySymbol: make(map[string]bool),
		cacheDuration:      15 * time.Second,
	}
}

// convertSymbol converts symbol format (e.g., BTCUSDT -> BTC_USDT)
func (t *GateTrader) convertSymbol(symbol string) string {
	// If already in correct format
	if strings.Contains(symbol, "_") {
		return symbol
	}
	// Convert BTCUSDT to BTC_USDT
	if strings.HasSuffix(symbol, "USDT") {
		base := strings.TrimSuffix(symbol, "USDT")
		return base + "_USDT"
	}
	return symbol
}

// revertSymbol converts symbol back to standard format (e.g., BTC_USDT -> BTCUSDT)
func (t *GateTrader) revertSymbol(symbol string) string {
	return strings.ReplaceAll(symbol, "_", "")
}

// getContract fetches contract info with caching
func (t *GateTrader) getContract(symbol string) (*gateapi.Contract, error) {
	symbol = t.convertSymbol(symbol)

	// Check cache
	t.contractsCacheMutex.RLock()
	if contract, ok := t.contractsCache[symbol]; ok {
		t.contractsCacheMutex.RUnlock()
		return contract, nil
	}
	t.contractsCacheMutex.RUnlock()

	// Fetch from API
	contract, _, err := t.client.FuturesApi.GetFuturesContract(t.ctx, "usdt", symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get contract info: %w", err)
	}

	// Update cache
	t.contractsCacheMutex.Lock()
	t.contractsCache[symbol] = &contract
	t.contractsCacheMutex.Unlock()

	return &contract, nil
}

// clearCache clears all caches
func (t *GateTrader) clearCache() {
	t.balanceCacheMutex.Lock()
	t.cachedBalance = nil
	t.balanceCacheMutex.Unlock()

	t.positionsCacheMutex.Lock()
	t.cachedPositions = nil
	t.positionsCacheMutex.Unlock()
}

// Ensure GateTrader implements Trader interface
var _ types.Trader = (*GateTrader)(nil)
