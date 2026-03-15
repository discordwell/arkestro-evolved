package app

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/discordwell/arkessro-evolved/internal/httpui"
	"github.com/discordwell/arkessro-evolved/internal/predict"
	"github.com/discordwell/arkessro-evolved/internal/store"
)

type Config struct {
	DBPath          string
	PredictionSeed  int64
	Logger          *log.Logger
	StaticCacheBust string
	BasePath        string
}

type App struct {
	cfg    Config
	log    *log.Logger
	db     *sql.DB
	store  *store.Store
	ui     *httpui.UI
	mux    *http.ServeMux
	closed bool
}

func New(cfg Config) (*App, error) {
	if cfg.DBPath == "" {
		return nil, errors.New("DBPath is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}

	// Enable foreign keys via DSN; modernc also respects PRAGMA foreign_keys.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", cfg.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1) // sqlite
	db.SetConnMaxLifetime(0)

	st := store.New(db)
	if err := st.Migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	pred := predict.New(predict.Config{Seed: cfg.PredictionSeed})
	ui := httpui.New(httpui.Config{
		Store:           st,
		Predictor:       pred,
		Logger:          cfg.Logger,
		StaticCacheBust: cfg.StaticCacheBust,
		BasePath:        cfg.BasePath,
	})

	mux := http.NewServeMux()
	ui.Register(mux)

	return &App{cfg: cfg, log: cfg.Logger, db: db, store: st, ui: ui, mux: mux}, nil
}

func (a *App) Handler() http.Handler {
	return http.TimeoutHandler(a.mux, 60*time.Second, "request timeout")
}

func (a *App) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	return a.db.Close()
}
