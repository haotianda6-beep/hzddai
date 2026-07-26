package kernel

import (
	"strings"
	"unicode/utf8"
)

// AppendComkunListingFollowerBriefInstructions 主控上架模板：要求 AI 单独输出「## 跟单端简报」供广播截取给订阅方。
func AppendComkunListingFollowerBriefInstructions(userPrompt string, hasOpenLimitOrders bool) string {
	var b strings.Builder
	b.WriteString(userPrompt)
	b.WriteString("\n\n【跟单端输出规范（主控上架模板）】\n")
	if hasOpenLimitOrders {
		b.WriteString("在完成常规行情分析后，必须另起一节，标题**严格**为「## 跟单端简报」（简体中文）。该节**整节正文**会推送给订阅用户，请遵守：\n")
		b.WriteString("- 只写：① 本轮行情、关键结构或波动区间判断；② **逐条**说明当前每个未成交限价单为何挂在该价位（与现价、支撑/阻力或区间的逻辑），并简述关联止盈/止损意图与主要风险；\n")
		b.WriteString("- 不要写：账户执行过程、API、镜像同步、执行日志、JSON 技术细节、对跟单者的命令式话术；\n")
		b.WriteString("- 全节建议不超过 800 字，语言精炼。\n")
	} else {
		b.WriteString("在思维链末尾追加一节，标题**严格**为「## 跟单端简报」（简体中文）。该节会推送给订阅用户：\n")
		b.WriteString("- 只写本轮行情与结构判断（约 200 字内），并明确一句「当前无未成交限价类挂单」；\n")
		b.WriteString("- 不要写执行镜像、日志、JSON 细节。\n")
	}
	return b.String()
}

// ExtractFollowerBroadcastCoT 从主控完整思维链中截取「## 跟单端简报」一节，供写入 comkun 广播 analysis 字段。
func ExtractFollowerBroadcastCoT(full string) string {
	full = strings.TrimSpace(full)
	if full == "" {
		return ""
	}
	markers := []string{"## 跟单端简报", "### 跟单端简报", "【跟单端简报】"}
	for _, m := range markers {
		idx := strings.Index(full, m)
		if idx < 0 {
			continue
		}
		start := idx + len(m)
		rest := strings.TrimSpace(full[start:])
		if rest == "" {
			return ""
		}
		// 若简报后又出现同级「## 」标题（非 ###），截断到该标题前，避免把后续无关章节带进去
		if cut := indexNextH2Heading(rest); cut >= 0 {
			rest = strings.TrimSpace(rest[:cut])
		}
		rest = StripPendingOrdersScanBlocks(rest)
		rest = stripFollowerBannedPhrases(rest)
		return truncateUTF8(rest, 12000)
	}
	return ""
}

// CoTForComkunMasterBroadcast 写入主控广播 analysis 的正文：优先「跟单端简报」，其次 reasoning/节选全文，
// 避免主控有思维链但截取规则未命中时整轮不插广播 → 被控永远等不到新 broadcast_id、界面无思维输出。
func CoTForComkunMasterBroadcast(recordCoT string) string {
	full := strings.TrimSpace(recordCoT)
	if full == "" {
		return ""
	}
	if x := strings.TrimSpace(ExtractFollowerBroadcastCoT(full)); x != "" {
		return x
	}
	if x := strings.TrimSpace(FallbackFollowerBriefFromCoT(full)); x != "" {
		return x
	}
	clean := strings.TrimSpace(stripFollowerBannedPhrases(StripPendingOrdersScanBlocks(full)))
	if clean == "" {
		return ""
	}
	return truncateUTF8(clean, 12000)
}

// FallbackFollowerBriefFromCoT 未找到「跟单端简报」标题时，从 <reasoning> 或全文截取一段给订阅方（避免整篇技术链）。
func FallbackFollowerBriefFromCoT(full string) string {
	full = strings.TrimSpace(full)
	if full == "" {
		return ""
	}
	if m := reReasoningTag.FindStringSubmatch(full); len(m) > 1 {
		inner := strings.TrimSpace(m[1])
		if inner == "" {
			return truncateUTF8(stripFollowerBannedPhrases(StripPendingOrdersScanBlocks(full)), 4000)
		}
		inner = StripPendingOrdersScanBlocks(inner)
		inner = stripFollowerBannedPhrases(inner)
		return truncateUTF8(inner, 4000)
	}
	return truncateUTF8(stripFollowerBannedPhrases(StripPendingOrdersScanBlocks(full)), 4000)
}

// StripPendingOrdersScanBlocks 移除「挂单扫描摘要」等技术明细块（避免进入跟单广播/被控思考过程后刷屏）。
func StripPendingOrdersScanBlocks(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for {
		changed := false
		if t := stripOneExchangeScanBlock(s, "【挂单扫描摘要】"); t != s {
			s, changed = t, true
		}
		if t := stripOneExchangeScanBlock(s, "[Pending limit orders scan]"); t != s {
			s, changed = t, true
		}
		if t := stripOneExchangeScanBlock(s, "【止盈止损条件单（交易所）】"); t != s {
			s, changed = t, true
		}
		if t := stripOneExchangeScanBlock(s, "[TP / SL conditional orders (exchange)]"); t != s {
			s, changed = t, true
		}
		if !changed {
			break
		}
	}
	return strings.TrimSpace(s)
}

func stripOneExchangeScanBlock(full, marker string) string {
	i := strings.Index(full, marker)
	if i < 0 {
		return full
	}
	rest := full[i+len(marker):]
	next := len(full)
	for _, sep := range []string{"\n\n【", "\n\n##", "\n\n---"} {
		if j := strings.Index(rest, sep); j >= 0 {
			cand := i + len(marker) + j
			if cand < next {
				next = cand
			}
		}
	}
	if next < len(full) {
		return strings.TrimSpace(full[:i] + full[next:])
	}
	return strings.TrimSpace(full[:i])
}

func indexNextH2Heading(s string) int {
	// 在简报正文内查找下一处「\n## 」（二级标题），跳过「\n###」三级标题
	from := 0
	for {
		idx := strings.Index(s[from:], "\n## ")
		if idx < 0 {
			return -1
		}
		abs := from + idx
		if strings.HasPrefix(s[abs:], "\n###") {
			from = abs + 2
			continue
		}
		return abs
	}
}

func stripFollowerBannedPhrases(s string) string {
	// 删除跟单端不需要的常见段落标题整段（从标题到下一空行或文末）
	cuts := []string{"【本账户镜像同步】", "## 本账户镜像同步", "【执行日志】"}
	out := s
	for _, c := range cuts {
		if i := strings.Index(out, c); i >= 0 {
			out = strings.TrimSpace(out[:i])
		}
	}
	return strings.TrimSpace(out)
}

func truncateUTF8(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "\n…"
	}
	return s
}
