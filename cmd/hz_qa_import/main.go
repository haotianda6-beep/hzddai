package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appcrypto "nofx/crypto"
	"nofx/store"
	"nofx/trader/hz"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type credential struct {
	Account      string `json:"account"`
	UserID       string `json:"userId"`
	APIKey       string `json:"apiKey"`
	APISecret    string `json:"apiSecret"`
	Balance      string `json:"balance"`
	RatioPercent int    `json:"ratioPercent"`
}

type bundle struct {
	HZAPIURL    string       `json:"hzApiUrl"`
	EventURL    string       `json:"eventUrl"`
	EventSecret string       `json:"eventSecret"`
	Master      credential   `json:"master"`
	Followers   []credential `json:"followers"`
}

type snapshot struct {
	LastSequence    string            `json:"lastSequence"`
	MasterAccountID string            `json:"masterAccountId"`
	Account         snapshotAccount   `json:"account"`
	Positions       []json.RawMessage `json:"positions"`
}

type snapshotAccount struct {
	Equity   string `json:"equity"`
	Tradable bool   `json:"tradable"`
}

type validatedFollower struct {
	Credential credential
	Scope      *hz.VerifiedCapabilities
}

func main() {
	if len(os.Args) != 5 {
		panic("usage: hz_qa_import BUNDLE DATABASE RUNTIME_ENV EVENT_SECRET_FILE")
	}
	bundlePath, dbPath, envPath, eventSecretPath := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	if !strings.Contains(filepath.Clean(dbPath), "comkun-hz-qa") {
		panic("refusing non-QA database path")
	}
	pkg := readBundle(bundlePath)
	masterScope, err := hz.VerifyCapabilities(pkg.HZAPIURL, pkg.Master.APIKey, pkg.Master.APISecret)
	must(err)
	validated := make([]validatedFollower, 0, len(pkg.Followers))
	for _, follower := range pkg.Followers {
		scope, verifyErr := hz.VerifyCapabilities(pkg.HZAPIURL, follower.APIKey, follower.APISecret)
		must(verifyErr)
		validated = append(validated, validatedFollower{Credential: follower, Scope: scope})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rawSnapshot, err := hz.FetchMasterSnapshot(ctx, pkg.HZAPIURL, pkg.Master.APIKey, pkg.Master.APISecret)
	must(err)
	var current snapshot
	must(json.Unmarshal(rawSnapshot, &current))
	for i := range rawSnapshot {
		rawSnapshot[i] = 0
	}
	sequence, err := strconv.ParseInt(strings.TrimSpace(current.LastSequence), 10, 64)
	must(err)
	if sequence < 0 || current.MasterAccountID != masterScope.AccountID || !current.Account.Tradable {
		panic("master snapshot scope or tradable state mismatch")
	}
	if len(current.Positions) != 0 {
		panic("master must be flat before QA import")
	}
	masterEquity, err := strconv.ParseFloat(strings.TrimSpace(current.Account.Equity), 64)
	must(err)
	if masterEquity <= 0 {
		panic("invalid master equity")
	}

	cryptoService, err := appcrypto.NewCryptoService()
	must(err)
	appcrypto.SetGlobalCryptoService(cryptoService)
	db, err := store.InitGorm(dbPath)
	must(err)
	defer closeDB(db)
	must(provision(db, pkg, masterScope, validated, masterEquity, sequence))
	must(updateRuntimeEnv(envPath, map[string]string{
		"COMKUN_AI_EVENT_SECRET":          pkg.EventSecret,
		"HZ_MASTER_POLL_API_URL":          pkg.HZAPIURL,
		"HZ_MASTER_POLL_API_KEY":          pkg.Master.APIKey,
		"HZ_MASTER_POLL_API_SECRET":       pkg.Master.APISecret,
		"HZ_MASTER_POLL_ACCOUNT_ID":       masterScope.AccountID,
		"HZ_MASTER_POLL_INTERVAL_SECONDS": "5",
	}))
	must(writeRootOnly(eventSecretPath, []byte(pkg.EventSecret+"\n")))
	fmt.Printf("imported master=1 followers=%d capabilities=%d source=%s baseline_sequence=%d baseline_positions=%d\n",
		len(validated), len(validated)+1, store.HZMasterSourceStrategyID(masterScope.AccountID), sequence, len(current.Positions))
}

func updateRuntimeEnv(path string, updates map[string]string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	seen := make(map[string]bool, len(updates))
	for i, line := range lines {
		key, _, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if value, ok := updates[key]; ok {
			lines[i] = key + "=" + value
			seen[key] = true
		}
	}
	for key, value := range updates {
		if !seen[key] {
			lines = append(lines, key+"="+value)
		}
	}
	return writeRootOnly(path, []byte(strings.Join(lines, "\n")+"\n"))
}

func writeRootOnly(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".hz-qa-import-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func readBundle(path string) bundle {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20))
	must(err)
	var pkg bundle
	must(json.Unmarshal(raw, &pkg))
	for i := range raw {
		raw[i] = 0
	}
	if !strings.HasPrefix(pkg.HZAPIURL, "https://") || !strings.HasPrefix(pkg.EventURL, "https://") ||
		pkg.EventSecret == "" || pkg.Master.APIKey == "" || pkg.Master.APISecret == "" || len(pkg.Followers) != 5 {
		panic("credential package shape mismatch")
	}
	want := []int{20, 40, 60, 80, 100}
	seen := map[string]bool{}
	for i, follower := range pkg.Followers {
		if follower.RatioPercent != want[i] || follower.APIKey == "" || follower.APISecret == "" || follower.Account == "" {
			panic("credential package follower mismatch")
		}
		if seen[follower.Account] {
			panic("duplicate follower account")
		}
		seen[follower.Account] = true
	}
	return pkg
}

func provision(db *gorm.DB, pkg bundle, masterScope *hz.VerifiedCapabilities, followers []validatedFollower, masterEquity float64, sequence int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var exchangeCount, traderCount int64
		if err := tx.Model(&store.Exchange{}).Where("exchange_type = ?", "hz").Count(&exchangeCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.Trader{}).Count(&traderCount).Error; err != nil {
			return err
		}
		if exchangeCount != 0 || traderCount != 0 {
			return fmt.Errorf("QA target is not clean")
		}
		st, err := store.NewFromGorm(tx)
		if err != nil {
			return err
		}
		sourceID := store.HZMasterSourceStrategyID(masterScope.AccountID)
		if err := tx.Where("id = ?", "hz-qa-linkage").Delete(&store.Strategy{}).Error; err != nil {
			return err
		}
		listingCfg := store.GetDefaultStrategyConfig("zh")
		listingCfg.ComkunFollowListingTemplate = true
		listingCfg.ComkunListingMasterSkipExchangeExecution = true
		listingRaw, err := json.Marshal(listingCfg)
		if err != nil {
			return err
		}
		publicStrategy := &store.Strategy{
			ID: sourceID, UserID: "hz-qa-admin", Name: "HZ 主控联动 QA", Description: "COMKUN-AI / AI策略执行",
			IsActive: true, IsPublic: true, ConfigVisible: false, MarketAccess: store.MarketAccessSubscription,
			MarketAIModel: "comkun_ai", ContentLocked: true, Config: string(listingRaw), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := tx.Create(publicStrategy).Error; err != nil {
			return err
		}

		traderIDs := make([]string, 0, len(followers))
		for _, follower := range followers {
			ratio := follower.Credential.RatioPercent
			userID := fmt.Sprintf("hz-qa-follower-%03d", ratio)
			var users int64
			if err := tx.Model(&store.User{}).Where("id = ?", userID).Count(&users).Error; err != nil || users != 1 {
				return fmt.Errorf("missing QA user for ratio %d", ratio)
			}
			modelID := deterministicID("model", userID)
			exchangeID := deterministicID("exchange", userID)
			strategyID := deterministicID("strategy", userID)
			traderID := deterministicID("trader", userID)
			model := &store.AIModel{ID: modelID, UserID: userID, Name: "COMKUN-AI", Provider: "comkun_ai", Enabled: true}
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			exchange := &store.Exchange{
				ID: exchangeID, ExchangeType: "hz", AccountName: "HZ AI QA", UserID: userID, Name: "HZ 交易账户", Type: "cex", Enabled: true,
				APIKey: appcrypto.EncryptedString(follower.Credential.APIKey), SecretKey: appcrypto.EncryptedString(follower.Credential.APISecret), APIURL: pkg.HZAPIURL,
				HZAccountFingerprint: follower.Scope.AccountFingerprint, HZWalletFingerprint: follower.Scope.WalletFingerprint,
				HZPositionBookFingerprint: follower.Scope.PositionBookFingerprint,
			}
			if err := tx.Create(exchange).Error; err != nil {
				return err
			}
			cfg := store.GetDefaultStrategyConfig("zh")
			cfg.ComkunMarketFollow = true
			cfg.ComkunMarketSourceStrategyID = sourceID
			cfg.ComkunMirrorFollowerEquityRatio = float64(ratio) / 100
			cfgRaw, err := json.Marshal(cfg)
			if err != nil {
				return err
			}
			privateStrategy := &store.Strategy{
				ID: strategyID, UserID: userID, Name: "AI策略执行", Description: "HZ 主控联动 QA",
				IsActive: true, IsPublic: false, ConfigVisible: false, MarketAccess: store.MarketAccessOff,
				SourceStrategyID: sourceID, SourceMarketAccess: store.MarketAccessSubscription, ContentLocked: true,
				Config: string(cfgRaw), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}
			if err := tx.Create(privateStrategy).Error; err != nil {
				return err
			}
			balance, err := strconv.ParseFloat(strings.TrimSpace(follower.Credential.Balance), 64)
			if err != nil || balance <= 0 {
				return fmt.Errorf("invalid follower equity for ratio %d", ratio)
			}
			trader := &store.Trader{
				ID: traderID, UserID: userID, Name: "AI策略执行", AIModelID: modelID, ExchangeID: exchangeID,
				StrategyID: strategyID, InitialBalance: balance, ScanIntervalMinutes: 1, IsRunning: true,
				IsCrossMargin: true, ShowInCompetition: false,
			}
			if err := tx.Create(trader).Error; err != nil {
				return err
			}
			traderIDs = append(traderIDs, traderID)
		}
		stateRaw, err := json.Marshal(map[string]any{
			"v": 1, "source_sequence": sequence, "event_type": "RECONCILE", "polling_reconcile": true,
			"positions": []any{}, "pending_orders": []any{},
		})
		if err != nil {
			return err
		}
		baseline, err := st.ComkunFollow().InsertBroadcast(sourceID, masterEquity, "AI策略执行", "[]", string(stateRaw))
		if err != nil {
			return err
		}
		for _, traderID := range traderIDs {
			if err := st.ComkunFollow().MarkConsumptionStartupBaseline(traderID, baseline.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func deterministicID(kind, userID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("comkun-hz-qa/"+kind+"/"+userID)).String()
}

func closeDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
