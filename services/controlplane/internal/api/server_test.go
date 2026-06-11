package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/api"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/catalog"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/repo"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/service"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/storage"
)

const (
	testOrgID    = "org-1"
	testEmail    = "admin@test.local"
	testPassword = "test-password"
)

func newTestServer(t *testing.T) (*httptest.Server, *service.ControlPlane) {
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
		ID:        testOrgID,
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: time.Now().UTC(),
	})
	svc.ConfigureBootstrapAdmin(testEmail, testPassword, "Test Admin")
	if err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	server := httptest.NewServer(api.New(svc).Handler())
	t.Cleanup(server.Close)
	return server, svc
}

type apiClient struct {
	t       *testing.T
	baseURL string
	token   string
}

func (c *apiClient) do(method, path string, body any) (*http.Response, map[string]any) {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	decoded := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		c.t.Fatalf("%s %s: decode response: %v", method, path, err)
	}
	return resp, decoded
}

func (c *apiClient) mustDo(method, path string, body any, wantStatus int) map[string]any {
	c.t.Helper()
	resp, decoded := c.do(method, path, body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: expected status %d, got %d (%v)", method, path, wantStatus, resp.StatusCode, decoded)
	}
	return decoded
}

func login(t *testing.T, server *httptest.Server) *apiClient {
	t.Helper()
	anon := &apiClient{t: t, baseURL: server.URL}
	decoded := anon.mustDo(http.MethodPost, "/v1/auth/login", map[string]string{
		"email":    testEmail,
		"password": testPassword,
		"label":    "api-tests",
	}, http.StatusOK)
	item, _ := decoded["item"].(map[string]any)
	token, _ := item["access_token"].(string)
	if token == "" {
		t.Fatalf("login returned no access token: %v", decoded)
	}
	return &apiClient{t: t, baseURL: server.URL, token: token}
}

func itemField(t *testing.T, decoded map[string]any, keys ...string) any {
	t.Helper()
	var current any = decoded
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("expected object at %q in %v", key, decoded)
		}
		current = m[key]
	}
	return current
}

func TestRoutesRequireBearerToken(t *testing.T) {
	server, _ := newTestServer(t)
	anon := &apiClient{t: t, baseURL: server.URL}

	for _, path := range []string{"/v1/workspaces", "/v1/runbooks", "/v1/auth/me"} {
		resp, _ := anon.do(http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s without token: expected 401, got %d", path, resp.StatusCode)
		}
	}

	resp, _ := anon.do(http.MethodPost, "/v1/auth/login", map[string]string{
		"email":    testEmail,
		"password": "wrong-password",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login with bad password: expected 401, got %d", resp.StatusCode)
	}
}

func TestRunLifecycleOverHTTP(t *testing.T) {
	server, svc := newTestServer(t)
	client := login(t, server)

	me := client.mustDo(http.MethodGet, "/v1/auth/me", nil, http.StatusOK)
	if email := itemField(t, me, "item", "user", "email"); email != testEmail {
		t.Fatalf("expected %s, got %v", testEmail, email)
	}

	workspace := client.mustDo(http.MethodPost, "/v1/workspaces", map[string]string{
		"name": "Ops", "slug": "ops", "description": "Ops workspace",
	}, http.StatusCreated)
	workspaceID, _ := itemField(t, workspace, "item", "id").(string)

	env := client.mustDo(http.MethodPost, "/v1/environments", map[string]string{
		"workspace_id": workspaceID, "name": "Staging", "slug": "staging", "kind": "staging",
	}, http.StatusCreated)
	envID, _ := itemField(t, env, "item", "id").(string)

	created := client.mustDo(http.MethodPost, "/v1/runs", map[string]any{
		"workspace_id":   workspaceID,
		"environment_id": envID,
		"runbook_slug":   "compliance-pack",
		"context":        map[string]any{"period": "weekly"},
	}, http.StatusCreated)
	runID, _ := itemField(t, created, "item", "run", "id").(string)
	if runID == "" {
		t.Fatalf("expected run id, got %v", created)
	}

	if _, err := svc.ProcessNextRun(context.Background()); err != nil {
		t.Fatalf("process run: %v", err)
	}

	envelope := client.mustDo(http.MethodGet, "/v1/runs/"+runID, nil, http.StatusOK)
	if status := itemField(t, envelope, "item", "run", "status"); status != "completed" {
		t.Fatalf("expected completed run, got %v", status)
	}

	artifacts := client.mustDo(http.MethodGet, "/v1/artifacts?run_id="+runID, nil, http.StatusOK)
	items, _ := artifacts["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected artifacts, got %v", artifacts)
	}
	first, _ := items[0].(map[string]any)
	artifactID, _ := first["id"].(string)

	document := client.mustDo(http.MethodGet, "/v1/artifacts/"+artifactID, nil, http.StatusOK)
	if content, _ := itemField(t, document, "item", "content").(string); content == "" {
		t.Fatalf("expected artifact content, got %v", document)
	}

	events := client.mustDo(http.MethodGet, "/v1/audit-events?workspace_id="+workspaceID+"&run_id="+runID, nil, http.StatusOK)
	if eventItems, _ := events["items"].([]any); len(eventItems) == 0 {
		t.Fatalf("expected audit events, got %v", events)
	}
}

