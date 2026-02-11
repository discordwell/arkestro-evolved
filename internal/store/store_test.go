package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/discordwell/arkessro-evolved/internal/store"
)

func TestStoreCRUD(t *testing.T) {
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	st := store.New(db)
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	s1, err := st.CreateSupplier(ctx, "Acme", "", "", 50, 50)
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	if s1.ID == 0 {
		t.Fatalf("expected supplier id")
	}

	e1, err := st.CreateEvent(ctx, "Aluminum Q2", "")
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	li1, err := st.CreateLineItem(ctx, e1.ID, "6061 sheet", "metals", 1000, "kg", 1250, "manual", 1100, "USD")
	if err != nil {
		t.Fatalf("create line item: %v", err)
	}

	if _, err := st.CreateQuote(ctx, li1.ID, s1.ID, 1, 1199); err != nil {
		t.Fatalf("create quote: %v", err)
	}

	quotesByItem, err := st.ListQuotesByEvent(ctx, e1.ID)
	if err != nil {
		t.Fatalf("list quotes: %v", err)
	}
	if got := len(quotesByItem[li1.ID]); got != 1 {
		t.Fatalf("expected 1 quote, got %d", got)
	}

	if err := st.UpsertAward(ctx, li1.ID, s1.ID, 1199); err != nil {
		t.Fatalf("upsert award: %v", err)
	}
	awards, err := st.ListAwardsByEvent(ctx, e1.ID)
	if err != nil {
		t.Fatalf("list awards: %v", err)
	}
	if _, ok := awards[li1.ID]; !ok {
		t.Fatalf("expected award")
	}
}
