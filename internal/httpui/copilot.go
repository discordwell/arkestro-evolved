package httpui

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/discordwell/arkessro-evolved/internal/copilot"
	"github.com/discordwell/arkessro-evolved/internal/predict"
	"github.com/discordwell/arkessro-evolved/internal/store"
)

type BacktestRow struct {
	LineItemID         int64
	LineItemName       string
	DecisionRound      int
	Action             string
	Confidence         float64
	ExpectedCents      int64
	ActualOutcomeCents int64
	DeltaCents         int64 // actual - expected
}

type BacktestSummary struct {
	Rows                    int
	TotalExpectedCents      int64
	TotalActualOutcomeCents int64
	TotalDeltaCents         int64
}

type ReplayImportResult struct {
	Rows             int
	SuppliersCreated int
	SuppliersUpdated int
	LineItemsCreated int
	QuotesUpserted   int
	AwardsUpserted   int
}

func (u *UI) buildCopilotData(
	items []store.LineItem,
	suppliers []store.Supplier,
	quotesByItem map[int64][]store.Quote,
	awardsByItem map[int64]store.Award,
	predByItem map[int64]predict.Target,
) (map[int64]copilot.Decision, map[int64]copilot.QuoteFeature, map[int64][]QuoteFlag, []BacktestRow, BacktestSummary) {
	decisions := make(map[int64]copilot.Decision, len(items))
	featuresByQuoteID := make(map[int64]copilot.QuoteFeature)
	flagsByQuoteID := make(map[int64][]QuoteFlag)
	backtestRows := make([]BacktestRow, 0, len(items))
	summary := BacktestSummary{}

	supplierProfile := make(map[int64]copilot.Supplier, len(suppliers))
	for _, s := range suppliers {
		supplierProfile[s.ID] = copilot.Supplier{
			ID:               s.ID,
			Name:             s.Name,
			RiskScore:        s.RiskScore,
			PerformanceScore: s.PerformanceScore,
		}
	}

	for _, li := range items {
		itemQuotes := quotesByItem[li.ID]
		target := predByItem[li.ID]

		cctx := copilot.Context{
			LineItemID:      li.ID,
			LineItemName:    li.Name,
			BaselineCents:   li.BaselineCents,
			TargetCents:     target.TargetCents,
			WalkAwayCents:   target.WalkAwayCents,
			SupplierCount:   len(suppliers),
			Quotes:          toCopilotQuotes(itemQuotes),
			SupplierProfile: supplierProfile,
		}
		features, decision := copilot.Analyze(cctx)
		decisions[li.ID] = decision
		for _, f := range features {
			featuresByQuoteID[f.QuoteID] = f
			flagsByQuoteID[f.QuoteID] = quoteFlagsFromFeature(f)
		}

		holdoutRound := maxRound(itemQuotes) - 1
		if holdoutRound <= 0 {
			continue
		}
		btDecision, actual, ok := copilot.BacktestSnapshot(cctx, holdoutRound)
		if !ok {
			continue
		}
		if a, ok := awardsByItem[li.ID]; ok && a.UnitPriceCents > 0 {
			actual = a.UnitPriceCents
		}

		row := BacktestRow{
			LineItemID:         li.ID,
			LineItemName:       li.Name,
			DecisionRound:      holdoutRound,
			Action:             btDecision.Action,
			Confidence:         btDecision.Confidence,
			ExpectedCents:      btDecision.ExpectedOutcomeCents,
			ActualOutcomeCents: actual,
			DeltaCents:         actual - btDecision.ExpectedOutcomeCents,
		}
		backtestRows = append(backtestRows, row)
		summary.Rows++
		summary.TotalExpectedCents += row.ExpectedCents
		summary.TotalActualOutcomeCents += row.ActualOutcomeCents
		summary.TotalDeltaCents += row.DeltaCents
	}

	sort.Slice(backtestRows, func(i, j int) bool {
		if backtestRows[i].DeltaCents == backtestRows[j].DeltaCents {
			return backtestRows[i].LineItemID < backtestRows[j].LineItemID
		}
		return backtestRows[i].DeltaCents > backtestRows[j].DeltaCents
	})

	return decisions, featuresByQuoteID, flagsByQuoteID, backtestRows, summary
}

