package gate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gateio/gateapi-go/v6"
	"github.com/stretchr/testify/assert"
	"nofx/trader/testutil"
	"nofx/trader/types"
)

// ============================================================
// Part 1: GateTraderTestSuite - Inherits base test suite
// ============================================================

// GateTraderTestSuite Gate trader test suite
// Inherits TraderTestSuite and adds Gate-specific mock logic
type GateTraderTestSuite struct {
	*testutil.TraderTestSuite
	mockServer *httptest.Server
}

// NewGateTraderTestSuite creates Gate test suite with mock server
func NewGateTraderTestSuite(t *testing.T) *GateTraderTestSuite {
	// Create mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		var respBody interface{}

		switch {
		// Mock GetBalance - /api/v4/futures/usdt/accounts
		case strings.Contains(path, "/futures/usdt/accounts"):
			respBody = map[string]interface{}{
				"total":          "10000.00",
				"unrealised_pnl": "100.50",
				"available":      "8000.00",
				"currency":       "USDT",
			}

		// Mock GetPositions - /api/v4/futures/usdt/positions
		case strings.Contains(path, "/futures/usdt/positions"):
			respBody = []map[string]interface{}{
				{
					"contract":       "BTC_USDT",
					"size":           500,
					"entry_price":    "50000.00",
					"mark_price":     "50500.00",
					"unrealised_pnl": "250.00",
					"liq_price":      "45000.00",
					"leverage":       "10",
				},
			}

		// Mock GetContract - /api/v4/futures/usdt/contracts/{contract}
		case strings.Contains(path, "/futures/usdt/contracts/"):
			respBody = map[string]interface{}{
				"name":              "BTC_USDT",
				"quanto_multiplier": "0.001",
				"order_price_round": "0.1",
			}

		// Mock ListFuturesContracts - /api/v4/futures/usdt/contracts
		case strings.Contains(path, "/futures/usdt/contracts"):
			respBody = []map[string]interface{}{
				{
					"name":              "BTC_USDT",
					"quanto_multiplier": "0.001",
					"order_price_round": "0.1",
				},
				{
					"name":              "ETH_USDT",
					"quanto_multiplier": "0.01",
					"order_price_round": "0.01",
				},
			}

		// Mock ListFuturesTickers - /api/v4/futures/usdt/tickers
		case strings.Contains(path, "/futures/usdt/tickers"):
			contract := r.URL.Query().Get("contract")
			if contract == "" {
				contract = "BTC_USDT"
			}
			price := "50000.00"
			if contract == "ETH_USDT" {
				price = "3000.00"
			}
			respBody = []map[string]interface{}{
				{
					"contract": contract,
					"last":     price,
				},
			}

		// Mock CreateFuturesOrder - /api/v4/futures/usdt/orders (POST)
		case strings.Contains(path, "/futures/usdt/orders") && r.Method == "POST":
			respBody = map[string]interface{}{
				"id":         123456,
				"contract":   "BTC_USDT",
				"size":       100,
				"status":     "finished",
				"finish_as":  "filled",
				"fill_price": "50000.00",
			}

		// Mock ListFuturesOrders - /api/v4/futures/usdt/orders
		case strings.Contains(path, "/futures/usdt/orders"):
			respBody = []map[string]interface{}{}

		// Mock GetFuturesOrder - /api/v4/futures/usdt/orders/{order_id}
		case strings.Contains(path, "/futures/usdt/orders/"):
			respBody = map[string]interface{}{
				"id":          123456,
				"contract":    "BTC_USDT",
				"size":        100,
				"status":      "finished",
				"finish_as":   "filled",
				"fill_price":  "50000.00",
				"create_time": 1234567890.0,
				"update_time": 1234567890.0,
				"tkfr":        "0.0005",
				"mkfr":        "0.0002",
			}

		// Mock UpdatePositionLeverage
		case strings.Contains(path, "/futures/usdt/positions/") && strings.Contains(path, "/leverage"):
			respBody = map[string]interface{}{
				"leverage": 10,
			}

		// Mock ListPriceTriggeredOrders
		case strings.Contains(path, "/futures/usdt/price_orders"):
			respBody = []map[string]interface{}{}

		// Mock ListPositionClose
		case strings.Contains(path, "/futures/usdt/position_close"):
			respBody = []map[string]interface{}{}

		// Default: empty response
		default:
			respBody = map[string]interface{}{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody)
	}))

	// Create trader instance (will need to override URL in actual usage)
	traderInstance := NewGateTrader("test_api_key", "test_secret_key")

	// Create base suite
	baseSuite := testutil.NewTraderTestSuite(t, traderInstance)

	return &GateTraderTestSuite{
		TraderTestSuite: baseSuite,
		mockServer:      mockServer,
	}
}

// Cleanup cleans up resources
func (s *GateTraderTestSuite) Cleanup() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
	s.TraderTestSuite.Cleanup()
}

