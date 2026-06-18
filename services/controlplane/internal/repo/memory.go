package repo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
)

type Memory struct {
	mu           sync.Mutex
	orgs         map[string]domain.Org
	users        map[string]domain.UserRecord
	usersByMail  map[string]string
	tokens       map[string]domain.AuthTokenRecord
	tokensByHash map[string]string
	workspaces   map[string]domain.Workspace
	envs         map[string]domain.Environment
	tools        map[string]domain.ToolConnection
	runs         map[string]domain.TaskRun
	artifacts    map[string]domain.Artifact
	approvals    map[string]domain.ApprovalRequest
	policies     map[string]domain.PolicyRule
	events       map[string]domain.AuditEvent
}

func NewMemory() *Memory {
	return &Memory{
		orgs:         make(map[string]domain.Org),
		users:        make(map[string]domain.UserRecord),
		usersByMail:  make(map[string]string),
		tokens:       make(map[string]domain.AuthTokenRecord),
		tokensByHash: make(map[string]string),
		workspaces:   make(map[string]domain.Workspace),
		envs:         make(map[string]domain.Environment),
		tools:        make(map[string]domain.ToolConnection),
		runs:         make(map[string]domain.TaskRun),
		artifacts:    make(map[string]domain.Artifact),
		approvals:    make(map[string]domain.ApprovalRequest),
		policies:     make(map[string]domain.PolicyRule),
		events:       make(map[string]domain.AuditEvent),
	}
}

// sortByCreation orders items by creation time with the ID as tiebreaker so
// list results are deterministic even when timestamps collide, mirroring the
// ORDER BY created_at, id clauses in the Postgres implementation.
func sortByCreation[T any](items []T, newestFirst bool, key func(T) (time.Time, string)) {
	sort.Slice(items, func(i, j int) bool {
		ti, idi := key(items[i])
		tj, idj := key(items[j])
		if ti.Equal(tj) {
			if newestFirst {
				return idi > idj
			}
			return idi < idj
		}
		if newestFirst {
			return ti.After(tj)
		}
		return ti.Before(tj)
	})
}

func (m *Memory) EnsureSchema(context.Context) error { return nil }

func (m *Memory) EnsureDefaultOrg(_ context.Context, org domain.Org) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgs[org.ID] = org
	return nil
}

func (m *Memory) GetOrg(_ context.Context, id string) (domain.Org, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	org, ok := m.orgs[id]
	if !ok {
		return domain.Org{}, fmt.Errorf("org %w", ErrNotFound)
	}
	return org, nil
}

func (m *Memory) EnsureDefaultUser(_ context.Context, user domain.UserRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.usersByMail[user.Email]; exists {
		return nil
	}
	m.users[user.ID] = user
	m.usersByMail[user.Email] = user.ID
	return nil
}

func (m *Memory) GetUserByEmail(_ context.Context, email string) (domain.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.usersByMail[email]
	if !ok {
		return domain.UserRecord{}, fmt.Errorf("user %w", ErrNotFound)
	}
	return m.users[id], nil
}

func (m *Memory) GetUserByID(_ context.Context, id string) (domain.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, ok := m.users[id]
	if !ok {
		return domain.UserRecord{}, fmt.Errorf("user %w", ErrNotFound)
	}
	return user, nil
}

func (m *Memory) CreateAuthToken(_ context.Context, token domain.AuthTokenRecord) (domain.AuthTokenRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[token.ID] = token
	m.tokensByHash[token.TokenHash] = token.ID
	return token, nil
}

func (m *Memory) GetAuthTokenByHash(_ context.Context, tokenHash string) (domain.AuthTokenRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.tokensByHash[tokenHash]
	if !ok {
		return domain.AuthTokenRecord{}, fmt.Errorf("auth token %w", ErrNotFound)
	}
	return m.tokens[id], nil
}

func (m *Memory) TouchAuthToken(_ context.Context, id string, token domain.AuthToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.tokens[id]
	if !ok {
		return fmt.Errorf("auth token %w", ErrNotFound)
	}
	record.AuthToken = token
	m.tokens[id] = record
	return nil
}

