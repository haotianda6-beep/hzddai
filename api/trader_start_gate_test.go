package api

import (
	"sync"
	"testing"
)

func TestTraderStartGateAllowsOnlyOneConcurrentStart(t *testing.T) {
	gate := newTraderStartGate()
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	var attempted sync.WaitGroup
	var mu sync.Mutex
	start := make(chan struct{})
	release := make(chan struct{})
	accepted := 0
	ready.Add(8)
	attempted.Add(8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			acquired := gate.acquire("user", "trader")
			mu.Lock()
			if acquired {
				accepted++
			}
			mu.Unlock()
			attempted.Done()
			if acquired {
				<-release
				gate.release("user", "trader")
			}
		}()
	}
	ready.Wait()
	close(start)
	attempted.Wait()
	mu.Lock()
	got := accepted
	mu.Unlock()
	if got != 1 {
		t.Fatalf("concurrent starts accepted=%d, want 1", got)
	}
	close(release)
	wg.Wait()
	if !gate.acquire("user", "trader") {
		t.Fatal("gate must be reusable after the first start finishes")
	}
	gate.release("user", "trader")
}
