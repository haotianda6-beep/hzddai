package crypto

import "testing"

func TestEncryptedStringFailsClosedWithoutStorageEncryption(t *testing.T) {
	previous := globalCryptoService
	globalCryptoService = nil
	t.Cleanup(func() { globalCryptoService = previous })
	if value, err := (EncryptedString("sensitive-value")).Value(); err == nil || value != nil {
		t.Fatalf("sensitive value must not fall back to plaintext: value=%v err=%v", value, err)
	}
}
