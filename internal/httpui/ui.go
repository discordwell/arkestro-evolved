package httpui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/discordwell/arkessro-evolved/internal/copilot"
	"github.com/discordwell/arkessro-evolved/internal/predict"
	"github.com/discordwell/arkessro-evolved/internal/store"
	"github.com/discordwell/arkessro-evolved/web"
)

type Config struct {
	Store           *store.Store
	Predictor       *predict.Predictor
	Logger          *log.Logger
	StaticCacheBust string
	BasePath        string
}

type UI struct {
	st       *store.Store
	pred     *predict.Predictor
	log      *log.Logger
	tmpl     *template.Template
	bust     string
	basePath string
}

func New(cfg Config) *UI {
	if cfg.Logger == nil {
		cfg.Logger = log.New(log.Writer(), "", log.LstdFlags)
	}
	basePath := normalizeBasePath(cfg.BasePath)

	funcs := template.FuncMap{
		"money": moneyUSD,
		"pct":   pct,
		"since": since,
		"sub":   subInt64,
		"path": func(p string) string {
			return joinBasePath(basePath, p)
		},
	}

	tmpl := template.Must(template.New("base").Funcs(funcs).ParseFS(web.TemplatesFS, "templates/*.html"))

	return &UI{
		st:       cfg.Store,
		pred:     cfg.Predictor,
		log:      cfg.Logger,
		tmpl:     tmpl,
		bust:     cfg.StaticCacheBust,
		basePath: basePath,
	}
}

func (u *UI) Register(mux *http.ServeMux) {
	appMux := http.NewServeMux()

	static, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		panic(err)
	}
	appMux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	appMux.HandleFunc("GET /", u.wrap(u.handleDashboard))
	appMux.HandleFunc("GET /suppliers", u.wrap(u.handleSuppliers))
	appMux.HandleFunc("POST /suppliers", u.wrap(u.handleCreateSupplier))
	appMux.HandleFunc("POST /suppliers/", u.wrap(u.handleSupplierPost))

	appMux.HandleFunc("GET /events", u.wrap(u.handleEvents))
	appMux.HandleFunc("GET /events/new", u.wrap(u.handleNewEvent))
	appMux.HandleFunc("POST /events", u.wrap(u.handleCreateEvent))

	appMux.HandleFunc("GET /events/", u.wrap(u.handleEvent))
	appMux.HandleFunc("POST /events/", u.wrap(u.handleEventPost))

	appMux.HandleFunc("GET /api/health", u.wrap(u.handleAPIHealth))
	appMux.HandleFunc("GET /api/events", u.wrap(u.handleAPIEvents))
	appMux.HandleFunc("GET /api/events/", u.wrap(u.handleAPIEventScoped))

	if u.basePath == "" {
		mux.Handle("/", appMux)
		return
	}

	mux.Handle(u.basePath+"/", http.StripPrefix(u.basePath, appMux))
	mux.Handle(u.basePath, http.RedirectHandler(u.basePath+"/", http.StatusPermanentRedirect))
}

type handler func(http.ResponseWriter, *http.Request) error

func (u *UI) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")

		if err := h(w, r); err != nil {
			u.log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}

func (u *UI) render(w http.ResponseWriter, name string, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return u.tmpl.ExecuteTemplate(w, name, data)
}

func (u *UI) path(p string) string {
	return joinBasePath(u.basePath, p)
}

func (u *UI) handleDashboard(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	events, err := u.st.ListEvents(ctx)
	if err != nil {
		return err
	}
	suppliers, err := u.st.ListSuppliers(ctx)
	if err != nil {
		return err
	}

	type vm struct {
		Bust      string
		Events    []store.Event
		Suppliers []store.Supplier
	}
	return u.render(w, "dashboard.html", vm{Bust: u.bust, Events: events, Suppliers: suppliers})
}

func (u *UI) handleSuppliers(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	suppliers, err := u.st.ListSuppliers(ctx)
	if err != nil {
		return err
	}

	type vm struct {
		Bust      string
		Suppliers []store.Supplier
	}
	return u.render(w, "suppliers.html", vm{Bust: u.bust, Suppliers: suppliers})
}

