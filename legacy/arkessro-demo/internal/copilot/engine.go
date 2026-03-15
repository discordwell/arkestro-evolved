package copilot

import (
	"math"
	"sort"
)

const (
	ActionAwardNow    = "award_now"
	ActionCounterAt   = "counter_at"
	ActionAddSupplier = "add_supplier"
)

type Supplier struct {
	ID               int64
	Name             string
	RiskScore        int
	PerformanceScore int
}

type Quote struct {
	ID             int64
	SupplierID     int64
	SupplierName   string
	Round          int
	UnitPriceCents int64
}

type Context struct {
	LineItemID      int64
	LineItemName    string
	BaselineCents   int64
	TargetCents     int64
	WalkAwayCents   int64
	SupplierCount   int
	Quotes          []Quote
	SupplierProfile map[int64]Supplier
}

type QuoteFeature struct {
	QuoteID             int64
	SupplierID          int64
	SupplierName        string
	Round               int
	UnitPriceCents      int64
	GapToTarget         float64 // (quote-target)/target
	GapToWalkAway       float64 // (quote-walkaway)/walkaway
	ImprovementSlope    float64 // (prev-current)/prev, supplier-local
	SupplierRisk        int
	SupplierPerformance int
	Volatility          float64 // supplier stddev/mean across rounds
	OutlierHigh         bool
	OutlierLow          bool
	OutlierVolatile     bool
}

type Decision struct {
	Action               string
	CounterAtCents       int64
	Confidence           float64
	ReasonCodes          []string
	ExpectedOutcomeCents int64
	ScoreAwardNow        float64
	ScoreCounterAt       float64
	ScoreAddSupplier     float64
}

func Analyze(ctx Context) ([]QuoteFeature, Decision) {
	features := extractFeatures(ctx)
	decision := decide(ctx, features)
	return features, decision
}

func extractFeatures(ctx Context) []QuoteFeature {
	if len(ctx.Quotes) == 0 {
		return nil
	}

	quotes := make([]Quote, 0, len(ctx.Quotes))
	for _, q := range ctx.Quotes {
		if q.UnitPriceCents <= 0 {
			continue
		}
		quotes = append(quotes, q)
	}
	if len(quotes) == 0 {
		return nil
	}

	sort.Slice(quotes, func(i, j int) bool {
		if quotes[i].SupplierID == quotes[j].SupplierID {
			if quotes[i].Round == quotes[j].Round {
				return quotes[i].ID < quotes[j].ID
			}
			return quotes[i].Round < quotes[j].Round
		}
		if quotes[i].Round == quotes[j].Round {
			return quotes[i].SupplierID < quotes[j].SupplierID
		}
		return quotes[i].Round < quotes[j].Round
	})

	meanAll, stdAll := priceMeanStd(quotes)
	supplierVol := supplierVolatility(quotes)
	prevBySupplier := make(map[int64]int64)

	out := make([]QuoteFeature, 0, len(quotes))
	for _, q := range quotes {
		f := QuoteFeature{
			QuoteID:        q.ID,
			SupplierID:     q.SupplierID,
			SupplierName:   q.SupplierName,
			Round:          q.Round,
			UnitPriceCents: q.UnitPriceCents,
		}

		if ctx.TargetCents > 0 {
			f.GapToTarget = float64(q.UnitPriceCents-ctx.TargetCents) / float64(ctx.TargetCents)
		}
		if ctx.WalkAwayCents > 0 {
			f.GapToWalkAway = float64(q.UnitPriceCents-ctx.WalkAwayCents) / float64(ctx.WalkAwayCents)
		}

		if prev, ok := prevBySupplier[q.SupplierID]; ok && prev > 0 {
			f.ImprovementSlope = float64(prev-q.UnitPriceCents) / float64(prev)
		}
		prevBySupplier[q.SupplierID] = q.UnitPriceCents

		sup := ctx.SupplierProfile[q.SupplierID]
		f.SupplierRisk = clampInt(sup.RiskScore, 0, 100)
		f.SupplierPerformance = clampInt(sup.PerformanceScore, 0, 100)
		f.Volatility = supplierVol[q.SupplierID]

		if stdAll > 0 {
			high := meanAll + 1.5*stdAll
			low := meanAll - 1.5*stdAll
			f.OutlierHigh = float64(q.UnitPriceCents) > high
			f.OutlierLow = float64(q.UnitPriceCents) < low
		}
		f.OutlierVolatile = f.Volatility >= 0.10

		out = append(out, f)
	}

	return out
}

