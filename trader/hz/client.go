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
	Code          string
	Message       string
	Status        int
	RetryAfter    time.Duration
	RetryAfterSet bool
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
	binding *bindingState
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
		binding: newBindingState(baseURL, apiKey, secret),
	}, nil
}

func (c *client) do(
	ctx context.Context,
	method, path string,
	query url.Values,
	idempotency string,
	out any,
) error {
	call := func() error { return c.doOnce(ctx, method, path, query, idempotency, nil, out) }
	err := retryIdempotent(ctx, idempotency, method == http.MethodGet || method == http.MethodHead, call)
	apiErr, expired := err.(*APIError)
	if expired && apiErr.Code == "TIMESTAMP_EXPIRED" {
		if syncErr := c.syncTime(ctx); syncErr == nil {
			return retryIdempotent(ctx, idempotency, method == http.MethodGet || method == http.MethodHead, call)
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
	call := func() error { return c.doOnce(ctx, method, path, nil, idempotency, raw, out) }
	err = retryIdempotent(ctx, idempotency, method == http.MethodGet || method == http.MethodHead, call)
	apiErr, expired := err.(*APIError)
	if expired && apiErr.Code == "TIMESTAMP_EXPIRED" {
		if syncErr := c.syncTime(ctx); syncErr == nil {
			return retryIdempotent(ctx, idempotency, method == http.MethodGet || method == http.MethodHead, call)
		}
	}
	return err
}

func retryIdempotent(ctx context.Context, idempotency string, retryGET bool, call func() error) error {
	err := call()
	for attempt := 0; attempt < 2; attempt++ {
		if !retryable(err, idempotency, retryGET) {
			return err
		}
		timer := time.NewTimer(retryDelay(err, attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		err = call()
	}
	return err
}

func retryable(err error, idempotency string, retryGET bool) bool {
	if err == nil {
		return false
	}
	if apiErr, ok := err.(*APIError); ok {
		if apiErr.Status == http.StatusTooManyRequests {
			return retryGET
		}
		return apiErr.Status >= http.StatusInternalServerError && (retryGET || idempotency != "")
	}
	return retryGET || idempotency != ""
}

func retryDelay(err error, attempt int) time.Duration {
	if apiErr, ok := err.(*APIError); ok && apiErr.RetryAfterSet {
		if apiErr.RetryAfter < 0 {
			return 0
		}
		if apiErr.RetryAfter > hzMaxRetryAfter {
			return hzMaxRetryAfter
		}
		return apiErr.RetryAfter
	}
	delay := 100 * time.Millisecond
	for i := 0; i < attempt; i++ {
		delay *= 2
	}
	if delay > time.Second {
		return time.Second
	}
	return delay
}

func (c *client) doOnce(
	ctx context.Context,
	method, path string,
	query url.Values,
	idempotency string,
	body []byte,
	out any,
) error {
	if c.binding != nil && path != "/time" {
		if err := c.binding.wait(ctx); err != nil {
			return err
		}
	}
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
		retryAfter, retryAfterSet := parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
		if response.StatusCode == http.StatusTooManyRequests && c.binding != nil {
			c.binding.noteRateLimit(retryAfter, retryAfterSet)
		}
		return &APIError{Code: payload.Error.Code, Message: payload.Error.Message, Status: response.StatusCode, RetryAfter: retryAfter, RetryAfterSet: retryAfterSet}
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
