package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"nofx/config"
	"nofx/crypto"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

// CryptoHandler Encryption API handler
type CryptoHandler struct {
	cryptoService *crypto.CryptoService
}

// NewCryptoHandler Creates encryption handler
func NewCryptoHandler(cryptoService *crypto.CryptoService) *CryptoHandler {
	return &CryptoHandler{
		cryptoService: cryptoService,
	}
}

// ==================== Crypto Config Endpoint ====================

// HandleGetCryptoConfig Get crypto configuration
func (h *CryptoHandler) HandleGetCryptoConfig(c *gin.Context) {
	cfg := config.Get()
	c.JSON(http.StatusOK, gin.H{
		"transport_encryption": cfg.TransportEncryption,
	})
}

// ==================== Public Key Endpoint ====================

// HandleGetPublicKey Get server public key
func (h *CryptoHandler) HandleGetPublicKey(c *gin.Context) {
	cfg := config.Get()
	if !cfg.TransportEncryption {
		c.JSON(http.StatusOK, gin.H{
			"public_key":           "",
			"algorithm":            "",
			"transport_encryption": false,
		})
		return
	}

	publicKey := h.cryptoService.GetPublicKeyPEM()
	c.JSON(http.StatusOK, gin.H{
		"public_key":           publicKey,
		"algorithm":            "RSA-OAEP-2048",
		"transport_encryption": true,
	})
}

// ==================== Audit Log Query Endpoint ====================

// Audit log functionality removed, not needed in current simplified implementation

// ==================== Utility Functions ====================

// isValidPrivateKey Validate private key format
func isValidPrivateKey(key string) bool {
	// EVM private key: 64 hex characters (optional 0x prefix)
	if len(key) == 64 || (len(key) == 66 && key[:2] == "0x") {
		return true
	}
	// TODO: Add validation for other chains
	return false
}

// ==================== ED25519 Key Pair Generation Endpoint ====================

// HandleGenerateED25519KeyPair Generate ED25519 key pair for exchange API authentication
func (h *CryptoHandler) HandleGenerateED25519KeyPair(c *gin.Context) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Key generation failed: %v", err)})
		return
	}

	// Private key: full 64-byte ED25519 key (seed 32 + public 32), NOT just Seed()
	privateKeyBytes := []byte(priv)
	privateKeyHex := hex.EncodeToString(privateKeyBytes)
	privateKeyB64 := base64.StdEncoding.EncodeToString(privateKeyBytes)

	// Private key seed only (32 bytes) — useful for some tools
	privateKeySeedHex := hex.EncodeToString(priv.Seed())
	privateKeySeedB64 := base64.StdEncoding.EncodeToString(priv.Seed())

	// Public key: 32-byte raw public key
	publicKeyBytes := []byte(pub)
	publicKeyHex := hex.EncodeToString(publicKeyBytes)
	publicKeyB64 := base64.StdEncoding.EncodeToString(publicKeyBytes)

	// PKCS8 private key PEM (standard format accepted by exchanges)
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("PKCS8 marshaling failed: %v", err)})
		return
	}
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privDER,
	}))

	// PKIX / SPKI public key PEM (standard format accepted by exchanges)
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("PKIX marshaling failed: %v", err)})
		return
	}
	publicKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	}))

	// SSH format public key (also accepted by some exchanges)
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("SSH key conversion failed: %v", err)})
		return
	}
	sshPublicKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))

	c.JSON(http.StatusOK, gin.H{
		"private_key_hex":      privateKeyHex,
		"private_key_b64":      privateKeyB64,
		"private_key_seed_hex": privateKeySeedHex,
		"private_key_seed_b64": privateKeySeedB64,
		"public_key_hex":       publicKeyHex,
		"public_key_b64":       publicKeyB64,
		"private_key_pem":      privateKeyPEM,
		"public_key_pem":       publicKeyPEM,
		"ssh_public_key":       sshPublicKey,
	})
}
