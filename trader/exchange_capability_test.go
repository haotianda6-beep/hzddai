package trader

import (
	"testing"

	"nofx/trader/binance"
	"nofx/trader/bitget"
	"nofx/trader/bybit"
	"nofx/trader/gate"
	"nofx/trader/okx"
)

func TestCEXFollowCapabilitiesCompile(t *testing.T) {
	var _ Trader = (*binance.FuturesTrader)(nil)
	var _ Trader = (*bybit.BybitTrader)(nil)
	var _ Trader = (*okx.OKXTrader)(nil)
	var _ Trader = (*bitget.BitgetTrader)(nil)
	var _ Trader = (*gate.GateTrader)(nil)

	var _ GridTrader = (*binance.FuturesTrader)(nil)
	var _ GridTrader = (*bybit.BybitTrader)(nil)
	var _ GridTrader = (*okx.OKXTrader)(nil)
	var _ GridTrader = (*bitget.BitgetTrader)(nil)
	var _ GridTrader = (*gate.GateTrader)(nil)
}
