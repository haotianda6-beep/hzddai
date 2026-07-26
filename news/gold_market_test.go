package news

import (
	"strings"
	"testing"
	"time"
)

func floatPointer(value float64) *float64 { return &value }

func TestMarketChangeUsesRequestedWindows(t *testing.T) {
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	snapshots := []goldMarketSnapshot{
		{ObservedAt: now.Add(-15 * time.Minute), XAUPrice: 2000, DollarPrice: 100},
		{ObservedAt: now.Add(-5 * time.Minute), XAUPrice: 2010, DollarPrice: 99.5},
		{ObservedAt: now, XAUPrice: 2020, DollarPrice: 99},
	}
	latest := snapshots[len(snapshots)-1]
	xau15 := marketChange(snapshots, latest, 15*time.Minute, true)
	dollar5 := marketChange(snapshots, latest, 5*time.Minute, false)
	if xau15 == nil || *xau15 < 0.999 || *xau15 > 1.001 {
		t.Fatalf("unexpected XAU 15m change: %v", xau15)
	}
	if dollar5 == nil || *dollar5 > -0.49 || *dollar5 < -0.51 {
		t.Fatalf("unexpected dollar 5m change: %v", dollar5)
	}
	if marketChange(snapshots, latest, 60*time.Minute, true) != nil {
		t.Fatal("60m change should be unavailable without enough history")
	}
}

func TestGoldMarketConfirmation(t *testing.T) {
	impact := GoldImpact{
		Direction: "bullish", RiskScore: 60, RiskLevel: "medium",
		Confirmation: "pending", Reason: "事件利多。 等待同步行情确认。",
	}
	market := &GoldMarketStatus{
		Status: "live",
		XAU:    &GoldMarketQuote{Change15M: floatPointer(0.12)},
		Dollar: &GoldMarketQuote{Change15M: floatPointer(-0.04)},
		US10Y:  Treasury10YQuote{ChangeBP: floatPointer(-2.5)},
	}
	got := applyGoldMarketConfirmation(impact, market)
	if got.Confirmation != "confirmed" || got.RiskScore != 85 || got.RiskLevel != "extreme" {
		t.Fatalf("unexpected confirmation: %+v", got)
	}
	if !strings.Contains(got.Reason, "15分钟") || !strings.Contains(got.Reason, "-2.5bp") {
		t.Fatalf("missing market context: %s", got.Reason)
	}

	impact.Direction = "bearish"
	got = applyGoldMarketConfirmation(impact, market)
	if got.Confirmation != "divergent" {
		t.Fatalf("expected divergence, got %+v", got)
	}
}

func TestParseTreasury10Y(t *testing.T) {
	xmlData := `<feed xmlns:d="urn:data" xmlns:m="urn:meta">
<entry><content><m:properties><d:NEW_DATE>2026-07-14T00:00:00</d:NEW_DATE><d:BC_10YEAR>4.42</d:BC_10YEAR></m:properties></content></entry>
<entry><content><m:properties><d:NEW_DATE>2026-07-15T00:00:00</d:NEW_DATE><d:BC_10YEAR>4.39</d:BC_10YEAR></m:properties></content></entry>
</feed>`
	points, err := parseTreasury10Y(strings.NewReader(xmlData))
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 || points[1].date != "2026-07-15T00:00:00" || points[1].value != 4.39 {
		t.Fatalf("unexpected points: %+v", points)
	}
}

func TestRecordGoldMarketSnapshotRejectsInvalidQuote(t *testing.T) {
	err := RecordGoldMarketSnapshot(GoldMarketSnapshotInput{
		XAUBid: 2000, XAUAsk: 1999, DollarBid: 99, DollarAsk: 99.01,
	})
	if err == nil {
		t.Fatal("expected crossed XAU quote to be rejected")
	}
}
