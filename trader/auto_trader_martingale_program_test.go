package trader

import (
	"math"
	"testing"

	"nofx/store"
)

func TestNormalizeMartingaleWeightsPadsToMaxLayers(t *testing.T) {
	w := normalizeMartingaleWeights([]float64{0.5, 0.5}, 7)
	if len(w) != 7 {
		t.Fatalf("want 7 layers, got %d", len(w))
	}
	sum := 0.0
	for _, v := range w {
		sum += v
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("weights should sum to 1, got %v", sum)
	}
}

func TestMartingaleLayerQtysUsesBudget(t *testing.T) {
	mp := &store.MartingaleProgramConfig{
		MaxLayers:       3,
		LayerWeights:    []float64{0.1, 0.3, 0.6},
		MarginBudgetPct: 0.1,
		Leverage:        20,
	}
	qtys := martingaleLayerQtys(100, 2000, mp) // budget=100 USDT margin total
	if len(qtys) != 3 {
		t.Fatalf("want 3 qtys, got %d", len(qtys))
	}
	// layer0 margin = 1000*0.1*0.1 = 10, qty = 10*20/2000 = 0.1
	if math.Abs(qtys[0]-0.1) > 1e-6 {
		t.Fatalf("layer0 qty want 0.1, got %v", qtys[0])
	}
}

func TestInferMartingaleLayer(t *testing.T) {
	cum := []float64{0.1, 0.3, 0.6}
	if inferMartingaleLayer(0.05, cum) != -1 {
		t.Fatal("below layer0")
	}
	if inferMartingaleLayer(0.1, cum) != 0 {
		t.Fatal("at layer0")
	}
	if inferMartingaleLayer(0.35, cum) != 1 {
		t.Fatal("at layer1")
	}
}
