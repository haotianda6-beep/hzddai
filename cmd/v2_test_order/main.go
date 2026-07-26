// 对主控交易员下一笔小额市价单，触发 v2 镜像跟单测试。
// 用法：go run ./cmd/v2_test_order/main.go
package main

import (
	"context"
	"fmt"
	"log"
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

	// 找到小黄猎头交易员
	masterName := "小黄猎头"
	var traders []store.Trader
	if err := st.GormDB().Where("name LIKE ?", "%"+masterName+"%").Find(&traders).Error; err != nil {
		log.Fatalf("查询交易员: %v", err)
	}

	var master *store.Trader
	for i := range traders {
		if strings.Contains(traders[i].Name, masterName) {
			master = &traders[i]
			break
		}
	}
	if master == nil {
		log.Fatalf("未找到交易员: %s", masterName)
	}
	fmt.Printf("找到主控: %s (%s)\n", master.Name, master.ID)

	fc, err := st.Trader().GetFullConfig(master.UserID, master.ID)
	if err != nil {
		log.Fatalf("GetFullConfig: %v", err)
	}
	ex := fc.Exchange
	if ex == nil || !ex.Enabled {
		log.Fatal("交易所未启用")
	}

	proxyURL := strings.TrimSpace(string(ex.OutboundProxyURL))
	ft := binance.NewFuturesTrader(string(ex.APIKey), string(ex.SecretKey), master.UserID, proxyURL, ex.Testnet)

	// 查询当前余额和持仓
	bal, err := ft.GetBalance()
	if err != nil {
		log.Fatalf("GetBalance: %v", err)
	}
	var avail, totalEq float64
	if v, ok := bal["availableBalance"].(float64); ok {
		avail = v
	}
	if v, ok := bal["totalEquity"].(float64); ok && v > 0 {
		totalEq = v
	}
	fmt.Printf("可用余额: %.4f USDT, 总权益: %.4f USDT\n", avail, totalEq)

	// 获取 SOL 当前价格
	price, err := ft.GetMarketPrice("SOLUSDT")
	if err != nil {
		log.Fatalf("GetMarketPrice: %v", err)
	}
	fmt.Printf("SOL 当前价: %.4f\n", price)

	// 计算最小下单量（最少约 12 USDT 名义值）
	qty := 12.0 / price
	qty = float64(int(qty*100)) / 100
	if qty < 0.01 {
		qty = 0.01
	}
	fmt.Printf("计划下单: %f SOL (~%.2f USDT)\n", qty, qty*price)

	// 设置杠杆和保证金模式
	if err := ft.SetMarginMode("SOLUSDT", true); err != nil {
		fmt.Printf("SetMarginMode 警告: %v\n", err)
	}
	if err := ft.SetLeverage("SOLUSDT", 20); err != nil {
		fmt.Printf("SetLeverage 警告: %v\n", err)
	}
	time.Sleep(5 * time.Second) // 杠杆冷却

	// 用 REST API 下一笔市价多单（HMAC 密钥只支持 REST）
	ctx := context.Background()
	_ = ctx
	result, err := ft.OpenLong("SOLUSDT", qty, 20)
	if err != nil {
		log.Fatalf("开多失败: %v", err)
	}
	fmt.Printf("\n✅ 市价开多成功! (REST API)\n")
	fmt.Printf("   交易员: %s\n", master.Name)
	fmt.Printf("   币对: SOLUSDT\n")
	fmt.Printf("   数量: %.2f SOL\n", qty)
	fmt.Printf("   杠杆: 20x\n")
	fmt.Printf("   结果: %v\n", result)

	// 如果配置了止盈止损，也一并设置
	time.Sleep(2 * time.Second)
	if err := ft.SetStopLoss("SOLUSDT", "LONG", qty, price*0.95); err != nil {
		fmt.Printf("SetStopLoss 警告: %v\n", err)
	} else {
		fmt.Printf("   止损: %.4f (-5%%)\n", price*0.95)
	}
	if err := ft.SetTakeProfit("SOLUSDT", "LONG", qty, price*1.03); err != nil {
		fmt.Printf("SetTakeProfit 警告: %v\n", err)
	} else {
		fmt.Printf("   止盈: %.4f (+3%%)\n", price*1.03)
	}

	fmt.Printf("\n⏱ 现在观察大黑猎手的 v2 跟单延迟...\n")
}
