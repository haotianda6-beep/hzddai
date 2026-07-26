package news

import (
	"testing"
	"time"
)

func TestAnalyzeGoldImpact(t *testing.T) {
	recent := time.Now().UTC().Format(time.RFC3339)
	tests := []struct {
		name        string
		title       string
		wantDir     string
		minScore    int
		maxScore    int
		wantState   string
		wantHorizon string
	}{
		{
			name:      "dovish surprise is bullish",
			title:     "美联储意外降息，美元走弱且美债收益率下降",
			wantDir:   "bullish",
			minScore:  60,
			maxScore:  75,
			wantState: "pending",
		},
		{
			name:      "hawkish surprise is bearish",
			title:     "美联储鹰派表态推动美元走强，美债收益率上升",
			wantDir:   "bearish",
			minScore:  50,
			maxScore:  75,
			wantState: "pending",
		},
		{
			name:      "unrelated crypto news stays neutral",
			title:     "某交易所上线新的加密货币交易对",
			wantDir:   "neutral",
			minScore:  0,
			maxScore:  0,
			wantState: "not_required",
		},
		{
			name:        "long term gold bull outlook",
			title:       "机构预测黄金明年重返牛市",
			wantDir:     "bullish",
			minScore:    50,
			maxScore:    75,
			wantState:   "pending",
			wantHorizon: "1–12个月",
		},
		{
			name:        "oil spike is a gold macro driver",
			title:       "霍尔木兹紧张推升供应风险，原油大涨带动通胀预期升温",
			wantDir:     "bearish",
			minScore:    45,
			maxScore:    75,
			wantState:   "pending",
			wantHorizon: "数小时–3日",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzeGoldImpact(tt.title, "", recent)
			if got.Direction != tt.wantDir {
				t.Fatalf("direction = %q, want %q", got.Direction, tt.wantDir)
			}
			if got.RiskScore < tt.minScore || got.RiskScore > tt.maxScore {
				t.Fatalf("score = %d, want %d..%d", got.RiskScore, tt.minScore, tt.maxScore)
			}
			if got.Confirmation != tt.wantState {
				t.Fatalf("confirmation = %q, want %q", got.Confirmation, tt.wantState)
			}
			if tt.wantHorizon != "" && got.Horizon != tt.wantHorizon {
				t.Fatalf("horizon = %q, want %q", got.Horizon, tt.wantHorizon)
			}
		})
	}
}