func (u *UI) handleCreateSupplier(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		return err
	}
	name := r.FormValue("name")
	email := r.FormValue("email")
	tags := r.FormValue("tags")
	risk := parseIntDefault(r.FormValue("risk_score"), 50)
	perf := parseIntDefault(r.FormValue("performance_score"), 50)
	if _, err := u.st.CreateSupplier(ctx, name, email, tags, risk, perf); err != nil {
		return err
	}
	http.Redirect(w, r, u.path("/suppliers"), http.StatusSeeOther)
	return nil
}

func (u *UI) handleSupplierPost(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := parseIDFromPath(r.URL.Path, "/suppliers/")
	if err != nil {
		return err
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	action := r.FormValue("action")
	switch action {
	case "update_profile":
		tags := r.FormValue("tags")
		risk := parseIntDefault(r.FormValue("risk_score"), 50)
		perf := parseIntDefault(r.FormValue("performance_score"), 50)
		if err := u.st.UpdateSupplierProfile(ctx, id, tags, risk, perf); err != nil {
			return err
		}
	default:
		return errors.New("unknown action")
	}
	http.Redirect(w, r, u.path("/suppliers"), http.StatusSeeOther)
	return nil
}

func (u *UI) handleEvents(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	events, err := u.st.ListEvents(ctx)
	if err != nil {
		return err
	}

	type vm struct {
		Bust   string
		Events []store.Event
	}
	return u.render(w, "events.html", vm{Bust: u.bust, Events: events})
}

func (u *UI) handleNewEvent(w http.ResponseWriter, r *http.Request) error {
	type vm struct{ Bust string }
	return u.render(w, "event_new.html", vm{Bust: u.bust})
}

func (u *UI) handleCreateEvent(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		return err
	}
	title := r.FormValue("title")
	desc := r.FormValue("description")
	e, err := u.st.CreateEvent(ctx, title, desc)
	if err != nil {
		return err
	}
	http.Redirect(w, r, u.path(fmt.Sprintf("/events/%d", e.ID)), http.StatusSeeOther)
	return nil
}

