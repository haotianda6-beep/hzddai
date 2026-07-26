package trader

import "testing"

func TestMirrorPositionKeyNormalizesExchangeSymbols(t *testing.T) {
	if posKey("BTC_USDT", "short") != posKey("BTCUSDT", "short") {
		t.Fatalf("Gate symbol key must match normalized master key")
	}
	if posKey("BTC-USDT-SWAP", "long") != posKey("BTCUSDT", "long") {
		t.Fatalf("OKX swap symbol key must match normalized master key")
	}

	got := mapFollowerPositionQuantities([]map[string]interface{}{
		{"symbol": "BTC_USDT", "side": "short", "positionAmt": 0.0047},
	})
	if got[posKey("BTCUSDT", "short")] != 0.0047 {
		t.Fatalf("normalized follower position not found: %#v", got)
	}
}
