package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/gorm"

	"nofx/config"
	"nofx/crypto"
	"nofx/logger"
	"nofx/store"
)

type assignment struct {
	TraderName string `json:"trader_name"`
	UserID     string `json:"user_id"`
	ExchangeID string `json:"exchange_id"`
	Line       string `json:"line"`
}

func main() {
	_ = godotenv.Load()
	logger.Init(nil)
	config.Init()

	dryRun := flag.Bool("dry-run", false, "parse and validate only")
	flag.Parse()

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

	assignments, err := readAssignments(os.Stdin)
	if err != nil {
		log.Fatalf("read assignments: %v", err)
	}
	if len(assignments) == 0 {
		log.Fatalf("no assignments on stdin")
	}

	proxyStore := store.NewProxyPoolStore(gdb)
	for _, a := range assignments {
		a.UserID = strings.TrimSpace(a.UserID)
		a.ExchangeID = strings.TrimSpace(a.ExchangeID)
		a.Line = strings.TrimSpace(a.Line)
		if a.UserID == "" || a.ExchangeID == "" || a.Line == "" {
			log.Fatalf("assignment missing user_id/exchange_id/line: %+v", a)
		}
		proxyURL, displayHost, expiresAt, err := store.ParseProxyImportLine(a.Line)
		if err != nil {
			log.Fatalf("%s parse proxy: %v", a.TraderName, err)
		}
		poolID, err := ensurePoolRow(gdb, proxyURL, displayHost, expiresAt)
		if err != nil {
			log.Fatalf("%s ensure pool row: %v", a.TraderName, err)
		}
		if *dryRun {
			fmt.Printf("DRY %s exchange=%s host=%s pool=%s\n", a.TraderName, a.ExchangeID, displayHost, poolID)
			continue
		}
		if err := proxyStore.AssignToExchange(poolID, a.UserID, a.ExchangeID); err != nil {
			log.Fatalf("%s assign proxy: %v", a.TraderName, err)
		}
		fmt.Printf("OK %s exchange=%s host=%s pool=%s\n", a.TraderName, a.ExchangeID, displayHost, poolID)
	}
}

func readAssignments(f *os.File) ([]assignment, error) {
	var out []assignment
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var a assignment
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, sc.Err()
}

func ensurePoolRow(gdb *gorm.DB, proxyURL, displayHost string, expiresAt *time.Time) (string, error) {
	var rows []store.OutboundProxyPool
	if err := gdb.Order("created_at ASC").Find(&rows).Error; err != nil {
		return "", err
	}
	for _, row := range rows {
		if strings.TrimSpace(row.DisplayHost) == strings.TrimSpace(displayHost) &&
			strings.TrimSpace(string(row.ProxyURL)) == strings.TrimSpace(proxyURL) {
			if row.AssignedExchangeID != "" {
				return "", fmt.Errorf("proxy %s already assigned to exchange %s", displayHost, row.AssignedExchangeID)
			}
			return row.ID, nil
		}
	}

	now := time.Now().UTC()
	row := store.OutboundProxyPool{
		ID:          uuid.NewString(),
		DisplayHost: displayHost,
		ProxyURL:    crypto.EncryptedString(proxyURL),
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := gdb.Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return "", fmt.Errorf("duplicate proxy row")
		}
		return "", err
	}
	return row.ID, nil
}