func (u *UI) handleEvent(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := parseIDFromPath(r.URL.Path, "/events/")
	if err != nil {
		return err
	}
	e, err := u.st.GetEvent(ctx, id)
	if err != nil {
		return err
	}
	items, err := u.st.ListLineItemsByEvent(ctx, id)
	if err != nil {
		return err
	}
	suppliers, err := u.st.ListSuppliers(ctx)
	if err != nil {
		return err
	}
	quotesByItem, err := u.st.ListQuotesByEvent(ctx, id)
	if err != nil {
		return err
	}
	awardsByItem, err := u.st.ListAwardsByEvent(ctx, id)
	if err != nil {
		return err
	}

	weights := parseAwardWeights(r)

	supplierByID := make(map[int64]store.Supplier, len(suppliers))
	for _, s := range suppliers {
		supplierByID[s.ID] = s
	}

	// Negotiation science: per-item offer guidance.
	predByItem := make(map[int64]predict.Target, len(items))
	for i := range items {
		li := items[i]
		// Smart baselining: if no baseline, model one so the rest of the demo works.
		if li.BaselineCents == 0 {
			modeled := u.pred.ModelBaseline(li.Name, li.Category, li.Quantity)
			_ = u.st.UpdateLineItemBaseline(ctx, li.ID, modeled, "modeled")
			li.BaselineCents = modeled
			li.BaselineSource = "modeled"
			items[i] = li
		}

		p := u.pred.Predict(li.BaselineCents, li.Quantity, len(suppliers))
		predByItem[li.ID] = p

		// Keep persisted targets in sync with current supplier set.
		if li.TargetCents != p.TargetCents {
			_ = u.st.UpdateLineItemTarget(ctx, li.ID, p.TargetCents)
			li.TargetCents = p.TargetCents
			items[i] = li
		}
	}

	bestQuote := make(map[int64]store.Quote)
	for liID, qs := range quotesByItem {
		for _, q := range qs {
			b, ok := bestQuote[liID]
			if !ok || q.UnitPriceCents < b.UnitPriceCents {
				bestQuote[liID] = q
			}
		}
	}

	// Supplier science: recommend suppliers per line item.
	supplierRecsByItem := make(map[int64][]SupplierRec, len(items))
	for _, li := range items {
		supplierRecsByItem[li.ID] = recommendSuppliers(li, suppliers)
	}

	// Best-value award modeling (cost vs risk vs performance).
	bestValueByItem := make(map[int64]AwardRec)
	for _, li := range items {
		rec, ok := bestValueAward(li, quotesByItem[li.ID], supplierByID, weights)
		if ok {
			bestValueByItem[li.ID] = rec
		}
	}

	copilotByItem, quoteFeatureByID, quoteFlagsByID, backtestRows, backtestSummary := u.buildCopilotData(items, suppliers, quotesByItem, awardsByItem, predByItem)

	var (
		totalBaseline int64
		totalAwarded  int64
	)
	for _, li := range items {
		totalBaseline += int64(math.Round(float64(li.BaselineCents) * li.Quantity))
		if a, ok := awardsByItem[li.ID]; ok {
			totalAwarded += int64(math.Round(float64(a.UnitPriceCents) * li.Quantity))
		}
	}

	type vm struct {
		Bust               string
		Event              store.Event
		Items              []store.LineItem
		Suppliers          []store.Supplier
		QuotesByItem       map[int64][]store.Quote
		AwardsByItem       map[int64]store.Award
		BestQuote          map[int64]store.Quote
		TotalBaselineCents int64
		TotalAwardedCents  int64
		PredByItem         map[int64]predict.Target
		SupplierRecsByItem map[int64][]SupplierRec
		BestValueByItem    map[int64]AwardRec
		CopilotByItem      map[int64]copilot.Decision
		QuoteFeatureByID   map[int64]copilot.QuoteFeature
		QuoteFlagsByID     map[int64][]QuoteFlag
		BacktestRows       []BacktestRow
		BacktestSummary    BacktestSummary
		ImportResult       *ReplayImportResult
		Weights            AwardWeights
	}

	var importResult *ReplayImportResult
	if r.URL.Query().Get("imported") == "1" {
		importResult = &ReplayImportResult{
			Rows:             parseIntDefault(r.URL.Query().Get("rows"), 0),
			SuppliersCreated: parseIntDefault(r.URL.Query().Get("suppliers_created"), 0),
			SuppliersUpdated: parseIntDefault(r.URL.Query().Get("suppliers_updated"), 0),
			LineItemsCreated: parseIntDefault(r.URL.Query().Get("line_items_created"), 0),
			QuotesUpserted:   parseIntDefault(r.URL.Query().Get("quotes_upserted"), 0),
			AwardsUpserted:   parseIntDefault(r.URL.Query().Get("awards_upserted"), 0),
		}
	}

	return u.render(w, "event.html", vm{
		Bust:               u.bust,
		Event:              e,
		Items:              items,
		Suppliers:          suppliers,
		QuotesByItem:       quotesByItem,
		AwardsByItem:       awardsByItem,
		BestQuote:          bestQuote,
		TotalBaselineCents: totalBaseline,
		TotalAwardedCents:  totalAwarded,
		PredByItem:         predByItem,
		SupplierRecsByItem: supplierRecsByItem,
		BestValueByItem:    bestValueByItem,
		CopilotByItem:      copilotByItem,
		QuoteFeatureByID:   quoteFeatureByID,
		QuoteFlagsByID:     quoteFlagsByID,
		BacktestRows:       backtestRows,
		BacktestSummary:    backtestSummary,
		ImportResult:       importResult,
		Weights:            weights,
	})
}

