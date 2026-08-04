package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nofx/auth"
	"nofx/crypto"
	"nofx/store"

	"github.com/google/uuid"
)

const (
	qaAdminID    = "hz-qa-admin"
	qaStrategyID = "hz-qa-linkage"
)

type bootstrapOptions struct {
	dbPath, email, password string
}

func optionsFromEnv() (bootstrapOptions, error) {
	opts := bootstrapOptions{
		dbPath:   strings.TrimSpace(os.Getenv("HZ_QA_DB_PATH")),
		email:    strings.ToLower(strings.TrimSpace(os.Getenv("HZ_QA_ADMIN_EMAIL"))),
		password: os.Getenv("HZ_QA_ADMIN_PASSWORD"),
	}
	if opts.dbPath == "" || opts.email == "" || len(opts.password) < 12 {
		return bootstrapOptions{}, fmt.Errorf("HZ_QA_DB_PATH, HZ_QA_ADMIN_EMAIL and a 12+ character HZ_QA_ADMIN_PASSWORD are required")
	}
	abs, err := filepath.Abs(opts.dbPath)
	if err != nil || !strings.Contains(strings.ToLower(abs), "hz-qa") {
		return bootstrapOptions{}, fmt.Errorf("refusing non-QA database path")
	}
	opts.dbPath = abs
	return opts, nil
}

func bootstrap(opts bootstrapOptions) error {
	cryptoService, err := crypto.NewCryptoService()
	if err != nil {
		return err
	}
	crypto.SetGlobalCryptoService(cryptoService)

	st, err := store.New(opts.dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	var admin store.User
	err = st.GormDB().Where("email = ?", opts.email).First(&admin).Error
	if err != nil {
		hash, hashErr := auth.HashPassword(opts.password)
		if hashErr != nil {
			return hashErr
		}
		admin = store.User{
			ID: qaAdminID, Email: opts.email, PasswordHash: hash,
			DisplayName: "HZ QA 管理员", ProfileNamed: true, BalanceUSDT: 10_000,
			AvatarURL: store.DefaultAvatarURL(qaAdminID),
		}
		if err := st.User().Create(&admin); err != nil {
			return err
		}
	}
	for _, ratio := range []int{20, 40, 60, 80, 100} {
		id := fmt.Sprintf("hz-qa-follower-%03d", ratio)
		var count int64
		if err := st.GormDB().Model(&store.User{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			continue
		}
		hash, err := auth.HashPassword("disabled-" + uuid.NewString())
		if err != nil {
			return err
		}
		if err := st.User().Create(&store.User{
			ID: id, Email: fmt.Sprintf("hz-qa-follower-%03d@comkun.invalid", ratio), PasswordHash: hash,
			DisplayName: fmt.Sprintf("HZ QA %d%%", ratio), ProfileNamed: true, BalanceUSDT: 10_000,
			AvatarURL: store.DefaultAvatarURL(id),
		}); err != nil {
			return err
		}
	}

	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.ComkunFollowListingTemplate = true
	cfg.ComkunListingMasterSkipExchangeExecution = true
	cfg.ComkunMarketFollow = false
	cfg.ComkunMarketSourceStrategyID = ""
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	strategy := store.Strategy{
		ID: qaStrategyID, UserID: admin.ID, Name: "HZ 主控联动 QA",
		Description: "隔离 QA：B AI 钱包主控事件通过现有 COMKUN-AI 镜像引擎驱动 HZ 跟单账户。",
		Config:      string(raw), IsActive: false, IsDefault: false,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	store.SyncListingFlagsFromAccess(&strategy, store.MarketAccessSubscription)
	return st.GormDB().Where("id = ?", qaStrategyID).Assign(map[string]any{
		"user_id": admin.ID, "name": strategy.Name, "description": strategy.Description,
		"config": strategy.Config, "market_access": strategy.MarketAccess,
		"is_public": strategy.IsPublic, "config_visible": strategy.ConfigVisible,
		"updated_at": time.Now().UTC(),
	}).FirstOrCreate(&strategy).Error
}

func main() {
	opts, err := optionsFromEnv()
	if err == nil {
		err = bootstrap(opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "HZ QA bootstrap failed:", err)
		os.Exit(1)
	}
	fmt.Printf("HZ QA bootstrap ready: admin_id=%s strategy_id=%s\n", qaAdminID, qaStrategyID)
}
