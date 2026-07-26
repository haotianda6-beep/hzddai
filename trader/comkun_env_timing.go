package trader

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"nofx/logger"
)

// COMKUN 跟单相关节奏（可被环境变量覆盖）。
// 默认偏保守；当你为每个客户配置独立 REST 出口代理、分散币安按 IP 统计的压力时，可适当收紧间隔。
var (
	comkunFollowFollowMasterPollInterval = 5 * time.Second // 被控：轮询主控新广播
	comkunMasterSnapshotPollInterval      = 5 * time.Second // 主控：轻量快照检测持仓/挂单变化并发广播
	mirrorSafetyMarketCooldown           = 5 * time.Second // 被控：镜像市价操作最短间隔（防爆刷 / 非平仓）
	mirrorCloseCooldown                  = 2 * time.Second // 被控：仅「镜像市价全平」冷却，可单独收紧以便跟上主控平仓
	// mirrorCancelSymbolSleep 某币对 CancelAll 后稍候再下一币对，避免交易所状态未更新（过小可能偶发错单）
	mirrorCancelSymbolSleep = 80 * time.Millisecond
)

// mirrorSchemeAEnabled 方案A：新加入的被控在「水位线之后的首次实盘镜像」不跟主控快照挂止盈止损。
// 默认关闭：首轮即同步 TP/SL，避免跟单用户（含 SOL 专项）长时间无交易所止损保护。
// 若需旧行为，设置 COMKUN_MIRROR_SCHEME_A=1。
var mirrorSchemeAEnabled = false

// ---- v2 镜像跟单配置 ----

var (
	// v2WSRateMaxPerSec v2 WS API 每秒最大发送消息数（安全边际：Binance 限制 50，默认 40）
	v2WSRateMaxPerSec int = 40
	// v2WSPartialCloseEnabled 是否允许 v2 市价部分平仓（v1 禁止；v2 默认开启）
	v2WSPartialCloseEnabled bool = true
	// v2WSOrderCancelEnabled 是否优先用 WS API 撤单（默认优先 WS；回退到 REST CancelAllOrders）
	v2WSOrderCancelEnabled bool = true
	// v2WSLimitSleep 限价单之间最短间隔
	v2WSLimitSleep = 120 * time.Millisecond
	// v2WSTPSLSleep 止盈止损设置之间最短间隔
	v2WSTPSLSleep = 100 * time.Millisecond
)

// v2WSTokenBucket v2 WS API 简单令牌桶限速器。
type v2WSTokenBucket struct {
	mu     sync.Mutex
	rate   int // 每秒令牌数
	tokens float64
	last   time.Time
}

var v2WSBucket = &v2WSTokenBucket{rate: 40, tokens: 40}

func initV2WSBucket() {
	v2WSBucket.mu.Lock()
	defer v2WSBucket.mu.Unlock()
	v2WSBucket.rate = v2WSRateMaxPerSec
	v2WSBucket.tokens = float64(v2WSBucket.rate)
	v2WSBucket.last = time.Now()
}

func (b *v2WSTokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens += elapsed * float64(b.rate)
	if b.tokens > float64(b.rate) {
		b.tokens = float64(b.rate)
	}
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

func (b *v2WSTokenBucket) wait(ctx context.Context) error {
	for i := 0; i < 100; i++ {
		if b.allow() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1000/b.rate+5) * time.Millisecond):
		}
	}
	return fmt.Errorf("v2 WS rate limit wait timeout")
}

