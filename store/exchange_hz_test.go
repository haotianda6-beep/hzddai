package store

import (
	"nofx/crypto"
	"testing"
)

func TestHZExchangeConfiguration(t *testing.T) {
	name, typ := getExchangeNameAndType("hz")
	if name != "HZ 交易账户" || typ != "cex" {
		t.Fatalf("unexpected HZ display config: %q %q", name, typ)
	}

	exchange := Exchange{APIURL: "https://trade.kunai.fun/api/v1"}
	if exchange.APIURL == "" {
		t.Fatal("HZ API URL was not stored")
	}
	var _ crypto.EncryptedString = exchange.APIKey
	var _ crypto.EncryptedString = exchange.SecretKey
}
