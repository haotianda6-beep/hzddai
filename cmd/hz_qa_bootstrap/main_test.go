package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"nofx/crypto"
	"nofx/store"
)

func TestBootstrapCreatesQAUsersAndPublicStrategyIdempotently(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv(crypto.EnvDataEncryptionKey, base64.StdEncoding.EncodeToString(key))
	t.Setenv(crypto.EnvRSAPrivateKey, testRSAPrivateKey(t))
	dbPath := filepath.Join(t.TempDir(), "hz-qa", "data.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	opts := bootstrapOptions{dbPath: dbPath, email: "admin@qa.invalid", password: "qa-password-12345"}
	if err := bootstrap(opts); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap(opts); err != nil {
		t.Fatal(err)
	}

	st, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var users, strategies int64
	if err := st.GormDB().Model(&store.User{}).Count(&users).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.GormDB().Model(&store.Strategy{}).Where("id = ? AND name = ? AND is_public = ?", qaStrategyID, "HZ 主控联动 QA", true).Count(&strategies).Error; err != nil {
		t.Fatal(err)
	}
	if users != 6 || strategies != 1 {
		t.Fatalf("users=%d public_strategies=%d", users, strategies)
	}
}

func testRSAPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}
