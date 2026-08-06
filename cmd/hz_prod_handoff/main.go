package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nofx/auth"
	"nofx/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const productionDB = "/root/hzddai/data/data.db"

type credential struct {
	Account      string `json:"account"`
	APIKey       string `json:"apiKey"`
	APISecret    string `json:"apiSecret"`
	RatioPercent int    `json:"ratioPercent"`
}

type bundle struct {
	HZAPIURL  string       `json:"hzApiUrl"`
	Master    credential   `json:"master"`
	Followers []credential `json:"followers"`
}

type verifiedState struct {
	MasterAccountID string `json:"masterAccountId"`
	LastSequence    string `json:"lastSequence"`
	MasterEquity    string `json:"masterEquity"`
}

type followerState struct {
	Account      string `json:"account"`
	RatioPercent int    `json:"ratioPercent"`
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	ModelID      string `json:"modelId"`
	StrategyID   string `json:"strategyId"`
	ExchangeID   string `json:"exchangeId,omitempty"`
	TraderID     string `json:"traderId,omitempty"`
}

type handoffState struct {
	SourceID     string          `json:"sourceId"`
	MasterEquity float64         `json:"masterEquity"`
	Baseline     int64           `json:"baseline"`
	Followers    []followerState `json:"followers"`
}

func main() {
	if len(os.Args) < 2 {
		panic("usage: hz_prod_handoff prepare|baseline ...")
	}
	var err error
	switch os.Args[1] {
	case "prepare":
		if len(os.Args) != 7 {
			panic("usage: hz_prod_handoff prepare BUNDLE VERIFIED DB ENV STATE")
		}
		err = prepare(os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6])
	case "baseline":
		if len(os.Args) != 4 {
			panic("usage: hz_prod_handoff baseline STATE DB")
		}
		err = baseline(os.Args[2], os.Args[3])
	default:
		panic("unknown command")
	}
	if err != nil {
		panic(err)
	}
}

func prepare(bundlePath, verifiedPath, dbPath, envPath, statePath string) error {
	if filepath.Clean(dbPath) != productionDB {
		return fmt.Errorf("refusing unexpected database path")
	}
	var pkg bundle
	if err := readJSON(bundlePath, &pkg); err != nil {
		return err
	}
	if pkg.HZAPIURL != "https://direct-test.kunai.fun/api/v1" || len(pkg.Followers) != 3 {
		return fmt.Errorf("credential package shape mismatch")
	}
	var verified verifiedState
	if err := readJSON(verifiedPath, &verified); err != nil {
		return err
	}
	if strings.TrimSpace(verified.MasterAccountID) == "" || strings.TrimSpace(verified.LastSequence) != "0" {
		return fmt.Errorf("new master verification mismatch")
	}
	equity, err := strconv.ParseFloat(strings.TrimSpace(verified.MasterEquity), 64)
	if err != nil || equity <= 0 {
		return fmt.Errorf("invalid verified master equity")
	}
	db, err := store.InitGorm(dbPath)
	if err != nil {
		return err
	}
	defer closeDB(db)
	state, err := prepareRows(db, pkg, verified.MasterAccountID, equity)
	if err != nil {
		return err
	}
	if err := updateEnv(envPath, map[string]string{
		"HZ_MASTER_POLL_API_URL":          pkg.HZAPIURL,
		"HZ_MASTER_POLL_API_KEY":          pkg.Master.APIKey,
		"HZ_MASTER_POLL_API_SECRET":       pkg.Master.APISecret,
		"HZ_MASTER_POLL_ACCOUNT_ID":       verified.MasterAccountID,
		"HZ_MASTER_POLL_INTERVAL_SECONDS": "15",
	}); err != nil {
		return err
	}
	if err := writeJSON0600(statePath, state); err != nil {
		return err
	}
	fmt.Printf("prepared users=%d public_strategies=1 private_strategies=%d baseline=%d\n", len(state.Followers), len(state.Followers), state.Baseline)
	return nil
}

func prepareRows(db *gorm.DB, pkg bundle, masterAccountID string, equity float64) (*handoffState, error) {
	sourceID := store.HZMasterSourceStrategyID(masterAccountID)
	state := &handoffState{SourceID: sourceID, MasterEquity: equity, Baseline: 9}
	want := map[int]bool{20: true, 50: true, 100: true}
	return state, db.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&store.Strategy{}).Where("id = ?", sourceID).Count(&existing).Error; err != nil {
			return err
		}
		if existing != 0 {
			return fmt.Errorf("handoff source strategy already exists")
		}
		cfg := store.GetDefaultStrategyConfig("zh")
		cfg.ComkunFollowListingTemplate = true
		cfg.ComkunListingMasterSkipExchangeExecution = true
		raw, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Create(&store.Strategy{
			ID: sourceID, UserID: "hz-production-handoff", Name: "COMKUN-AI", Description: "HZAPP 手工跟单联调",
			IsActive: true, IsPublic: true, ConfigVisible: false, MarketAccess: store.MarketAccessSubscription,
			MarketAIModel: "comkun_ai", ContentLocked: true, Config: string(raw), CreatedAt: now, UpdatedAt: now,
		}).Error; err != nil {
			return err
		}
		for _, follower := range pkg.Followers {
			if !want[follower.RatioPercent] || follower.Account == "" || follower.APIKey == "" || follower.APISecret == "" {
				return fmt.Errorf("invalid follower credential")
			}
			delete(want, follower.RatioPercent)
			row, err := prepareFollower(tx, sourceID, follower, now)
			if err != nil {
				return err
			}
			state.Followers = append(state.Followers, row)
		}
		if len(want) != 0 {
			return fmt.Errorf("missing follower ratio")
		}
		return nil
	})
}

