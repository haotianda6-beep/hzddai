package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMarketSubscriptionWaiverAndExpiry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:billing-subscription?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&StrategyMarketEntitlement{}); err != nil {
		t.Fatal(err)
	}
	billing := NewBillingStore(db)
	if err := billing.ExtendMarketSubscription(db, "buyer", "market-slot-1", 100, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if waived, err := billing.HasScanFeeWaiver("buyer", "market-slot-1"); err != nil || !waived {
		t.Fatalf("active subscription waiver=%v err=%v", waived, err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	if err := db.Model(&StrategyMarketEntitlement{}).
		Where("user_id = ? AND strategy_id = ?", "buyer", "market-slot-1").
		Update("subscription_until", past).Error; err != nil {
		t.Fatal(err)
	}
	if expired, err := billing.ComkunSourceSubscriptionExpired("buyer", "market-slot-1"); err != nil || !expired {
		t.Fatalf("expired subscription=%v err=%v", expired, err)
	}
}
