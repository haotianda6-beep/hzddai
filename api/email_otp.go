package api

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"mime"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"time"

	"nofx/logger"
)

const (
	emailOTPLength       = 6
	emailOTPTTL          = 10 * time.Minute
	emailOTPResendWindow = 60 * time.Second
	emailOTPMaxAttempts  = 8
)

type emailOTPRecord struct {
	code       string
	expiresAt  time.Time
	lastSentAt time.Time
	attempts   int
}

// 内存验证码（单实例部署足够；多副本需改为 Redis 等）
var emailOTPStore = struct {
	mu sync.Mutex
	m  map[string]*emailOTPRecord
}{m: make(map[string]*emailOTPRecord)}

func emailOTPKey(purpose, email string) string {
	return strings.ToLower(strings.TrimSpace(purpose)) + ":" + strings.ToLower(strings.TrimSpace(email))
}

func randomDigits6() (string, error) {
	n := big.NewInt(1000000)
	v, err := rand.Int(rand.Reader, n)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", v.Int64()), nil
}

func emailOTPCleanupLocked() {
	now := time.Now()
	for k, r := range emailOTPStore.m {
		if now.After(r.expiresAt) {
			delete(emailOTPStore.m, k)
		}
	}
}

func emailOTPSave(purpose, email, code string) {
	key := emailOTPKey(purpose, email)
	now := time.Now()
	emailOTPStore.mu.Lock()
	defer emailOTPStore.mu.Unlock()
	emailOTPCleanupLocked()
	emailOTPStore.m[key] = &emailOTPRecord{
		code:       code,
		expiresAt:  now.Add(emailOTPTTL),
		lastSentAt: now,
		attempts:   0,
	}
}

func emailOTPCheckAndConsume(purpose, email, input string) bool {
	key := emailOTPKey(purpose, email)
	want := strings.TrimSpace(input)
	if len(want) != emailOTPLength {
		return false
	}

	emailOTPStore.mu.Lock()
	defer emailOTPStore.mu.Unlock()
	rec, ok := emailOTPStore.m[key]
	if !ok || time.Now().After(rec.expiresAt) {
		return false
	}
	rec.attempts++
	if rec.attempts > emailOTPMaxAttempts {
		delete(emailOTPStore.m, key)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(want), []byte(rec.code)) != 1 {
		return false
	}
	delete(emailOTPStore.m, key)
	return true
}

func emailOTPResendAllowed(purpose, email string) (ok bool, retryAfter time.Duration) {
	key := emailOTPKey(purpose, email)
	emailOTPStore.mu.Lock()
	defer emailOTPStore.mu.Unlock()
	rec, ok := emailOTPStore.m[key]
	if !ok {
		return true, 0
	}
	now := time.Now()
	if now.After(rec.expiresAt) {
		return true, 0
	}
	since := now.Sub(rec.lastSentAt)
	if since < emailOTPResendWindow {
		return false, emailOTPResendWindow - since
	}
	return true, 0
}

// smtpGet 读取 SMTP 配置：优先 NOFX_SMTP_*，兼容简写 SMTP_*（与部分文档/习惯一致）
func smtpGet(nofxKey, shortKey string) string {
	v := strings.TrimSpace(os.Getenv(nofxKey))
	if v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv(shortKey))
}

func smtpBrand() string {
	b := smtpGet("NOFX_SMTP_BRAND", "SMTP_BRAND")
	if b == "" {
		return "COMKUN-AI"
	}
	return b
}

func smtpFromDisplayName() string {
	n := smtpGet("NOFX_SMTP_FROM_NAME", "SMTP_FROM_NAME")
	if n == "" {
		return smtpBrand()
	}
	return n
}

func formatFromHeader(displayName, addr string) string {
	displayName = strings.TrimSpace(displayName)
	addr = strings.TrimSpace(addr)
	if displayName == "" {
		return addr
	}
	esc := strings.ReplaceAll(displayName, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return fmt.Sprintf(`"%s" <%s>`, esc, addr)
}

func encodeMailSubject(s string) string {
	return mime.QEncoding.Encode("utf-8", s)
}

func buildMultipartMessage(fromHeader, toAddr, subjectEncoded, plainBody, htmlBody string) []byte {
	bnd := fmt.Sprintf("nf_%d", time.Now().UnixNano())
	var sb strings.Builder
	sb.WriteString("From: ")
	sb.WriteString(fromHeader)
	sb.WriteString("\r\nTo: ")
	sb.WriteString(toAddr)
	sb.WriteString("\r\nSubject: ")
	sb.WriteString(subjectEncoded)
	sb.WriteString("\r\nMIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: multipart/alternative; boundary=")
	sb.WriteString(bnd)
	sb.WriteString("\r\n\r\n--")
	sb.WriteString(bnd)
	sb.WriteString("\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	sb.WriteString(plainBody)
	sb.WriteString("\r\n--")
	sb.WriteString(bnd)
	sb.WriteString("\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	sb.WriteString(htmlBody)
	sb.WriteString("\r\n--")
	sb.WriteString(bnd)
	sb.WriteString("--\r\n")
	return []byte(sb.String())
}

func buildPlainMessage(fromHeader, toAddr, subjectEncoded, plainBody string) []byte {
	var sb strings.Builder
	sb.WriteString("From: ")
	sb.WriteString(fromHeader)
	sb.WriteString("\r\nTo: ")
	sb.WriteString(toAddr)
	sb.WriteString("\r\nSubject: ")
	sb.WriteString(subjectEncoded)
	sb.WriteString("\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	sb.WriteString(plainBody)
	sb.WriteString("\r\n")
	return []byte(sb.String())
}

// sendEmailSMTP 发送邮件：支持 HTML+纯文本 multipart；主题 subjectRaw 可为中文（自动 RFC2047 编码）。
// 发件显示名：NOFX_SMTP_FROM_NAME / SMTP_FROM_NAME（未设置则用 NOFX_SMTP_BRAND / SMTP_BRAND，默认 COMKUN-AI）。
func sendEmailSMTP(to, subjectRaw, plainBody, htmlBody string) (sent bool, err error) {
	host := smtpGet("NOFX_SMTP_HOST", "SMTP_HOST")
	port := smtpGet("NOFX_SMTP_PORT", "SMTP_PORT")
	user := smtpGet("NOFX_SMTP_USER", "SMTP_USER")
	pass := smtpGet("NOFX_SMTP_PASSWORD", "SMTP_PASSWORD")
	fromAddr := smtpGet("NOFX_SMTP_FROM", "SMTP_FROM")
	if port == "" {
		port = "587"
	}
	if host == "" || fromAddr == "" {
		logger.Warnf("未配置发信邮箱（需 NOFX_SMTP_HOST+NOFX_SMTP_FROM 或 SMTP_HOST+SMTP_FROM），验证码已生成但未发邮件。收件人=%s 主题=%s", to, subjectRaw)
		logger.Warnf("邮件正文：\n%s", plainBody)
		return false, nil
	}

	fromHeader := formatFromHeader(smtpFromDisplayName(), fromAddr)
	subjEnc := encodeMailSubject(subjectRaw)

	var msg []byte
	if strings.TrimSpace(htmlBody) != "" {
		msg = buildMultipartMessage(fromHeader, to, subjEnc, plainBody, htmlBody)
	} else {
		msg = buildPlainMessage(fromHeader, to, subjEnc, plainBody)
	}

	addr := host + ":" + port
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if err := smtp.SendMail(addr, auth, fromAddr, []string{to}, msg); err != nil {
		return false, err
	}
	return true, nil
}

func purposeRegister() string { return "register" }
func purposeReset() string    { return "reset_password" }
