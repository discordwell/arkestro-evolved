package repo

import (
	"context"
	"errors"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
)

// ErrNotFound is wrapped by every repository implementation when a requested
// record does not exist, so callers can branch with errors.Is regardless of
// the backend (the API layer maps it to HTTP 404).
var ErrNotFound = errors.New("not found")

type Repository interface {
	EnsureSchema(context.Context) error
	EnsureDefaultOrg(context.Context, domain.Org) error
	GetOrg(context.Context, string) (domain.Org, error)

	EnsureDefaultUser(context.Context, domain.UserRecord) error
	GetUserByEmail(context.Context, string) (domain.UserRecord, error)
	GetUserByID(context.Context, string) (domain.UserRecord, error)
	CreateAuthToken(context.Context, domain.AuthTokenRecord) (domain.AuthTokenRecord, error)
	GetAuthTokenByHash(context.Context, string) (domain.AuthTokenRecord, error)
	TouchAuthToken(context.Context, string, domain.AuthToken) error

	ListWorkspaces(context.Context, string) ([]domain.Workspace, error)
	GetWorkspace(context.Context, string) (domain.Workspace, error)
	CreateWorkspace(context.Context, domain.Workspace) (domain.Workspace, error)

	ListEnvironments(context.Context, string) ([]domain.Environment, error)
	GetEnvironment(context.Context, string) (domain.Environment, error)
	CreateEnvironment(context.Context, domain.Environment) (domain.Environment, error)

	ListToolConnections(context.Context, string) ([]domain.ToolConnection, error)
	CreateToolConnection(context.Context, domain.ToolConnection) (domain.ToolConnection, error)

	CreateTaskRun(context.Context, domain.TaskRun) (domain.TaskRun, error)
	ListTaskRuns(context.Context, string) ([]domain.TaskRun, error)
	GetTaskRun(context.Context, string) (domain.TaskRun, error)
	UpdateTaskRun(context.Context, domain.TaskRun) (domain.TaskRun, error)
	ClaimQueuedRun(context.Context) (domain.TaskRun, bool, error)

	CreateArtifact(context.Context, domain.Artifact) (domain.Artifact, error)
	ListArtifactsByRun(context.Context, string) ([]domain.Artifact, error)
	GetArtifact(context.Context, string) (domain.Artifact, error)

	CreateApproval(context.Context, domain.ApprovalRequest) (domain.ApprovalRequest, error)
	ListApprovals(context.Context, string) ([]domain.ApprovalRequest, error)
	GetApproval(context.Context, string) (domain.ApprovalRequest, error)
	UpdateApproval(context.Context, domain.ApprovalRequest) (domain.ApprovalRequest, error)

	ListPolicies(context.Context, string) ([]domain.PolicyRule, error)
	CreatePolicy(context.Context, domain.PolicyRule) (domain.PolicyRule, error)

	CreateAuditEvent(context.Context, domain.AuditEvent) (domain.AuditEvent, error)
	ListAuditEvents(context.Context, string, string) ([]domain.AuditEvent, error)
}
