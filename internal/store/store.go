package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schema := `
CREATE TABLE IF NOT EXISTS suppliers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '',
  risk_score INTEGER NOT NULL DEFAULT 50,
  performance_score INTEGER NOT NULL DEFAULT 50,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS line_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  quantity REAL NOT NULL,
  unit TEXT NOT NULL DEFAULT '',
  baseline_cents INTEGER NOT NULL DEFAULT 0,
  baseline_source TEXT NOT NULL DEFAULT 'manual',
  target_cents INTEGER NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'USD',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_line_items_event_id ON line_items(event_id);

CREATE TABLE IF NOT EXISTS quotes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  line_item_id INTEGER NOT NULL REFERENCES line_items(id) ON DELETE CASCADE,
  supplier_id INTEGER NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
  round INTEGER NOT NULL DEFAULT 1,
  unit_price_cents INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_quotes_line_item_id ON quotes(line_item_id);
CREATE INDEX IF NOT EXISTS idx_quotes_supplier_id ON quotes(supplier_id);

CREATE TABLE IF NOT EXISTS awards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  line_item_id INTEGER NOT NULL UNIQUE REFERENCES line_items(id) ON DELETE CASCADE,
  supplier_id INTEGER NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
  unit_price_cents INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_awards_supplier_id ON awards(supplier_id);
`

	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}

	// Older dev DBs may exist without newer columns. Add missing columns without
	// requiring a full schema reset.
	if err := ensureColumn(ctx, s.db, "suppliers", "tags", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, s.db, "suppliers", "risk_score", "INTEGER NOT NULL DEFAULT 50"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, s.db, "suppliers", "performance_score", "INTEGER NOT NULL DEFAULT 50"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, s.db, "line_items", "category", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(ctx, s.db, "line_items", "baseline_source", "TEXT NOT NULL DEFAULT 'manual'"); err != nil {
		return err
	}

	// Bump user_version for debugging/visibility only; we still do column checks.
	_, _ = s.db.ExecContext(ctx, `PRAGMA user_version = 2`)

	return nil
}

func ensureColumn(ctx context.Context, db *sql.DB, table, column, decl string) error {
	exists, err := columnExists(ctx, db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, decl))
	if err != nil {
		return fmt.Errorf("alter table %s add column %s: %w", table, column, err)
	}
	return nil
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notnull int
			dflt    sql.NullString
			isPK    int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &isPK); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

type Supplier struct {
	ID               int64
	Name             string
	Email            string
	Tags             string
	RiskScore        int
	PerformanceScore int
	CreatedAt        time.Time
}

type Event struct {
	ID          int64
	Title       string
	Description string
	Status      string
	CreatedAt   time.Time
}

type LineItem struct {
	ID             int64
	EventID        int64
	Name           string
	Category       string
	Quantity       float64
	Unit           string
	BaselineCents  int64
	BaselineSource string
	TargetCents    int64
	Currency       string
	CreatedAt      time.Time
}

type Quote struct {
	ID             int64
	LineItemID     int64
	SupplierID     int64
	Round          int
	UnitPriceCents int64
	CreatedAt      time.Time
	SupplierName   string // join helper
}

type Award struct {
	ID             int64
	LineItemID     int64
	SupplierID     int64
	UnitPriceCents int64
	CreatedAt      time.Time
	SupplierName   string // join helper
}

