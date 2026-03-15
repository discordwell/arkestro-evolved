package app_test

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/discordwell/arkessro-evolved/internal/app"
)

func TestAppServesDashboardAndSuppliers(t *testing.T) {
	cfg := app.Config{
		DBPath:          filepath.Join(t.TempDir(), "dev.db"),
		PredictionSeed:  0,
		Logger:          log.New(io.Discard, "", 0),
		StaticCacheBust: "test",
	}

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer a.Close()

	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(b), "Evochain") {
		t.Fatalf("expected Evochain in body")
	}

	form := url.Values{}
	form.Set("name", "Acme")
	form.Set("email", "")
	resp, err = http.PostForm(srv.URL+"/suppliers", form)
	if err != nil {
		t.Fatalf("post /suppliers: %v", err)
	}
	b, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after redirect follow, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(b), "Acme") {
		t.Fatalf("expected supplier to appear")
	}
}
