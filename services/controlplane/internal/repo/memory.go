package repo

import (
	"context"
	"errors"
	"sort"
	"sync"

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
	claimedRuns  map[string]bool
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
		claimedRuns:  make(map[string]bool),
	}
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
		return domain.Org{}, errors.New("org not found")
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
		return domain.UserRecord{}, errors.New("user not found")
	}
	return m.users[id], nil
}

func (m *Memory) GetUserByID(_ context.Context, id string) (domain.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, ok := m.users[id]
	if !ok {
		return domain.UserRecord{}, errors.New("user not found")
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
		return domain.AuthTokenRecord{}, errors.New("auth token not found")
	}
	return m.tokens[id], nil
}

func (m *Memory) TouchAuthToken(_ context.Context, id string, token domain.AuthToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.tokens[id]
	if !ok {
		return errors.New("auth token not found")
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) GetWorkspace(_ context.Context, id string) (domain.Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	workspace, ok := m.workspaces[id]
	if !ok {
		return domain.Workspace{}, errors.New("workspace not found")
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) GetEnvironment(_ context.Context, id string) (domain.Environment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	env, ok := m.envs[id]
	if !ok {
		return domain.Environment{}, errors.New("environment not found")
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) GetTaskRun(_ context.Context, id string) (domain.TaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[id]
	if !ok {
		return domain.TaskRun{}, errors.New("task run not found")
	}
	return run, nil
}

func (m *Memory) UpdateTaskRun(_ context.Context, run domain.TaskRun) (domain.TaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[run.ID] = run
	delete(m.claimedRuns, run.ID)
	return run, nil
}

func (m *Memory) ClaimQueuedRun(_ context.Context) (domain.TaskRun, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.runs))
	for id := range m.runs {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		run := m.runs[id]
		if run.Status == "queued" && !m.claimedRuns[id] {
			m.claimedRuns[id] = true
			return run, true, nil
		}
	}
	return domain.TaskRun{}, false, nil
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) GetArtifact(_ context.Context, id string) (domain.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	artifact, ok := m.artifacts[id]
	if !ok {
		return domain.Artifact{}, errors.New("artifact not found")
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) GetApproval(_ context.Context, id string) (domain.ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	approval, ok := m.approvals[id]
	if !ok {
		return domain.ApprovalRequest{}, errors.New("approval not found")
	}
	return approval, nil
}

func (m *Memory) UpdateApproval(_ context.Context, approval domain.ApprovalRequest) (domain.ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.approvals[approval.ID] = approval
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
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
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