func parseV2WSEnv() {
	raw := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_V2_WS_RATE_MAX_PER_SEC"))
	if raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 10 || n > 50 {
			logger.Warnf("comkun v2: COMKUN_MIRROR_V2_WS_RATE_MAX_PER_SEC=%q 无效（需 10–50），保持 %d", raw, v2WSRateMaxPerSec)
		} else {
			v2WSRateMaxPerSec = n
			logger.Infof("comkun v2: COMKUN_MIRROR_V2_WS_RATE_MAX_PER_SEC=%d", n)
		}
	}

	raw2 := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_V2_PARTIAL_CLOSE"))
	if raw2 != "" {
		switch strings.ToLower(raw2) {
		case "0", "false", "no", "off":
			v2WSPartialCloseEnabled = false
			logger.Infof("comkun v2: COMKUN_MIRROR_V2_PARTIAL_CLOSE=%s -> 关闭部分平仓", raw2)
		default:
			v2WSPartialCloseEnabled = true
		}
	}

	raw3 := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_V2_CANCEL_WS"))
	if raw3 != "" {
		switch strings.ToLower(raw3) {
		case "0", "false", "no", "off":
			v2WSOrderCancelEnabled = false
			logger.Infof("comkun v2: COMKUN_MIRROR_V2_CANCEL_WS=%s -> 撤单回退 REST", raw3)
		default:
			v2WSOrderCancelEnabled = true
		}
	}

	raw4 := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_V2_LIMIT_SLEEP_MS"))
	if raw4 != "" {
		ms, err := strconv.Atoi(raw4)
		if err != nil || ms < 0 || ms > 1000 {
			logger.Warnf("comkun v2: COMKUN_MIRROR_V2_LIMIT_SLEEP_MS=%q 无效（需 0–1000），保持 %v", raw4, v2WSLimitSleep)
		} else {
			v2WSLimitSleep = time.Duration(ms) * time.Millisecond
		}
	}

	raw5 := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_V2_TPSL_SLEEP_MS"))
	if raw5 != "" {
		ms, err := strconv.Atoi(raw5)
		if err != nil || ms < 0 || ms > 1000 {
			logger.Warnf("comkun v2: COMKUN_MIRROR_V2_TPSL_SLEEP_MS=%q 无效（需 0–1000），保持 %v", raw5, v2WSTPSLSleep)
		} else {
			v2WSTPSLSleep = time.Duration(ms) * time.Millisecond
		}
	}
}

func init() {
	parseComkunDurationEnv("COMKUN_FOLLOW_POLL_INTERVAL_SEC", 1, 60, &comkunFollowFollowMasterPollInterval, "被控轮询主控广播")
	parseComkunDurationEnv("COMKUN_MASTER_SNAPSHOT_POLL_INTERVAL_SEC", 1, 60, &comkunMasterSnapshotPollInterval, "主控快照监控")
	parseComkunDurationEnv("COMKUN_MIRROR_MARKET_COOLDOWN_SEC", 2, 120, &mirrorSafetyMarketCooldown, "镜像市价冷却")
	parseMirrorCloseCooldownEnv()
	parseMirrorCancelSleepMs()
	parseMirrorSchemeAEnv()
	parseV2WSEnv()
	initV2WSBucket()
}

func parseMirrorCloseCooldownEnv() {
	raw := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_CLOSE_COOLDOWN_SEC"))
	if raw == "" {
		return
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < 0 || sec > 120 {
		logger.Warnf("comkun: COMKUN_MIRROR_CLOSE_COOLDOWN_SEC=%q 无效（需 0–120 整数秒），保持 %v [镜像市价全平冷却]", raw, mirrorCloseCooldown)
		return
	}
	mirrorCloseCooldown = time.Duration(sec) * time.Second
	logger.Infof("comkun: COMKUN_MIRROR_CLOSE_COOLDOWN_SEC=%d -> %v [镜像市价全平冷却]", sec, mirrorCloseCooldown)
}

func parseMirrorSchemeAEnv() {
	raw := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_SCHEME_A"))
	if raw == "" {
		return
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		mirrorSchemeAEnabled = true
		logger.Infof("comkun: COMKUN_MIRROR_SCHEME_A=%s -> 启用方案A（新被控首轮跳过 TP/SL）", raw)
	default:
		mirrorSchemeAEnabled = false
		logger.Infof("comkun: COMKUN_MIRROR_SCHEME_A=%s -> 关闭方案A（首轮即同步 TP/SL）", raw)
	}
}

func parseComkunDurationEnv(key string, minSec, maxSec int, target *time.Duration, label string) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < minSec || sec > maxSec {
		logger.Warnf("comkun: %s=%q 无效（需 %d–%d 整数秒），保持 %v [%s]", key, raw, minSec, maxSec, *target, label)
		return
	}
	*target = time.Duration(sec) * time.Second
	logger.Infof("comkun: %s=%d -> %v [%s]", key, sec, *target, label)
}

func parseMirrorCancelSleepMs() {
	raw := strings.TrimSpace(os.Getenv("COMKUN_MIRROR_CANCEL_SYMBOL_SLEEP_MS"))
	if raw == "" {
		return
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 || ms > 500 {
		logger.Warnf("comkun: COMKUN_MIRROR_CANCEL_SYMBOL_SLEEP_MS=%q 无效（需 0–500），保持 %v", raw, mirrorCancelSymbolSleep)
		return
	}
	mirrorCancelSymbolSleep = time.Duration(ms) * time.Millisecond
	logger.Infof("comkun: COMKUN_MIRROR_CANCEL_SYMBOL_SLEEP_MS=%d -> %v [限价同步撤单间隔]", ms, mirrorCancelSymbolSleep)
}
