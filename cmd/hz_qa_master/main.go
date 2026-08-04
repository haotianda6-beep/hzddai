package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"nofx/trader/hz"
)

func main() {
	if len(os.Args) < 2 {
		panic("usage: hz_qa_master ACTION [ARGS]")
	}
	trader, err := hz.NewTrader(requiredEnv("HZ_MASTER_POLL_API_URL"), requiredEnv("HZ_MASTER_POLL_API_KEY"), requiredEnv("HZ_MASTER_POLL_API_SECRET"), true)
	must(err)
	defer trader.Close()
	must(trader.Reconcile())
	action := os.Args[1]
	switch action {
	case "status":
		positions, statusErr := trader.GetPositions()
		must(statusErr)
		fmt.Printf("ready=%t positions=%d\n", trader.IsReady(), len(positions))
	case "price":
		requireArgs(3)
		price, priceErr := trader.GetMarketPrice(os.Args[2])
		must(priceErr)
		fmt.Printf("symbol=%s price=%.8f\n", strings.ToUpper(os.Args[2]), price)
	case "open-long", "open-short":
		requireArgs(6)
		symbol, quantity := lotQuantity(trader, os.Args[2], os.Args[3])
		leverage, parseErr := strconv.Atoi(os.Args[4])
		must(parseErr)
		var result map[string]interface{}
		result, err = trader.ExecuteWithIntent(os.Args[5], func() (map[string]interface{}, error) {
			if action == "open-long" {
				return trader.OpenLong(symbol, quantity, leverage)
			}
			return trader.OpenShort(symbol, quantity, leverage)
		})
		must(err)
		printResult(action, result)
	case "close-long", "close-short":
		requireArgs(5)
		symbol := strings.ToUpper(os.Args[2])
		lots, parseErr := strconv.ParseFloat(os.Args[3], 64)
		must(parseErr)
		quantity := 0.0
		if lots > 0 {
			quantity, err = trader.QuantityForLots(symbol, lots)
			must(err)
		}
		var result map[string]interface{}
		result, err = trader.ExecuteWithIntent(os.Args[4], func() (map[string]interface{}, error) {
			if action == "close-long" {
				return trader.CloseLong(symbol, quantity)
			}
			return trader.CloseShort(symbol, quantity)
		})
		must(err)
		printResult(action, result)
	case "protect-long", "protect-short":
		requireArgs(5)
		symbol := strings.ToUpper(os.Args[2])
		tp, parseErr := strconv.ParseFloat(os.Args[3], 64)
		must(parseErr)
		sl, parseErr := strconv.ParseFloat(os.Args[4], 64)
		must(parseErr)
		side := "long"
		if action == "protect-short" {
			side = "short"
		}
		must(trader.SetTakeProfit(symbol, side, 0, tp))
		must(trader.SetStopLoss(symbol, side, 0, sl))
		fmt.Printf("action=%s symbol=%s protection=updated\n", action, symbol)
	case "clear-protection":
		requireArgs(3)
		must(trader.CancelStopOrders(strings.ToUpper(os.Args[2])))
		fmt.Printf("action=%s symbol=%s protection=cleared\n", action, strings.ToUpper(os.Args[2]))
	default:
		panic("unsupported action")
	}
}

func lotQuantity(trader *hz.Trader, symbol, rawLots string) (string, float64) {
	lots, err := strconv.ParseFloat(rawLots, 64)
	must(err)
	symbol = strings.ToUpper(symbol)
	quantity, err := trader.QuantityForLots(symbol, lots)
	must(err)
	return symbol, quantity
}

func printResult(action string, result map[string]interface{}) {
	fmt.Printf("action=%s order=%v status=%v\n", action, result["orderId"], result["status"])
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		panic("missing required environment")
	}
	return value
}

func requireArgs(count int) {
	if len(os.Args) != count {
		panic("invalid arguments")
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func init() {
	time.Local = time.UTC
}
