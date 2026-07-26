package marketboard

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const (
	boardCacheTTL   = 45 * time.Second
	boardStaleMax   = 5 * time.Minute
	quickCacheTTL   = 20 * time.Second
)

type cachedBoard struct {
	payload *BoardPayload
	at      time.Time
}

var (
	fullCacheMu sync.RWMutex
	fullCache   cachedBoard

	quickCacheMu sync.RWMutex
	quickCache   cachedBoard

	refreshMu     sync.Mutex
	refreshing    bool
)

func clonePayload(p *BoardPayload) *BoardPayload {
	if p == nil {
		return nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return p
	}
	var out BoardPayload
	if err := json.Unmarshal(b, &out); err != nil {
		return p
	}
	return &out
}

func getFullCached(allowStale bool) (*BoardPayload, time.Duration) {
	fullCacheMu.RLock()
	defer fullCacheMu.RUnlock()
	if fullCache.payload == nil || fullCache.at.IsZero() {
		return nil, 0
	}
	age := time.Since(fullCache.at)
	if age <= boardCacheTTL {
		return clonePayload(fullCache.payload), age
	}
	if allowStale && age <= boardStaleMax {
		return clonePayload(fullCache.payload), age
	}
	return nil, age
}

func setFullCached(p *BoardPayload) {
	if p == nil {
		return
	}
	fullCacheMu.Lock()
	fullCache = cachedBoard{payload: clonePayload(p), at: time.Now()}
	fullCacheMu.Unlock()
}

func getQuickCached() *BoardPayload {
	quickCacheMu.RLock()
	defer quickCacheMu.RUnlock()
	if quickCache.payload == nil || quickCache.at.IsZero() {
		return nil
	}
	if time.Since(quickCache.at) > quickCacheTTL {
		return nil
	}
	return clonePayload(quickCache.payload)
}

func setQuickCached(p *BoardPayload) {
	if p == nil {
		return
	}
	quickCacheMu.Lock()
	quickCache = cachedBoard{payload: clonePayload(p), at: time.Now()}
	quickCacheMu.Unlock()
}

func refreshFullAsync() {
	refreshMu.Lock()
	if refreshing {
		refreshMu.Unlock()
		return
	}
	refreshing = true
	refreshMu.Unlock()

	go func() {
		defer func() {
			refreshMu.Lock()
			refreshing = false
			refreshMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		if p := buildBoardPayload(ctx, false); p != nil {
			setFullCached(p)
		}
	}()
}
