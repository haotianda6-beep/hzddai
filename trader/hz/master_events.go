package hz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// FetchMasterSnapshot retrieves B's authoritative full AI-position snapshot.
// The caller validates and persists the raw response before fan-out.
func FetchMasterSnapshot(ctx context.Context, apiURL, apiKey, secret string) (json.RawMessage, error) {
	client, err := newClient(apiURL, apiKey, secret)
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := client.do(ctx, http.MethodGet, "/master-snapshot", nil, "", &raw); err != nil {
		return nil, fmt.Errorf("poll HZ master snapshot: %w", err)
	}
	return raw, nil
}
