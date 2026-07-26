// 绑定「只做XAU」马丁策略 + 币安合约虚拟盘交易所 + COMKUN-AI 交易员（干跑/实盘前一键初始化）。
//
// 环境变量（必填）：
//
//	BINANCE_DEMO_API_KEY
//	BINANCE_DEMO_API_SECRET
//
// 可选：
//
//	SETUP_OWNER_EMAIL       默认 haotianda6@gmail.com
//	SETUP_TRADER_NAME       默认 XAU马丁虚拟
//	SETUP_EXCHANGE_NAME     默认 XAU虚拟盘
//	SETUP_START_TRADER=1    创建后自动 is_running=1
package main

import (
	"fmt"
	"log"
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

const (
	strategyID   = "mt5-xau-martingale-bn-draft-v1"
	exchangeAcct = "XAU虚拟盘"
	traderName   = "XAU马丁虚拟"
)

func main() {
	_ = godotenv.Load()
	logger.Init(nil)
	config.Init()

	apiKey := strings.TrimSpace(os.Getenv("BINANCE_DEMO_API_KEY"))
	secret := strings.TrimSpace(os.Getenv("BINANCE_DEMO_API_SECRET"))
	if apiKey == "" || secret == "" {
		log.Fatal("请设置 BINANCE_DEMO_API_KEY 与 BINANCE_DEMO_API_SECRET（币安 App → 模拟交易 → API 管理）")
	}

	ownerEmail := strings.TrimSpace(os.Getenv("SETUP_OWNER_EMAIL"))
	if ownerEmail == "" {
		ownerEmail = "haotianda6@gmail.com"
	}
	exName := strings.TrimSpace(os.Getenv("SETUP_EXCHANGE_NAME"))
	if exName == "" {
		exName = exchangeAcct
	}
	tName := strings.TrimSpace(os.Getenv("SETUP_TRADER_NAME"))
	if tName == "" {
		tName = traderName
	}

	cs, err := crypto.NewCryptoService()
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}
	crypto.SetGlobalCryptoService(cs)

	cfg := config.Get()
	st, err := store.NewWithConfig(store.DBConfig{Type: store.DBTypeSQLite, Path: cfg.DBPath})
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	user, err := st.User().GetByEmail(ownerEmail)
	if err != nil {
		log.Fatalf("用户 %s 不存在: %v", ownerEmail, err)
	}
	userID := user.ID

	ft := binance.NewFuturesTrader(apiKey, secret, userID, "", true)
	bal, err := ft.GetBalance()
	if err != nil {
		log.Fatalf("虚拟盘 API 校验失败: %v", err)
	}
	equity, _ := bal["totalWalletBalance"].(float64)
	fmt.Printf("✓ 虚拟盘连通，钱包约 %.2f USDT\n", equity)

	strategy, err := st.Strategy().Get(userID, strategyID)
	if err != nil {
		log.Fatalf("策略 %s 不存在，请先运行 scripts/seed_mt5_xau_market_strategy.py: %v", strategyID, err)
	}
	fmt.Printf("✓ 策略: %s\n", strategy.Name)

	modelID := userID + "_comkun_ai"
	model, err := st.AIModel().Get(userID, modelID)
	if err != nil {
		log.Fatalf("COMKUN-AI 模型未配置: %v", err)
	}
	if !model.Enabled {
		log.Fatal("请先在设置页启用 COMKUN-AI 模型")
	}

	exID, err := ensureDemoExchange(st, userID, exName, apiKey, secret)
	if err != nil {
		log.Fatalf("交易所: %v", err)
	}
	fmt.Printf("✓ 交易所: %s (%s, testnet=1)\n", exName, exID)

	traderID, created, err := ensureMartingaleTrader(st, userID, exID, modelID, tName, equity)
	if err != nil {
		log.Fatalf("交易员: %v", err)
	}
	if created {
		fmt.Printf("✓ 已创建交易员: %s (%s)\n", tName, traderID)
	} else {
		fmt.Printf("✓ 已更新交易员: %s (%s)\n", tName, traderID)
	}

	if strings.TrimSpace(os.Getenv("SETUP_START_TRADER")) == "1" {
		if err := st.Trader().UpdateStatus(userID, traderID, true); err != nil {
			log.Fatalf("启动交易员失败: %v", err)
		}
		fmt.Println("✓ 已标记 is_running=1，重启 nofx 容器后生效")
	} else {
		fmt.Println("→ 下一步：个人中心确认交易所 Key，部署向导创建/启动交易员，或 SETUP_START_TRADER=1 重跑本脚本")
	}
}

func ensureDemoExchange(st *store.Store, userID, accountName, apiKey, secret string) (string, error) {
	exs, err := st.Exchange().List(userID)
	if err != nil {
		return "", err
	}
	for _, ex := range exs {
		if ex.ExchangeType == "binance" && ex.Testnet && strings.TrimSpace(ex.AccountName) == accountName {
			if err := st.Exchange().Update(userID, ex.ID, true, apiKey, secret, "", true,
				ex.APIURL,
				ex.HyperliquidWalletAddr, ex.HyperliquidUnifiedAcct,
				ex.AsterUser, ex.AsterSigner, string(ex.AsterPrivateKey),
				ex.LighterWalletAddr, string(ex.LighterPrivateKey), string(ex.LighterAPIKeyPrivateKey), ex.LighterAPIKeyIndex,
				nil, false); err != nil {
				return "", err
			}
			return ex.ID, nil
		}
	}
	return st.Exchange().Create(userID, "binance", accountName, true,
		apiKey, secret, "", true,
		"",
		"", true,
		"", "", "",
		"", "", "", 0,
		"")
}

func ensureMartingaleTrader(st *store.Store, userID, exchangeID, modelID, name string, equity float64) (string, bool, error) {
	traders, err := st.Trader().List(userID)
	if err != nil {
		return "", false, err
	}
	for _, tr := range traders {
		if tr.StrategyID == strategyID && tr.ExchangeID == exchangeID {
			tr.Name = name
			tr.AIModelID = modelID
			if equity > 0 {
				tr.InitialBalance = equity
			}
			if tr.ScanIntervalMinutes <= 0 {
				tr.ScanIntervalMinutes = 3
			}
			if err := st.Trader().Update(tr); err != nil {
				return "", false, err
			}
			if equity > 0 {
				_ = st.Trader().UpdateInitialBalance(userID, tr.ID, equity)
			}
			return tr.ID, false, nil
		}
	}

	exShort := exchangeID
	if len(exShort) > 8 {
		exShort = exShort[:8]
	}
	traderID := fmt.Sprintf("%s_%s_%d", exShort, modelID, time.Now().Unix())
	initial := equity
	if initial <= 0 {
		initial = 5000
	}
	tr := &store.Trader{
		ID:                  traderID,
		UserID:              userID,
		Name:                name,
		AIModelID:           modelID,
		ExchangeID:          exchangeID,
		StrategyID:          strategyID,
		InitialBalance:      initial,
		ScanIntervalMinutes: 3,
		IsRunning:           false,
		IsCrossMargin:       true,
		ShowInCompetition:   false,
	}
	if err := st.Trader().Create(tr); err != nil {
		return "", false, err
	}
	return traderID, true, nil
}
