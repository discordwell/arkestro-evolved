package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/repo"
)

func TestClaimQueuedRunIsFIFOAndMarksRunning(t *testing.T) {
	ctx := context.Background()
	m := repo.NewMemory()
	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"run-c", "run-a", "run-b"} {
		if _, err := m.CreateTaskRun(ctx, domain.TaskRun{
			ID:        id,
			Status:    "queued",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("create run: %v", err)
		}
	}
	// run-x is not queued and must never be claimed.
	if _, err := m.CreateTaskRun(ctx, domain.TaskRun{ID: "run-x", Status: "completed", CreatedAt: base}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	wantOrder := []string{"run-c", "run-a", "run-b"}
	for _, want := range wantOrder {
		run, ok, err := m.ClaimQueuedRun(ctx)
		if err != nil || !ok {
			t.Fatalf("claim: ok=%v err=%v", ok, err)
		}
		if run.ID != want {
			t.Fatalf("expected oldest queued run %s, got %s", want, run.ID)
		}
		if run.Status != "running" {
			t.Fatalf("claimed run must be marked running, got %s", run.Status)
		}
	}
	if _, ok, err := m.ClaimQueuedRun(ctx); err != nil || ok {
		t.Fatalf("expected no more claimable runs, ok=%v err=%v", ok, err)
	}
}

func TestMemoryGettersWrapErrNotFound(t *testing.T) {
	ctx := context.Background()
	m := repo.NewMemory()
	if _, err := m.GetWorkspace(ctx, "nope"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("workspace: expected ErrNotFound, got %v", err)
	}
	if _, err := m.GetTaskRun(ctx, "nope"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("task run: expected ErrNotFound, got %v", err)
	}
	if _, err := m.GetApproval(ctx, "nope"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("approval: expected ErrNotFound, got %v", err)
	}
}
