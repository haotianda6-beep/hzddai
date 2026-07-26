// 压测入口占位：真实压测见 trader/binance/futures_ws_api_bench_test.go（go test）。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("币安 U 本位 WebSocket API 压测请运行：")
	fmt.Println("  go test ./trader/binance -run TestBinanceWSAPIBench -count=1 -v")
	fmt.Println("需 BINANCE_API_KEY / BINANCE_SECRET_KEY；可选 BINANCE_FUTURES_WS_TESTNET=1")
	if os.Getenv("BINANCE_API_KEY") == "" {
		os.Exit(0)
	}
}
