package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	db *pgxpool.Pool
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &Postgres{db: db}, nil
}

func (p *Postgres) Close() {
	p.db.Close()
}

// mapNotFound converts pgx's no-rows error into the backend-agnostic
// repo.ErrNotFound wrapper so the API layer can map it to HTTP 404.
func mapNotFound(entity string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s %w", entity, ErrNotFound)
	}
	return err
}

func (p *Postgres) EnsureSchema(ctx context.Context) error {
	conn, err := p.db.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(42424242)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(42424242)`)

	schema := `
CREATE TABLE IF NOT EXISTS orgs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  role TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_users_org_id ON users(org_id);

CREATE TABLE IF NOT EXISTS auth_tokens (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  token_preview TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_user_id ON auth_tokens(user_id);

CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workspaces_org_id ON workspaces(org_id);

CREATE TABLE IF NOT EXISTS environments (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  kind TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_environments_workspace_id ON environments(workspace_id);

CREATE TABLE IF NOT EXISTS tool_connections (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tool_connections_workspace_id ON tool_connections(workspace_id);

CREATE TABLE IF NOT EXISTS task_runs (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  environment_id TEXT NOT NULL DEFAULT '',
  runbook_slug TEXT NOT NULL,
  status TEXT NOT NULL,
  current_step INTEGER NOT NULL DEFAULT 0,
  approval_state TEXT NOT NULL DEFAULT '',
  requested_by_surface TEXT NOT NULL,
  requested_by_agent TEXT NOT NULL,
  context_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_task_runs_workspace_id ON task_runs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_task_runs_status ON task_runs(status);

CREATE TABLE IF NOT EXISTS artifacts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES task_runs(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  content_type TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  created_by_surface TEXT NOT NULL,
  created_by_agent TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_artifacts_run_id ON artifacts(run_id);

CREATE TABLE IF NOT EXISTS approval_requests (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES task_runs(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  reason TEXT NOT NULL,
  decision_note TEXT NOT NULL DEFAULT '',
  requested_by_surface TEXT NOT NULL,
  requested_by_agent TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  decided_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_approval_requests_workspace_id ON approval_requests(workspace_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_run_id ON approval_requests(run_id);

CREATE TABLE IF NOT EXISTS policy_rules (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  action_pattern TEXT NOT NULL,
  approval_required BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_policy_rules_workspace_id ON policy_rules(workspace_id);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL DEFAULT '',
  approval_request_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  message TEXT NOT NULL,
  payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_events_workspace_id ON audit_events(workspace_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_run_id ON audit_events(run_id);
`
	_, err = conn.Exec(ctx, schema)
	return err
}

func (p *Postgres) EnsureDefaultOrg(ctx context.Context, org domain.Org) error {
	_, err := p.db.Exec(ctx, `
INSERT INTO orgs(id, name, slug, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO NOTHING
`, org.ID, org.Name, org.Slug, org.CreatedAt)
	return err
}

func (p *Postgres) GetOrg(ctx context.Context, id string) (domain.Org, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, name, slug, created_at
FROM orgs WHERE id = $1
`, id)
	out, err := scanOrg(row)
	return out, mapNotFound("org", err)
}

func (p *Postgres) EnsureDefaultUser(ctx context.Context, user domain.UserRecord) error {
	_, err := p.db.Exec(ctx, `
INSERT INTO users(id, org_id, email, display_name, role, password_hash, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (email) DO NOTHING
`, user.ID, user.OrgID, user.Email, user.DisplayName, user.Role, user.PasswordHash, user.CreatedAt)
	return err
}

func (p *Postgres) GetUserByEmail(ctx context.Context, email string) (domain.UserRecord, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, org_id, email, display_name, role, password_hash, created_at
FROM users WHERE email = $1
`, email)
	out, err := scanUser(row)
	return out, mapNotFound("user", err)
}

func (p *Postgres) GetUserByID(ctx context.Context, id string) (domain.UserRecord, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, org_id, email, display_name, role, password_hash, created_at
FROM users WHERE id = $1
`, id)
	out, err := scanUser(row)
	return out, mapNotFound("user", err)
}

func (p *Postgres) CreateAuthToken(ctx context.Context, token domain.AuthTokenRecord) (domain.AuthTokenRecord, error) {
	var lastUsedAt any
	if !token.LastUsedAt.IsZero() {
		lastUsedAt = token.LastUsedAt
	}
	_, err := p.db.Exec(ctx, `
INSERT INTO auth_tokens(id, org_id, user_id, label, token_hash, token_preview, created_at, last_used_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, token.ID, token.OrgID, token.UserID, token.Label, token.TokenHash, token.TokenPreview, token.CreatedAt, lastUsedAt)
	return token, err
}

func (p *Postgres) GetAuthTokenByHash(ctx context.Context, tokenHash string) (domain.AuthTokenRecord, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, org_id, user_id, label, token_hash, token_preview, created_at, last_used_at
FROM auth_tokens WHERE token_hash = $1
`, tokenHash)
	out, err := scanAuthToken(row)
	return out, mapNotFound("auth token", err)
}

func (p *Postgres) TouchAuthToken(ctx context.Context, id string, token domain.AuthToken) error {
	var lastUsedAt any
	if !token.LastUsedAt.IsZero() {
		lastUsedAt = token.LastUsedAt
	}
	_, err := p.db.Exec(ctx, `
UPDATE auth_tokens
SET label = $2, token_preview = $3, last_used_at = $4
WHERE id = $1
`, id, token.Label, token.TokenPreview, lastUsedAt)
	return err
}

func (p *Postgres) ListWorkspaces(ctx context.Context, orgID string) ([]domain.Workspace, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, org_id, name, slug, description, created_at
FROM workspaces
WHERE org_id = $1
ORDER BY created_at ASC
`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanWorkspace)
}

func (p *Postgres) GetWorkspace(ctx context.Context, id string) (domain.Workspace, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, org_id, name, slug, description, created_at
FROM workspaces WHERE id = $1
`, id)
	out, err := scanWorkspace(row)
	return out, mapNotFound("workspace", err)
}

func (p *Postgres) CreateWorkspace(ctx context.Context, workspace domain.Workspace) (domain.Workspace, error) {
	_, err := p.db.Exec(ctx, `
INSERT INTO workspaces(id, org_id, name, slug, description, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
`, workspace.ID, workspace.OrgID, workspace.Name, workspace.Slug, workspace.Description, workspace.CreatedAt)
	return workspace, err
}

func (p *Postgres) ListEnvironments(ctx context.Context, workspaceID string) ([]domain.Environment, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, workspace_id, name, slug, kind, created_at
FROM environments
WHERE workspace_id = $1
ORDER BY created_at ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanEnvironment)
}

func (p *Postgres) GetEnvironment(ctx context.Context, id string) (domain.Environment, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, workspace_id, name, slug, kind, created_at
FROM environments WHERE id = $1
`, id)
	out, err := scanEnvironment(row)
	return out, mapNotFound("environment", err)
}

func (p *Postgres) CreateEnvironment(ctx context.Context, env domain.Environment) (domain.Environment, error) {
	_, err := p.db.Exec(ctx, `
INSERT INTO environments(id, workspace_id, name, slug, kind, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
`, env.ID, env.WorkspaceID, env.Name, env.Slug, env.Kind, env.CreatedAt)
	return env, err
}

func (p *Postgres) ListToolConnections(ctx context.Context, workspaceID string) ([]domain.ToolConnection, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, workspace_id, environment_id, name, kind, config_json, created_at
FROM tool_connections
WHERE workspace_id = $1
ORDER BY created_at ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanTool)
}

func (p *Postgres) CreateToolConnection(ctx context.Context, tool domain.ToolConnection) (domain.ToolConnection, error) {
	configJSON, err := json.Marshal(tool.Config)
	if err != nil {
		return domain.ToolConnection{}, err
	}
	_, err = p.db.Exec(ctx, `
INSERT INTO tool_connections(id, workspace_id, environment_id, name, kind, config_json, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, tool.ID, tool.WorkspaceID, tool.EnvironmentID, tool.Name, tool.Kind, configJSON, tool.CreatedAt)
	return tool, err
}

func (p *Postgres) CreateTaskRun(ctx context.Context, run domain.TaskRun) (domain.TaskRun, error) {
	contextJSON, err := json.Marshal(run.Context)
	if err != nil {
		return domain.TaskRun{}, err
	}
	_, err = p.db.Exec(ctx, `
INSERT INTO task_runs(id, org_id, workspace_id, environment_id, runbook_slug, status, current_step, approval_state, requested_by_surface, requested_by_agent, context_json, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, run.ID, run.OrgID, run.WorkspaceID, run.EnvironmentID, run.RunbookSlug, run.Status, run.CurrentStep, run.ApprovalState, run.RequestedBySurface, run.RequestedByAgent, contextJSON, run.CreatedAt, run.UpdatedAt)
	return run, err
}

func (p *Postgres) ListTaskRuns(ctx context.Context, workspaceID string) ([]domain.TaskRun, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, org_id, workspace_id, environment_id, runbook_slug, status, current_step, approval_state, requested_by_surface, requested_by_agent, context_json, created_at, updated_at
FROM task_runs
WHERE workspace_id = $1
ORDER BY created_at DESC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanRun)
}

func (p *Postgres) GetTaskRun(ctx context.Context, id string) (domain.TaskRun, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, org_id, workspace_id, environment_id, runbook_slug, status, current_step, approval_state, requested_by_surface, requested_by_agent, context_json, created_at, updated_at
FROM task_runs WHERE id = $1
`, id)
	out, err := scanRun(row)
	return out, mapNotFound("task run", err)
}

func (p *Postgres) UpdateTaskRun(ctx context.Context, run domain.TaskRun) (domain.TaskRun, error) {
	contextJSON, err := json.Marshal(run.Context)
	if err != nil {
		return domain.TaskRun{}, err
	}
	_, err = p.db.Exec(ctx, `
UPDATE task_runs
SET environment_id = $2, status = $3, current_step = $4, approval_state = $5, context_json = $6, updated_at = $7
WHERE id = $1
`, run.ID, run.EnvironmentID, run.Status, run.CurrentStep, run.ApprovalState, contextJSON, run.UpdatedAt)
	return run, err
}

func (p *Postgres) ClaimQueuedRun(ctx context.Context) (domain.TaskRun, bool, error) {
	row := p.db.QueryRow(ctx, `
WITH next_run AS (
  SELECT id
  FROM task_runs
  WHERE status = 'queued'
  ORDER BY created_at ASC
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
UPDATE task_runs t
SET status = 'running', updated_at = NOW()
FROM next_run
WHERE t.id = next_run.id
RETURNING t.id, t.org_id, t.workspace_id, t.environment_id, t.runbook_slug, t.status, t.current_step, t.approval_state, t.requested_by_surface, t.requested_by_agent, t.context_json, t.created_at, t.updated_at
`)
	run, err := scanRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.TaskRun{}, false, nil
		}
		return domain.TaskRun{}, false, err
	}
	return run, true, nil
}

func (p *Postgres) CreateArtifact(ctx context.Context, artifact domain.Artifact) (domain.Artifact, error) {
	_, err := p.db.Exec(ctx, `
INSERT INTO artifacts(id, run_id, workspace_id, kind, content_type, storage_key, created_by_surface, created_by_agent, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, artifact.ID, artifact.RunID, artifact.WorkspaceID, artifact.Kind, artifact.ContentType, artifact.StorageKey, artifact.CreatedBySurface, artifact.CreatedByAgent, artifact.CreatedAt)
	return artifact, err
}

func (p *Postgres) ListArtifactsByRun(ctx context.Context, runID string) ([]domain.Artifact, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, run_id, workspace_id, kind, content_type, storage_key, created_by_surface, created_by_agent, created_at
FROM artifacts
WHERE run_id = $1
ORDER BY created_at ASC
`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanArtifact)
}

func (p *Postgres) GetArtifact(ctx context.Context, id string) (domain.Artifact, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, run_id, workspace_id, kind, content_type, storage_key, created_by_surface, created_by_agent, created_at
FROM artifacts WHERE id = $1
`, id)
	out, err := scanArtifact(row)
	return out, mapNotFound("artifact", err)
}

func (p *Postgres) CreateApproval(ctx context.Context, approval domain.ApprovalRequest) (domain.ApprovalRequest, error) {
	_, err := p.db.Exec(ctx, `
INSERT INTO approval_requests(id, run_id, workspace_id, status, reason, decision_note, requested_by_surface, requested_by_agent, created_at, decided_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL)
`, approval.ID, approval.RunID, approval.WorkspaceID, approval.Status, approval.Reason, approval.DecisionNote, approval.RequestedBySurface, approval.RequestedByAgent, approval.CreatedAt)
	return approval, err
}

func (p *Postgres) ListApprovals(ctx context.Context, workspaceID string) ([]domain.ApprovalRequest, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, run_id, workspace_id, status, reason, decision_note, requested_by_surface, requested_by_agent, created_at, decided_at
FROM approval_requests
WHERE workspace_id = $1
ORDER BY created_at DESC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanApproval)
}

func (p *Postgres) GetApproval(ctx context.Context, id string) (domain.ApprovalRequest, error) {
	row := p.db.QueryRow(ctx, `
SELECT id, run_id, workspace_id, status, reason, decision_note, requested_by_surface, requested_by_agent, created_at, decided_at
FROM approval_requests WHERE id = $1
`, id)
	out, err := scanApproval(row)
	return out, mapNotFound("approval", err)
}

func (p *Postgres) UpdateApproval(ctx context.Context, approval domain.ApprovalRequest) (domain.ApprovalRequest, error) {
	var decidedAt any
	if !approval.DecidedAt.IsZero() {
		decidedAt = approval.DecidedAt
	}
	_, err := p.db.Exec(ctx, `
UPDATE approval_requests
SET status = $2, decision_note = $3, decided_at = $4
WHERE id = $1
`, approval.ID, approval.Status, approval.DecisionNote, decidedAt)
	return approval, err
}

func (p *Postgres) ListPolicies(ctx context.Context, workspaceID string) ([]domain.PolicyRule, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, workspace_id, name, action_pattern, approval_required, created_at
FROM policy_rules
WHERE workspace_id = '' OR workspace_id = $1
ORDER BY created_at ASC
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanPolicy)
}

func (p *Postgres) CreatePolicy(ctx context.Context, rule domain.PolicyRule) (domain.PolicyRule, error) {
	_, err := p.db.Exec(ctx, `
INSERT INTO policy_rules(id, workspace_id, name, action_pattern, approval_required, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
`, rule.ID, rule.WorkspaceID, rule.Name, rule.ActionPattern, rule.ApprovalRequired, rule.CreatedAt)
	return rule, err
}

func (p *Postgres) CreateAuditEvent(ctx context.Context, event domain.AuditEvent) (domain.AuditEvent, error) {
	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return domain.AuditEvent{}, err
	}
	_, err = p.db.Exec(ctx, `
INSERT INTO audit_events(id, org_id, workspace_id, run_id, approval_request_id, kind, message, payload_json, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, event.ID, event.OrgID, event.WorkspaceID, event.RunID, event.ApprovalRequestID, event.Kind, event.Message, payloadJSON, event.CreatedAt)
	return event, err
}

func (p *Postgres) ListAuditEvents(ctx context.Context, workspaceID, runID string) ([]domain.AuditEvent, error) {
	rows, err := p.db.Query(ctx, `
SELECT id, org_id, workspace_id, run_id, approval_request_id, kind, message, payload_json, created_at
FROM audit_events
WHERE workspace_id = $1 AND ($2 = '' OR run_id = $2)
ORDER BY created_at ASC
`, workspaceID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, scanAuditEvent)
}

type scanner interface {
	Scan(dest ...any) error
}

func collectRows[T any](rows pgx.Rows, scan func(scanner) (T, error)) ([]T, error) {
	out := make([]T, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanOrg(row scanner) (domain.Org, error) {
	var out domain.Org
	err := row.Scan(&out.ID, &out.Name, &out.Slug, &out.CreatedAt)
	return out, err
}

func scanUser(row scanner) (domain.UserRecord, error) {
	var out domain.UserRecord
	err := row.Scan(&out.ID, &out.OrgID, &out.Email, &out.DisplayName, &out.Role, &out.PasswordHash, &out.CreatedAt)
	return out, err
}

func scanAuthToken(row scanner) (domain.AuthTokenRecord, error) {
	var (
		out        domain.AuthTokenRecord
		lastUsedAt *time.Time
	)
	err := row.Scan(&out.ID, &out.OrgID, &out.UserID, &out.Label, &out.TokenHash, &out.TokenPreview, &out.CreatedAt, &lastUsedAt)
	if err != nil {
		return out, err
	}
	if lastUsedAt != nil {
		out.LastUsedAt = *lastUsedAt
	}
	return out, nil
}

func scanWorkspace(row scanner) (domain.Workspace, error) {
	var out domain.Workspace
	err := row.Scan(&out.ID, &out.OrgID, &out.Name, &out.Slug, &out.Description, &out.CreatedAt)
	return out, err
}

func scanEnvironment(row scanner) (domain.Environment, error) {
	var out domain.Environment
	err := row.Scan(&out.ID, &out.WorkspaceID, &out.Name, &out.Slug, &out.Kind, &out.CreatedAt)
	return out, err
}

func scanTool(row scanner) (domain.ToolConnection, error) {
	var (
		out       domain.ToolConnection
		configRaw []byte
	)
	err := row.Scan(&out.ID, &out.WorkspaceID, &out.EnvironmentID, &out.Name, &out.Kind, &configRaw, &out.CreatedAt)
	if err != nil {
		return out, err
	}
	out.Config = map[string]any{}
	if len(configRaw) > 0 {
		if err := json.Unmarshal(configRaw, &out.Config); err != nil {
			return out, err
		}
	}
	return out, nil
}

func scanRun(row scanner) (domain.TaskRun, error) {
	var (
		out        domain.TaskRun
		contextRaw []byte
	)
	err := row.Scan(&out.ID, &out.OrgID, &out.WorkspaceID, &out.EnvironmentID, &out.RunbookSlug, &out.Status, &out.CurrentStep, &out.ApprovalState, &out.RequestedBySurface, &out.RequestedByAgent, &contextRaw, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return out, err
	}
	out.Context = map[string]any{}
	if len(contextRaw) > 0 {
		if err := json.Unmarshal(contextRaw, &out.Context); err != nil {
			return out, err
		}
	}
	return out, nil
}

func scanArtifact(row scanner) (domain.Artifact, error) {
	var out domain.Artifact
	err := row.Scan(&out.ID, &out.RunID, &out.WorkspaceID, &out.Kind, &out.ContentType, &out.StorageKey, &out.CreatedBySurface, &out.CreatedByAgent, &out.CreatedAt)
	return out, err
}

func scanApproval(row scanner) (domain.ApprovalRequest, error) {
	var (
		out       domain.ApprovalRequest
		decidedAt *time.Time
	)
	err := row.Scan(&out.ID, &out.RunID, &out.WorkspaceID, &out.Status, &out.Reason, &out.DecisionNote, &out.RequestedBySurface, &out.RequestedByAgent, &out.CreatedAt, &decidedAt)
	if err != nil {
		return out, err
	}
	if decidedAt != nil {
		out.DecidedAt = *decidedAt
	}
	return out, nil
}

func scanPolicy(row scanner) (domain.PolicyRule, error) {
	var out domain.PolicyRule
	err := row.Scan(&out.ID, &out.WorkspaceID, &out.Name, &out.ActionPattern, &out.ApprovalRequired, &out.CreatedAt)
	return out, err
}

func scanAuditEvent(row scanner) (domain.AuditEvent, error) {
	var (
		out        domain.AuditEvent
		payloadRaw []byte
	)
	err := row.Scan(&out.ID, &out.OrgID, &out.WorkspaceID, &out.RunID, &out.ApprovalRequestID, &out.Kind, &out.Message, &payloadRaw, &out.CreatedAt)
	if err != nil {
		return out, err
	}
	out.Payload = map[string]any{}
	if len(payloadRaw) > 0 {
		if err := json.Unmarshal(payloadRaw, &out.Payload); err != nil {
			return out, err
		}
	}
	return out, nil
}
