package hz

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type APIError struct {
	Code    string
	Message string
	Status  int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HZ API %s: %s", e.Code, e.Message)
}

type client struct {
	baseURL *url.URL
	apiKey  string
	secret  string
	http    *http.Client
	now     func() time.Time
	nonce   func() string
	offset  atomic.Int64
}

func newClient(apiURL, apiKey, secret string) (*client, error) {
	baseURL, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid HZ API URL")
	}
	return &client{
		baseURL: baseURL,
		apiKey:  apiKey,
		secret:  secret,
		http:    &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
		nonce:   randomToken,
	}, nil
}

func (c *client) do(
	ctx context.Context,
	method, path string,
	query url.Values,
	idempotency string,
	out any,
) error {
	err := c.doOnce(ctx, method, path, query, idempotency, nil, out)
	apiErr, expired := err.(*APIError)
	if expired && apiErr.Code == "TIMESTAMP_EXPIRED" {
		if syncErr := c.syncTime(ctx); syncErr == nil {
			return c.doOnce(ctx, method, path, query, idempotency, nil, out)
		}
	}
	return err
}

func (c *client) doJSON(
	ctx context.Context,
	method, path string,
	body, out any,
	idempotency string,
) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	err = c.doOnce(ctx, method, path, nil, idempotency, raw, out)
	apiErr, expired := err.(*APIError)
	if expired && apiErr.Code == "TIMESTAMP_EXPIRED" {
		if syncErr := c.syncTime(ctx); syncErr == nil {
			return c.doOnce(ctx, method, path, nil, idempotency, raw, out)
		}
	}
	return err
}

func (c *client) doOnce(
	ctx context.Context,
	method, path string,
	query url.Values,
	idempotency string,
	body []byte,
	out any,
) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + "/" + strings.TrimLeft(path, "/")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	for name, value := range c.signedHeaders(method, endpoint.Path, endpoint.RawQuery, body, idempotency) {
		req.Header[name] = append([]string(nil), value...)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		var payload struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &payload)
		if payload.Error.Code == "" {
			payload.Error.Code = "HTTP_ERROR"
			payload.Error.Message = http.StatusText(response.StatusCode)
		}
		return &APIError{payload.Error.Code, payload.Error.Message, response.StatusCode}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode HZ response: %w", err)
	}
	return nil
}

func (c *client) signedHeaders(method, path, query string, body []byte, idempotency string) http.Header {
	timestamp := strconv.FormatInt(c.now().UnixMilli()+c.offset.Load(), 10)
	nonce := c.nonce()
	headers := http.Header{
		"X-HZ-API-KEY":   []string{c.apiKey},
		"X-HZ-TIMESTAMP": []string{timestamp},
		"X-HZ-NONCE":     []string{nonce},
		"X-HZ-SIGNATURE": []string{sign(c.secret, canonicalPayload(timestamp, nonce, method, path, query, body))},
	}
	if idempotency != "" {
		headers.Set("X-HZ-IDEMPOTENCY-KEY", idempotency)
	}
	return headers
}

func (c *client) syncTime(ctx context.Context) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + "/time"
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var result struct {
		UnixMilliseconds string `json:"unixMilliseconds"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return err
	}
	serverTime, err := strconv.ParseInt(result.UnixMilliseconds, 10, 64)
	if err != nil {
		return err
	}
	c.offset.Store(serverTime - c.now().UnixMilli())
	return nil
}

func randomToken() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(value)
}