func (u *UI) handleEventPost(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := parseIDFromPath(r.URL.Path, "/events/")
	if err != nil {
		return err
	}

	if err := r.ParseForm(); err != nil {
		return err
	}
	action := r.FormValue("action")
	switch action {
	case "add_line_item":
		name := r.FormValue("name")
		category := r.FormValue("category")
		qty, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue("quantity")), 64)
		if err != nil {
			return errors.New("invalid quantity")
		}
		unit := r.FormValue("unit")
		baselineCents, err := parseMoneyToCents(r.FormValue("baseline"))
		if err != nil {
			return err
		}

		suppliers, err := u.st.ListSuppliers(ctx)
		if err != nil {
			return err
		}
		baselineSource := "manual"
		if baselineCents == 0 {
			baselineCents = u.pred.ModelBaseline(name, category, qty)
			baselineSource = "modeled"
		}
		pred := u.pred.Predict(baselineCents, qty, len(suppliers))

		if _, err := u.st.CreateLineItem(ctx, id, name, category, qty, unit, baselineCents, baselineSource, pred.TargetCents, "USD"); err != nil {
			return err
		}
	case "add_quote":
		liID, err := strconv.ParseInt(r.FormValue("line_item_id"), 10, 64)
		if err != nil {
			return errors.New("invalid line_item_id")
		}
		supplierID, err := strconv.ParseInt(r.FormValue("supplier_id"), 10, 64)
		if err != nil {
			return errors.New("invalid supplier_id")
		}
		round, _ := strconv.Atoi(r.FormValue("round"))
		unitPriceCents, err := parseMoneyToCents(r.FormValue("unit_price"))
		if err != nil {
			return err
		}
		if _, err := u.st.CreateQuote(ctx, liID, supplierID, round, unitPriceCents); err != nil {
			return err
		}
	case "import_replay_csv":
		file, _, err := r.FormFile("replay_csv")
		if err != nil {
			return errors.New("replay_csv file is required")
		}
		defer file.Close()

		result, err := u.importReplayCSV(ctx, id, file)
		if err != nil {
			return err
		}

		q := url.Values{}
		q.Set("imported", "1")
		q.Set("rows", strconv.Itoa(result.Rows))
		q.Set("suppliers_created", strconv.Itoa(result.SuppliersCreated))
		q.Set("suppliers_updated", strconv.Itoa(result.SuppliersUpdated))
		q.Set("line_items_created", strconv.Itoa(result.LineItemsCreated))
		q.Set("quotes_upserted", strconv.Itoa(result.QuotesUpserted))
		q.Set("awards_upserted", strconv.Itoa(result.AwardsUpserted))
		http.Redirect(w, r, u.path(fmt.Sprintf("/events/%d?%s", id, q.Encode())), http.StatusSeeOther)
		return nil
	case "simulate_round":
		items, err := u.st.ListLineItemsByEvent(ctx, id)
		if err != nil {
			return err
		}
		suppliers, err := u.st.ListSuppliers(ctx)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return errors.New("add at least one line item before simulating quotes")
		}
		if len(suppliers) == 0 {
			return errors.New("add at least one supplier before simulating quotes")
		}

		quotesByItem, err := u.st.ListQuotesByEvent(ctx, id)
		if err != nil {
			return err
		}

		maxRound := 0
		for _, qs := range quotesByItem {
			for _, q := range qs {
				if q.Round > maxRound {
					maxRound = q.Round
				}
			}
		}
		nextRound := maxRound + 1
		if nextRound < 1 {
			nextRound = 1
		}

		for _, li := range items {
			baselineCents := li.BaselineCents
			if baselineCents == 0 {
				baselineCents = u.pred.ModelBaseline(li.Name, li.Category, li.Quantity)
				_ = u.st.UpdateLineItemBaseline(ctx, li.ID, baselineCents, "modeled")
			}
			p := u.pred.Predict(baselineCents, li.Quantity, len(suppliers))
			_ = u.st.UpdateLineItemTarget(ctx, li.ID, p.TargetCents)

			for _, s := range suppliers {
				unitPrice := simulateQuoteCents(li, s, nextRound, p)
				if _, err := u.st.CreateQuote(ctx, li.ID, s.ID, nextRound, unitPrice); err != nil {
					return err
				}
			}
		}
		_ = u.st.UpdateEventStatus(ctx, id, "active")
	case "award_best_value":
		items, err := u.st.ListLineItemsByEvent(ctx, id)
		if err != nil {
			return err
		}
		quotesByItem, err := u.st.ListQuotesByEvent(ctx, id)
		if err != nil {
			return err
		}
		suppliers, err := u.st.ListSuppliers(ctx)
		if err != nil {
			return err
		}

		supplierByID := make(map[int64]store.Supplier, len(suppliers))
		for _, s := range suppliers {
			supplierByID[s.ID] = s
		}

		weights := normalizeAwardWeights(AwardWeights{
			Cost:        parseIntDefault(r.FormValue("w_cost"), 70),
			Risk:        parseIntDefault(r.FormValue("w_risk"), 15),
			Performance: parseIntDefault(r.FormValue("w_perf"), 15),
		})

		for _, li := range items {
			rec, ok := bestValueAward(li, quotesByItem[li.ID], supplierByID, weights)
			if !ok {
				continue
			}
			if err := u.st.UpsertAward(ctx, li.ID, rec.SupplierID, rec.UnitPriceCents); err != nil {
				return err
			}
		}
		_ = u.st.UpdateEventStatus(ctx, id, "awarded")
	case "award_lowest":
		items, err := u.st.ListLineItemsByEvent(ctx, id)
		if err != nil {
			return err
		}
		quotesByItem, err := u.st.ListQuotesByEvent(ctx, id)
		if err != nil {
			return err
		}
		for _, li := range items {
			qs := quotesByItem[li.ID]
			if len(qs) == 0 {
				continue
			}
			best := qs[0]
			for _, q := range qs[1:] {
				if q.UnitPriceCents < best.UnitPriceCents {
					best = q
				}
			}
			if err := u.st.UpsertAward(ctx, li.ID, best.SupplierID, best.UnitPriceCents); err != nil {
				return err
			}
		}
		_ = u.st.UpdateEventStatus(ctx, id, "awarded")
	case "set_status":
		status := r.FormValue("status")
		if err := u.st.UpdateEventStatus(ctx, id, status); err != nil {
			return err
		}
	default:
		return errors.New("unknown action")
	}

	http.Redirect(w, r, u.path(fmt.Sprintf("/events/%d", id)), http.StatusSeeOther)
	return nil
}

