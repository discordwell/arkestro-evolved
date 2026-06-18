package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/repo"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/service"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	service *service.ControlPlane
	mux     *chi.Mux
}

type principalKey struct{}

func New(svc *service.ControlPlane) *Server {
	s := &Server{service: svc, mux: chi.NewRouter()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.Get("/api/health", s.wrap(s.handleHealth))

	s.mux.Route("/v1", func(r chi.Router) {
		r.Post("/auth/login", s.wrap(s.handleLogin))

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)

			r.Get("/auth/me", s.wrap(s.handleMe))
			r.Post("/auth/tokens", s.wrap(s.handleCreateToken))
			r.Get("/workspaces", s.wrap(s.handleListWorkspaces))
			r.Post("/workspaces", s.wrap(s.handleCreateWorkspace))
			r.Get("/environments", s.wrap(s.handleListEnvironments))
			r.Post("/environments", s.wrap(s.handleCreateEnvironment))
			r.Get("/tool-connections", s.wrap(s.handleListTools))
			r.Post("/tool-connections", s.wrap(s.handleCreateTool))
			r.Get("/runbooks", s.wrap(s.handleListRunbooks))
			r.Get("/task-templates", s.wrap(s.handleListTaskTemplates))
			r.Get("/runs", s.wrap(s.handleListRuns))
			r.Post("/runs", s.wrap(s.handleCreateRun))
			r.Get("/runs/{runID}", s.wrap(s.handleGetRun))
			r.Get("/runs/{runID}/stream", s.handleRunStream)
			r.Get("/artifacts", s.wrap(s.handleListArtifacts))
			r.Get("/artifacts/{artifactID}", s.wrap(s.handleGetArtifact))
			r.Get("/approvals", s.wrap(s.handleListApprovals))
			r.Post("/approvals/{approvalID}/approve", s.wrap(s.handleApprove))
			r.Post("/approvals/{approvalID}/reject", s.wrap(s.handleReject))
			r.Get("/policies", s.wrap(s.handleListPolicies))
			r.Get("/audit-events", s.wrap(s.handleListAuditEvents))
		})
	})
}

type handler func(http.ResponseWriter, *http.Request) error

func (s *Server) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := h(w, r); err != nil {
			status := s.statusForError(err)
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		}
	}
}

