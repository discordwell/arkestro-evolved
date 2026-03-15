package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/catalog"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/repo"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/storage"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type ControlPlane struct {
	repo                   repo.Repository
	store                  storage.Store
	catalog                catalog.Catalog
	defaultOrg             domain.Org
	bootstrapAdminEmail    string
	bootstrapAdminName     string
	bootstrapAdminPassword string
	now                    func() time.Time
}

func New(repository repo.Repository, store storage.Store, catalog catalog.Catalog, defaultOrg domain.Org) *ControlPlane {
	return &ControlPlane{
		repo:       repository,
		store:      store,
		catalog:    catalog,
		defaultOrg: defaultOrg,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (s *ControlPlane) ConfigureBootstrapAdmin(email, password, displayName string) {
	s.bootstrapAdminEmail = strings.TrimSpace(strings.ToLower(email))
	s.bootstrapAdminPassword = password
	s.bootstrapAdminName = strings.TrimSpace(displayName)
}

func (s *ControlPlane) Bootstrap(ctx context.Context) error {
	if err := s.repo.EnsureSchema(ctx); err != nil {
		return err
	}
	if err := s.repo.EnsureDefaultOrg(ctx, s.defaultOrg); err != nil {
		return err
	}
	if s.bootstrapAdminEmail != "" && s.bootstrapAdminPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(s.bootstrapAdminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		displayName := s.bootstrapAdminName
		if displayName == "" {
			displayName = "Platform Admin"
		}
		if err := s.repo.EnsureDefaultUser(ctx, domain.UserRecord{
			User: domain.User{
				ID:          uuid.NewString(),
				OrgID:       s.defaultOrg.ID,
				Email:       s.bootstrapAdminEmail,
				DisplayName: displayName,
				Role:        "admin",
				CreatedAt:   s.now(),
			},
			PasswordHash: string(hash),
		}); err != nil {
			return err
		}
	}
	policies, err := s.repo.ListPolicies(ctx, "")
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		_, err = s.repo.CreatePolicy(ctx, domain.PolicyRule{
			ID:               uuid.NewString(),
			Name:             "approval-required-for-write",
			ActionPattern:    "write.*",
			ApprovalRequired: true,
			CreatedAt:        s.now(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ControlPlane) DefaultOrg() domain.Org { return s.defaultOrg }

func (s *ControlPlane) Login(ctx context.Context, email, password, label string) (domain.AuthSession, error) {
	user, err := s.repo.GetUserByEmail(ctx, strings.TrimSpace(strings.ToLower(email)))
	if err != nil {
		return domain.AuthSession{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return domain.AuthSession{}, ErrInvalidCredentials
	}
	return s.createSession(ctx, user, label)
}

func (s *ControlPlane) CreateToken(ctx context.Context, principal domain.AuthPrincipal, label string) (domain.AuthSession, error) {
	user, err := s.repo.GetUserByID(ctx, principal.User.ID)
	if err != nil {
		return domain.AuthSession{}, err
	}
	return s.createSession(ctx, user, label)
}

func (s *ControlPlane) Authenticate(ctx context.Context, rawToken string) (domain.AuthPrincipal, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return domain.AuthPrincipal{}, ErrUnauthorized
	}
	token, err := s.repo.GetAuthTokenByHash(ctx, hashToken(rawToken))
	if err != nil {
		return domain.AuthPrincipal{}, ErrUnauthorized
	}
	token.LastUsedAt = s.now()
	if err := s.repo.TouchAuthToken(ctx, token.ID, token.AuthToken); err != nil {
		return domain.AuthPrincipal{}, err
	}
	user, err := s.repo.GetUserByID(ctx, token.UserID)
	if err != nil {
		return domain.AuthPrincipal{}, ErrUnauthorized
	}
	org, err := s.repo.GetOrg(ctx, token.OrgID)
	if err != nil {
		return domain.AuthPrincipal{}, ErrUnauthorized
	}
	return domain.AuthPrincipal{
		AuthIdentity: domain.AuthIdentity{
			Org:   org,
			User:  user.User,
			Token: token.AuthToken,
		},
	}, nil
}

func (s *ControlPlane) GetAuthIdentity(ctx context.Context, principal domain.AuthPrincipal) (domain.AuthIdentity, error) {
	return principal.AuthIdentity, nil
}

func (s *ControlPlane) ListWorkspaces(ctx context.Context, orgID string) ([]domain.Workspace, error) {
	return s.repo.ListWorkspaces(ctx, orgID)
}

func (s *ControlPlane) CreateWorkspace(ctx context.Context, orgID, name, slug, description string) (domain.Workspace, error) {
	workspace := domain.Workspace{
		ID:          uuid.NewString(),
		OrgID:       orgID,
		Name:        strings.TrimSpace(name),
		Slug:        strings.TrimSpace(slug),
		Description: strings.TrimSpace(description),
		CreatedAt:   s.now(),
	}
	if workspace.Name == "" || workspace.Slug == "" {
		return domain.Workspace{}, errors.New("name and slug are required")
	}
	return s.repo.CreateWorkspace(ctx, workspace)
}

func (s *ControlPlane) ListEnvironments(ctx context.Context, orgID, workspaceID string) ([]domain.Environment, error) {
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.ListEnvironments(ctx, workspaceID)
}

func (s *ControlPlane) CreateEnvironment(ctx context.Context, orgID, workspaceID, name, slug, kind string) (domain.Environment, error) {
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return domain.Environment{}, err
	}
	env := domain.Environment{
		ID:          uuid.NewString(),
		WorkspaceID: workspaceID,
		Name:        strings.TrimSpace(name),
		Slug:        strings.TrimSpace(slug),
		Kind:        strings.TrimSpace(kind),
		CreatedAt:   s.now(),
	}
	if env.WorkspaceID == "" || env.Name == "" || env.Slug == "" || env.Kind == "" {
		return domain.Environment{}, errors.New("workspace_id, name, slug, and kind are required")
	}
	return s.repo.CreateEnvironment(ctx, env)
}

func (s *ControlPlane) ListToolConnections(ctx context.Context, orgID, workspaceID string) ([]domain.ToolConnection, error) {
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.ListToolConnections(ctx, workspaceID)
}

func (s *ControlPlane) CreateToolConnection(ctx context.Context, orgID, workspaceID, environmentID, name, kind string, config map[string]any) (domain.ToolConnection, error) {
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return domain.ToolConnection{}, err
	}
	if environmentID != "" {
		env, err := s.repo.GetEnvironment(ctx, environmentID)
		if err != nil {
			return domain.ToolConnection{}, err
		}
		if env.WorkspaceID != workspaceID {
			return domain.ToolConnection{}, ErrForbidden
		}
	}
	tool := domain.ToolConnection{
		ID:            uuid.NewString(),
		WorkspaceID:   workspaceID,
		EnvironmentID: environmentID,
		Name:          strings.TrimSpace(name),
		Kind:          strings.TrimSpace(kind),
		Config:        config,
		CreatedAt:     s.now(),
	}
	if tool.WorkspaceID == "" || tool.Name == "" || tool.Kind == "" {
		return domain.ToolConnection{}, errors.New("workspace_id, name, and kind are required")
	}
	return s.repo.CreateToolConnection(ctx, tool)
}

func (s *ControlPlane) ListRunbooks() []domain.Runbook {
	return append([]domain.Runbook(nil), s.catalog.Runbooks...)
}

func (s *ControlPlane) ListTaskTemplates() []domain.TaskTemplate {
	return s.catalog.TaskTemplates()
}

func (s *ControlPlane) CreateRun(ctx context.Context, orgID, workspaceID, environmentID, runbookSlug string, actor domain.Actor, contextData map[string]any) (domain.RunEnvelope, error) {
	runbook, ok := s.catalog.Runbook(runbookSlug)
	if !ok {
		return domain.RunEnvelope{}, errors.New("runbook not found")
	}
	if workspaceID == "" {
		return domain.RunEnvelope{}, errors.New("workspace_id is required")
	}
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return domain.RunEnvelope{}, err
	}
	if environmentID != "" {
		env, err := s.repo.GetEnvironment(ctx, environmentID)
		if err != nil {
			return domain.RunEnvelope{}, err
		}
		if env.WorkspaceID != workspaceID {
			return domain.RunEnvelope{}, ErrForbidden
		}
	}
	if actor.Surface == "" {
		actor.Surface = "api"
	}
	if actor.Agent == "" {
		actor.Agent = "human"
	}
	run := domain.TaskRun{
		ID:                 uuid.NewString(),
		OrgID:              orgID,
		WorkspaceID:        workspaceID,
		EnvironmentID:      environmentID,
		RunbookSlug:        runbookSlug,
		Status:             "queued",
		CurrentStep:        0,
		ApprovalState:      "pending",
		RequestedBySurface: actor.Surface,
		RequestedByAgent:   actor.Agent,
		Context:            contextData,
		CreatedAt:          s.now(),
		UpdatedAt:          s.now(),
	}
	if !runbook.ApprovalRequired {
		run.ApprovalState = "not_required"
	}
	if _, err := s.repo.CreateTaskRun(ctx, run); err != nil {
		return domain.RunEnvelope{}, err
	}
	_, _ = s.repo.CreateAuditEvent(ctx, domain.AuditEvent{
		ID:          uuid.NewString(),
		OrgID:       orgID,
		WorkspaceID: workspaceID,
		RunID:       run.ID,
		Kind:        "run.created",
		Message:     "Run created",
		Payload: map[string]any{
			"runbook_slug": runbookSlug,
			"surface":      actor.Surface,
			"agent":        actor.Agent,
		},
		CreatedAt: s.now(),
	})
	return s.GetRunEnvelope(ctx, orgID, run.ID)
}

func (s *ControlPlane) ListRuns(ctx context.Context, orgID, workspaceID string) ([]domain.TaskRun, error) {
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.ListTaskRuns(ctx, workspaceID)
}

func (s *ControlPlane) GetRunEnvelope(ctx context.Context, orgID, runID string) (domain.RunEnvelope, error) {
	run, err := s.repo.GetTaskRun(ctx, runID)
	if err != nil {
		return domain.RunEnvelope{}, err
	}
	if run.OrgID != orgID {
		return domain.RunEnvelope{}, ErrForbidden
	}
	approvals, err := s.repo.ListApprovals(ctx, run.WorkspaceID)
	if err != nil {
		return domain.RunEnvelope{}, err
	}
	filteredApprovals := make([]domain.ApprovalRequest, 0)
	for _, approval := range approvals {
		if approval.RunID == run.ID {
			filteredApprovals = append(filteredApprovals, approval)
		}
	}
	artifacts, err := s.repo.ListArtifactsByRun(ctx, run.ID)
	if err != nil {
		return domain.RunEnvelope{}, err
	}
	events, err := s.repo.ListAuditEvents(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		return domain.RunEnvelope{}, err
	}
	runbook, _ := s.catalog.Runbook(run.RunbookSlug)
	return domain.RunEnvelope{
		Run:       run,
		Approvals: filteredApprovals,
		Artifacts: artifacts,
		Events:    events,
		Runbook:   &runbook,
	}, nil
}

func (s *ControlPlane) ListArtifactsByRun(ctx context.Context, orgID, runID string) ([]domain.Artifact, error) {
	run, err := s.repo.GetTaskRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, ErrForbidden
	}
	return s.repo.ListArtifactsByRun(ctx, runID)
}

func (s *ControlPlane) GetArtifactDocument(ctx context.Context, orgID, artifactID string) (domain.ArtifactDocument, error) {
	artifact, err := s.repo.GetArtifact(ctx, artifactID)
	if err != nil {
		return domain.ArtifactDocument{}, err
	}
	run, err := s.repo.GetTaskRun(ctx, artifact.RunID)
	if err != nil {
		return domain.ArtifactDocument{}, err
	}
	if run.OrgID != orgID {
		return domain.ArtifactDocument{}, ErrForbidden
	}
	body, err := s.store.Get(ctx, artifact.StorageKey)
	if err != nil {
		return domain.ArtifactDocument{}, err
	}
	return domain.ArtifactDocument{Artifact: artifact, Content: string(body)}, nil
}

func (s *ControlPlane) ListApprovals(ctx context.Context, orgID, workspaceID string) ([]domain.ApprovalRequest, error) {
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.ListApprovals(ctx, workspaceID)
}

func (s *ControlPlane) DecideApproval(ctx context.Context, orgID, approvalID, decision, note string) (domain.RunEnvelope, error) {
	approval, err := s.repo.GetApproval(ctx, approvalID)
	if err != nil {
		return domain.RunEnvelope{}, err
	}
	if approval.Status != "pending" {
		return domain.RunEnvelope{}, errors.New("approval is not pending")
	}
	now := s.now()
	switch decision {
	case "approve":
		approval.Status = "approved"
	case "reject":
		approval.Status = "rejected"
	default:
		return domain.RunEnvelope{}, errors.New("decision must be approve or reject")
	}
	approval.DecisionNote = strings.TrimSpace(note)
	approval.DecidedAt = now
	if _, err := s.repo.UpdateApproval(ctx, approval); err != nil {
		return domain.RunEnvelope{}, err
	}
	run, err := s.repo.GetTaskRun(ctx, approval.RunID)
	if err != nil {
		return domain.RunEnvelope{}, err
	}
	if run.OrgID != orgID {
		return domain.RunEnvelope{}, ErrForbidden
	}
	if approval.Status == "approved" {
		run.Status = "queued"
		run.ApprovalState = "approved"
	} else {
		run.Status = "rejected"
		run.ApprovalState = "rejected"
	}
	run.UpdatedAt = now
	if _, err := s.repo.UpdateTaskRun(ctx, run); err != nil {
		return domain.RunEnvelope{}, err
	}
	_, _ = s.repo.CreateAuditEvent(ctx, domain.AuditEvent{
		ID:                uuid.NewString(),
		OrgID:             run.OrgID,
		WorkspaceID:       run.WorkspaceID,
		RunID:             run.ID,
		ApprovalRequestID: approval.ID,
		Kind:              "approval." + approval.Status,
		Message:           "Approval " + approval.Status,
		Payload: map[string]any{
			"decision_note": approval.DecisionNote,
		},
		CreatedAt: now,
	})
	return s.GetRunEnvelope(ctx, orgID, run.ID)
}

func (s *ControlPlane) ListAuditEvents(ctx context.Context, orgID, workspaceID, runID string) ([]domain.AuditEvent, error) {
	if _, err := s.authorizeWorkspace(ctx, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.ListAuditEvents(ctx, workspaceID, runID)
}

func (s *ControlPlane) createSession(ctx context.Context, user domain.UserRecord, label string) (domain.AuthSession, error) {
	org, err := s.repo.GetOrg(ctx, user.OrgID)
	if err != nil {
		return domain.AuthSession{}, err
	}
	tokenValue, err := randomToken()
	if err != nil {
		return domain.AuthSession{}, err
	}
	trimmedLabel := strings.TrimSpace(label)
	if trimmedLabel == "" {
		trimmedLabel = "interactive"
	}
	record := domain.AuthTokenRecord{
		AuthToken: domain.AuthToken{
			ID:           uuid.NewString(),
			OrgID:        user.OrgID,
			UserID:       user.ID,
			Label:        trimmedLabel,
			TokenPreview: previewToken(tokenValue),
			CreatedAt:    s.now(),
			LastUsedAt:   s.now(),
		},
		TokenHash: hashToken(tokenValue),
	}
	if _, err := s.repo.CreateAuthToken(ctx, record); err != nil {
		return domain.AuthSession{}, err
	}
	return domain.AuthSession{
		AuthIdentity: domain.AuthIdentity{
			Org:   org,
			User:  user.User,
			Token: record.AuthToken,
		},
		AccessToken: tokenValue,
	}, nil
}

func (s *ControlPlane) authorizeWorkspace(ctx context.Context, orgID, workspaceID string) (domain.Workspace, error) {
	if workspaceID == "" {
		return domain.Workspace{}, errors.New("workspace_id is required")
	}
	workspace, err := s.repo.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return domain.Workspace{}, err
	}
	if workspace.OrgID != orgID {
		return domain.Workspace{}, ErrForbidden
	}
	return workspace, nil
}

func (s *ControlPlane) ProcessNextRun(ctx context.Context) (bool, error) {
	run, ok, err := s.repo.ClaimQueuedRun(ctx)
	if err != nil || !ok {
		return ok, err
	}
	if err := s.processRun(ctx, run); err != nil {
		run.Status = "failed"
		run.UpdatedAt = s.now()
		_, _ = s.repo.UpdateTaskRun(ctx, run)
		_, _ = s.repo.CreateAuditEvent(ctx, domain.AuditEvent{
			ID:          uuid.NewString(),
			OrgID:       run.OrgID,
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			Kind:        "run.failed",
			Message:     err.Error(),
			Payload:     map[string]any{},
			CreatedAt:   s.now(),
		})
		return true, err
	}
	return true, nil
}

func (s *ControlPlane) processRun(ctx context.Context, run domain.TaskRun) error {
	runbook, ok := s.catalog.Runbook(run.RunbookSlug)
	if !ok {
		return errors.New("missing runbook")
	}
	if run.Status == "queued" {
		run.Status = "running"
		run.UpdatedAt = s.now()
		if _, err := s.repo.UpdateTaskRun(ctx, run); err != nil {
			return err
		}
	}
	for run.CurrentStep < len(runbook.Steps) {
		step := runbook.Steps[run.CurrentStep]
		switch step.Kind {
		case "read":
			if err := s.appendEvent(ctx, run, "step.read", "Completed read step "+step.Slug, map[string]any{"step": step.Slug}); err != nil {
				return err
			}
			run.CurrentStep++
		case "artifact":
			if _, err := s.createArtifact(ctx, run, runbook, step); err != nil {
				return err
			}
			run.CurrentStep++
		case "approval":
			approval, found, err := s.findApproval(ctx, run)
			if err != nil {
				return err
			}
			if !found {
				approval = domain.ApprovalRequest{
					ID:                 uuid.NewString(),
					RunID:              run.ID,
					WorkspaceID:        run.WorkspaceID,
					Status:             "pending",
					Reason:             "Approval required for " + runbook.Title,
					RequestedBySurface: run.RequestedBySurface,
					RequestedByAgent:   run.RequestedByAgent,
					CreatedAt:          s.now(),
				}
				if _, err := s.repo.CreateApproval(ctx, approval); err != nil {
					return err
				}
				if err := s.appendEvent(ctx, run, "approval.requested", "Approval requested", map[string]any{"approval_id": approval.ID}); err != nil {
					return err
				}
				run.Status = "awaiting_approval"
				run.ApprovalState = "pending"
				run.UpdatedAt = s.now()
				_, err = s.repo.UpdateTaskRun(ctx, run)
				return err
			}
			if approval.Status == "pending" {
				run.Status = "awaiting_approval"
				run.ApprovalState = "pending"
				run.UpdatedAt = s.now()
				_, err = s.repo.UpdateTaskRun(ctx, run)
				return err
			}
			if approval.Status == "rejected" {
				run.Status = "rejected"
				run.ApprovalState = "rejected"
				run.UpdatedAt = s.now()
				_, err = s.repo.UpdateTaskRun(ctx, run)
				return err
			}
			if err := s.appendEvent(ctx, run, "approval.consumed", "Approval consumed", map[string]any{"approval_id": approval.ID}); err != nil {
				return err
			}
			run.CurrentStep++
		case "write":
			if err := s.appendEvent(ctx, run, "step.write", "Executed write step "+step.Slug, map[string]any{"step": step.Slug}); err != nil {
				return err
			}
			run.CurrentStep++
		default:
			return fmt.Errorf("unknown step kind %q", step.Kind)
		}
		run.Status = "running"
		run.UpdatedAt = s.now()
		if _, err := s.repo.UpdateTaskRun(ctx, run); err != nil {
			return err
		}
	}
	run.Status = "completed"
	run.UpdatedAt = s.now()
	if _, err := s.repo.UpdateTaskRun(ctx, run); err != nil {
		return err
	}
	return s.appendEvent(ctx, run, "run.completed", "Run completed", map[string]any{"runbook_slug": run.RunbookSlug})
}

func (s *ControlPlane) findApproval(ctx context.Context, run domain.TaskRun) (domain.ApprovalRequest, bool, error) {
	approvals, err := s.repo.ListApprovals(ctx, run.WorkspaceID)
	if err != nil {
		return domain.ApprovalRequest{}, false, err
	}
	for _, approval := range approvals {
		if approval.RunID == run.ID {
			return approval, true, nil
		}
	}
	return domain.ApprovalRequest{}, false, nil
}

func (s *ControlPlane) createArtifact(ctx context.Context, run domain.TaskRun, runbook domain.Runbook, step domain.RunbookStep) (domain.Artifact, error) {
	artifactKind := step.Slug
	for _, candidate := range runbook.ExpectedArtifacts {
		if strings.Contains(step.Slug, "draft") || strings.Contains(step.Slug, "publish") {
			artifactKind = candidate
			break
		}
	}
	body := fmt.Sprintf("# %s\n\nRun: `%s`\n\nStep: `%s`\n\nContext:\n\n```\n%v\n```\n", runbook.Title, run.ID, step.Slug, run.Context)
	key := filepath.Join(run.WorkspaceID, run.ID, step.Slug+".md")
	if err := s.store.Put(ctx, key, []byte(body), "text/markdown"); err != nil {
		return domain.Artifact{}, err
	}
	artifact := domain.Artifact{
		ID:               uuid.NewString(),
		RunID:            run.ID,
		WorkspaceID:      run.WorkspaceID,
		Kind:             artifactKind,
		ContentType:      "text/markdown",
		StorageKey:       key,
		CreatedBySurface: run.RequestedBySurface,
		CreatedByAgent:   run.RequestedByAgent,
		CreatedAt:        s.now(),
	}
	created, err := s.repo.CreateArtifact(ctx, artifact)
	if err != nil {
		return domain.Artifact{}, err
	}
	if err := s.appendEvent(ctx, run, "artifact.created", "Artifact created", map[string]any{"artifact_id": created.ID, "kind": created.Kind}); err != nil {
		return domain.Artifact{}, err
	}
	return created, nil
}

func (s *ControlPlane) appendEvent(ctx context.Context, run domain.TaskRun, kind, message string, payload map[string]any) error {
	_, err := s.repo.CreateAuditEvent(ctx, domain.AuditEvent{
		ID:          uuid.NewString(),
		OrgID:       run.OrgID,
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Kind:        kind,
		Message:     message,
		Payload:     payload,
		CreatedAt:   s.now(),
	})
	return err
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "evo_" + hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func previewToken(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:12]
}