func (u *UI) handleAPIHealth(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"time": time.Now().UTC().Format(time.RFC3339Nano),
	})
	return nil
}

func (u *UI) handleAPIEvents(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	events, err := u.st.ListEvents(ctx)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(events)
}

func (u *UI) handleAPIEventScoped(w http.ResponseWriter, r *http.Request) error {
	const prefix = "/api/events/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return errors.New("bad path")
	}
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return errors.New("missing id")
	}

	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return errors.New("invalid id")
	}
	suffix := ""
	if len(parts) == 2 {
		suffix = "/" + strings.Trim(parts[1], "/")
	}

	switch suffix {
	case "/copilot":
		return u.handleAPIEventCopilot(w, r, id)
	case "/copilot/backtest":
		return u.handleAPIEventCopilotBacktest(w, r, id)
	default:
		return errors.New("unknown api route")
	}
}

func (u *UI) handleAPIEventCopilot(w http.ResponseWriter, r *http.Request, eventID int64) error {
	ctx := r.Context()
	items, suppliers, quotesByItem, awardsByItem, predByItem, err := u.loadEventCopilotInputs(ctx, eventID)
	if err != nil {
		return err
	}
	decisionsByItem, featuresByQuoteID, _, _, _ := u.buildCopilotData(items, suppliers, quotesByItem, awardsByItem, predByItem)

	type itemResp struct {
		LineItemID   int64            `json:"line_item_id"`
		LineItemName string           `json:"line_item_name"`
		Decision     copilot.Decision `json:"decision"`
		QuoteCount   int              `json:"quote_count"`
	}
	out := struct {
		EventID  int64      `json:"event_id"`
		Items    []itemResp `json:"items"`
		Features int        `json:"feature_rows"`
	}{
		EventID:  eventID,
		Items:    make([]itemResp, 0, len(items)),
		Features: len(featuresByQuoteID),
	}
	for _, li := range items {
		out.Items = append(out.Items, itemResp{
			LineItemID:   li.ID,
			LineItemName: li.Name,
			Decision:     decisionsByItem[li.ID],
			QuoteCount:   len(quotesByItem[li.ID]),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(out)
}

func (u *UI) handleAPIEventCopilotBacktest(w http.ResponseWriter, r *http.Request, eventID int64) error {
	ctx := r.Context()
	items, suppliers, quotesByItem, awardsByItem, predByItem, err := u.loadEventCopilotInputs(ctx, eventID)
	if err != nil {
		return err
	}
	_, _, _, rows, summary := u.buildCopilotData(items, suppliers, quotesByItem, awardsByItem, predByItem)
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(map[string]any{
		"event_id": eventID,
		"summary":  summary,
		"rows":     rows,
	})
}

func (u *UI) loadEventCopilotInputs(
	ctx context.Context,
	eventID int64,
) ([]store.LineItem, []store.Supplier, map[int64][]store.Quote, map[int64]store.Award, map[int64]predict.Target, error) {
	items, err := u.st.ListLineItemsByEvent(ctx, eventID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	suppliers, err := u.st.ListSuppliers(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	quotesByItem, err := u.st.ListQuotesByEvent(ctx, eventID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	awardsByItem, err := u.st.ListAwardsByEvent(ctx, eventID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	predByItem := make(map[int64]predict.Target, len(items))
	for _, li := range items {
		baseline := li.BaselineCents
		if baseline == 0 {
			baseline = u.pred.ModelBaseline(li.Name, li.Category, li.Quantity)
		}
		predByItem[li.ID] = u.pred.Predict(baseline, li.Quantity, len(suppliers))
	}
	return items, suppliers, quotesByItem, awardsByItem, predByItem, nil
}

func parseIDFromPath(pth, prefix string) (int64, error) {
	if !strings.HasPrefix(pth, prefix) {
		return 0, errors.New("bad path")
	}
	rest := strings.TrimPrefix(pth, prefix)
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return 0, errors.New("missing id")
	}
	seg := strings.SplitN(rest, "/", 2)[0]
	id, err := strconv.ParseInt(seg, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func normalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	return "/" + strings.Trim(p, "/")
}

func joinBasePath(basePath, p string) string {
	if p == "" {
		if basePath == "" {
			return "/"
		}
		return basePath
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if basePath == "" {
		return p
	}
	return basePath + p
}

func parseMoneyToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0, nil
	}

	parts := strings.SplitN(s, ".", 3)
	if len(parts) > 2 {
		return 0, errors.New("invalid money")
	}
	dollars, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, errors.New("invalid money")
	}
	cents := int64(0)
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		}
		if len(frac) > 2 {
			frac = frac[:2]
		}
		c, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, errors.New("invalid money")
		}
		cents = c
	}
	if dollars < 0 {
		return 0, errors.New("money must be >= 0")
	}
	return dollars*100 + cents, nil
}

func moneyUSD(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	d := cents / 100
	c := cents % 100
	out := fmt.Sprintf("$%d.%02d", d, c)
	if neg {
		out = "-" + out
	}
	return out
}

func subInt64(a, b int64) int64 { return a - b }

func pct(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", v*100)
}

func since(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Format("2006-01-02")
}

type AwardWeights struct {
	Cost        int
	Risk        int
	Performance int
}

type SupplierRec struct {
	Supplier store.Supplier
	Score    float64
	Reason   string
}

type QuoteFlag struct {
	Class string // ok | warn | danger
	Text  string
}

type AwardRec struct {
	SupplierID       int64
	SupplierName     string
	UnitPriceCents   int64
	Score            float64
	CostScore        float64
	RiskScore        float64
	PerformanceScore float64
}

func parseIntDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseAwardWeights(r *http.Request) AwardWeights {
	q := r.URL.Query()
	return normalizeAwardWeights(AwardWeights{
		Cost:        parseIntDefault(q.Get("w_cost"), 70),
		Risk:        parseIntDefault(q.Get("w_risk"), 15),
		Performance: parseIntDefault(q.Get("w_perf"), 15),
	})
}

func normalizeAwardWeights(w AwardWeights) AwardWeights {
	w.Cost = clampInt(w.Cost, 0, 100)
	w.Risk = clampInt(w.Risk, 0, 100)
	w.Performance = clampInt(w.Performance, 0, 100)

	sum := w.Cost + w.Risk + w.Performance
	if sum <= 0 {
		return AwardWeights{Cost: 70, Risk: 15, Performance: 15}
	}
	if sum == 100 {
		return w
	}

	// Normalize to 100 for easier UI display and predictable scoring.
	wc := int(math.Round(float64(w.Cost) * 100.0 / float64(sum)))
	wr := int(math.Round(float64(w.Risk) * 100.0 / float64(sum)))
	wp := int(math.Round(float64(w.Performance) * 100.0 / float64(sum)))
	diff := 100 - (wc + wr + wp)
	wc += diff

	return AwardWeights{
		Cost:        clampInt(wc, 0, 100),
		Risk:        clampInt(wr, 0, 100),
		Performance: clampInt(wp, 0, 100),
	}
}

func recommendSuppliers(li store.LineItem, suppliers []store.Supplier) []SupplierRec {
	itemTags := splitTags(li.Category)
	recs := make([]SupplierRec, 0, len(suppliers))

	for _, s := range suppliers {
		sTags := splitTags(s.Tags)
		matched := intersect(itemTags, sTags)
		fit := 0.0
		if len(itemTags) > 0 {
			fit = float64(len(matched)) / float64(len(itemTags))
		}

		perf := float64(clampInt(s.PerformanceScore, 0, 100)) / 100.0
		risk := 1.0 - float64(clampInt(s.RiskScore, 0, 100))/100.0
		score := 0.55*fit + 0.30*perf + 0.15*risk

		reason := ""
		if len(itemTags) == 0 {
			reason = "no item category; "
		} else if len(matched) > 0 {
			reason = fmt.Sprintf("tag match: %s; ", strings.Join(matched, ", "))
		} else {
			reason = "no tag match; "
		}
		reason += fmt.Sprintf("perf %d; risk %d", s.PerformanceScore, s.RiskScore)

		recs = append(recs, SupplierRec{Supplier: s, Score: score, Reason: reason})
	}

	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Score == recs[j].Score {
			return recs[i].Supplier.ID < recs[j].Supplier.ID
		}
		return recs[i].Score > recs[j].Score
	})
	if len(recs) > 3 {
		recs = recs[:3]
	}
	return recs
}

