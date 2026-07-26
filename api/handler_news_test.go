package api

import "testing"

func TestCanAccessNewsMonitor(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		{email: "haotianda6@gmail.com", want: true},
		{email: " HAOTIANDA6@gmail.com ", want: true},
		{email: "customer@example.com", want: false},
		{email: "", want: false},
	}

	for _, tt := range tests {
		if got := canAccessNewsMonitor(tt.email); got != tt.want {
			t.Fatalf("canAccessNewsMonitor(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}