func (m *Memory) ListWorkspaces(_ context.Context, orgID string) ([]domain.Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Workspace, 0)
	for _, workspace := range m.workspaces {
		if workspace.OrgID == orgID {
			out = append(out, workspace)
		}
	}
	sortByCreation(out, false, func(w domain.Workspace) (time.Time, string) { return w.CreatedAt, w.ID })
	return out, nil
}

func (m *Memory) GetWorkspace(_ context.Context, id string) (domain.Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	workspace, ok := m.workspaces[id]
	if !ok {
		return domain.Workspace{}, fmt.Errorf("workspace %w", ErrNotFound)
	}
	return workspace, nil
}

func (m *Memory) CreateWorkspace(_ context.Context, workspace domain.Workspace) (domain.Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaces[workspace.ID] = workspace
	return workspace, nil
}

func (m *Memory) ListEnvironments(_ context.Context, workspaceID string) ([]domain.Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Environment, 0)
	for _, env := range m.envs {
		if env.WorkspaceID == workspaceID {
			out = append(out, env)
		}
	}
	sortByCreation(out, false, func(e domain.Environment) (time.Time, string) { return e.CreatedAt, e.ID })
	return out, nil
}

func (m *Memory) GetEnvironment(_ context.Context, id string) (domain.Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	env, ok := m.envs[id]
	if !ok {
		return domain.Environment{}, fmt.Errorf("environment %w", ErrNotFound)
	}
	return env, nil
}

func (m *Memory) CreateEnvironment(_ context.Context, env domain.Environment) (domain.Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.envs[env.ID] = env
	return env, nil
}

func (m *Memory) ListToolConnections(_ context.Context, workspaceID string) ([]domain.ToolConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ToolConnection, 0)
	for _, tool := range m.tools {
		if tool.WorkspaceID == workspaceID {
			out = append(out, tool)
		}
	}
	sortByCreation(out, false, func(tc domain.ToolConnection) (time.Time, string) { return tc.CreatedAt, tc.ID })
	return out, nil
}

func (m *Memory) CreateToolConnection(_ context.Context, tool domain.ToolConnection) (domain.ToolConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[tool.ID] = tool
	return tool, nil
}

func (m *Memory) CreateTaskRun(_ context.Context, run domain.TaskRun) (domain.TaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return run, nil
}

func (m *Memory) ListTaskRuns(_ context.Context, workspaceID string) ([]domain.TaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.TaskRun, 0)
	for _, run := range m.runs {
		if run.WorkspaceID == workspaceID {
			out = append(out, run)
		}
	}
	// Newest first, matching the Postgres implementation.
	sortByCreation(out, true, func(r domain.TaskRun) (time.Time, string) { return r.CreatedAt, r.ID })
	return out, nil
}

func (m *Memory) GetTaskRun(_ context.Context, id string) (domain.TaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[id]
	if !ok {
		return domain.TaskRun{}, fmt.Errorf("task run %w", ErrNotFound)
	}
	return run, nil
}

func (m *Memory) UpdateTaskRun(_ context.Context, run domain.TaskRun) (domain.TaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	return run, nil
}

// ClaimQueuedRun mirrors the Postgres implementation: the oldest queued run
// is atomically marked running so concurrent workers never claim it twice.
func (m *Memory) ClaimQueuedRun(_ context.Context) (domain.TaskRun, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var oldest domain.TaskRun
	found := false
	for _, run := range m.runs {
		if run.Status != "queued" {
			continue
		}
		if !found || run.CreatedAt.Before(oldest.CreatedAt) ||
			(run.CreatedAt.Equal(oldest.CreatedAt) && run.ID < oldest.ID) {
			oldest = run
			found = true
		}
	}
	if !found {
		return domain.TaskRun{}, false, nil
	}
	oldest.Status = "running"
	oldest.UpdatedAt = time.Now().UTC()
	m.runs[oldest.ID] = oldest
	return oldest, true, nil
}