func (s *Store) ListSuppliers(ctx context.Context) ([]Supplier, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, email, tags, risk_score, performance_score, created_at FROM suppliers ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Supplier
	for rows.Next() {
		var sup Supplier
		var created string
		if err := rows.Scan(&sup.ID, &sup.Name, &sup.Email, &sup.Tags, &sup.RiskScore, &sup.PerformanceScore, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		sup.CreatedAt = t
		out = append(out, sup)
	}
	return out, rows.Err()
}

func (s *Store) CreateSupplier(ctx context.Context, name, email, tags string, riskScore, performanceScore int) (Supplier, error) {
	name = normalizeSpace(name)
	email = normalizeSpace(email)
	tags = normalizeSpace(tags)
	if name == "" {
		return Supplier{}, errors.New("supplier name is required")
	}
	riskScore = clampInt(riskScore, 0, 100)
	performanceScore = clampInt(performanceScore, 0, 100)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `INSERT INTO suppliers(name, email, tags, risk_score, performance_score, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		name, email, tags, riskScore, performanceScore, now)
	if err != nil {
		return Supplier{}, err
	}
	id, _ := res.LastInsertId()
	createdAt, _ := time.Parse(time.RFC3339Nano, now)
	return Supplier{ID: id, Name: name, Email: email, Tags: tags, RiskScore: riskScore, PerformanceScore: performanceScore, CreatedAt: createdAt}, nil
}

func (s *Store) UpdateSupplierProfile(ctx context.Context, id int64, tags string, riskScore, performanceScore int) error {
	if id <= 0 {
		return errors.New("supplier id is required")
	}
	tags = normalizeSpace(tags)
	riskScore = clampInt(riskScore, 0, 100)
	performanceScore = clampInt(performanceScore, 0, 100)
	_, err := s.db.ExecContext(ctx, `UPDATE suppliers SET tags = ?, risk_score = ?, performance_score = ? WHERE id = ?`,
		tags, riskScore, performanceScore, id)
	return err
}

func (s *Store) ListEvents(ctx context.Context) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, title, description, status, created_at FROM events ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var created string
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.Status, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		e.CreatedAt = t
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	var e Event
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, title, description, status, created_at FROM events WHERE id = ?`, id).
		Scan(&e.ID, &e.Title, &e.Description, &e.Status, &created)
	if err != nil {
		return Event{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Event{}, err
	}
	e.CreatedAt = t
	return e, nil
}

func (s *Store) CreateEvent(ctx context.Context, title, description string) (Event, error) {
	title = normalizeSpace(title)
	description = normalizeSpace(description)
	if title == "" {
		return Event{}, errors.New("event title is required")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `INSERT INTO events(title, description, status, created_at) VALUES(?, ?, 'draft', ?)`, title, description, now)
	if err != nil {
		return Event{}, err
	}
	id, _ := res.LastInsertId()
	createdAt, _ := time.Parse(time.RFC3339Nano, now)
	return Event{ID: id, Title: title, Description: description, Status: "draft", CreatedAt: createdAt}, nil
}

func (s *Store) UpdateEventStatus(ctx context.Context, id int64, status string) error {
	status = normalizeSpace(status)
	if status == "" {
		return errors.New("status is required")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE events SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *Store) ListLineItemsByEvent(ctx context.Context, eventID int64) ([]LineItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, name, category, quantity, unit, baseline_cents, baseline_source, target_cents, currency, created_at FROM line_items WHERE event_id = ? ORDER BY id DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LineItem
	for rows.Next() {
		var li LineItem
		var created string
		if err := rows.Scan(&li.ID, &li.EventID, &li.Name, &li.Category, &li.Quantity, &li.Unit, &li.BaselineCents, &li.BaselineSource, &li.TargetCents, &li.Currency, &created); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		li.CreatedAt = t
		out = append(out, li)
	}
	return out, rows.Err()
}

func (s *Store) CreateLineItem(ctx context.Context, eventID int64, name, category string, qty float64, unit string, baselineCents int64, baselineSource string, targetCents int64, currency string) (LineItem, error) {
	name = normalizeSpace(name)
	category = normalizeSpace(category)
	unit = normalizeSpace(unit)
	baselineSource = normalizeSpace(baselineSource)
	currency = normalizeSpace(currency)
	if currency == "" {
		currency = "USD"
	}
	if baselineSource == "" {
		baselineSource = "manual"
	}
	if baselineSource != "manual" && baselineSource != "modeled" {
		return LineItem{}, errors.New("baseline source must be manual or modeled")
	}
	if name == "" {
		return LineItem{}, errors.New("line item name is required")
	}
	if qty <= 0 {
		return LineItem{}, errors.New("quantity must be > 0")
	}
	if baselineCents < 0 {
		return LineItem{}, errors.New("baseline price must be >= 0")
	}
	if targetCents < 0 {
		return LineItem{}, errors.New("target price must be >= 0")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `INSERT INTO line_items(event_id, name, category, quantity, unit, baseline_cents, baseline_source, target_cents, currency, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, name, category, qty, unit, baselineCents, baselineSource, targetCents, currency, now)
	if err != nil {
		return LineItem{}, err
	}
	id, _ := res.LastInsertId()
	createdAt, _ := time.Parse(time.RFC3339Nano, now)
	return LineItem{ID: id, EventID: eventID, Name: name, Category: category, Quantity: qty, Unit: unit, BaselineCents: baselineCents, BaselineSource: baselineSource, TargetCents: targetCents, Currency: currency, CreatedAt: createdAt}, nil
}

func (s *Store) UpdateLineItemTarget(ctx context.Context, id int64, targetCents int64) error {
	if id <= 0 {
		return errors.New("line item id is required")
	}
	if targetCents < 0 {
		return errors.New("target price must be >= 0")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE line_items SET target_cents = ? WHERE id = ?`, targetCents, id)
	return err
}

func (s *Store) UpdateLineItemBaseline(ctx context.Context, id int64, baselineCents int64, baselineSource string) error {
	if id <= 0 {
		return errors.New("line item id is required")
	}
	baselineSource = normalizeSpace(baselineSource)
	if baselineSource == "" {
		baselineSource = "manual"
	}
	if baselineSource != "manual" && baselineSource != "modeled" {
		return errors.New("baseline source must be manual or modeled")
	}
	if baselineCents < 0 {
		return errors.New("baseline price must be >= 0")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE line_items SET baseline_cents = ?, baseline_source = ? WHERE id = ?`, baselineCents, baselineSource, id)
	return err
}

func (s *Store) CreateQuote(ctx context.Context, lineItemID, supplierID int64, round int, unitPriceCents int64) (Quote, error) {
	if round <= 0 {
		round = 1
	}
	if unitPriceCents <= 0 {
		return Quote{}, errors.New("unit price must be > 0")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `INSERT INTO quotes(line_item_id, supplier_id, round, unit_price_cents, created_at) VALUES(?, ?, ?, ?, ?)`,
		lineItemID, supplierID, round, unitPriceCents, now)
	if err != nil {
		return Quote{}, err
	}
	id, _ := res.LastInsertId()
	createdAt, _ := time.Parse(time.RFC3339Nano, now)
	return Quote{ID: id, LineItemID: lineItemID, SupplierID: supplierID, Round: round, UnitPriceCents: unitPriceCents, CreatedAt: createdAt}, nil
}

func (s *Store) ListQuotesByEvent(ctx context.Context, eventID int64) (map[int64][]Quote, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT q.id, q.line_item_id, q.supplier_id, q.round, q.unit_price_cents, q.created_at, s.name
FROM quotes q
JOIN line_items li ON li.id = q.line_item_id
JOIN suppliers s ON s.id = q.supplier_id
WHERE li.event_id = ?
ORDER BY q.line_item_id, q.round DESC, q.unit_price_cents ASC
`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64][]Quote)
	for rows.Next() {
		var q Quote
		var created string
		if err := rows.Scan(&q.ID, &q.LineItemID, &q.SupplierID, &q.Round, &q.UnitPriceCents, &created, &q.SupplierName); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		q.CreatedAt = t
		out[q.LineItemID] = append(out[q.LineItemID], q)
	}
	return out, rows.Err()
}

func (s *Store) UpsertAward(ctx context.Context, lineItemID, supplierID int64, unitPriceCents int64) error {
	if unitPriceCents <= 0 {
		return errors.New("unit price must be > 0")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO awards(line_item_id, supplier_id, unit_price_cents, created_at)
VALUES(?, ?, ?, ?)
ON CONFLICT(line_item_id) DO UPDATE SET supplier_id=excluded.supplier_id, unit_price_cents=excluded.unit_price_cents, created_at=excluded.created_at
`, lineItemID, supplierID, unitPriceCents, now)
	return err
}

func (s *Store) ListAwardsByEvent(ctx context.Context, eventID int64) (map[int64]Award, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.line_item_id, a.supplier_id, a.unit_price_cents, a.created_at, s.name
FROM awards a
JOIN line_items li ON li.id = a.line_item_id
JOIN suppliers s ON s.id = a.supplier_id
WHERE li.event_id = ?
`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]Award)
	for rows.Next() {
		var a Award
		var created string
		if err := rows.Scan(&a.ID, &a.LineItemID, &a.SupplierID, &a.UnitPriceCents, &created, &a.SupplierName); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		a.CreatedAt = t
		out[a.LineItemID] = a
	}
	return out, rows.Err()
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
