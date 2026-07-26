// 运维：为指定交易所账号写入「独立 REST 出口代理」（加密存储），不落日志明文。
// 用法（在仓库根目录，且 .env 可用）：
//
//	go run ./cmd/set_exchange_proxy/main.go -exchange-id <uuid> -proxy 'socks5://user:pass@host:port'
//
// 说明：通过 store 正式加密写入 outbound_proxy_url，与前端「交易所配置」一致。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"nofx/config"
	"nofx/crypto"
	"nofx/logger"
	"nofx/store"
)

func main() {
	_ = godotenv.Load()
	logger.Init(nil)
	config.Init()

	exchangeID := flag.String("exchange-id", "", "exchanges.id（必填）")
	proxyURL := flag.String("proxy", "", "完整代理 URL，如 socks5://user:pass@host:port（必填）")
	flag.Parse()

	if *exchangeID == "" || *proxyURL == "" {
		flag.Usage()
		os.Exit(2)
	}

	cs, err := crypto.NewCryptoService()
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}
	crypto.SetGlobalCryptoService(cs)

	cfg := config.Get()
	gdb, err := store.InitGormWithConfig(store.DBConfig{
		Type: store.DBTypeSQLite,
		Path: cfg.DBPath,
	})
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	var ex store.Exchange
	if err := gdb.Where("id = ?", *exchangeID).First(&ex).Error; err != nil {
		log.Fatalf("查交易所失败（请核对 exchange-id）: %v", err)
	}

	es := store.NewExchangeStore(gdb)
	p := *proxyURL
	if err := es.Update(ex.UserID, ex.ID, ex.Enabled, "", "", "", ex.Testnet,
		ex.APIURL, ex.HyperliquidWalletAddr, ex.HyperliquidUnifiedAcct,
		ex.AsterUser, ex.AsterSigner, "", ex.LighterWalletAddr, "", "", ex.LighterAPIKeyIndex,
		&p, false); err != nil {
		log.Fatalf("写入代理失败: %v", err)
	}

	fmt.Printf("已写入 outbound_proxy_url（exchange_id=%s user_id=%s account=%s）\n",
		ex.ID, ex.UserID, ex.AccountName)
}
