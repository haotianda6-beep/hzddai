package api

import "sync"

type traderStartGate struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func newTraderStartGate() *traderStartGate {
	return &traderStartGate{active: make(map[string]struct{})}
}

func (g *traderStartGate) acquire(userID, traderID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active == nil {
		g.active = make(map[string]struct{})
	}
	key := userID + "\x00" + traderID
	if _, exists := g.active[key]; exists {
		return false
	}
	g.active[key] = struct{}{}
	return true
}

func (g *traderStartGate) release(userID, traderID string) {
	g.mu.Lock()
	delete(g.active, userID+"\x00"+traderID)
	g.mu.Unlock()
}
