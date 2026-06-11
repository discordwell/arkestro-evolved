package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/storage"
)

func TestFSStorePutGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := storage.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("new fs store: %v", err)
	}

	key := "ws-1/run-1/draft.md"
	body := []byte("# hello\n")
	if err := store.Put(ctx, key, body, "text/markdown"); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("expected %q, got %q", body, got)
	}
}

func TestFSStoreGetMissingKeyFails(t *testing.T) {
	store, err := storage.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("new fs store: %v", err)
	}
	if _, err := store.Get(context.Background(), "ws-1/run-1/missing.md"); err == nil {
		t.Fatalf("expected error for missing key")
	}
}

// Storage keys are built from workspace/run IDs and catalog step slugs; none
// of those may ever resolve outside the artifact root, so the store rejects
// escaping keys outright instead of trusting its callers.
func TestFSStoreRejectsKeysOutsideRoot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := storage.NewFS(filepath.Join(root, "artifacts"))
	if err != nil {
		t.Fatalf("new fs store: %v", err)
	}

	outside := filepath.Join(root, "escape.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}

	badKeys := []string{
		"",
		"../escape.md",
		"ws-1/../../escape.md",
		"/etc/passwd",
		outside,
	}
	for _, key := range badKeys {
		if err := store.Put(ctx, key, []byte("x"), "text/plain"); err == nil || !strings.Contains(err.Error(), "invalid storage key") {
			t.Fatalf("put %q: expected invalid-key error, got %v", key, err)
		}
		if _, err := store.Get(ctx, key); err == nil || !strings.Contains(err.Error(), "invalid storage key") {
			t.Fatalf("get %q: expected invalid-key error, got %v", key, err)
		}
	}

	// The escape attempt must not have touched the file outside the root.
	content, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if string(content) != "secret" {
		t.Fatalf("file outside the root was modified: %q", content)
	}
}
