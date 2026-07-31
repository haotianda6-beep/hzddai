package hz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type VerifiedCapabilities struct {
	Capabilities
	AccountFingerprint      string
	WalletFingerprint       string
	PositionBookFingerprint string
}

// VerifyCapabilities validates the server-owned permission boundary before an
// HZ credential can be enabled or used.
func VerifyCapabilities(apiURL, apiKey, secret string) (*VerifiedCapabilities, error) {
	client, err := newClient(apiURL, apiKey, secret)
	if err != nil {
		return nil, err
	}
	return verifyCapabilities(client)
}

func verifyCapabilities(client *client) (*VerifiedCapabilities, error) {
	var value Capabilities
	if err := client.do(context.Background(), http.MethodGet, "/capabilities", nil, "", &value); err != nil {
		return nil, fmt.Errorf("verify HZ capabilities: %w", err)
	}
	if !value.Read || !value.Trade || value.Withdraw || value.Transfer || value.Security ||
		!strings.EqualFold(strings.TrimSpace(value.AccountScope), "AI") {
		return nil, fmt.Errorf("HZ credential must be read+trade only and restricted to AI account scope")
	}
	if len(value.OrderTypes) != 1 || !strings.EqualFold(strings.TrimSpace(value.OrderTypes[0]), "MARKET") {
		return nil, fmt.Errorf("HZ AI scope must allow MARKET orders only")
	}
	if strings.TrimSpace(value.AccountID) == "" || strings.TrimSpace(value.WalletID) == "" ||
		strings.TrimSpace(value.PositionBookID) == "" {
		return nil, fmt.Errorf("HZ AI scope identifiers are incomplete")
	}
	return &VerifiedCapabilities{
		Capabilities:            value,
		AccountFingerprint:      fingerprint(value.AccountID),
		WalletFingerprint:       fingerprint(value.WalletID),
		PositionBookFingerprint: fingerprint(value.PositionBookID),
	}, nil
}

func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

// InstrumentNames returns the instrument allow-list supplied by B.
func (t *Trader) InstrumentNames() []string {
	values, err := t.instruments()
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func (t *Trader) ScopeFingerprints() (string, string, string) {
	return t.accountFingerprint, t.walletFingerprint, t.positionBookFingerprint
}
