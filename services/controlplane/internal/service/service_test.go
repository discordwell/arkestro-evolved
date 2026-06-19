package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/catalog"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/repo"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/service"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/storage"
)

func newService(t *testing.T) *service.ControlPlane {
	t.Helper()
	cat, err := catalog.Load("../../../../catalog/runbooks.json")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	store, err := storage.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	svc := service.New(repo.NewMemory(), store, cat, domain.Org{
		ID:        "org-1",
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: time.Now().UTC(),
	})
	svc.ConfigureBootstrapAdmin("admin@test.local", "test-password", "Test Admin")
	if err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return svc
}

func setupWorkspace(t *testing.T, svc *service.ControlPlane) (domain.Workspace, domain.Environment) {
	t.Helper()
	ctx := context.Background()
	workspace, err := svc.CreateWorkspace(ctx, "org-1", "Ops", "ops", "Ops workspace")
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}
	env, err := svc.CreateEnvironment(ctx, "org-1", workspace.ID, "Staging", "staging", "staging")
	if err != nil {
		t.Fatalf("environment: %v", err)
	}
	return workspace, env
}

func TestComplianceRunCompletesWithoutApproval(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)

	run, err := svc.CreateRun(context.Background(), "org-1", workspace.ID, env.ID, "compliance-pack", domain.Actor{Surface: "cli", Agent: "codex"}, map[string]any{"period": "weekly"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(context.Background()); err != nil {
		t.Fatalf("process: %v", err)
	}
	updated, err := svc.GetRunEnvelope(context.Background(), "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.Run.Status != "completed" {
		t.Fatalf("expected completed, got %s", updated.Run.Status)
	}
	if len(updated.Artifacts) == 0 {
		t.Fatalf("expected artifacts")
	}
}

func TestReleaseRunRequiresApprovalThenCompletes(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)

	run, err := svc.CreateRun(context.Background(), "org-1", workspace.ID, env.ID, "release-coordination", domain.Actor{Surface: "mcp", Agent: "claude"}, map[string]any{"release": "2026.03.14"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(context.Background()); err != nil {
		t.Fatalf("process: %v", err)
	}
	waiting, err := svc.GetRunEnvelope(context.Background(), "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if waiting.Run.Status != "awaiting_approval" {
		t.Fatalf("expected awaiting_approval, got %s", waiting.Run.Status)
	}
	if len(waiting.Approvals) != 1 {
		t.Fatalf("expected approval request")
	}
	if _, err := svc.DecideApproval(context.Background(), "org-1", waiting.Approvals[0].ID, "approve", "ship it", domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := svc.ProcessNextRun(context.Background()); err != nil {
		t.Fatalf("process after approval: %v", err)
	}
	completed, err := svc.GetRunEnvelope(context.Background(), "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if completed.Run.Status != "completed" {
		t.Fatalf("expected completed, got %s", completed.Run.Status)
	}
}

func TestLoginAndTenantGuards(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)

	session, err := svc.Login(context.Background(), "admin@test.local", "test-password", "tests")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if session.AccessToken == "" {
		t.Fatalf("expected access token")
	}
	principal, err := svc.Authenticate(context.Background(), session.AccessToken)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if principal.User.Email != "admin@test.local" {
		t.Fatalf("unexpected user: %s", principal.User.Email)
	}

	run, err := svc.CreateRun(context.Background(), "org-1", workspace.ID, env.ID, "compliance-pack", domain.Actor{Surface: "cli", Agent: "codex"}, map[string]any{"period": "daily"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.GetRunEnvelope(context.Background(), "org-2", run.Run.ID); err == nil {
		t.Fatalf("expected cross-org access to fail")
	}
}

func TestLoginUnknownEmailReturnsInvalidCredentials(t *testing.T) {
	svc := newService(t)
	if _, err := svc.Login(context.Background(), "nobody@test.local", "whatever", ""); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

// last_used_at refreshes at most once per touch interval, so steady request
// traffic does not turn into a token-store write per request.
func TestAuthenticateThrottlesTokenTouches(t *testing.T) {
	svc := newService(t)
	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	current := base
	svc.SetNowFunc(func() time.Time { return current })

	session, err := svc.Login(context.Background(), "admin@test.local", "test-password", "throttle-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	current = base.Add(30 * time.Second)
	principal, err := svc.Authenticate(context.Background(), session.AccessToken)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if !principal.Token.LastUsedAt.Equal(base) {
		t.Fatalf("touch within the interval must be skipped: expected %v, got %v", base, principal.Token.LastUsedAt)
	}

	current = base.Add(2 * time.Minute)
	principal, err = svc.Authenticate(context.Background(), session.AccessToken)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if !principal.Token.LastUsedAt.Equal(current) {
		t.Fatalf("touch past the interval must persist: expected %v, got %v", current, principal.Token.LastUsedAt)
	}
}

func awaitApproval(t *testing.T, svc *service.ControlPlane, orgID, workspaceID, envID string) domain.RunEnvelope {
	t.Helper()
	ctx := context.Background()
	run, err := svc.CreateRun(ctx, orgID, workspaceID, envID, "release-coordination", domain.Actor{Surface: "mcp", Agent: "claude"}, nil)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process: %v", err)
	}
	waiting, err := svc.GetRunEnvelope(ctx, orgID, run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if waiting.Run.Status != "awaiting_approval" || len(waiting.Approvals) != 1 {
		t.Fatalf("expected pending approval, got status=%s approvals=%d", waiting.Run.Status, len(waiting.Approvals))
	}
	return waiting
}

func TestDecideApprovalCrossOrgIsForbiddenAndDoesNotMutate(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	waiting := awaitApproval(t, svc, "org-1", workspace.ID, env.ID)
	approvalID := waiting.Approvals[0].ID

	if _, err := svc.DecideApproval(ctx, "org-2", approvalID, "approve", "sneaky", domain.Actor{Surface: "api", Agent: "human", User: "intruder@test.local"}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	after, err := svc.GetRunEnvelope(ctx, "org-1", waiting.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if after.Approvals[0].Status != "pending" {
		t.Fatalf("cross-org decision must not persist, approval status=%s", after.Approvals[0].Status)
	}
	if after.Run.Status != "awaiting_approval" {
		t.Fatalf("run must stay awaiting approval, got %s", after.Run.Status)
	}

	// The rightful org can still decide the untouched approval.
	if _, err := svc.DecideApproval(ctx, "org-1", approvalID, "approve", "ok", domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"}); err != nil {
		t.Fatalf("legitimate approval failed: %v", err)
	}
}

// Concurrent decisions on one approval must produce exactly one winner and a
// consistent run state, never duplicate transitions.
func TestConcurrentApprovalDecisionsHaveOneWinner(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	waiting := awaitApproval(t, svc, "org-1", workspace.ID, env.ID)
	approvalID := waiting.Approvals[0].ID

	const attempts = 8
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.DecideApproval(ctx, "org-1", approvalID, "approve", "go", domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"})
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, repo.ErrNotPending):
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("expected exactly one winning decision, got %d", wins.Load())
	}

	after, err := svc.GetRunEnvelope(ctx, "org-1", waiting.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if after.Run.Status != "queued" || after.Run.ApprovalState != "approved" {
		t.Fatalf("expected queued/approved run, got %s/%s", after.Run.Status, after.Run.ApprovalState)
	}
	if after.Approvals[0].Status != "approved" {
		t.Fatalf("expected approved approval, got %s", after.Approvals[0].Status)
	}
}

func TestDecideApprovalRejectsInvalidDecision(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	waiting := awaitApproval(t, svc, "org-1", workspace.ID, env.ID)
	if _, err := svc.DecideApproval(ctx, "org-1", waiting.Approvals[0].ID, "maybe", "", domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"}); err == nil {
		t.Fatalf("expected invalid decision to fail")
	}
	after, err := svc.GetRunEnvelope(ctx, "org-1", waiting.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if after.Approvals[0].Status != "pending" {
		t.Fatalf("invalid decision must not persist, approval status=%s", after.Approvals[0].Status)
	}
}

func eventByKind(t *testing.T, events []domain.AuditEvent, kind string) domain.AuditEvent {
	t.Helper()
	for _, event := range events {
		if event.Kind == kind {
			return event
		}
	}
	t.Fatalf("no %q audit event found among %d events", kind, len(events))
	return domain.AuditEvent{}
}

// A decided approval must attribute the decision to whoever made it — both as
// a first-class field on the approval and in the immutable audit trail — so a
// control plane can answer "who approved this write?". The initiating user must
// likewise survive on the run.created event.
func TestApprovalDecisionRecordsDecider(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	run, err := svc.CreateRun(ctx, "org-1", workspace.ID, env.ID, "release-coordination",
		domain.Actor{Surface: "mcp", Agent: "claude", User: "initiator@test.local"}, nil)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process: %v", err)
	}
	waiting, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if waiting.Run.Status != "awaiting_approval" || len(waiting.Approvals) != 1 {
		t.Fatalf("expected one pending approval, got status=%s approvals=%d", waiting.Run.Status, len(waiting.Approvals))
	}
	if waiting.Approvals[0].DecidedBy != "" {
		t.Fatalf("a pending approval must not carry a decider, got %q", waiting.Approvals[0].DecidedBy)
	}

	// The initiating human is preserved on run.created, not just surface/agent.
	created := eventByKind(t, waiting.Events, "run.created")
	if created.Payload["user"] != "initiator@test.local" {
		t.Fatalf("run.created must record the initiating user, got %v", created.Payload["user"])
	}

	decided, err := svc.DecideApproval(ctx, "org-1", waiting.Approvals[0].ID, "approve", "ship it",
		domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if decided.Approvals[0].DecidedBy != "approver@test.local" {
		t.Fatalf("expected approval attributed to approver, got %q", decided.Approvals[0].DecidedBy)
	}

	approved := eventByKind(t, decided.Events, "approval.approved")
	if approved.Payload["decided_by"] != "approver@test.local" {
		t.Fatalf("approval.approved must record decided_by, got %v", approved.Payload["decided_by"])
	}
	if approved.Payload["surface"] != "console" || approved.Payload["agent"] != "human" {
		t.Fatalf("approval.approved must record decider surface/agent, got surface=%v agent=%v", approved.Payload["surface"], approved.Payload["agent"])
	}
}

// A rejection terminates the run, so it must be recorded by a run.rejected
// event the same way completion emits run.completed and failure emits
// run.failed — every terminal transition is uniformly represented, so a run's
// outcome is recoverable from run.* events alone. The event names the gate that
// stopped the run and who rejected it, alongside the approval-scoped event.
func TestRejectedRunEmitsTerminalRunEvent(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	waiting := awaitApproval(t, svc, "org-1", workspace.ID, env.ID)
	approval := pendingApproval(t, waiting)

	rejected, err := svc.DecideApproval(ctx, "org-1", approval.ID, "reject", "not safe",
		domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Run.Status != "rejected" {
		t.Fatalf("expected rejected run, got %s", rejected.Run.Status)
	}

	// The approval-scoped event is still present; run.rejected complements it.
	eventByKind(t, rejected.Events, "approval.rejected")

	event := eventByKind(t, rejected.Events, "run.rejected")
	if event.Payload["approval_id"] != approval.ID {
		t.Fatalf("run.rejected must name the rejecting approval, got %v", event.Payload["approval_id"])
	}
	if event.Payload["decided_by"] != "approver@test.local" {
		t.Fatalf("run.rejected must record who rejected, got %v", event.Payload["decided_by"])
	}
	// JSON round-trips numbers as float64; the in-memory repo keeps the int.
	if got := event.Payload["step_index"]; got != approval.StepIndex && got != float64(approval.StepIndex) {
		t.Fatalf("run.rejected must name the gated step index (%d), got %v", approval.StepIndex, got)
	}

	// run.rejected is the run's only terminal event: a rejected run must not also
	// carry run.completed or run.failed, so each terminal state stays distinct.
	for _, e := range rejected.Events {
		if e.Kind == "run.completed" || e.Kind == "run.failed" {
			t.Fatalf("a rejected run must not also carry %s", e.Kind)
		}
	}
}

// When the deciding actor carries no user identity (e.g. a pure agent with no
// X-Actor-User), the decision is still attributed — falling back to the agent.
func TestDecideApprovalFallsBackToAgentWhenNoUser(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	waiting := awaitApproval(t, svc, "org-1", workspace.ID, env.ID)
	decided, err := svc.DecideApproval(ctx, "org-1", waiting.Approvals[0].ID, "reject", "not now",
		domain.Actor{Surface: "mcp", Agent: "codex"})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if decided.Approvals[0].DecidedBy != "codex" {
		t.Fatalf("expected fallback to agent, got %q", decided.Approvals[0].DecidedBy)
	}
}

func newServiceWithCatalog(t *testing.T, cat catalog.Catalog) (*service.ControlPlane, *repo.Memory) {
	t.Helper()
	store, err := storage.NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	mem := repo.NewMemory()
	svc := service.New(mem, store, cat, domain.Org{
		ID:        "org-1",
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: time.Now().UTC(),
	})
	if err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return svc, mem
}

func pendingApproval(t *testing.T, env domain.RunEnvelope) domain.ApprovalRequest {
	t.Helper()
	for _, approval := range env.Approvals {
		if approval.Status == "pending" {
			return approval
		}
	}
	t.Fatalf("no pending approval in envelope (have %d approvals)", len(env.Approvals))
	return domain.ApprovalRequest{}
}

// A runbook with two approval steps must gate on each of them; the first
// decision must not satisfy the second gate.
func TestRunbookWithTwoApprovalStepsGatesTwice(t *testing.T) {
	cat := catalog.Catalog{
		Version: 1,
		Runbooks: []domain.Runbook{{
			Slug:             "dual-gate",
			Title:            "Dual Gate",
			Family:           "test",
			ApprovalRequired: true,
			Steps: []domain.RunbookStep{
				{Slug: "draft", Kind: "artifact"},
				{Slug: "first-gate", Kind: "approval"},
				{Slug: "apply", Kind: "write"},
				{Slug: "second-gate", Kind: "approval"},
				{Slug: "finalize", Kind: "write"},
			},
		}},
	}
	svc, _ := newServiceWithCatalog(t, cat)
	ctx := context.Background()
	workspace, err := svc.CreateWorkspace(ctx, "org-1", "Ops", "ops", "")
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	run, err := svc.CreateRun(ctx, "org-1", workspace.ID, "", "dual-gate", domain.Actor{Surface: "test", Agent: "test"}, nil)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process to first gate: %v", err)
	}
	atFirst, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if atFirst.Run.Status != "awaiting_approval" || atFirst.Run.CurrentStep != 1 {
		t.Fatalf("expected run parked at step 1, got status=%s step=%d", atFirst.Run.Status, atFirst.Run.CurrentStep)
	}
	first := pendingApproval(t, atFirst)
	if first.StepIndex != 1 {
		t.Fatalf("expected first approval scoped to step 1, got %d", first.StepIndex)
	}

	if _, err := svc.DecideApproval(ctx, "org-1", first.ID, "approve", "gate one", domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"}); err != nil {
		t.Fatalf("approve first gate: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process to second gate: %v", err)
	}
	atSecond, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if atSecond.Run.Status != "awaiting_approval" || atSecond.Run.CurrentStep != 3 {
		t.Fatalf("second gate must park the run again, got status=%s step=%d", atSecond.Run.Status, atSecond.Run.CurrentStep)
	}
	second := pendingApproval(t, atSecond)
	if second.StepIndex != 3 {
		t.Fatalf("expected second approval scoped to step 3, got %d", second.StepIndex)
	}
	if second.ID == first.ID {
		t.Fatalf("second gate must create its own approval request")
	}

	if _, err := svc.DecideApproval(ctx, "org-1", second.ID, "approve", "gate two", domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"}); err != nil {
		t.Fatalf("approve second gate: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process to completion: %v", err)
	}
	done, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if done.Run.Status != "completed" {
		t.Fatalf("expected completed run, got %s", done.Run.Status)
	}
}

// Approvals persisted before step scoping carry step_index 0; the worker must
// still honor them for the runbook's first approval step after an upgrade.
func TestLegacyApprovalWithoutStepIndexIsStillConsumed(t *testing.T) {
	cat, err := catalog.Load("../../../../catalog/runbooks.json")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	svc, mem := newServiceWithCatalog(t, cat)
	ctx := context.Background()
	workspace, err := svc.CreateWorkspace(ctx, "org-1", "Ops", "ops", "")
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	// release-coordination parks at step 2 (request-rollout-approval). Seed a
	// pre-upgrade run that was already approved and re-queued, plus its
	// legacy approval with the zero step index.
	now := time.Now().UTC()
	run := domain.TaskRun{
		ID:                 "legacy-run",
		OrgID:              "org-1",
		WorkspaceID:        workspace.ID,
		RunbookSlug:        "release-coordination",
		Status:             "queued",
		CurrentStep:        2,
		ApprovalState:      "approved",
		RequestedBySurface: "api",
		RequestedByAgent:   "human",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if _, err := mem.CreateTaskRun(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if _, err := mem.CreateApproval(ctx, domain.ApprovalRequest{
		ID:          "legacy-approval",
		RunID:       run.ID,
		WorkspaceID: workspace.ID,
		StepIndex:   0,
		Status:      "approved",
		Reason:      "legacy",
		CreatedAt:   now,
		DecidedAt:   now,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process legacy run: %v", err)
	}
	after, err := svc.GetRunEnvelope(ctx, "org-1", run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if after.Run.Status != "completed" {
		t.Fatalf("legacy approval must satisfy the gate, got status=%s", after.Run.Status)
	}
	if len(after.Approvals) != 1 {
		t.Fatalf("no new approval may be created for a legacy-approved run, got %d", len(after.Approvals))
	}
}

// A run whose runbook has been removed from the catalog must not surface an
// empty Runbook{} masquerading as a real definition.
func TestRunEnvelopeOmitsRunbookMissingFromCatalog(t *testing.T) {
	cat, err := catalog.Load("../../../../catalog/runbooks.json")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	svc, mem := newServiceWithCatalog(t, cat)
	ctx := context.Background()
	workspace, err := svc.CreateWorkspace(ctx, "org-1", "Ops", "ops", "")
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	now := time.Now().UTC()
	if _, err := mem.CreateTaskRun(ctx, domain.TaskRun{
		ID:          "orphan-run",
		OrgID:       "org-1",
		WorkspaceID: workspace.ID,
		RunbookSlug: "retired-runbook",
		Status:      "completed",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	envelope, err := svc.GetRunEnvelope(ctx, "org-1", "orphan-run")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if envelope.Runbook != nil {
		t.Fatalf("expected nil runbook for retired slug, got %+v", envelope.Runbook)
	}
}

func TestMissingResourcesWrapErrNotFound(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	if _, err := svc.CreateRun(ctx, "org-1", workspace.ID, env.ID, "no-such-runbook", domain.Actor{}, nil); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown runbook, got %v", err)
	}
	if _, err := svc.GetRunEnvelope(ctx, "org-1", "no-such-run"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown run, got %v", err)
	}
	if _, err := svc.GetArtifactDocument(ctx, "org-1", "no-such-artifact"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown artifact, got %v", err)
	}
}

// The seeded write.* policy must make the worker refuse a write that has no
// satisfied approval gate. The catalog validator rejects such a runbook at boot,
// so this injects one directly to stand in for any path that reached the worker
// without that check — the platform contract that external writes stop for
// approval must still hold at execution time, not only at boot.
func TestPolicyRefusesUngatedWrite(t *testing.T) {
	cat := catalog.Catalog{
		Version: 1,
		Runbooks: []domain.Runbook{{
			Slug:             "ungated",
			Title:            "Ungated Write",
			Family:           "test",
			ApprovalRequired: false,
			Steps: []domain.RunbookStep{
				{Slug: "collect", Kind: "read"},
				{Slug: "apply", Kind: "write"},
			},
		}},
	}
	svc, _ := newServiceWithCatalog(t, cat)
	ctx := context.Background()
	workspace, err := svc.CreateWorkspace(ctx, "org-1", "Ops", "ops", "")
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	run, err := svc.CreateRun(ctx, "org-1", workspace.ID, "", "ungated", domain.Actor{Surface: "test", Agent: "test"}, nil)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err == nil {
		t.Fatalf("expected the ungated write to fail the run")
	}

	after, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if after.Run.Status != "failed" {
		t.Fatalf("expected failed run, got %s", after.Run.Status)
	}
	for _, e := range after.Events {
		if e.Kind == "step.write" {
			t.Fatalf("the write must not execute when no approval gate is satisfied")
		}
	}
	violation := eventByKind(t, after.Events, "policy.violation")
	if violation.Payload["policy"] != "approval-required-for-write" {
		t.Fatalf("policy.violation must cite the governing rule, got %v", violation.Payload["policy"])
	}
	if violation.Payload["action"] != "write.apply" {
		t.Fatalf("policy.violation must cite the blocked action, got %v", violation.Payload["action"])
	}
}

// A run that fails partway through must record the step it actually reached, not
// reset current_step to its claim-time value. The worker advances current_step
// as it executes, so a failed run's terminal record has to reflect that progress
// or it contradicts the audit trail (a write that failed at step 1 would
// misreport as failing at step 0). The run.failed event names the failing step.
func TestFailedRunPreservesProgressStep(t *testing.T) {
	cat := catalog.Catalog{
		Version: 1,
		Runbooks: []domain.Runbook{{
			Slug:             "ungated",
			Title:            "Ungated Write",
			Family:           "test",
			ApprovalRequired: false,
			Steps: []domain.RunbookStep{
				{Slug: "collect", Kind: "read"}, // index 0
				{Slug: "apply", Kind: "write"},  // index 1 — fails the policy gate here
			},
		}},
	}
	svc, _ := newServiceWithCatalog(t, cat)
	ctx := context.Background()
	workspace, err := svc.CreateWorkspace(ctx, "org-1", "Ops", "ops", "")
	if err != nil {
		t.Fatalf("workspace: %v", err)
	}

	run, err := svc.CreateRun(ctx, "org-1", workspace.ID, "", "ungated", domain.Actor{Surface: "test", Agent: "test"}, nil)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err == nil {
		t.Fatalf("expected the ungated write to fail the run")
	}

	after, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if after.Run.Status != "failed" {
		t.Fatalf("expected failed run, got %s", after.Run.Status)
	}
	if after.Run.CurrentStep != 1 {
		t.Fatalf("failed run must retain the step it reached (1, the write), got %d", after.Run.CurrentStep)
	}

	failed := eventByKind(t, after.Events, "run.failed")
	// JSON round-trips numbers as float64; the in-memory repo keeps the int.
	if got := failed.Payload["current_step"]; got != 1 && got != float64(1) {
		t.Fatalf("run.failed must record the failing step index, got %v", got)
	}
	if failed.Payload["step"] != "apply" {
		t.Fatalf("run.failed must name the failing step slug, got %v", failed.Payload["step"])
	}
}

// A gated write whose approval was granted still executes: the policy check is
// satisfied by the approved gate, so enforcement never blocks the happy path.
func TestPolicyAllowsApprovedWrite(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	run, err := svc.CreateRun(ctx, "org-1", workspace.ID, env.ID, "release-coordination", domain.Actor{Surface: "mcp", Agent: "claude"}, nil)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process to gate: %v", err)
	}
	waiting, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if _, err := svc.DecideApproval(ctx, "org-1", pendingApproval(t, waiting).ID, "approve", "ship it",
		domain.Actor{Surface: "console", Agent: "human", User: "approver@test.local"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process after approval: %v", err)
	}
	done, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if done.Run.Status != "completed" {
		t.Fatalf("approved write must complete the run, got %s", done.Run.Status)
	}
	eventByKind(t, done.Events, "step.write") // the write executed
}

// An approval gate must cite the policy that mandates it, both in the
// operator-visible reason and in the approval.requested audit payload, so the
// answer to "why am I being asked to approve this?" is recorded.
func TestApprovalGateCitesGoverningPolicy(t *testing.T) {
	svc := newService(t)
	workspace, env := setupWorkspace(t, svc)
	ctx := context.Background()

	run, err := svc.CreateRun(ctx, "org-1", workspace.ID, env.ID, "release-coordination", domain.Actor{Surface: "mcp", Agent: "claude"}, nil)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process: %v", err)
	}
	waiting, err := svc.GetRunEnvelope(ctx, "org-1", run.Run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	approval := pendingApproval(t, waiting)
	if !strings.Contains(approval.Reason, "approval-required-for-write") {
		t.Fatalf("approval reason must cite the governing policy, got %q", approval.Reason)
	}
	if !strings.Contains(approval.Reason, "write.execute-rollout") {
		t.Fatalf("approval reason must cite the guarded write action, got %q", approval.Reason)
	}
	requested := eventByKind(t, waiting.Events, "approval.requested")
	if requested.Payload["policy"] != "approval-required-for-write" {
		t.Fatalf("approval.requested must record the policy, got %v", requested.Payload["policy"])
	}
	if requested.Payload["action"] != "write.execute-rollout" {
		t.Fatalf("approval.requested must record the guarded action, got %v", requested.Payload["action"])
	}
}

// ListPolicies returns the seeded global rule for any workspace and is tenant
// scoped like every other workspace-bound list.
func TestListPoliciesReturnsSeededRuleAndIsTenantScoped(t *testing.T) {
	svc := newService(t)
	workspace, _ := setupWorkspace(t, svc)
	ctx := context.Background()

	policies, err := svc.ListPolicies(ctx, "org-1", workspace.ID)
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}
	found := false
	for _, p := range policies {
		if p.Name == "approval-required-for-write" && p.ActionPattern == "write.*" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the seeded global policy, got %+v", policies)
	}
	if _, err := svc.ListPolicies(ctx, "org-2", workspace.ID); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for cross-org access, got %v", err)
	}
}