func prepareFollower(tx *gorm.DB, sourceID string, follower credential, now time.Time) (followerState, error) {
	seed := "comkun-hz-production/" + follower.Account
	userID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed+"/user")).String()
	modelID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed+"/model")).String()
	strategyID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed+"/strategy")).String()
	email := fmt.Sprintf("hz-handoff-%03d@local.invalid", follower.RatioPercent)
	password, err := randomSecret()
	if err != nil {
		return followerState{}, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return followerState{}, err
	}
	var existing int64
	if err := tx.Model(&store.User{}).Where("id = ? OR email = ?", userID, email).Count(&existing).Error; err != nil || existing != 0 {
		return followerState{}, fmt.Errorf("target user already exists")
	}
	if err := tx.Create(&store.User{
		ID: userID, Email: email, PasswordHash: hash, DisplayName: fmt.Sprintf("HZ 跟单 %d%%", follower.RatioPercent),
		AvatarURL: store.DefaultAvatarURL(userID), BalanceUSDT: 10000, InviteCode: store.NewUserStore(tx).GenerateInviteCode(),
		ProfileNamed: true, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		return followerState{}, err
	}
	if err := tx.Create(&store.AIModel{ID: modelID, UserID: userID, Name: "COMKUN-AI", Provider: "comkun_ai", Enabled: true}).Error; err != nil {
		return followerState{}, err
	}
	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.ComkunMarketFollow = true
	cfg.ComkunMarketSourceStrategyID = sourceID
	cfg.ComkunMirrorFollowerEquityRatio = float64(follower.RatioPercent) / 100
	raw, err := json.Marshal(cfg)
	if err != nil {
		return followerState{}, err
	}
	if err := tx.Create(&store.Strategy{
		ID: strategyID, UserID: userID, Name: "AI策略执行", Description: "HZAPP 手工跟单联调",
		IsActive: true, IsPublic: false, ConfigVisible: false, MarketAccess: store.MarketAccessOff,
		SourceStrategyID: sourceID, SourceMarketAccess: store.MarketAccessSubscription, ContentLocked: true,
		Config: string(raw), CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		return followerState{}, err
	}
	return followerState{Account: follower.Account, RatioPercent: follower.RatioPercent, UserID: userID, Email: email, Password: password, ModelID: modelID, StrategyID: strategyID}, nil
}

func baseline(statePath, dbPath string) error {
	if filepath.Clean(dbPath) != productionDB {
		return fmt.Errorf("refusing unexpected database path")
	}
	var state handoffState
	if err := readJSON(statePath, &state); err != nil {
		return err
	}
	if state.Baseline != 9 || len(state.Followers) != 3 {
		return fmt.Errorf("handoff state mismatch")
	}
	db, err := store.InitGorm(dbPath)
	if err != nil {
		return err
	}
	defer closeDB(db)
	err = db.Transaction(func(tx *gorm.DB) error {
		st, err := store.NewFromGorm(tx)
		if err != nil {
			return err
		}
		stateRaw, err := json.Marshal(map[string]any{
			"v": 1, "source_sequence": state.Baseline, "event_type": "RECONCILE", "startup_baseline": true,
			"positions": []any{}, "pending_orders": []any{},
		})
		if err != nil {
			return err
		}
		br, err := st.ComkunFollow().InsertBroadcast(state.SourceID, state.MasterEquity, "AI策略执行", "[]", string(stateRaw))
		if err != nil {
			return err
		}
		for _, follower := range state.Followers {
			if follower.TraderID == "" {
				return fmt.Errorf("missing trader id")
			}
			var trader store.Trader
			if err := tx.Where("id = ? AND user_id = ?", follower.TraderID, follower.UserID).First(&trader).Error; err != nil {
				return err
			}
			if trader.IsRunning {
				return fmt.Errorf("trader must remain stopped before baseline")
			}
			if err := st.ComkunFollow().MarkConsumptionStartupBaseline(follower.TraderID, br.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("baseline_sequence=%d traders=%d running=0\n", state.Baseline, len(state.Followers))
	return nil
}

func randomSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func readJSON(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	defer clear(raw)
	return json.Unmarshal(raw, dst)
}

func writeJSON0600(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer clear(raw)
	return write0600(path, append(raw, '\n'))
}

func updateEnv(path string, updates map[string]string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	defer clear(raw)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if ok && updates[key] != "" {
			lines[i], seen[key] = key+"="+updates[key], true
		}
	}
	for key, value := range updates {
		if !seen[key] {
			lines = append(lines, key+"="+value)
		}
	}
	return write0600(path, []byte(strings.Join(lines, "\n")+"\n"))
}

func write0600(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hz-handoff-*")
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

func closeDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
