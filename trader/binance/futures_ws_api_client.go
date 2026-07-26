package binance

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"nofx/logger"
)

// 币安 U 本位合约 WebSocket API（与 REST 功能等价，用于 v2 跟单降低下单路径延迟）。
const (
	futuresWSAPIMainnet = "wss://ws-fapi.binance.com/ws-fapi/v1"
	futuresWSAPITestnet = "wss://testnet.binancefuture.com/ws-fapi/v1"
)

// FuturesWSAPIClient 单连接 + session.logon 后复用；并发写串行、读单协程按 id 分发。
type FuturesWSAPIClient struct {
	apiKey    string
	secret    string
	endpoint  string
	conn      *websocket.Conn
	connMu    sync.Mutex
	pending   map[string]chan []byte
	pendingMu sync.Mutex
	writeMu   sync.Mutex
	closed    bool
	timeFn    func(context.Context) (int64, error)
}

// NewFuturesWSAPIClient endpoint 传 futuresWSAPIMainnet 或 futuresWSAPITestnet。
func NewFuturesWSAPIClient(apiKey, secret, endpoint string, timeFn func(context.Context) (int64, error)) *FuturesWSAPIClient {
	return &FuturesWSAPIClient{
		apiKey:   strings.TrimSpace(apiKey),
		secret:   strings.TrimSpace(secret),
		endpoint: endpoint,
		pending:  make(map[string]chan []byte),
		timeFn:   timeFn,
	}
}

// signFuturesWSAPIPayload auto-detects Ed25519 PEM vs HMAC SHA256 hex key.
func signFuturesWSAPIPayload(secret string, params map[string]string) string {
	if isEd25519PEM(secret) {
		return signEd25519(secret, params)
	}
	return signHMACSHA256(secret, params)
}

func isEd25519PEM(s string) bool {
	return strings.Contains(s, "-----BEGIN") && strings.Contains(s, "PRIVATE KEY-----")
}

// normalizePEM fixes single-line PEM (newlines replaced by spaces) back to proper multi-line format.
func normalizePEM(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "\n") || strings.Contains(raw, "\r") {
		return raw // already multi-line
	}
	raw = strings.ReplaceAll(raw, "-----BEGIN PRIVATE KEY----- ", "-----BEGIN PRIVATE KEY-----\n")
	raw = strings.ReplaceAll(raw, " -----END PRIVATE KEY-----", "\n-----END PRIVATE KEY-----")
	return raw
}