// ============================================================
// Part 2: Interface compliance tests
// ============================================================

// TestGateTrader_InterfaceCompliance tests interface compliance
func TestGateTrader_InterfaceCompliance(t *testing.T) {
	var _ types.Trader = (*GateTrader)(nil)
}

// ============================================================
// Part 3: Gate-specific feature unit tests
// ============================================================

// TestNewGateTrader tests creating Gate trader
func TestNewGateTrader(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		secretKey string
		wantNil   bool
	}{
		{
			name:      "Successfully create",
			apiKey:    "test_api_key",
			secretKey: "test_secret_key",
			wantNil:   false,
		},
		{
			name:      "Empty API Key can still create",
			apiKey:    "",
			secretKey: "test_secret_key",
			wantNil:   false,
		},
		{
			name:      "Empty Secret Key can still create",
			apiKey:    "test_api_key",
			secretKey: "",
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt := NewGateTrader(tt.apiKey, tt.secretKey)

			if tt.wantNil {
				assert.Nil(t, gt)
			} else {
				assert.NotNil(t, gt)
				assert.NotNil(t, gt.client)
				assert.Equal(t, tt.apiKey, gt.apiKey)
				assert.Equal(t, tt.secretKey, gt.secretKey)
			}
		})
	}
}

func TestNewGateTraderWithTestnetUsesTestnetEndpoint(t *testing.T) {
	testnetTrader := NewGateTraderWithTestnet("test_api_key", "test_secret_key", true)
	assert.Equal(t, gateFuturesTestnetBasePath, testnetTrader.basePath)

	mainnetTrader := NewGateTraderWithTestnet("test_api_key", "test_secret_key", false)
	assert.Equal(t, "https://api.gateio.ws/api/v4", mainnetTrader.basePath)
}

// TestGateTrader_SymbolConversion tests symbol format conversion
func TestGateTrader_SymbolConversion(t *testing.T) {
	gt := NewGateTrader("test", "test")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "BTCUSDT to BTC_USDT",
			input:    "BTCUSDT",
			expected: "BTC_USDT",
		},
		{
			name:     "ETHUSDT to ETH_USDT",
			input:    "ETHUSDT",
			expected: "ETH_USDT",
		},
		{
			name:     "Already converted format",
			input:    "BTC_USDT",
			expected: "BTC_USDT",
		},
		{
			name:     "SOL symbol",
			input:    "SOLUSDT",
			expected: "SOL_USDT",
		},
		{
			name:     "XAUUSDT gold contract",
			input:    "XAUUSDT",
			expected: "XAU_USDT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gt.convertSymbol(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGateXAUContractQuantityConversion(t *testing.T) {
	contract := &gateapi.Contract{
		Name:             "XAU_USDT",
		QuantoMultiplier: "0.0001",
		OrderSizeMin:     1,
		OrderSizeMax:     30_000_000,
	}

	tests := []struct {
		name     string
		quantity float64
		want     int64
	}{
		{name: "MT4 0.01 lot on 10000U", quantity: 1, want: 10_000},
		{name: "1000U follower at ten percent", quantity: 0.1, want: 1_000},
		{name: "floating point close recovers exact contracts", quantity: 0.487999999999, want: 4_880},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gateContractSize(tt.quantity, contract)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGateXAUOpenLongUsesCrossMarginAndContractUnits(t *testing.T) {
	var placed gateapi.FuturesOrder
	var leverage, crossLimit string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/contracts/XAU_USDT"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"name": "XAU_USDT", "quanto_multiplier": "0.0001",
				"order_size_min": 1, "order_size_max": 30_000_000,
			})
		case strings.HasSuffix(r.URL.Path, "/positions/XAU_USDT/leverage"):
			leverage = r.URL.Query().Get("leverage")
			crossLimit = r.URL.Query().Get("cross_leverage_limit")
			json.NewEncoder(w).Encode(map[string]interface{}{"contract": "XAU_USDT", "leverage": "0"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/orders"):
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&placed))
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 901, "contract": placed.Contract, "size": placed.Size,
				"status": "finished", "finish_as": "filled", "fill_price": "4060.60",
			})
		case strings.Contains(r.URL.Path, "/price_orders"):
			json.NewEncoder(w).Encode([]interface{}{})
		default:
			json.NewEncoder(w).Encode([]interface{}{})
		}
	}))
	defer mockServer.Close()

	gt := NewGateTrader("test", "test")
	gt.client.ChangeBasePath(mockServer.URL)
	assert.NoError(t, gt.SetMarginMode("XAUUSDT", true))
	_, err := gt.OpenLong("XAUUSDT", 0.1, 20)
	assert.NoError(t, err)
	assert.Equal(t, "XAU_USDT", placed.Contract)
	assert.Equal(t, int64(1_000), placed.Size)
	assert.False(t, placed.ReduceOnly)
	assert.Equal(t, "0", leverage)
	assert.Equal(t, "20", crossLimit)

	_, err = gt.CloseLong("XAUUSDT", 0.487999999999)
	assert.NoError(t, err)
	assert.Equal(t, "XAU_USDT", placed.Contract)
	assert.Equal(t, int64(-4_880), placed.Size)
	assert.True(t, placed.ReduceOnly)
}

