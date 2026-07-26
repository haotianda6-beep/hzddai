package binance

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// TestBinanceWSAPIBenchLatency 压测 WS-API：logon 一次后连续 session.status，输出 p50/p95/均值（毫秒）。
// 运行：BINANCE_API_KEY=... BINANCE_SECRET_KEY=... go test ./trader/binance -run TestBinanceWSAPIBenchLatency -count=1 -v
// 测试网：再加 BINANCE_FUTURES_WS_TESTNET=1
func TestBinanceWSAPIBenchLatency(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("BINANCE_API_KEY"))
	sec := strings.TrimSpace(os.Getenv("BINANCE_SECRET_KEY"))
	if key == "" || sec == "" {
		t.Skip("未设置 BINANCE_API_KEY / BINANCE_SECRET_KEY，跳过")
	}
	ep := futuresWSAPIMainnet
	if os.Getenv("BINANCE_FUTURES_WS_TESTNET") == "1" {
		ep = futuresWSAPITestnet
	}
	rest := futures.NewClient(key, sec)
	if _, err := rest.NewSetServerTimeService().Do(context.Background()); err != nil {
		t.Fatalf("sync time: %v", err)
	}
	timeFn := func(ctx context.Context) (int64, error) {
		srv, err := rest.NewServerTimeService().Do(ctx)
		if err != nil {
			return 0, err
		}
		return srv + rest.TimeOffset, nil
	}
	cli := NewFuturesWSAPIClient(key, sec, ep, timeFn)
	ctx := context.Background()
	if err := cli.ensureConn(ctx); err != nil {
		t.Fatalf("dial: %v", err)
	}
	t0 := time.Now()
	if _, err := cli.SessionLogon(ctx, "bench-logon"); err != nil {
		_ = cli.Close()
		t.Fatalf("logon: %v", err)
	}
	logonMs := float64(time.Since(t0).Milliseconds())
	t.Logf("dial+logon: %.1f ms", logonMs)

	const n = 50
	lat := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		t1 := time.Now()
		if _, err := cli.SessionStatus(ctx, fmt.Sprintf("st-%d-%d", i, t1.UnixNano())); err != nil {
			_ = cli.Close()
			t.Fatalf("session.status %d: %v", i, err)
		}
		lat = append(lat, float64(time.Since(t1).Milliseconds()))
	}
	_ = cli.Close()

	sort.Float64s(lat)
	mean := 0.0
	for _, x := range lat {
		mean += x
	}
	mean /= float64(len(lat))
	p50 := lat[len(lat)/2]
	p95 := lat[int(float64(len(lat))*0.95)]
	if p95 < p50 {
		p95 = lat[len(lat)-1]
	}
	t.Logf("session.status x%d: mean=%.2f ms p50=%.2f ms p95=%.2f ms max=%.2f ms", n, mean, p50, p95, lat[len(lat)-1])
}