func quoteFlags(li store.LineItem, p predict.Target, q store.Quote) []QuoteFlag {
	var out []QuoteFlag

	add := func(class, text string) {
		for _, f := range out {
			if f.Text == text {
				return
			}
		}
		out = append(out, QuoteFlag{Class: class, Text: text})
	}

	if q.UnitPriceCents <= 0 {
		add("danger", "invalid price")
	}
	if q.UnitPriceCents > p.WalkAwayCents && p.WalkAwayCents > 0 {
		add("danger", "above walk-away")
	}
	if li.BaselineCents > 0 && q.UnitPriceCents > int64(math.Round(float64(li.BaselineCents)*1.20)) {
		add("warn", "high vs baseline")
	}
	if q.UnitPriceCents <= p.FirstOfferCents && p.FirstOfferCents > 0 {
		add("ok", "beats first offer")
	} else if q.UnitPriceCents <= p.TargetCents && p.TargetCents > 0 {
		add("ok", "at/under target")
	} else if q.UnitPriceCents > p.TargetCents && p.TargetCents > 0 {
		add("warn", "above target")
	}
	if q.UnitPriceCents < int64(math.Round(float64(p.FirstOfferCents)*0.85)) && p.FirstOfferCents > 0 {
		add("warn", "suspiciously low")
	}

	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func bestValueAward(li store.LineItem, quotes []store.Quote, supplierByID map[int64]store.Supplier, weights AwardWeights) (AwardRec, bool) {
	// Only consider the best (lowest) quote per supplier.
	bestBySupplier := make(map[int64]int64)
	for _, q := range quotes {
		if q.UnitPriceCents <= 0 {
			continue
		}
		if cur, ok := bestBySupplier[q.SupplierID]; !ok || q.UnitPriceCents < cur {
			bestBySupplier[q.SupplierID] = q.UnitPriceCents
		}
	}
	if len(bestBySupplier) == 0 {
		return AwardRec{}, false
	}

	var (
		minPrice int64
		maxPrice int64
		first    = true
	)
	for _, p := range bestBySupplier {
		if first {
			minPrice, maxPrice, first = p, p, false
			continue
		}
		if p < minPrice {
			minPrice = p
		}
		if p > maxPrice {
			maxPrice = p
		}
	}

	cw := float64(weights.Cost) / 100.0
	rw := float64(weights.Risk) / 100.0
	pw := float64(weights.Performance) / 100.0

	best := AwardRec{}
	has := false
	for supplierID, unitPrice := range bestBySupplier {
		sup, ok := supplierByID[supplierID]
		if !ok {
			continue
		}

		costScore := 1.0
		if maxPrice > minPrice {
			costScore = float64(maxPrice-unitPrice) / float64(maxPrice-minPrice)
		}
		riskScore := 1.0 - float64(clampInt(sup.RiskScore, 0, 100))/100.0
		perfScore := float64(clampInt(sup.PerformanceScore, 0, 100)) / 100.0

		score := cw*costScore + rw*riskScore + pw*perfScore
		if !has || score > best.Score {
			best = AwardRec{
				SupplierID:       supplierID,
				SupplierName:     sup.Name,
				UnitPriceCents:   unitPrice,
				Score:            score,
				CostScore:        costScore,
				RiskScore:        riskScore,
				PerformanceScore: perfScore,
			}
			has = true
		}
	}

	return best, has
}

func simulateQuoteCents(li store.LineItem, sup store.Supplier, round int, p predict.Target) int64 {
	baseline := li.BaselineCents
	if baseline <= 0 {
		baseline = 1000 // $10.00 fallback
	}

	itemTags := splitTags(li.Category)
	sTags := splitTags(sup.Tags)
	matched := intersect(itemTags, sTags)
	fit := 0.0
	if len(itemTags) > 0 {
		fit = float64(len(matched)) / float64(len(itemTags))
	}

	perf := float64(clampInt(sup.PerformanceScore, 0, 100)) / 100.0
	risk := float64(clampInt(sup.RiskScore, 0, 100)) / 100.0

	aggr := 0.25 + 0.50*fit + 0.25*perf - 0.20*risk
	aggr = clampFloat(aggr, 0.10, 0.90)
	roundPressure := clampFloat(float64(round-1)*0.12, 0.0, 0.50)

	jitter := (hash01(fmt.Sprintf("%d|%d|%d", li.ID, sup.ID, round)) - 0.5) * 0.03 // ~[-1.5%, +1.5%]
	discount := p.DiscountFraction*(0.35+0.45*aggr+roundPressure) + jitter
	discount = clampFloat(discount, 0.01, 0.35)

	unitPrice := int64(math.Round(float64(baseline) * (1.0 - discount)))
	if unitPrice <= 0 {
		unitPrice = 1
	}

	// Keep simulated prices from being unrealistically far below the buyer's first offer.
	if p.FirstOfferCents > 0 {
		floor := int64(math.Round(float64(p.FirstOfferCents) * 0.90))
		if unitPrice < floor {
			unitPrice = floor
		}
	}

	return unitPrice
}

func splitTags(s string) []string {
	s = strings.ToLower(s)
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func intersect(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(b))
	for _, x := range b {
		set[x] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, x := range a {
		if _, ok := set[x]; ok {
			out = append(out, x)
		}
	}
	return out
}

func hash01(key string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("arkessro"))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key))
	v := h.Sum64()
	return float64(v%10_000) / 10_000.0
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

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
