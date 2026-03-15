package service_test

import (
	"context"
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
	if _, err := svc.DecideApproval(context.Background(), "org-1", waiting.Approvals[0].ID, "approve", "ship it"); err != nil {
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
