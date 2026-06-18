package domain

import "time"

type Actor struct {
	Surface string `json:"surface"`
	Agent   string `json:"agent"`
	User    string `json:"user,omitempty"`
}

type Org struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

type UserRecord struct {
	User
	PasswordHash string `json:"-"`
}

type AuthToken struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	UserID       string    `json:"user_id"`
	Label        string    `json:"label"`
	TokenPreview string    `json:"token_preview"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at,omitempty"`
}

type AuthTokenRecord struct {
	AuthToken
	TokenHash string `json:"-"`
}

type AuthIdentity struct {
	Org   Org       `json:"org"`
	User  User      `json:"user"`
	Token AuthToken `json:"token"`
}

type AuthSession struct {
	AuthIdentity
	AccessToken string `json:"access_token"`
}

type AuthPrincipal struct {
	AuthIdentity
}

type Workspace struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Environment struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Kind        string    `json:"kind"`
	CreatedAt   time.Time `json:"created_at"`
}

type ToolConnection struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspace_id"`
	EnvironmentID string         `json:"environment_id,omitempty"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Config        map[string]any `json:"config"`
	CreatedAt     time.Time      `json:"created_at"`
}

type RunbookStep struct {
	Slug string `json:"slug"`
	Kind string `json:"kind"`
}

type Runbook struct {
	Slug              string        `json:"slug"`
	Title             string        `json:"title"`
	Family            string        `json:"family"`
	Description       string        `json:"description"`
	ToolConnections   []string      `json:"tool_connections"`
	ApprovalRequired  bool          `json:"approval_required"`
	ExpectedArtifacts []string      `json:"expected_artifacts"`
	FailureModes      []string      `json:"failure_modes"`
	RollbackNotes     string        `json:"rollback_notes"`
	Steps             []RunbookStep `json:"steps"`
}

type TaskTemplate struct {
	ID          string `json:"id"`
	RunbookSlug string `json:"runbook_slug"`
	StepSlug    string `json:"step_slug"`
	Kind        string `json:"kind"`
}

type TaskRun struct {
	ID                 string         `json:"id"`
	OrgID              string         `json:"org_id"`
	WorkspaceID        string         `json:"workspace_id"`
	EnvironmentID      string         `json:"environment_id,omitempty"`
	RunbookSlug        string         `json:"runbook_slug"`
	Status             string         `json:"status"`
	CurrentStep        int            `json:"current_step"`
	ApprovalState      string         `json:"approval_state"`
	RequestedBySurface string         `json:"requested_by_surface"`
	RequestedByAgent   string         `json:"requested_by_agent"`
	Context            map[string]any `json:"context"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type Artifact struct {
	ID               string    `json:"id"`
	RunID            string    `json:"run_id"`
	WorkspaceID      string    `json:"workspace_id"`
	Kind             string    `json:"kind"`
	ContentType      string    `json:"content_type"`
	StorageKey       string    `json:"storage_key"`
	CreatedBySurface string    `json:"created_by_surface"`
	CreatedByAgent   string    `json:"created_by_agent"`
	CreatedAt        time.Time `json:"created_at"`
}

type ApprovalRequest struct {
	ID                 string    `json:"id"`
	RunID              string    `json:"run_id"`
	WorkspaceID        string    `json:"workspace_id"`
	StepIndex          int       `json:"step_index"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason"`
	DecisionNote       string    `json:"decision_note,omitempty"`
	RequestedBySurface string    `json:"requested_by_surface"`
	RequestedByAgent   string    `json:"requested_by_agent"`
	CreatedAt          time.Time `json:"created_at"`
	// DecidedBy attributes the approve/reject decision to whoever made it —
	// the deciding actor's user identity, falling back to its agent. It is
	// empty while the approval is still pending. This is the auditable answer
	// to "who approved this write?", surfaced in the run envelope and console
	// inbox rather than only buried in audit-event payloads.
	DecidedBy string    `json:"decided_by,omitempty"`
	DecidedAt time.Time `json:"decided_at,omitempty"`
}

type PolicyRule struct {
	ID               string    `json:"id"`
	WorkspaceID      string    `json:"workspace_id,omitempty"`
	Name             string    `json:"name"`
	ActionPattern    string    `json:"action_pattern"`
	ApprovalRequired bool      `json:"approval_required"`
	CreatedAt        time.Time `json:"created_at"`
}

type AuditEvent struct {
	ID                string         `json:"id"`
	OrgID             string         `json:"org_id"`
	WorkspaceID       string         `json:"workspace_id"`
	RunID             string         `json:"run_id,omitempty"`
	ApprovalRequestID string         `json:"approval_request_id,omitempty"`
	Kind              string         `json:"kind"`
	Message           string         `json:"message"`
	Payload           map[string]any `json:"payload"`
	CreatedAt         time.Time      `json:"created_at"`
}

type ArtifactDocument struct {
	Artifact Artifact `json:"artifact"`
	Content  string   `json:"content"`
}

type RunEnvelope struct {
	Run       TaskRun           `json:"run"`
	Approvals []ApprovalRequest `json:"approvals"`
	Artifacts []Artifact        `json:"artifacts"`
	Events    []AuditEvent      `json:"events"`
	Runbook   *Runbook          `json:"runbook,omitempty"`
}
