package api

import (
	"testing"

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
			name:   "listing template subscription ignores old package price",
			access: store.MarketAccessSubscription,
			cfg: &store.StrategyConfig{
				MarketSalePriceUSDT:         599,
				ComkunFollowListingTemplate: true,
			},
			want: true,
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
