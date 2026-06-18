package repo_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

// Events created in the same instant must list in a stable order on every
// call; the SSE stream and any paginating client rely on it.
func TestListAuditEventsIsDeterministicForEqualTimestamps(t *testing.T) {
	ctx := context.Background()
	m := repo.NewMemory()
	at := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"ev-b", "ev-c", "ev-a"} {
		if _, err := m.CreateAuditEvent(ctx, domain.AuditEvent{
			ID:          id,
			WorkspaceID: "ws-1",
			Kind:        "test",
			CreatedAt:   at,
		}); err != nil {
			t.Fatalf("create event: %v", err)
		}
	}
	want := []string{"ev-a", "ev-b", "ev-c"}
	for attempt := 0; attempt < 5; attempt++ {
		events, err := m.ListAuditEvents(ctx, "ws-1", "")
		if err != nil {
			t.Fatalf("list events: %v", err)
		}
		if len(events) != len(want) {
			t.Fatalf("expected %d events, got %d", len(want), len(events))
		}
		for i, event := range events {
			if event.ID != want[i] {
				t.Fatalf("attempt %d: expected order %v, got %s at %d", attempt, want, event.ID, i)
			}
		}
	}
}

// Concurrent decisions on the same approval must resolve to exactly one
// winner; the rest get ErrNotPending.
func TestDecideApprovalIsCompareAndSwap(t *testing.T) {
	ctx := context.Background()
	m := repo.NewMemory()
	if _, err := m.CreateApproval(ctx, domain.ApprovalRequest{
		ID:          "ap-1",
		RunID:       "run-1",
		WorkspaceID: "ws-1",
		Status:      "pending",
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	const attempts = 8
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status := "approved"
			if i%2 == 1 {
				status = "rejected"
			}
			_, err := m.DecideApproval(ctx, "ap-1", status, "race", "decider@test.local", time.Now().UTC())
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, repo.ErrNotPending):
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("expected exactly one winning decision, got %d", wins.Load())
	}

	// The winning decision must have recorded the decider on the approval.
	decided, err := m.GetApproval(ctx, "ap-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if decided.DecidedBy != "decider@test.local" {
		t.Fatalf("expected decided_by recorded, got %q", decided.DecidedBy)
	}

	if _, err := m.DecideApproval(ctx, "missing", "approved", "", "decider@test.local", time.Now().UTC()); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing approval, got %v", err)
	}
}

// ListApprovalsByRun returns only the target run's approvals, newest first,
// mirroring the ordering of the workspace-wide ListApprovals.
func TestListApprovalsByRunScopesAndOrders(t *testing.T) {
	ctx := context.Background()
	m := repo.NewMemory()
	base := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	seed := []domain.ApprovalRequest{
		{ID: "ap-1", RunID: "run-1", WorkspaceID: "ws-1", Status: "approved", CreatedAt: base},
		{ID: "ap-2", RunID: "run-1", WorkspaceID: "ws-1", Status: "pending", CreatedAt: base.Add(time.Minute)},
		{ID: "ap-3", RunID: "run-2", WorkspaceID: "ws-1", Status: "pending", CreatedAt: base.Add(2 * time.Minute)},
	}
	for _, approval := range seed {
		if _, err := m.CreateApproval(ctx, approval); err != nil {
			t.Fatalf("create approval: %v", err)
		}
	}

	got, err := m.ListApprovalsByRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	want := []string{"ap-2", "ap-1"} // newest first
	if len(got) != len(want) {
		t.Fatalf("expected %d approvals for run-1, got %d", len(want), len(got))
	}
	for i, approval := range got {
		if approval.ID != want[i] {
			t.Fatalf("expected order %v, got %s at %d", want, approval.ID, i)
		}
	}

	none, err := m.ListApprovalsByRun(ctx, "run-3")
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	if none == nil {
		t.Fatalf("expected a non-nil empty slice for a run with no approvals")
	}
	if len(none) != 0 {
		t.Fatalf("expected no approvals for run-3, got %d", len(none))
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
