package httpui

import (
	"context"
	"database/sql"
	"io"
	"log"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/discordwell/arkessro-evolved/internal/predict"
	"github.com/discordwell/arkessro-evolved/internal/store"
)

func TestImportReplayCSVAndBacktest(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	st := store.New(db)
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ui := New(Config{
		Store:           st,
		Predictor:       predict.New(predict.Config{}),
		Logger:          log.New(io.Discard, "", 0),
		StaticCacheBust: "test",
	})

	ev, err := st.CreateEvent(ctx, "Historical Replay", "")
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	csvData := strings.TrimSpace(`
line_item,category,quantity,unit,baseline,supplier,supplier_tags,supplier_risk,supplier_performance,round,unit_price,award
6061 Sheet,metals,1000,kg,12.50,Acme,"metals,aluminum",30,84,1,12.20,
6061 Sheet,metals,1000,kg,12.50,Bolt,"metals",45,77,1,12.40,
6061 Sheet,metals,1000,kg,12.50,Acme,"metals,aluminum",30,84,2,11.80,yes
6061 Sheet,metals,1000,kg,12.50,Bolt,"metals",45,77,2,12.00,
`) + "\n"

	result, err := ui.importReplayCSV(ctx, ev.ID, strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("import replay csv: %v", err)
	}
	if result.Rows != 4 {
		t.Fatalf("expected 4 rows, got %d", result.Rows)
	}
	if result.SuppliersCreated != 2 {
		t.Fatalf("expected 2 suppliers created, got %d", result.SuppliersCreated)
	}
	if result.LineItemsCreated != 1 {
		t.Fatalf("expected 1 line item created, got %d", result.LineItemsCreated)
	}
	if result.QuotesUpserted != 4 {
		t.Fatalf("expected 4 quotes upserted, got %d", result.QuotesUpserted)
	}
	if result.AwardsUpserted != 1 {
		t.Fatalf("expected 1 award upserted, got %d", result.AwardsUpserted)
	}

	items, suppliers, quotesByItem, awardsByItem, predByItem, err := ui.loadEventCopilotInputs(ctx, ev.ID)
	if err != nil {
		t.Fatalf("load copilot inputs: %v", err)
	}
	decisions, featuresByQuoteID, _, backtestRows, backtestSummary := ui.buildCopilotData(items, suppliers, quotesByItem, awardsByItem, predByItem)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if len(featuresByQuoteID) != 4 {
		t.Fatalf("expected 4 feature rows, got %d", len(featuresByQuoteID))
	}
	if len(backtestRows) == 0 {
		t.Fatalf("expected backtest rows")
	}
	if backtestSummary.Rows == 0 {
		t.Fatalf("expected non-empty backtest summary")
	}
}