func toCopilotQuotes(quotes []store.Quote) []copilot.Quote {
	out := make([]copilot.Quote, 0, len(quotes))
	for _, q := range quotes {
		out = append(out, copilot.Quote{
			ID:             q.ID,
			SupplierID:     q.SupplierID,
			SupplierName:   q.SupplierName,
			Round:          q.Round,
			UnitPriceCents: q.UnitPriceCents,
		})
	}
	return out
}

func maxRound(quotes []store.Quote) int {
	maxR := 0
	for _, q := range quotes {
		if q.Round > maxR {
			maxR = q.Round
		}
	}
	return maxR
}

func quoteFlagsFromFeature(f copilot.QuoteFeature) []QuoteFlag {
	out := make([]QuoteFlag, 0, 4)
	add := func(class, text string) {
		for _, cur := range out {
			if cur.Text == text {
				return
			}
		}
		out = append(out, QuoteFlag{Class: class, Text: text})
	}

	if f.GapToWalkAway > 0 {
		add("danger", "above walk-away")
	}
	if f.GapToTarget > 0 {
		add("warn", "above target")
	} else {
		add("ok", "at/under target")
	}
	if f.OutlierHigh {
		add("warn", "high outlier")
	}
	if f.OutlierLow {
		add("warn", "low outlier")
	}
	if f.OutlierVolatile {
		add("warn", "supplier volatility high")
	}
	if f.ImprovementSlope > 0.02 {
		add("ok", "improving over prior round")
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func (u *UI) importReplayCSV(ctx context.Context, eventID int64, rdr io.Reader) (ReplayImportResult, error) {
	reader := csv.NewReader(rdr)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return ReplayImportResult{}, fmt.Errorf("read header: %w", err)
	}
	if len(header) == 0 {
		return ReplayImportResult{}, errors.New("csv header is empty")
	}

	col := make(map[string]int, len(header))
	for i, h := range header {
		key := normalizeHeader(h)
		if key != "" {
			col[key] = i
		}
	}

	findCol := func(keys ...string) int {
		for _, key := range keys {
			if idx, ok := col[normalizeHeader(key)]; ok {
				return idx
			}
		}
		return -1
	}

	lineItemCol := findCol("line_item", "item", "lineitem")
	supplierCol := findCol("supplier", "supplier_name")
	roundCol := findCol("round")
	unitPriceCol := findCol("unit_price", "price", "quote_price")
	if lineItemCol < 0 || supplierCol < 0 || roundCol < 0 || unitPriceCol < 0 {
		return ReplayImportResult{}, errors.New("csv requires columns: line_item, supplier, round, unit_price")
	}

	categoryCol := findCol("category")
	qtyCol := findCol("quantity", "qty")
	unitCol := findCol("unit")
	baselineCol := findCol("baseline", "baseline_price")
	targetCol := findCol("target", "target_price")
	supplierEmailCol := findCol("supplier_email", "email")
	supplierTagsCol := findCol("supplier_tags", "tags")
	supplierRiskCol := findCol("supplier_risk", "risk")
	supplierPerfCol := findCol("supplier_performance", "performance", "perf")
	awardCol := findCol("award", "is_awarded", "awarded")

	suppliers, err := u.st.ListSuppliers(ctx)
	if err != nil {
		return ReplayImportResult{}, err
	}
	supplierByName := make(map[string]store.Supplier, len(suppliers))
	for _, s := range suppliers {
		supplierByName[normalizeNameKey(s.Name)] = s
	}

	items, err := u.st.ListLineItemsByEvent(ctx, eventID)
	if err != nil {
		return ReplayImportResult{}, err
	}
	itemByName := make(map[string]store.LineItem, len(items))
	for _, li := range items {
		itemByName[normalizeNameKey(li.Name)] = li
	}

	result := ReplayImportResult{}
	rowNum := 1
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return result, fmt.Errorf("read row %d: %w", rowNum+1, err)
		}
		rowNum++
		result.Rows++

		lineItemName := strings.TrimSpace(cell(rec, lineItemCol))
		supplierName := strings.TrimSpace(cell(rec, supplierCol))
		if lineItemName == "" || supplierName == "" {
			return result, fmt.Errorf("row %d: line_item and supplier are required", rowNum)
		}

		round := parseIntDefault(cell(rec, roundCol), 1)
		if round <= 0 {
			round = 1
		}
		unitPriceCents, err := parseMoneyToCents(cell(rec, unitPriceCol))
		if err != nil || unitPriceCents <= 0 {
			return result, fmt.Errorf("row %d: invalid unit_price", rowNum)
		}

		supplierKey := normalizeNameKey(supplierName)
		sup, hasSupplier := supplierByName[supplierKey]
		if !hasSupplier {
			sEmail := strings.TrimSpace(cell(rec, supplierEmailCol))
			sTags := strings.TrimSpace(cell(rec, supplierTagsCol))
			sRisk := parseIntDefault(cell(rec, supplierRiskCol), 50)
			sPerf := parseIntDefault(cell(rec, supplierPerfCol), 50)
			created, err := u.st.CreateSupplier(ctx, supplierName, sEmail, sTags, sRisk, sPerf)
			if err != nil {
				return result, fmt.Errorf("row %d: create supplier: %w", rowNum, err)
			}
			sup = created
			supplierByName[supplierKey] = sup
			result.SuppliersCreated++
		} else {
			updated := false
			sTags := strings.TrimSpace(cell(rec, supplierTagsCol))
			sRisk := parseOptionalInt(cell(rec, supplierRiskCol), sup.RiskScore)
			sPerf := parseOptionalInt(cell(rec, supplierPerfCol), sup.PerformanceScore)
			if sTags != "" || sRisk != sup.RiskScore || sPerf != sup.PerformanceScore {
				tags := sup.Tags
				if sTags != "" {
					tags = sTags
				}
				if err := u.st.UpdateSupplierProfile(ctx, sup.ID, tags, sRisk, sPerf); err != nil {
					return result, fmt.Errorf("row %d: update supplier: %w", rowNum, err)
				}
				sup.Tags = tags
				sup.RiskScore = sRisk
				sup.PerformanceScore = sPerf
				supplierByName[supplierKey] = sup
				updated = true
			}
			if updated {
				result.SuppliersUpdated++
			}
		}

		itemKey := normalizeNameKey(lineItemName)
		li, hasItem := itemByName[itemKey]
		if !hasItem {
			category := strings.TrimSpace(cell(rec, categoryCol))
			qty := parseOptionalFloat(cell(rec, qtyCol), 1)
			if qty <= 0 {
				qty = 1
			}
			unit := strings.TrimSpace(cell(rec, unitCol))
			baselineCents, err := parseMoneyToCents(cell(rec, baselineCol))
			if err != nil {
				return result, fmt.Errorf("row %d: invalid baseline", rowNum)
			}
			baselineSource := "manual"
			if baselineCents == 0 {
				baselineCents = u.pred.ModelBaseline(lineItemName, category, qty)
				baselineSource = "modeled"
			}
			targetCents, err := parseMoneyToCents(cell(rec, targetCol))
			if err != nil {
				return result, fmt.Errorf("row %d: invalid target", rowNum)
			}
			if targetCents == 0 {
				p := u.pred.Predict(baselineCents, qty, len(supplierByName))
				targetCents = p.TargetCents
			}

			created, err := u.st.CreateLineItem(ctx, eventID, lineItemName, category, qty, unit, baselineCents, baselineSource, targetCents, "USD")
			if err != nil {
				return result, fmt.Errorf("row %d: create line item: %w", rowNum, err)
			}
			li = created
			itemByName[itemKey] = li
			result.LineItemsCreated++
		}

		if _, err := u.st.UpsertQuote(ctx, li.ID, sup.ID, round, unitPriceCents); err != nil {
			return result, fmt.Errorf("row %d: upsert quote: %w", rowNum, err)
		}
		result.QuotesUpserted++

		if parseLooseBool(cell(rec, awardCol)) {
			if err := u.st.UpsertAward(ctx, li.ID, sup.ID, unitPriceCents); err != nil {
				return result, fmt.Errorf("row %d: upsert award: %w", rowNum, err)
			}
			result.AwardsUpserted++
		}
	}

	if result.AwardsUpserted > 0 {
		_ = u.st.UpdateEventStatus(ctx, eventID, "awarded")
	} else if result.QuotesUpserted > 0 {
		_ = u.st.UpdateEventStatus(ctx, eventID, "active")
	}
	return result, nil
}

func normalizeHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return s
}

func normalizeNameKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func cell(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

func parseOptionalInt(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return parseIntDefault(s, def)
}

func parseOptionalFloat(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func parseLooseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "y", "yes", "true", "t", "award", "awarded":
		return true
	default:
		return false
	}
}