func decide(ctx Context, features []QuoteFeature) Decision {
	empty := Decision{
		Action:               ActionAddSupplier,
		Confidence:           0.35,
		ReasonCodes:          []string{"no quotes yet", "need supplier competition"},
		ScoreAwardNow:        0.05,
		ScoreCounterAt:       0.25,
		ScoreAddSupplier:     0.80,
		CounterAtCents:       max64(1, ctx.TargetCents),
		ExpectedOutcomeCents: max64(1, ctx.TargetCents),
	}

	if len(features) == 0 {
		if empty.ExpectedOutcomeCents <= 1 && ctx.BaselineCents > 0 {
			empty.ExpectedOutcomeCents = int64(math.Round(float64(ctx.BaselineCents) * 0.95))
		}
		return empty
	}

	bestPrice := int64(math.MaxInt64)
	bestSupplierID := int64(0)
	maxRound := 1
	uniqueSuppliers := map[int64]struct{}{}

	var (
		sumSlope      float64
		slopeN        float64
		sumVolatility float64
	)
	for _, f := range features {
		uniqueSuppliers[f.SupplierID] = struct{}{}
		if f.Round > maxRound {
			maxRound = f.Round
		}
		if f.UnitPriceCents < bestPrice {
			bestPrice = f.UnitPriceCents
			bestSupplierID = f.SupplierID
		}
		if f.ImprovementSlope != 0 {
			sumSlope += f.ImprovementSlope
			slopeN++
		}
		sumVolatility += f.Volatility
	}

	if bestPrice == int64(math.MaxInt64) {
		return empty
	}

	bestSup := ctx.SupplierProfile[bestSupplierID]
	competition := len(uniqueSuppliers)
	if ctx.SupplierCount > competition {
		competition = ctx.SupplierCount
	}

	trend := 0.0
	if slopeN > 0 {
		trend = sumSlope / slopeN
	}
	avgVolatility := sumVolatility / float64(len(features))

	target := ctx.TargetCents
	if target <= 0 {
		target = bestPrice
	}
	walk := ctx.WalkAwayCents
	if walk <= 0 {
		walk = int64(math.Round(float64(target) * 1.08))
	}

	gapToTarget := float64(bestPrice-target) / float64(max64(1, target))
	gapToWalk := float64(bestPrice-walk) / float64(max64(1, walk))
	riskPenalty := float64(clampInt(bestSup.RiskScore, 0, 100)) / 100.0
	perfScore := float64(clampInt(bestSup.PerformanceScore, 0, 100)) / 100.0
	compScore := clamp01(float64(competition) / 4.0)

	awardScore := 0.0
	awardScore += 0.42 * clamp01(1.0-max(0, gapToTarget)*1.5)
	awardScore += 0.18 * clamp01(1.0-max(0, gapToWalk)*1.4)
	awardScore += 0.14 * compScore
	awardScore += 0.14 * perfScore
	awardScore += 0.12 * clamp01(1.0-riskPenalty)

	counterScore := 0.0
	counterScore += 0.42 * clamp01(max(0, gapToTarget)*1.5+max(0, gapToWalk))
	counterScore += 0.24 * clamp01(trend*5.0+0.5)
	counterScore += 0.18 * compScore
	counterScore += 0.16 * clamp01(1.0-riskPenalty*0.7)

	addSupplierScore := 0.0
	addSupplierScore += 0.45 * clamp01((2.5-float64(competition))/2.5)
	addSupplierScore += 0.20 * clamp01(riskPenalty)
	addSupplierScore += 0.20 * clamp01(avgVolatility*4.0)
	addSupplierScore += 0.15 * clamp01(-trend*6.0+0.2)

	reasons := make([]string, 0, 5)
	if bestPrice <= target {
		reasons = append(reasons, "best quote is at or below target")
	}
	if bestPrice > walk {
		reasons = append(reasons, "best quote is above walk-away")
	}
	if trend > 0.02 {
		reasons = append(reasons, "quotes are still improving round-over-round")
	}
	if competition < 2 {
		reasons = append(reasons, "limited supplier competition")
	}
	if clampInt(bestSup.RiskScore, 0, 100) >= 70 {
		reasons = append(reasons, "current best quote comes from high-risk supplier")
	}
	if avgVolatility >= 0.08 {
		reasons = append(reasons, "quote volatility is elevated")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "balanced price, risk, and trend signals")
	}

	top := awardScore
	second := counterScore
	action := ActionAwardNow
	if counterScore > top {
		second = top
		top = counterScore
		action = ActionCounterAt
	}
	if addSupplierScore > top {
		second = top
		top = addSupplierScore
		action = ActionAddSupplier
	} else if addSupplierScore > second {
		second = addSupplierScore
	}

	counterAt := bestPrice
	if action == ActionCounterAt {
		if bestPrice > target {
			counterAt = int64(math.Round(float64(bestPrice) * 0.97))
			if counterAt < target {
				counterAt = target
			}
		} else {
			counterAt = int64(math.Round(float64(bestPrice) * 0.985))
		}
	}

	expected := bestPrice
	switch action {
	case ActionAwardNow:
		expected = bestPrice
	case ActionCounterAt:
		expected = min64(bestPrice, counterAt)
	case ActionAddSupplier:
		expected = min64(bestPrice, int64(math.Round(float64(bestPrice)*0.95)))
		if target > 0 {
			expected = min64(expected, target)
		}
	}
	if expected <= 0 {
		expected = bestPrice
	}

	coverage := clamp01(0.5*clamp01(float64(competition)/3.0) + 0.5*clamp01(float64(maxRound)/3.0))
	conf := clamp01(0.45 + 0.35*(top-second) + 0.20*coverage)

	return Decision{
		Action:               action,
		CounterAtCents:       max64(1, counterAt),
		Confidence:           conf,
		ReasonCodes:          dedupeStrings(reasons),
		ExpectedOutcomeCents: max64(1, expected),
		ScoreAwardNow:        clamp01(awardScore),
		ScoreCounterAt:       clamp01(counterScore),
		ScoreAddSupplier:     clamp01(addSupplierScore),
	}
}