func (m *Memory) CreateArtifact(_ context.Context, artifact domain.Artifact) (domain.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.artifacts[artifact.ID] = artifact
	return artifact, nil
}

func (m *Memory) ListArtifactsByRun(_ context.Context, runID string) ([]domain.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Artifact, 0)
	for _, artifact := range m.artifacts {
		if artifact.RunID == runID {
			out = append(out, artifact)
		}
	}
	sortByCreation(out, false, func(a domain.Artifact) (time.Time, string) { return a.CreatedAt, a.ID })
	return out, nil
}

func (m *Memory) GetArtifact(_ context.Context, id string) (domain.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	artifact, ok := m.artifacts[id]
	if !ok {
		return domain.Artifact{}, fmt.Errorf("artifact %w", ErrNotFound)
	}
	return artifact, nil
}

func (m *Memory) CreateApproval(_ context.Context, approval domain.ApprovalRequest) (domain.ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.approvals[approval.ID] = approval
	return approval, nil
}

func (m *Memory) ListApprovals(_ context.Context, workspaceID string) ([]domain.ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ApprovalRequest, 0)
	for _, approval := range m.approvals {
		if approval.WorkspaceID == workspaceID {
			out = append(out, approval)
		}
	}
	// Newest first, matching the Postgres implementation.
	sortByCreation(out, true, func(a domain.ApprovalRequest) (time.Time, string) { return a.CreatedAt, a.ID })
	return out, nil
}

func (m *Memory) ListApprovalsByRun(_ context.Context, runID string) ([]domain.ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ApprovalRequest, 0)
	for _, approval := range m.approvals {
		if approval.RunID == runID {
			out = append(out, approval)
		}
	}
	// Newest first, matching ListApprovals and the Postgres implementation.
	sortByCreation(out, true, func(a domain.ApprovalRequest) (time.Time, string) { return a.CreatedAt, a.ID })
	return out, nil
}

func (m *Memory) GetApproval(_ context.Context, id string) (domain.ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	approval, ok := m.approvals[id]
	if !ok {
		return domain.ApprovalRequest{}, fmt.Errorf("approval %w", ErrNotFound)
	}
	return approval, nil
}

func (m *Memory) DecideApproval(_ context.Context, id, status, note, decidedBy string, decidedAt time.Time) (domain.ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	approval, ok := m.approvals[id]
	if !ok {
		return domain.ApprovalRequest{}, fmt.Errorf("approval %w", ErrNotFound)
	}
	if approval.Status != "pending" {
		return domain.ApprovalRequest{}, ErrNotPending
	}
	approval.Status = status
	approval.DecisionNote = note
	approval.DecidedBy = decidedBy
	approval.DecidedAt = decidedAt
	m.approvals[id] = approval
	return approval, nil
}

func (m *Memory) ListPolicies(_ context.Context, workspaceID string) ([]domain.PolicyRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.PolicyRule, 0)
	for _, rule := range m.policies {
		if rule.WorkspaceID == "" || rule.WorkspaceID == workspaceID {
			out = append(out, rule)
		}
	}
	sortByCreation(out, false, func(p domain.PolicyRule) (time.Time, string) { return p.CreatedAt, p.ID })
	return out, nil
}

func (m *Memory) CreatePolicy(_ context.Context, rule domain.PolicyRule) (domain.PolicyRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[rule.ID] = rule
	return rule, nil
}

func (m *Memory) CreateAuditEvent(_ context.Context, event domain.AuditEvent) (domain.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[event.ID] = event
	return event, nil
}

func (m *Memory) ListAuditEvents(_ context.Context, workspaceID, runID string) ([]domain.AuditEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.AuditEvent, 0)
	for _, event := range m.events {
		if event.WorkspaceID != workspaceID {
			continue
		}
		if runID != "" && event.RunID != runID {
			continue
		}
		out = append(out, event)
	}
	sortByCreation(out, false, func(e domain.AuditEvent) (time.Time, string) { return e.CreatedAt, e.ID })
	return out, nil
}
