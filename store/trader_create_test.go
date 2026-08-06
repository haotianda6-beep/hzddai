package store

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTraderCreatePersistsIsolatedMargin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Trader{}); err != nil {
		t.Fatal(err)
	}
	trader := &Trader{ID: "isolated", UserID: "user", Name: "isolated", AIModelID: "model", ExchangeID: "exchange", IsCrossMargin: false}
	if err := NewTraderStore(db).Create(trader); err != nil {
		t.Fatal(err)
	}
	var saved Trader
	if err := db.First(&saved, "id = ?", trader.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.IsCrossMargin {
		t.Fatal("isolated trader was persisted as cross margin")
	}
}
