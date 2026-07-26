package trader

import (
	"strings"
	"testing"
)

func TestSanitizeComkunFollowDisplayAnalysisOnlyStripsPositionManagement(t *testing.T) {
	raw := strings.Join([]string{
		"1 市场结构",
		"BTC 震荡，SOL 空头继续观察。",
		"",
		"【风控与价位理解】",
		"止盈止损价位需要正常展示。",
		"",
		"5. 仓位管理评估",
		"这一段包含主控仓位管理，不应展示给跟单号。",
		"",
		"6 执行结论",
		"继续等待下一次主控信号。",
	}, "\n")

	got := sanitizeComkunFollowDisplayAnalysis(raw)
	if strings.Contains(got, "仓位管理评估") || strings.Contains(got, "不应展示") {
		t.Fatalf("expected position management section to be stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "风控与价位理解") || !strings.Contains(got, "止盈止损价位需要正常展示") {
		t.Fatalf("expected risk/price analysis section to be preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "6 执行结论") || !strings.Contains(got, "继续等待下一次主控信号") {
		t.Fatalf("expected later sections to be preserved, got:\n%s", got)
	}
}
