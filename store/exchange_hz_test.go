package store

import (
	"fmt"
	"nofx/crypto"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHZExchangeConfiguration(t *testing.T) {
	name, typ := getExchangeNameAndType("hz")
	if name != "BALIB 交易账户" || typ != "cex" {
		t.Fatalf("unexpected HZ display config: %q %q", name, typ)
	}

	exchange := Exchange{APIURL: "https://trade.kunai.fun/api/v1"}
	if exchange.APIURL == "" {
		t.Fatal("HZ API URL was not stored")
	}
	var _ crypto.EncryptedString = exchange.APIKey
	var _ crypto.EncryptedString = exchange.SecretKey
}

func TestHZScopeFingerprintsPersistWithoutExposingIdentifiers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Exchange{}); err != nil {
		t.Fatal(err)
	}
	row := &Exchange{ID: "exchange-1", UserID: "user-1", ExchangeType: "hz", AccountName: "AI", Name: "HZ", Type: "cex"}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	store := NewExchangeStore(db)
	if err := store.UpdateHZScopeFingerprints("user-1", "exchange-1", "account-hash", "wallet-hash", "book-hash"); err != nil {
		t.Fatal(err)
	}
	var saved Exchange
	if err := db.First(&saved, "id = ?", "exchange-1").Error; err != nil {
		t.Fatal(err)
	}
	if saved.HZAccountFingerprint != "account-hash" || saved.HZWalletFingerprint != "wallet-hash" ||
		saved.HZPositionBookFingerprint != "book-hash" {
		t.Fatalf("fingerprints not persisted: %+v", saved)
	}
}
