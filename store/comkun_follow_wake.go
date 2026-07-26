package store

import (
	"strings"
	"sync"
)

// 同一 nofx 进程内：主控 InsertBroadcast 成功后唤醒所有「跟该 source_strategy_id」的被控，
// 不必干等 COMKUN_FOLLOW_POLL_INTERVAL_SEC。多实例/多容器部署时无效，轮询仍兜底。
var comkunFollowWakeHub = struct {
	mu   sync.RWMutex
	subs map[string][]chan struct{}
}{
	subs: make(map[string][]chan struct{}),
}

// RegisterComkunFollowWake 跟单被控在 Run 开始时注册；wake 建议 make(chan struct{}, 1)。
// 返回 unregister 须在交易员 Run 退出时 defer 调用，避免泄漏。
func RegisterComkunFollowWake(sourceStrategyID string, wake chan struct{}) (unregister func()) {
	sid := strings.TrimSpace(sourceStrategyID)
	if sid == "" || wake == nil {
		return func() {}
	}
	comkunFollowWakeHub.mu.Lock()
	comkunFollowWakeHub.subs[sid] = append(comkunFollowWakeHub.subs[sid], wake)
	comkunFollowWakeHub.mu.Unlock()
	return func() { unregisterComkunFollowWake(sid, wake) }
}

func unregisterComkunFollowWake(sourceStrategyID string, wake chan struct{}) {
	comkunFollowWakeHub.mu.Lock()
	defer comkunFollowWakeHub.mu.Unlock()
	chs := comkunFollowWakeHub.subs[sourceStrategyID]
	if len(chs) == 0 {
		return
	}
	out := chs[:0]
	for _, c := range chs {
		if c != wake {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		delete(comkunFollowWakeHub.subs, sourceStrategyID)
	} else {
		comkunFollowWakeHub.subs[sourceStrategyID] = out
	}
}

// NotifyComkunFollowersOfBroadcast 在 comkun 主广播落库成功后调用（非阻塞）。
func NotifyComkunFollowersOfBroadcast(sourceStrategyID string) {
	sid := strings.TrimSpace(sourceStrategyID)
	if sid == "" {
		return
	}
	comkunFollowWakeHub.mu.RLock()
	chs := comkunFollowWakeHub.subs[sid]
	chCopy := append([]chan struct{}(nil), chs...)
	comkunFollowWakeHub.mu.RUnlock()
	for _, ch := range chCopy {
		if ch == nil {
			continue
		}
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