func BacktestSnapshot(ctx Context, holdoutRound int) (Decision, int64, bool) {
	if holdoutRound <= 0 {
		return Decision{}, 0, false
	}

	var before []Quote
	actual := int64(math.MaxInt64)
	for _, q := range ctx.Quotes {
		if q.UnitPriceCents <= 0 {
			continue
		}
		if q.Round <= holdoutRound {
			before = append(before, q)
		} else if q.UnitPriceCents < actual {
			actual = q.UnitPriceCents
		}
	}
	if len(before) == 0 || actual == int64(math.MaxInt64) {
		return Decision{}, 0, false
	}

	decisionCtx := ctx
	decisionCtx.Quotes = before
	_, decision := Analyze(decisionCtx)
	return decision, actual, true
}

func supplierVolatility(quotes []Quote) map[int64]float64 {
	bySupplier := make(map[int64][]float64)
	for _, q := range quotes {
		if q.UnitPriceCents <= 0 {
			continue
		}
		bySupplier[q.SupplierID] = append(bySupplier[q.SupplierID], float64(q.UnitPriceCents))
	}

	out := make(map[int64]float64, len(bySupplier))
	for supplierID, values := range bySupplier {
		if len(values) < 2 {
			out[supplierID] = 0
			continue
		}
		mean, std := meanStd(values)
		if mean <= 0 {
			out[supplierID] = 0
			continue
		}
		out[supplierID] = std / mean
	}
	return out
}

func priceMeanStd(quotes []Quote) (float64, float64) {
	values := make([]float64, 0, len(quotes))
	for _, q := range quotes {
		if q.UnitPriceCents <= 0 {
			continue
		}
		values = append(values, float64(q.UnitPriceCents))
	}
	return meanStd(values)
}

func meanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	if len(values) == 1 {
		return mean, 0
	}
	vars := 0.0
	for _, v := range values {
		d := v - mean
		vars += d * d
	}
	std := math.Sqrt(vars / float64(len(values)))
	return mean, std
}

func dedupeStrings(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
