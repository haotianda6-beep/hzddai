package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	appcrypto "nofx/crypto"
	"nofx/store"
	"nofx/trader/hz"
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
			followers++
		}
	}
	if followers != 5 {
		panic(fmt.Sprintf("expected 5 followers, got %d", followers))
	}
	fmt.Printf("capabilities=6/6 master_snapshot=true followers=%d sequence=%s positions=%d\n",
		followers, snapshot.LastSequence, len(snapshot.Positions))
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
