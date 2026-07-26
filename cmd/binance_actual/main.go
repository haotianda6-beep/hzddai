// 打印指定交易员在币安 U 本位的实盘余额与持仓（直连交易所 REST）。
// 用法：go run ./cmd/binance_actual/main.go [--direct] [--sync-orders] <trader_id> ...
//
//	--direct       忽略交易所里配置的 SOCKS5/HTTP 出口代理，从本机网卡出口访问币安
//	               （币安看到的请求 IP = 服务器公网 IP，需在 API Key 白名单中放行该 IP）
//	--sync-orders  查询前先同步近期待成交，补齐 WS 下单后本地订单/持仓记录
package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"nofx/config"
	"nofx/crypto"
	"nofx/logger"
	"nofx/store"
	"nofx/trader/binance"
)

func main() {
	_ = godotenv.Load()
	logger.Init(nil)
	config.Init()

	cs, err := crypto.NewCryptoService()
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}
	crypto.SetGlobalCryptoService(cs)

	cfg := config.Get()
	st, err := store.NewWithConfig(store.DBConfig{
		Type: store.DBTypeSQLite,
		Path: cfg.DBPath,
	})
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	args := os.Args[1:]
	direct := false
	syncOrders := false
	filtered := args[:0]
	for _, a := range args {
		if a == "--direct" || a == "-direct" {
			direct = true
			continue
		}
		if a == "--sync-orders" {
			syncOrders = true
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered

	if len(args) == 0 {
		// 默认：数字名交易员 154515 + 杜锋量化测试 + 亮亮（SOL 主策略）
		args = []string{
			"eef464cc_99958073-68ca-4060-a706-8a194be14e7f_comkun_ai_1778047237",
			"bbd9d402_c21e18c0-8433-4979-8421-8b9081a54b38_comkun_ai_1778134934",
			"4907a4da_ed8f791f-8984-456a-bc8f-e71502f5f2b8_deepseek_1777438617",
		}
	}

	if direct && len(args) > 0 {
		printOutboundIPDirect()
	}

	for _, tid := range args {
		tid = strings.TrimSpace(tid)
		if tid == "" {
			continue
		}
		dumpOne(st, tid, direct, syncOrders)
	}
}

func printOutboundIPDirect() {
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Get("https://api.ipify.org?format=text")
	if err != nil {
		fmt.Printf("（本机出口公网 IP 探测失败: %v）\n", err)
		return
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Printf("（本机出口公网 IP 读取失败: status=%s）\n", resp.Status)
		return
	}
	ip := strings.TrimSpace(string(b))
	if ip != "" {
		fmt.Printf("══ 当前直连出口公网 IP（币安会校验白名单）: %s ══\n\n", ip)
	}
}

func dumpOne(st *store.Store, traderID string, direct bool, syncOrders bool) {
	tr, err := st.Trader().GetByID(traderID)
	if err != nil || tr == nil {
		log.Printf("未找到交易员 %s\n", traderID)
		return
	}
	userLabel := tr.UserID
	if u, err := st.User().GetByID(tr.UserID); err == nil && u != nil {
		userLabel = fmt.Sprintf("%s (%s)", strings.TrimSpace(u.DisplayName), u.Email)
	}

	fc, err := st.Trader().GetFullConfig(tr.UserID, traderID)
	if err != nil {
		log.Printf("%s GetFullConfig: %v\n", traderID, err)
		return
	}
	ex := fc.Exchange
	if ex == nil || !ex.Enabled || strings.ToLower(strings.TrimSpace(ex.ExchangeType)) != "binance" {
		fmt.Printf("\n=== %s (%s) — 非启用币安，跳过 ===\n\n", tr.Name, traderID)
		return
	}

	proxyURL := strings.TrimSpace(string(ex.OutboundProxyURL))
	if direct {
		if proxyURL != "" {
			fmt.Printf("── %s：已忽略交易所出口代理，改为从服务器直连币安 ──\n", tr.Name)
		}
		proxyURL = ""
	}

	ft := binance.NewFuturesTrader(string(ex.APIKey), string(ex.SecretKey), tr.UserID, proxyURL, ex.Testnet)
	if syncOrders {
		if err := ft.SyncOrdersFromBinance(traderID, ex.ID, ex.ExchangeType, st); err != nil {
			log.Printf("SyncOrdersFromBinance: %v\n", err)
		} else {
			fmt.Printf("── 已执行 Binance 成交同步 ──\n")
		}
	}

	bal, err := ft.GetBalance()
	if err != nil {
		fmt.Printf("\n=== %s (%s) 用户侧备注名后续可从 users 表读 ===\n", tr.Name, traderID)
		log.Printf("GetBalance: %v\n", err)
		return
	}

	var wallet, avail, unreal, eq float64
	if v, ok := bal["totalWalletBalance"].(float64); ok {
		wallet = v
	}
	if v, ok := bal["availableBalance"].(float64); ok {
		avail = v
	}
	if v, ok := bal["totalUnrealizedProfit"].(float64); ok {
		unreal = v
	}
	if v, ok := bal["totalEquity"].(float64); ok && v > 0 {
		eq = v
	} else {
		eq = wallet + unreal
	}

	fmt.Printf("\n╔════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ 用户: %-51s ║\n", truncate(userLabel, 51))
	fmt.Printf("║ 交易员名: %-47s ║\n", truncate(tr.Name, 47))
	fmt.Printf("║ trader_id: %-46s ║\n", truncate(traderID, 46))
	fmt.Printf("║ 初始(策略): %-44.4f ║\n", tr.InitialBalance)
	fmt.Printf("╠════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ 币安 U 本位账户(REST 实时)                                ║\n")
	fmt.Printf("║ 钱包余额 totalWalletBalance: %-29.4f USDT ║\n", wallet)
	fmt.Printf("║ 可用余额 availableBalance:   %-29.4f USDT ║\n", avail)
	fmt.Printf("║ 未实现盈亏 totalUnrealized:  %-29.4f USDT ║\n", unreal)
	fmt.Printf("║ 总权益(钱包+浮盈或接口权益): %-29.4f USDT ║\n", eq)
	fmt.Printf("║ 相对初始总盈亏(估算):       %-29.4f USDT ║\n", eq-tr.InitialBalance)
	fmt.Printf("╠════════════════════════════════════════════════════════════╣\n")

	pos, err := ft.GetPositions()
	if err != nil {
		fmt.Printf("║ GetPositions 失败: %v\n", err)
		fmt.Printf("╚════════════════════════════════════════════════════════════╝\n")
		return
	}
	n := 0
	for _, p := range pos {
		sym, _ := p["symbol"].(string)
		side, _ := p["side"].(string)
		amt, _ := p["positionAmt"].(float64)
		if sym == "" || math.Abs(amt) < 1e-12 {
			continue
		}
		n++
		entry, _ := p["entryPrice"].(float64)
		mark, _ := p["markPrice"].(float64)
		uPnL, _ := p["unRealizedProfit"].(float64)
		lev, _ := p["leverage"].(float64)
		fmt.Printf("║ 持仓 #%d %-12s %-5s 张数:%.6f 入:%.6f 标:%.6f 浮盈:%.4f 杠杆:%.0f ║\n",
			n, sym, strings.ToUpper(side), math.Abs(amt), entry, mark, uPnL, lev)
	}
	if n == 0 {
		fmt.Printf("║ (当前无持仓)                                              ║\n")
	}
	fmt.Printf("╚════════════════════════════════════════════════════════════╝\n")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
