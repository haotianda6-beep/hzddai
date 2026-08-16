package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSensitivePublicRoutesAreAbsent(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("server.go"))
	if err != nil {
		t.Fatalf("read server routes: %v", err)
	}

	for _, route := range []string{"/reset-account", "/crypto/decrypt"} {
		if strings.Contains(string(source), route) {
			t.Fatalf("sensitive route %q must not be publicly registered", route)
		}
	}
}
