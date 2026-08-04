package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	appcrypto "nofx/crypto"
	"nofx/store"
	"nofx/trader/hz"
	"nofx/trader/types"
)

func main() {
	if len(os.Args) != 2 || !strings.Contains(os.Args[1], "comkun-hz-qa") {
		panic("usage: hz_qa_validate QA_DATABASE")
	}
	cryptoService, err := appcrypto.NewCryptoService()
	must(err)
	appcrypto.SetGlobalCryptoService(cryptoService)
	apiURL := requiredEnv("HZ_MASTER_POLL_API_URL")
	masterKey := requiredEnv("HZ_MASTER_POLL_API_KEY")
	masterSecret := requiredEnv("HZ_MASTER_POLL_API_SECRET")
	masterAccount := requiredEnv("HZ_MASTER_POLL_ACCOUNT_ID")
	master, err := hz.VerifyCapabilities(apiURL, masterKey, masterSecret)
	must(err)
	if master.AccountID != masterAccount {
		panic("master account mismatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	raw, err := hz.FetchMasterSnapshot(ctx, apiURL, masterKey, masterSecret)
	must(err)
	var snapshot struct {
		MasterAccountID string            `json:"masterAccountId"`
		LastSequence    string            `json:"lastSequence"`
		Positions       []json.RawMessage `json:"positions"`
	}
	must(json.Unmarshal(raw, &snapshot))
	if snapshot.MasterAccountID != masterAccount {
		panic("master snapshot mismatch")
	}
	evidenceSince := optionalEvidenceSince()
	masterTrader, err := hz.NewTrader(apiURL, masterKey, masterSecret, true)
	must(err)
	masterTrades, err := masterTrader.GetClosedPnL(evidenceSince, 100)
	masterTrader.Close()
	must(err)
	fmt.Printf("master %s\n", tradeEvidence(masterTrades))
	for i := range raw {
		raw[i] = 0
	}

	st, err := store.New(os.Args[1])
	must(err)
	users, err := st.User().GetAllIDs()
	must(err)
	followers := 0
	for _, userID := range users {
		exchanges, listErr := st.Exchange().List(userID)
		must(listErr)
		for _, exchange := range exchanges {
			if exchange.ExchangeType != "hz" {
				continue
			}
			verified, verifyErr := hz.VerifyCapabilities(exchange.APIURL, string(exchange.APIKey), string(exchange.SecretKey))
			must(verifyErr)
			if verified.AccountFingerprint != exchange.HZAccountFingerprint ||
				verified.WalletFingerprint != exchange.HZWalletFingerprint ||
				verified.PositionBookFingerprint != exchange.HZPositionBookFingerprint {
				panic("follower scope fingerprint mismatch")
			}
			follower, traderErr := hz.NewTrader(exchange.APIURL, string(exchange.APIKey), string(exchange.SecretKey), true)
			must(traderErr)
			positions, positionsErr := follower.GetPositions()
			trades, tradesErr := follower.GetClosedPnL(evidenceSince, 100)
			follower.Close()
			must(positionsErr)
			must(tradesErr)
			fmt.Printf("follower=%s positions=%d protected=%d totals=%s %s\n", userID, len(positions), protectedPositions(positions), positionTotals(positions), tradeEvidence(trades))
			followers++
		}
	}
	if followers != 5 {
		panic(fmt.Sprintf("expected 5 followers, got %d", followers))
	}
	fmt.Printf("capabilities=6/6 master_snapshot=true followers=%d sequence=%s positions=%d\n",
		followers, snapshot.LastSequence, len(snapshot.Positions))
}

func optionalEvidenceSince() time.Time {
	raw := strings.TrimSpace(os.Getenv("HZ_QA_EVIDENCE_SINCE"))
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	must(err)
	return parsed
}

func tradeEvidence(records []types.ClosedPnLRecord) string {
	orders := make(map[string]struct{}, len(records))
	quantity, realized := 0.0, 0.0
	for _, record := range records {
		orders[record.OrderID] = struct{}{}
		quantity += record.Quantity
		realized += record.RealizedPnL
	}
	return fmt.Sprintf("trades=%d orders=%d quantity=%.6f realized_pnl=%.6f", len(records), len(orders), quantity, realized)
}

func protectedPositions(positions []map[string]interface{}) int {
	count := 0
	for _, position := range positions {
		takeProfit, _ := position["takeProfit"].(float64)
		stopLoss, _ := position["stopLoss"].(float64)
		if takeProfit > 0 || stopLoss > 0 {
			count++
		}
	}
	return count
}

func positionTotals(positions []map[string]interface{}) string {
	totals := make(map[string]float64)
	for _, position := range positions {
		symbol, _ := position["symbol"].(string)
		side, _ := position["side"].(string)
		quantity, _ := position["positionAmt"].(float64)
		totals[strings.ToUpper(symbol)+"_"+strings.ToLower(side)] += quantity
	}
	keys := make([]string, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, fmt.Sprintf("%s=%.6f", key, totals[key]))
	}
	return strings.Join(values, ",")
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		panic("missing required environment")
	}
	return value
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
