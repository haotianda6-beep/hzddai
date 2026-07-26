package store

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"
)

// RandomDisplayName 注册时分配的随机展示昵称（稳定可读，不含邮箱）
func RandomDisplayName() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "Trader_0000"
	}
	return "Vergex_" + hex.EncodeToString(b)
}

// DefaultAvatarURL 根据用户 ID 生成稳定头像地址（外部 SVG，浏览器直接显示）
func DefaultAvatarURL(userID string) string {
	return "https://api.dicebear.com/7.x/avataaars/svg?seed=" + url.QueryEscape(userID)
}

func RandomInviteCode() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "CK" + strings.ToUpper(hex.EncodeToString([]byte("000")))
	}
	return "CK" + strings.ToUpper(hex.EncodeToString(b))
}

func NormalizeInviteCode(code string) string {
	code = strings.TrimSpace(code)
	code = strings.TrimPrefix(code, "#")
	code = strings.ToUpper(code)
	var b strings.Builder
	for _, r := range code {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