// The SSE stream must deliver every audit event exactly once and close with a
// terminal done event.
func TestRunStreamDeliversEachEventOnceThenDone(t *testing.T) {
	server, svc := newTestServer(t)
	client := login(t, server)
	ctx := context.Background()

	workspace := client.mustDo(http.MethodPost, "/v1/workspaces", map[string]string{
		"name": "Stream", "slug": "stream",
	}, http.StatusCreated)
	workspaceID, _ := itemField(t, workspace, "item", "id").(string)

	created := client.mustDo(http.MethodPost, "/v1/runs", map[string]any{
		"workspace_id": workspaceID,
		"runbook_slug": "compliance-pack",
	}, http.StatusCreated)
	runID, _ := itemField(t, created, "item", "run", "id").(string)

	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process run: %v", err)
	}

	events := client.mustDo(http.MethodGet, "/v1/audit-events?workspace_id="+workspaceID+"&run_id="+runID, nil, http.StatusOK)
	eventItems, _ := events["items"].([]any)
	if len(eventItems) == 0 {
		t.Fatalf("expected audit events before streaming")
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/runs/"+runID+"/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+client.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: expected 200, got %d", resp.StatusCode)
	}

	seen := map[string]int{}
	sawDone := false
	scanner := bufio.NewScanner(resp.Body)
	currentEvent := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			currentEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			payload := strings.TrimPrefix(line, "data: ")
			if currentEvent == "audit" {
				var event map[string]any
				if err := json.Unmarshal([]byte(payload), &event); err != nil {
					t.Fatalf("decode audit event: %v", err)
				}
				id, _ := event["id"].(string)
				seen[id]++
			}
			if currentEvent == "done" {
				if !strings.Contains(payload, "completed") {
					t.Fatalf("expected completed status in done event, got %s", payload)
				}
				sawDone = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !sawDone {
		t.Fatalf("stream ended without a done event")
	}
	if len(seen) != len(eventItems) {
		t.Fatalf("expected %d distinct events, got %d", len(eventItems), len(seen))
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("event %s delivered %d times", id, count)
		}
	}
}

func TestMissingResourcesReturn404(t *testing.T) {
	server, _ := newTestServer(t)
	client := login(t, server)

	resp, _ := client.do(http.MethodGet, "/v1/runs/no-such-run", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown run: expected 404, got %d", resp.StatusCode)
	}
	resp, _ = client.do(http.MethodGet, "/v1/artifacts/no-such-artifact", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown artifact: expected 404, got %d", resp.StatusCode)
	}
}

