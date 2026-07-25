package hz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"nofx/logger"
)

type streamEnvelope struct {
	Type     string          `json:"type"`
	Sequence int64           `json:"sequence"`
	Data     json.RawMessage `json:"data"`
}

type systemStatus struct {
	MarketStatus      string `json:"marketStatus"`
	APIOpeningStopped bool   `json:"apiOpeningStopped"`
	Maintenance       bool   `json:"maintenance"`
}

func (t *Trader) runStream(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		connected, permanent, err := t.streamSession(ctx)
		t.streamReady.Store(false)
		if ctx.Err() != nil {
			return
		}
		if permanent {
			logger.Errorf("[HZ] real-time authentication failed; reconnect stopped: %v", err)
			return
		}
		if connected {
			delay = time.Second
		} else if delay < 30*time.Second {
			delay *= 2
		}
		logger.Warnf("[HZ] real-time connection lost; retrying in %s: %v", delay, err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (t *Trader) streamSession(ctx context.Context) (connected, permanent bool, err error) {
	endpoint := *t.client.baseURL
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/ws"
	headers := t.client.signedHeaders(http.MethodGet, endpoint.Path, "", nil, "")
	dialer := websocket.Dialer{HandshakeTimeout: 8 * time.Second}
	socket, response, err := dialer.DialContext(ctx, endpoint.String(), headers)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
			_ = response.Body.Close()
		}
		return false, status == http.StatusUnauthorized || status == http.StatusForbidden, err
	}
	defer socket.Close()
	if err := t.Reconcile(); err != nil {
		return true, false, fmt.Errorf("REST reconciliation failed: %w", err)
	}
	var previous int64
	for {
		var envelope streamEnvelope
		if err := socket.ReadJSON(&envelope); err != nil {
			return true, false, err
		}
		if envelope.Sequence <= 0 || (previous > 0 && envelope.Sequence != previous+1) {
			return true, false, fmt.Errorf("HZ stream sequence gap")
		}
		previous = envelope.Sequence
		if envelope.Type == "system.status" {
			var status systemStatus
			if err := json.Unmarshal(envelope.Data, &status); err != nil {
				return true, false, err
			}
			trading := status.MarketStatus == "" ||
				strings.EqualFold(status.MarketStatus, "open") ||
				strings.EqualFold(status.MarketStatus, "trading")
			t.openingStopped.Store(status.APIOpeningStopped || status.Maintenance || !trading)
		}
		t.streamReady.Store(true)
	}
}
