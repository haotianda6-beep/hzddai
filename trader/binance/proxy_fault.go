package binance

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"

	"nofx/store"
)

// ProxyFaultMeta 经代理访问币安 REST 时的告警上下文（写入 HTTP Transport）
type ProxyFaultMeta struct {
	UserID      string
	TraderID    string
	ExchangeID  string
	ProxyURL    string
	DisplayHost string
}

var (
	proxyFaultRecorder   func(store.RecordFaultInput)
	proxyFaultRecorderMu sync.RWMutex
)

// SetProxyFaultRecorder 由 api 启动时注入（写 SQLite 管理告警）
func SetProxyFaultRecorder(fn func(store.RecordFaultInput)) {
	proxyFaultRecorderMu.Lock()
	defer proxyFaultRecorderMu.Unlock()
	proxyFaultRecorder = fn
}

func classifyProxyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "awaiting headers"):
		return "timeout"
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "connect: connection refused"):
		return "refused"
	case strings.Contains(msg, "no route to host"),
		strings.Contains(msg, "network is unreachable"),
		strings.Contains(msg, "connection reset"):
		return "refused"
	default:
		return "other"
	}
}

func recordProxyTransportError(meta *ProxyFaultMeta, err error) {
	if meta == nil || strings.TrimSpace(meta.ProxyURL) == "" || err == nil {
		return
	}
	errType := classifyProxyError(err)
	if errType == "" {
		return
	}
	in := store.RecordFaultInput{
		UserID:        meta.UserID,
		TraderID:      meta.TraderID,
		ExchangeID:    meta.ExchangeID,
		ProxyURL:      meta.ProxyURL,
		ProxyRedacted: store.RedactProxyURL(meta.ProxyURL),
		DisplayHost:   meta.DisplayHost,
		ErrorType:     errType,
		ErrorMessage:  err.Error(),
	}
	proxyFaultRecorderMu.RLock()
	fn := proxyFaultRecorder
	proxyFaultRecorderMu.RUnlock()
	if fn != nil {
		fn(in)
	}
}