// A token from one org must not be able to decide approvals owned by another
// org, and the failed attempt must not leak state changes.
func TestCrossOrgApprovalDecisionForbidden(t *testing.T) {
	server, svc := newTestServer(t)
	client := login(t, server)
	ctx := context.Background()

	foreignOrg := "org-2"
	workspace, err := svc.CreateWorkspace(ctx, foreignOrg, "Foreign", "foreign", "")
	if err != nil {
		t.Fatalf("foreign workspace: %v", err)
	}
	run, err := svc.CreateRun(ctx, foreignOrg, workspace.ID, "", "release-coordination", domain.Actor{Surface: "test", Agent: "test"}, nil)
	if err != nil {
		t.Fatalf("foreign run: %v", err)
	}
	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process foreign run: %v", err)
	}
	waiting, err := svc.GetRunEnvelope(ctx, foreignOrg, run.Run.ID)
	if err != nil {
		t.Fatalf("get foreign run: %v", err)
	}
	if len(waiting.Approvals) != 1 {
		t.Fatalf("expected pending foreign approval, got %d", len(waiting.Approvals))
	}
	approvalID := waiting.Approvals[0].ID

	resp, decoded := client.do(http.MethodPost, fmt.Sprintf("/v1/approvals/%s/approve", approvalID), map[string]string{"note": "sneaky"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-org approve: expected 403, got %d (%v)", resp.StatusCode, decoded)
	}

	after, err := svc.GetRunEnvelope(ctx, foreignOrg, run.Run.ID)
	if err != nil {
		t.Fatalf("get foreign run after attempt: %v", err)
	}
	if after.Approvals[0].Status != "pending" {
		t.Fatalf("cross-org attempt must not mutate approval, got %s", after.Approvals[0].Status)
	}
	if after.Run.Status != "awaiting_approval" {
		t.Fatalf("cross-org attempt must not advance run, got %s", after.Run.Status)
	}
}

func TestApprovalFlowOverHTTP(t *testing.T) {
	server, svc := newTestServer(t)
	client := login(t, server)
	ctx := context.Background()

	workspace := client.mustDo(http.MethodPost, "/v1/workspaces", map[string]string{
		"name": "Release", "slug": "release",
	}, http.StatusCreated)
	workspaceID, _ := itemField(t, workspace, "item", "id").(string)

	created := client.mustDo(http.MethodPost, "/v1/runs", map[string]any{
		"workspace_id": workspaceID,
		"runbook_slug": "release-coordination",
	}, http.StatusCreated)
	runID, _ := itemField(t, created, "item", "run", "id").(string)

	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process run: %v", err)
	}

	approvals := client.mustDo(http.MethodGet, "/v1/approvals?workspace_id="+workspaceID, nil, http.StatusOK)
	approvalItems, _ := approvals["items"].([]any)
	if len(approvalItems) != 1 {
		t.Fatalf("expected one approval, got %v", approvals)
	}
	approval, _ := approvalItems[0].(map[string]any)
	approvalID, _ := approval["id"].(string)

	decided := client.mustDo(http.MethodPost, "/v1/approvals/"+approvalID+"/approve", map[string]string{"note": "ship it"}, http.StatusOK)
	if state := itemField(t, decided, "item", "run", "approval_state"); state != "approved" {
		t.Fatalf("expected approved state, got %v", state)
	}

	// Re-deciding a settled approval must fail without changing anything.
	resp, _ := client.do(http.MethodPost, "/v1/approvals/"+approvalID+"/reject", map[string]string{"note": "too late"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("double decision: expected 400, got %d", resp.StatusCode)
	}

	if _, err := svc.ProcessNextRun(ctx); err != nil {
		t.Fatalf("process after approval: %v", err)
	}
	envelope := client.mustDo(http.MethodGet, "/v1/runs/"+runID, nil, http.StatusOK)
	if status := itemField(t, envelope, "item", "run", "status"); status != "completed" {
		t.Fatalf("expected completed run, got %v", status)
	}
}
