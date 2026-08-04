package hz

import "testing"

func TestCanonicalSigningPathForQAReverseProxy(t *testing.T) {
	tests := map[string]string{
		"/qa-api/v1/capabilities": "/api/v1/capabilities",
		"/api/v1/capabilities":    "/api/v1/capabilities",
		"/custom/v1/account":      "/custom/v1/account",
	}
	for input, want := range tests {
		if got := canonicalSigningPath(input); got != want {
			t.Fatalf("canonicalSigningPath(%q) = %q, want %q", input, got, want)
		}
	}
}
