package api

import "testing"

func TestAllowsMultipleMarketSourceTradersForEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{name: "approved test account", email: "2497937010@qq.com", want: true},
		{name: "case insensitive match", email: "2497937010@QQ.COM", want: true},
		{name: "other account remains restricted", email: "other@example.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowsMultipleMarketSourceTradersForEmail(tt.email); got != tt.want {
				t.Fatalf("allowsMultipleMarketSourceTradersForEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}
