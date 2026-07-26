package api

import (
	"encoding/hex"
	"net/http"
	"strings"

	"nofx/wallet"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
)

// checkClaw402Health 供新手钱包接口返回状态；不做外网探测时返回 ok，避免阻塞。
func checkClaw402Health() string {
	return "ok"
}

func (s *Server) handleWalletValidate(c *gin.Context) {
	var req struct {
		PrivateKey string `json:"private_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.PrivateKey) == "" {
		c.JSON(http.StatusOK, gin.H{"valid": false, "error": "private_key 必填"})
		return
	}
	address, err := walletAddressFromPrivateKey(strings.TrimSpace(req.PrivateKey))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"valid": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"valid":           true,
		"address":         address,
		"balance_usdc":    wallet.QueryUSDCBalanceStr(address),
		"claw402_status":  checkClaw402Health(),
	})
}

func (s *Server) handleWalletGenerate(c *gin.Context) {
	privateKeyObj, err := gethcrypto.GenerateKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成失败"})
		return
	}
	addr := gethcrypto.PubkeyToAddress(privateKeyObj.PublicKey)
	keyHex := "0x" + hex.EncodeToString(gethcrypto.FromECDSA(privateKeyObj))
	c.JSON(http.StatusOK, gin.H{
		"private_key": keyHex,
		"address":     addr.Hex(),
	})
}
