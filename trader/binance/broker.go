package binance

import (
	"os"
	"strings"
)

// 币安 API 经纪商返佣：合约下单通过 newClientOrderId / clientAlgoId 前缀 x-{8位经纪商标识}… 归因。
// 文档见币安 Broker / API 经纪商计划说明。

const (
	defaultFuturesBrokerOrderTag = "6UajqqpR"

	// DefaultSpotBrokerOrderTag 当前后端仅实现币安 U 本位合约；若日后增加现货下单，
	// 下单 newClientOrderId 应使用同格式前缀，默认现货侧标识为：
	DefaultSpotBrokerOrderTag = "EJ2MQNCB"
)

// futuresBrokerOrderTag 返回合约订单 ID 中「x-」后的 8 位经纪商标识。
// 环境变量 BINANCE_FUTURES_BROKER_ORDER_TAG 可覆盖（须恰好 8 字符）；否则用 defaultFuturesBrokerOrderTag。
func futuresBrokerOrderTag() string {
	v := strings.TrimSpace(os.Getenv("BINANCE_FUTURES_BROKER_ORDER_TAG"))
	if len(v) == 8 {
		return v
	}
	return defaultFuturesBrokerOrderTag
}