func (s *Server) statusForError(err error) int {
	switch {
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout
	case errors.Is(err, service.ErrUnauthorized), errors.Is(err, service.ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, repo.ErrNotFound),
		strings.Contains(strings.ToLower(err.Error()), "not found"):
		return http.StatusNotFound
	default:
		return http.StatusBadRequest
	}
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		principal, err := s.service.Authenticate(r.Context(), token)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(s.statusForError(err))
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) error {
	return json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Label    string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	session, err := s.service.Login(r.Context(), req.Email, req.Password, req.Label)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"item": session})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) error {
	return json.NewEncoder(w).Encode(map[string]any{"item": s.principal(r).AuthIdentity})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	session, err := s.service.CreateToken(r.Context(), s.principal(r), req.Label)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(map[string]any{"item": session})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListWorkspaces(r.Context(), s.orgID(r))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	item, err := s.service.CreateWorkspace(r.Context(), s.orgID(r), req.Name, req.Slug, req.Description)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListEnvironments(r.Context(), s.orgID(r), r.URL.Query().Get("workspace_id"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Kind        string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	item, err := s.service.CreateEnvironment(r.Context(), s.orgID(r), req.WorkspaceID, req.Name, req.Slug, req.Kind)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListToolConnections(r.Context(), s.orgID(r), r.URL.Query().Get("workspace_id"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleCreateTool(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		WorkspaceID   string         `json:"workspace_id"`
		EnvironmentID string         `json:"environment_id"`
		Name          string         `json:"name"`
		Kind          string         `json:"kind"`
		Config        map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	item, err := s.service.CreateToolConnection(r.Context(), s.orgID(r), req.WorkspaceID, req.EnvironmentID, req.Name, req.Kind, req.Config)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleListRunbooks(w http.ResponseWriter, r *http.Request) error {
	return json.NewEncoder(w).Encode(map[string]any{"items": s.service.ListRunbooks()})
}

func (s *Server) handleListTaskTemplates(w http.ResponseWriter, r *http.Request) error {
	return json.NewEncoder(w).Encode(map[string]any{"items": s.service.ListTaskTemplates()})
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListRuns(r.Context(), s.orgID(r), r.URL.Query().Get("workspace_id"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		WorkspaceID   string         `json:"workspace_id"`
		EnvironmentID string         `json:"environment_id"`
		RunbookSlug   string         `json:"runbook_slug"`
		Context       map[string]any `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	item, err := s.service.CreateRun(r.Context(), s.orgID(r), req.WorkspaceID, req.EnvironmentID, req.RunbookSlug, s.actor(r), req.Context)
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) error {
	item, err := s.service.GetRunEnvelope(r.Context(), s.orgID(r), chi.URLParam(r, "runID"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListArtifactsByRun(r.Context(), s.orgID(r), r.URL.Query().Get("run_id"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) error {
	item, err := s.service.GetArtifactDocument(r.Context(), s.orgID(r), chi.URLParam(r, "artifactID"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListApprovals(r.Context(), s.orgID(r), r.URL.Query().Get("workspace_id"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	item, err := s.service.DecideApproval(r.Context(), s.orgID(r), chi.URLParam(r, "approvalID"), "approve", req.Note, s.actor(r))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleReject(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	item, err := s.service.DecideApproval(r.Context(), s.orgID(r), chi.URLParam(r, "approvalID"), "reject", req.Note, s.actor(r))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"item": item})
}

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListPolicies(r.Context(), s.orgID(r), r.URL.Query().Get("workspace_id"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) error {
	items, err := s.service.ListAuditEvents(r.Context(), s.orgID(r), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("run_id"))
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"items": items})
}

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	env, err := s.service.GetRunEnvelope(r.Context(), s.orgID(r), runID)
	if err != nil {
		http.Error(w, err.Error(), s.statusForError(err))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flush, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Dedupe by event ID rather than slicing by index: list ordering for
	// events with identical timestamps is not guaranteed to be stable between
	// polls, and index slicing would then drop or repeat events.
	sent := make(map[string]struct{})
	send := func(events []domain.AuditEvent) error {
		wrote := false
		for _, event := range events {
			if _, seen := sent[event.ID]; seen {
				continue
			}
			raw, err := json.Marshal(event)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "event: audit\ndata: %s\n\n", raw); err != nil {
				return err
			}
			sent[event.ID] = struct{}{}
			wrote = true
		}
		if wrote {
			flush.Flush()
		}
		return nil
	}
	sendDone := func(status string) {
		_, _ = fmt.Fprintf(w, "event: done\ndata: {\"status\":%q}\n\n", status)
		flush.Flush()
	}

	if err := send(env.Events); err != nil {
		return
	}
	// Flush headers right away so clients see the stream open even when the
	// run has produced no events yet.
	flush.Flush()
	// A run that is already terminal closes immediately instead of making the
	// client wait out a poll tick for the done event.
	if isTerminalRunStatus(env.Run.Status) {
		sendDone(env.Run.Status)
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			runEnv, err := s.service.GetRunEnvelope(r.Context(), s.orgID(r), runID)
			if err != nil {
				return
			}
			if err := send(runEnv.Events); err != nil {
				return
			}
			if isTerminalRunStatus(runEnv.Run.Status) {
				sendDone(runEnv.Run.Status)
				return
			}
		}
	}
}

func isTerminalRunStatus(status string) bool {
	return status == "completed" || status == "failed" || status == "rejected"
}

func (s *Server) orgID(r *http.Request) string {
	return s.principal(r).Org.ID
}

func (s *Server) principal(r *http.Request) domain.AuthPrincipal {
	principal, _ := r.Context().Value(principalKey{}).(domain.AuthPrincipal)
	return principal
}

func (s *Server) actor(r *http.Request) domain.Actor {
	principal := s.principal(r)
	actor := domain.Actor{
		Surface: r.Header.Get("X-Actor-Surface"),
		Agent:   r.Header.Get("X-Actor-Agent"),
		User:    principal.User.Email,
	}
	if v := r.Header.Get("X-Actor-User"); v != "" {
		actor.User = v
	}
	if actor.Surface == "" {
		actor.Surface = "api"
	}
	if actor.Agent == "" {
		actor.Agent = "human"
	}
	return actor
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
