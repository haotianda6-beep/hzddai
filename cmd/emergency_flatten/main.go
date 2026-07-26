// 一次性紧急工具：对币安 U 本位持仓市价全平。
//
// 按名称子串：
//
//	go run ./cmd/emergency_flatten/main.go 静默猎手 静默测
//
// 按合约标的（扫描库内所有启用币安的交易员，只平该币种）：
//
//	go run ./cmd/emergency_flatten/main.go --symbol SOLUSDT
//	go run ./cmd/emergency_flatten/main.go --symbol SOL
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"strings"

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
	cfg := config.Get()

	cs, err := crypto.NewCryptoService()
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}
	crypto.SetGlobalCryptoService(cs)

	st, err := store.NewWithConfig(store.DBConfig{
		Type: store.DBTypeSQLite,
		Path: cfg.DBPath,
	})
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	symFlag := flag.String("symbol", "", "只平该 U 本位合约（如 SOL / SOLUSDT）；扫描库内所有启用币安的交易员")
	flag.Parse()

	if strings.TrimSpace(*symFlag) != "" {
		target := normalizeFuturesSymbol(*symFlag)
		if target == "" {
			log.Fatal("无效的 --symbol")
		}
		fmt.Printf("按标的平仓: %s（所有币安交易员）\n", target)
		var traders []store.Trader
		if err := st.GormDB().Find(&traders).Error; err != nil {
			log.Fatalf("列出交易员: %v", err)
		}
		for i := range traders {
			flattenTrader(st, &traders[i], target)
		}
		return
	}

	patterns := flag.Args()
	if len(patterns) == 0 {
		log.Fatal("用法: ... [--symbol SOLUSDT] 或 ... <名称子串1> [名称子串2] ...")
	}

	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		var traders []store.Trader
		q := "%" + pat + "%"
		if err := st.GormDB().Where("name LIKE ?", q).Find(&traders).Error; err != nil {
			log.Fatalf("查询交易员: %v", err)
		}
		if len(traders) == 0 {
			log.Printf("未找到名称包含 %q 的交易员", pat)
			continue
		}
		for i := range traders {
			flattenTrader(st, &traders[i], "")
		}
	}
}

func normalizeFuturesSymbol(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "USDT") && !strings.HasSuffix(s, "BUSD") && !strings.HasSuffix(s, "USDC") {
		s = s + "USDT"
	}
	return s
}

// flattenTrader 若 onlySymbol 为空则平掉全部持仓；否则只平该合约。
func flattenTrader(st *store.Store, t *store.Trader, onlySymbol string) {
	fmt.Printf("—— 交易员 %s (%s) ——\n", t.Name, t.ID)
	fc, err := st.Trader().GetFullConfig(t.UserID, t.ID)
	if err != nil {
		log.Printf("GetFullConfig: %v", err)
		return
	}
	ex := fc.Exchange
	if ex == nil || !ex.Enabled {
		log.Printf("交易所未启用，跳过")
		return
	}
	if strings.ToLower(strings.TrimSpace(ex.ExchangeType)) != "binance" {
		log.Printf("非 binance（%s），跳过；请手动处理", ex.ExchangeType)
		return
	}

	ft := binance.NewFuturesTrader(string(ex.APIKey), string(ex.SecretKey), t.UserID, strings.TrimSpace(string(ex.OutboundProxyURL)), ex.Testnet)
	positions, err := ft.GetPositions()
	if err != nil {
		log.Printf("GetPositions: %v", err)
		return
	}
	n := 0
	for _, pos := range positions {
		sym, _ := pos["symbol"].(string)
		sym = strings.ToUpper(strings.TrimSpace(sym))
		side, _ := pos["side"].(string)
		amt, _ := pos["positionAmt"].(float64)
		if sym == "" || math.Abs(amt) < 1e-12 {
			continue
		}
		if onlySymbol != "" && sym != onlySymbol {
			continue
		}
		n++
		fmt.Printf("  平仓 %s %s 数量 %.8f\n", sym, strings.ToUpper(side), math.Abs(amt))
		_ = ft.CancelAllOrders(sym)
		var cerr error
		if side == "long" {
			_, cerr = ft.CloseLong(sym, 0)
		} else {
			_, cerr = ft.CloseShort(sym, 0)
		}
		if cerr != nil {
			log.Printf("  ❌ %s: %v", sym, cerr)
		} else {
			fmt.Printf("  ✓ 已提交平仓 %s\n", sym)
		}
	}
	if n == 0 {
		if onlySymbol != "" {
			fmt.Printf("  （无 %s 持仓）\n", onlySymbol)
		} else {
			fmt.Println("  （当前无持仓）")
		}
	}
}