// signHMACSHA256 legacy HMAC SHA256 signing for REST-style API keys.
func signHMACSHA256(secret string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "signature" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(b.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// signEd25519 signs with Ed25519 private key (PEM PKCS8), outputs Base64.
func signEd25519(secretPEM string, params map[string]string) string {
	block, _ := pem.Decode([]byte(normalizePEM(secretPEM)))
	if block == nil {
		logger.Warnf("binance WS-API: Ed25519 PEM 解析失败，回退空签名")
		return ""
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		logger.Warnf("binance WS-API: Ed25519 PKCS8 解析失败: %v", err)
		return ""
	}
	edKey, ok := priv.(ed25519.PrivateKey)
	if !ok {
		logger.Warnf("binance WS-API: PEM 密钥不是 Ed25519 类型")
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "signature" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	sig := ed25519.Sign(edKey, []byte(b.String()))
	return base64.StdEncoding.EncodeToString(sig)
}

func flattenSignMap(in map[string]interface{}) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if k == "signature" {
			continue
		}
		switch t := v.(type) {
		case string:
			out[k] = t
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case int:
			out[k] = strconv.Itoa(t)
		case int64:
			out[k] = strconv.FormatInt(t, 10)
		case bool:
			out[k] = strconv.FormatBool(t)
		default:
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}

func (c *FuturesWSAPIClient) ensureConn(ctx context.Context) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil && !c.closed {
		return nil
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, c.endpoint, nil)
	if err != nil {
		return err
	}
	c.conn = conn
	c.closed = false
	go c.readLoop()
	return nil
}

func (c *FuturesWSAPIClient) readLoop() {
	defer func() {
		c.connMu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.closed = true
		c.connMu.Unlock()
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var top struct {
			ID     string          `json:"id"`
			Status int             `json:"status"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(msg, &top); err != nil {
			continue
		}
		c.pendingMu.Lock()
		ch, ok := c.pending[top.ID]
		if ok {
			delete(c.pending, top.ID)
		}
		c.pendingMu.Unlock()
		if ok && ch != nil {
			ch <- msg
		}
	}
}

func (c *FuturesWSAPIClient) sendRequest(ctx context.Context, id string, method string, params map[string]interface{}) ([]byte, error) {
	if err := c.ensureConn(ctx); err != nil {
		return nil, err
	}
	req := map[string]interface{}{
		"id":     id,
		"method": method,
		"params": params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
	c.writeMu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, body)
	c.writeMu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case data := <-ch:
		return data, nil
	}
}

func (c *FuturesWSAPIClient) Close() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil && !c.closed {
		c.closed = true
		return c.conn.Close()
	}
	return nil
}

func wsAPIParseStatus(raw []byte) (int, map[string]interface{}, error) {
	var top struct {
		Status int                    `json:"status"`
		Error  map[string]interface{} `json:"error"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return 0, nil, err
	}
	if top.Status != 200 {
		msg := fmt.Sprintf("ws api status=%d", top.Status)
		if em, ok := top.Error["msg"].(string); ok {
			msg += " msg=" + em
		}
		if ec, ok := top.Error["code"]; ok {
			msg += fmt.Sprintf(" code=%v", ec)
		}
		return top.Status, top.Error, fmt.Errorf("%s", msg)
	}
	return top.Status, top.Error, nil
}

// SessionLogon 建立已签名的会话，后续 order.place 可省略 apiKey/signature。
func (c *FuturesWSAPIClient) SessionLogon(ctx context.Context, id string) ([]byte, error) {
	ts, err := c.timeFn(ctx)
	if err != nil {
		return nil, err
	}
	pm := map[string]string{
		"apiKey":     c.apiKey,
		"timestamp":  strconv.FormatInt(ts, 10),
		"recvWindow": "5000",
	}
	sig := signFuturesWSAPIPayload(c.secret, pm)
	params := map[string]interface{}{
		"apiKey":     c.apiKey,
		"timestamp":  ts,
		"signature":  sig,
		"recvWindow": 5000,
	}
	return c.sendRequest(ctx, id, "session.logon", params)
}

func (c *FuturesWSAPIClient) SessionStatus(ctx context.Context, id string) ([]byte, error) {
	return c.sendRequest(ctx, id, "session.status", map[string]interface{}{})
}

// OrderPlace 通用 order.place；params 需含业务字段。
func (c *FuturesWSAPIClient) OrderPlace(ctx context.Context, id string, params map[string]interface{}) ([]byte, error) {
	ts, err := c.timeFn(ctx)
	if err != nil {
		return nil, err
	}
	p := make(map[string]interface{}, len(params)+6)
	for k, v := range params {
		p[k] = v
	}
	p["timestamp"] = ts
	p["recvWindow"] = 5000
	p["apiKey"] = c.apiKey
	flat := flattenSignMap(p)
	sig := signFuturesWSAPIPayload(c.secret, flat)
	p["signature"] = sig
	return c.sendRequest(ctx, id, "order.place", p)
}

// OrderCancel 通过 WS API 撤单。
func (c *FuturesWSAPIClient) OrderCancel(ctx context.Context, id string, params map[string]interface{}) ([]byte, error) {
	ts, err := c.timeFn(ctx)
	if err != nil {
		return nil, err
	}
	p := make(map[string]interface{}, len(params)+6)
	for k, v := range params {
		p[k] = v
	}
	p["timestamp"] = ts
	p["recvWindow"] = 5000
	p["apiKey"] = c.apiKey
	flat := flattenSignMap(p)
	sig := signFuturesWSAPIPayload(c.secret, flat)
	p["signature"] = sig
	return c.sendRequest(ctx, id, "order.cancel", p)
}

// AccountInfo 通过 WS API 查询账户信息。
func (c *FuturesWSAPIClient) AccountInfo(ctx context.Context, id string) ([]byte, error) {
	ts, err := c.timeFn(ctx)
	if err != nil {
		return nil, err
	}
	pm := map[string]string{
		"apiKey":     c.apiKey,
		"timestamp":  strconv.FormatInt(ts, 10),
		"recvWindow": "5000",
	}
	sig := signFuturesWSAPIPayload(c.secret, pm)
	params := map[string]interface{}{
		"apiKey":     c.apiKey,
		"timestamp":  ts,
		"signature":  sig,
		"recvWindow": 5000,
	}
	return c.sendRequest(ctx, id, "account.info", params)
}