// TestGateTrader_RevertSymbol tests symbol reversion
func TestGateTrader_RevertSymbol(t *testing.T) {
	gt := NewGateTrader("test", "test")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "BTC_USDT to BTCUSDT",
			input:    "BTC_USDT",
			expected: "BTCUSDT",
		},
		{
			name:     "ETH_USDT to ETHUSDT",
			input:    "ETH_USDT",
			expected: "ETHUSDT",
		},
		{
			name:     "Already standard format",
			input:    "BTCUSDT",
			expected: "BTCUSDT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gt.revertSymbol(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGateClosedPnLRecordParsesShortPositionClose(t *testing.T) {
	record, ok := gateClosedPnLRecord(gateapi.PositionClose{
		Time:          1783518600,
		Contract:      "BTC_USDT",
		Side:          "short",
		Pnl:           "4.12",
		PnlFee:        "-0.05",
		AccumSize:     "15.8",
		FirstOpenTime: 1783517822,
		LongPrice:     "62483",
		ShortPrice:    "62784.6",
	}, 0.001)

	assert.True(t, ok)
	assert.Equal(t, "BTCUSDT", record.Symbol)
	assert.Equal(t, "SHORT", record.Side)
	assert.Equal(t, 62784.6, record.EntryPrice)
	assert.Equal(t, 62483.0, record.ExitPrice)
	assert.Equal(t, 0.0158, record.Quantity)
	assert.Equal(t, 4.12, record.RealizedPnL)
	assert.Equal(t, 0.05, record.Fee)
	assert.Equal(t, int64(1783517822), record.EntryTime.Unix())
	assert.Equal(t, int64(1783518600), record.ExitTime.Unix())
}

// TestGateTrader_CacheDuration tests cache duration
func TestGateTrader_CacheDuration(t *testing.T) {
	gt := NewGateTrader("test", "test")

	// Verify default cache time is 15 seconds
	assert.Equal(t, 15*time.Second, gt.cacheDuration)
}

// TestGateTrader_ClearCache tests cache clearing
func TestGateTrader_ClearCache(t *testing.T) {
	gt := NewGateTrader("test", "test")

	// Set some cached data
	gt.cachedBalance = map[string]interface{}{"test": "data"}
	gt.cachedPositions = []map[string]interface{}{{"test": "data"}}

	// Clear cache
	gt.clearCache()

	// Verify cache is cleared
	assert.Nil(t, gt.cachedBalance)
	assert.Nil(t, gt.cachedPositions)
}

func TestGateTrader_CrossMarginLeverageParameters(t *testing.T) {
	var gotLeverage, gotCrossLimit string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/futures/usdt/positions/BTC_USDT/leverage") {
			gotLeverage = r.URL.Query().Get("leverage")
			gotCrossLimit = r.URL.Query().Get("cross_leverage_limit")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"contract": "BTC_USDT",
				"leverage": "0",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	gt := NewGateTrader("test", "test")
	gt.client.ChangeBasePath(mockServer.URL)

	assert.NoError(t, gt.SetMarginMode("BTCUSDT", true))
	assert.NoError(t, gt.SetLeverage("BTCUSDT", 9))
	assert.Equal(t, "0", gotLeverage)
	assert.Equal(t, "9", gotCrossLimit)
}

func TestGateTrader_DefaultLeverageUsesCrossMargin(t *testing.T) {
	var gotLeverage, gotCrossLimit string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLeverage = r.URL.Query().Get("leverage")
		gotCrossLimit = r.URL.Query().Get("cross_leverage_limit")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"contract": "BTC_USDT",
			"leverage": "0",
		})
	}))
	defer mockServer.Close()

	gt := NewGateTrader("test", "test")
	gt.client.ChangeBasePath(mockServer.URL)

	assert.NoError(t, gt.SetLeverage("BTCUSDT", 9))
	assert.Equal(t, "0", gotLeverage)
	assert.Equal(t, "9", gotCrossLimit)
}

// ============================================================
// Part 4: Mock server integration tests
// ============================================================

// TestGateTrader_MockServerResponseFormat tests mock server response format
func TestGateTrader_MockServerResponseFormat(t *testing.T) {
	suite := NewGateTraderTestSuite(t)
	defer suite.Cleanup()

	// Verify mock server is running
	assert.NotNil(t, suite.mockServer)
	assert.NotEmpty(t, suite.mockServer.URL)
}
