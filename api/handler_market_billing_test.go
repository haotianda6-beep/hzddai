package api

import (
	"testing"
	"time"

	"nofx/store"
)

func TestIsFreeComkunMarketSubscription(t *testing.T) {
	tests := []struct {
		name   string
		access string
		cfg    *store.StrategyConfig
		want   bool
	}{
		{
			name:   "free follower subscription",
			access: store.MarketAccessSubscription,
			cfg: &store.StrategyConfig{
				MarketSalePriceUSDT:          0,
				ComkunMarketFollow:           true,
				ComkunMarketSourceStrategyID: "source-strategy",
			},
			want: true,
		},
		{
			name:   "free listing template subscription",
			access: store.MarketAccessSubscription,
			cfg: &store.StrategyConfig{
				MarketSalePriceUSDT:         0,
				ComkunFollowListingTemplate: true,
			},
			want: true,
		},
		{
			name:   "priced listing template is paid",
			access: store.MarketAccessSubscription,
			cfg: &store.StrategyConfig{
				MarketSalePriceUSDT:         599,
				ComkunFollowListingTemplate: true,
			},
			want: false,
		},
		{
			name:   "ordinary zero price subscription is not free comkun",
			access: store.MarketAccessSubscription,
			cfg: &store.StrategyConfig{
				MarketSalePriceUSDT: 0,
			},
			want: false,
		},
		{
			name:   "private listing is not free comkun",
			access: store.MarketAccessPrivate,
			cfg: &store.StrategyConfig{
				MarketSalePriceUSDT:          0,
				ComkunMarketFollow:           true,
				ComkunMarketSourceStrategyID: "source-strategy",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFreeComkunMarketSubscription(tt.access, tt.cfg); got != tt.want {
				t.Fatalf("isFreeComkunMarketSubscription() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarketPlanPriceAndDurationUsesListingPriceAndRejectsWeekly(t *testing.T) {
	cfg := &store.StrategyConfig{MarketSalePriceUSDT: 500}
	price, duration, reason, ok := marketPlanPriceAndDuration("monthly", cfg)
	if !ok || price != 500 || duration != 30*24*time.Hour || reason != "market_subscription_monthly" {
		t.Fatalf("monthly plan mismatch: price=%v duration=%v reason=%s ok=%v", price, duration, reason, ok)
	}
	if _, _, _, ok := marketPlanPriceAndDuration("weekly", cfg); ok {
		t.Fatal("weekly subscriptions must be disabled for every listing")
	}
}
