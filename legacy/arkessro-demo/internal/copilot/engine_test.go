package copilot_test

import (
	"testing"

	"github.com/discordwell/arkessro-evolved/internal/copilot"
)

func TestAnalyzeFeaturesAndDecision(t *testing.T) {
	ctx := copilot.Context{
		LineItemID:    1,
		LineItemName:  "Aluminum Sheet",
		BaselineCents: 1250,
		TargetCents:   1100,
		WalkAwayCents: 1225,
		SupplierCount: 3,
		SupplierProfile: map[int64]copilot.Supplier{
			11: {ID: 11, Name: "Acme", RiskScore: 25, PerformanceScore: 82},
			22: {ID: 22, Name: "Bolt", RiskScore: 60, PerformanceScore: 76},
			33: {ID: 33, Name: "Core", RiskScore: 40, PerformanceScore: 70},
		},
		Quotes: []copilot.Quote{
			{ID: 1, SupplierID: 11, SupplierName: "Acme", Round: 1, UnitPriceCents: 1210},
			{ID: 2, SupplierID: 11, SupplierName: "Acme", Round: 2, UnitPriceCents: 1160},
			{ID: 3, SupplierID: 22, SupplierName: "Bolt", Round: 1, UnitPriceCents: 1230},
			{ID: 4, SupplierID: 22, SupplierName: "Bolt", Round: 2, UnitPriceCents: 1185},
			{ID: 5, SupplierID: 33, SupplierName: "Core", Round: 1, UnitPriceCents: 1240},
		},
	}

	features, decision := copilot.Analyze(ctx)
	if len(features) != len(ctx.Quotes) {
		t.Fatalf("expected %d features, got %d", len(ctx.Quotes), len(features))
	}
	if decision.Action == "" {
		t.Fatalf("expected action")
	}
	if decision.Confidence <= 0 {
		t.Fatalf("expected confidence > 0")
	}
	if decision.ExpectedOutcomeCents <= 0 {
		t.Fatalf("expected expected outcome > 0")
	}

	foundSlope := false
	for _, f := range features {
		if f.SupplierID == 11 && f.Round == 2 {
			if f.ImprovementSlope <= 0 {
				t.Fatalf("expected positive improvement slope, got %.4f", f.ImprovementSlope)
			}
			foundSlope = true
		}
	}
	if !foundSlope {
		t.Fatalf("expected feature for supplier 11 round 2")
	}
}

func TestAnalyzeNoQuotesDefaultsToAddSupplier(t *testing.T) {
	ctx := copilot.Context{
		LineItemID:    1,
		LineItemName:  "Bearing",
		BaselineCents: 500,
		TargetCents:   450,
		WalkAwayCents: 490,
		SupplierCount: 1,
	}

	features, decision := copilot.Analyze(ctx)
	if len(features) != 0 {
		t.Fatalf("expected no features")
	}
	if decision.Action != copilot.ActionAddSupplier {
		t.Fatalf("expected %s, got %s", copilot.ActionAddSupplier, decision.Action)
	}
}

func TestBacktestSnapshot(t *testing.T) {
	ctx := copilot.Context{
		LineItemID:    1,
		LineItemName:  "Steel Coil",
		BaselineCents: 1000,
		TargetCents:   900,
		WalkAwayCents: 980,
		SupplierCount: 2,
		SupplierProfile: map[int64]copilot.Supplier{
			1: {ID: 1, Name: "A", RiskScore: 20, PerformanceScore: 80},
			2: {ID: 2, Name: "B", RiskScore: 40, PerformanceScore: 75},
		},
		Quotes: []copilot.Quote{
			{ID: 1, SupplierID: 1, SupplierName: "A", Round: 1, UnitPriceCents: 980},
			{ID: 2, SupplierID: 2, SupplierName: "B", Round: 1, UnitPriceCents: 970},
			{ID: 3, SupplierID: 1, SupplierName: "A", Round: 2, UnitPriceCents: 940},
			{ID: 4, SupplierID: 2, SupplierName: "B", Round: 2, UnitPriceCents: 930},
		},
	}

	decision, actual, ok := copilot.BacktestSnapshot(ctx, 1)
	if !ok {
		t.Fatalf("expected backtest to be possible")
	}
	if decision.Action == "" {
		t.Fatalf("expected backtest decision")
	}
	if actual != 930 {
		t.Fatalf("expected actual future best 930, got %d", actual)
	}
}
