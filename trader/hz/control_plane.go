package hz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	hzCapabilitiesCacheTTL = 30 * time.Second
	hzAccountCacheTTL      = 3 * time.Second
	hzInstrumentsCacheTTL  = 30 * time.Second
	hzControlMinInterval   = 25 * time.Millisecond
	hzMaxRetryAfter        = 2 * time.Second
)

type controlFlight struct {
	done chan struct{}
	err  error
}

type bindingState struct {
	mu              sync.Mutex
	inflight        map[string]*controlFlight
	nextRequestAt   time.Time
	cooldownUntil   time.Time
	capabilities    Capabilities
	capabilitiesAt  time.Time
	account         account
	accountAt       time.Time
	accountComplete bool
	instruments     []instrument
	instrumentsAt   time.Time
}

var bindingStates sync.Map // credential fingerprint -> *bindingState

func newBindingState(baseURL *url.URL, apiKey, secret string) *bindingState {
	sum := sha256.Sum256([]byte(baseURL.String() + "\x00" + apiKey + "\x00" + secret))
	key := hex.EncodeToString(sum[:])
	if value, ok := bindingStates.Load(key); ok {
		return value.(*bindingState)
	}
	state := &bindingState{inflight: make(map[string]*controlFlight)}
	actual, _ := bindingStates.LoadOrStore(key, state)
	return actual.(*bindingState)
}

func (s *bindingState) wait(ctx context.Context) error {
	for {
		now := time.Now()
		s.mu.Lock()
		until := s.nextRequestAt
		if s.cooldownUntil.After(until) {
			until = s.cooldownUntil
		}
		if !until.After(now) {
			s.nextRequestAt = now.Add(hzControlMinInterval)
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		timer := time.NewTimer(time.Until(until))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *bindingState) noteRateLimit(after time.Duration, specified bool) {
	if !specified {
		after = 100 * time.Millisecond
	}
	if after < 0 {
		after = 0
	}
	if after > hzMaxRetryAfter {
		after = hzMaxRetryAfter
	}
	s.mu.Lock()
	until := time.Now().Add(after)
	if until.After(s.cooldownUntil) {
		s.cooldownUntil = until
	}
	s.mu.Unlock()
}

func (c *client) getCapabilities(ctx context.Context) (Capabilities, error) {
	state := c.binding
	for {
		state.mu.Lock()
		if time.Since(state.capabilitiesAt) < hzCapabilitiesCacheTTL {
			value := cloneCapabilities(state.capabilities)
			state.mu.Unlock()
			return value, nil
		}
		if flight, ok := state.inflight["capabilities"]; ok {
			done := flight.done
			state.mu.Unlock()
			if err := waitControlFlight(ctx, done, flight); err != nil {
				return Capabilities{}, err
			}
			continue
		}
		flight := &controlFlight{done: make(chan struct{})}
		state.inflight["capabilities"] = flight
		state.mu.Unlock()

		var value Capabilities
		err := c.do(ctx, "GET", "/capabilities", nil, "", &value)
		state.mu.Lock()
		if err == nil {
			state.capabilities = cloneCapabilities(value)
			state.capabilitiesAt = time.Now()
		}
		flight.err = err
		delete(state.inflight, "capabilities")
		close(flight.done)
		state.mu.Unlock()
		return value, err
	}
}

func (c *client) getAccount(ctx context.Context, requireBalance bool) (account, error) {
	state := c.binding
	for {
		state.mu.Lock()
		if time.Since(state.accountAt) < hzAccountCacheTTL && (!requireBalance || state.accountComplete) {
			value := state.account
			state.mu.Unlock()
			return value, nil
		}
		if flight, ok := state.inflight["account"]; ok {
			done := flight.done
			state.mu.Unlock()
			if err := waitControlFlight(ctx, done, flight); err != nil {
				return account{}, err
			}
			continue
		}
		flight := &controlFlight{done: make(chan struct{})}
		state.inflight["account"] = flight
		state.mu.Unlock()

		var value account
		err := c.do(ctx, "GET", "/account", nil, "", &value)
		if err == nil {
			complete := accountHasBalanceFields(value)
			if requireBalance && !complete {
				err = fmt.Errorf("HZ account balance fields are missing")
			}
			state.mu.Lock()
			state.account = value
			state.accountAt = time.Now()
			state.accountComplete = complete
			state.mu.Unlock()
		}
		state.mu.Lock()
		flight.err = err
		delete(state.inflight, "account")
		close(flight.done)
		state.mu.Unlock()
		return value, err
	}
}

func (c *client) getInstruments(ctx context.Context) ([]instrument, error) {
	state := c.binding
	for {
		state.mu.Lock()
		if time.Since(state.instrumentsAt) < hzInstrumentsCacheTTL {
			values := append([]instrument(nil), state.instruments...)
			state.mu.Unlock()
			return values, nil
		}
		if flight, ok := state.inflight["instruments"]; ok {
			done := flight.done
			state.mu.Unlock()
			if err := waitControlFlight(ctx, done, flight); err != nil {
				return nil, err
			}
			continue
		}
		flight := &controlFlight{done: make(chan struct{})}
		state.inflight["instruments"] = flight
		state.mu.Unlock()

		var values []instrument
		err := c.do(ctx, "GET", "/instruments", nil, "", &values)
		state.mu.Lock()
		if err == nil {
			state.instruments = append([]instrument(nil), values...)
			state.instrumentsAt = time.Now()
		}
		flight.err = err
		delete(state.inflight, "instruments")
		close(flight.done)
		state.mu.Unlock()
		return values, err
	}
}

func waitControlFlight(ctx context.Context, done <-chan struct{}, flight *controlFlight) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return flight.err
	}
}

func cloneCapabilities(value Capabilities) Capabilities {
	value.OrderTypes = append([]string(nil), value.OrderTypes...)
	return value
}

func accountHasBalanceFields(value account) bool {
	for _, raw := range []string{value.Balance, value.Equity, value.AvailableMargin, value.UsedMargin, value.UnrealizedPnL} {
		if strings.TrimSpace(raw) == "" {
			return false
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return false
		}
	}
	return true
}

func parseRetryAfter(raw string, now time.Time) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	if at, err := http.ParseTime(raw); err == nil {
		return at.Sub(now), true
	}
	return 0, false
}
