import createClient from "openapi-fetch";
import type { components, paths } from "./generated/controlplane.js";

export type Org = components["schemas"]["Org"];
export type User = components["schemas"]["User"];
export type AuthToken = components["schemas"]["AuthToken"];
export type AuthIdentity = components["schemas"]["AuthIdentity"];
export type AuthSession = components["schemas"]["AuthSession"];
export type Workspace = components["schemas"]["Workspace"];
export type Environment = components["schemas"]["Environment"];
export type ToolConnection = components["schemas"]["ToolConnection"];
export type RunbookStep = components["schemas"]["RunbookStep"];
export type Runbook = components["schemas"]["Runbook"];
export type TaskTemplate = components["schemas"]["TaskTemplate"];
export type TaskRun = components["schemas"]["TaskRun"];
export type Artifact = components["schemas"]["Artifact"];
export type ApprovalRequest = components["schemas"]["ApprovalRequest"];
export type AuditEvent = components["schemas"]["AuditEvent"];
export type ArtifactDocument = components["schemas"]["ArtifactDocument"];
export type RunEnvelope = components["schemas"]["RunEnvelope"];

type AuthIdentityEnvelope = components["schemas"]["AuthIdentityEnvelope"];
type AuthSessionEnvelope = components["schemas"]["AuthSessionEnvelope"];
type WorkspaceEnvelope = components["schemas"]["WorkspaceEnvelope"];
type EnvironmentEnvelope = components["schemas"]["EnvironmentEnvelope"];
type ToolConnectionEnvelope = components["schemas"]["ToolConnectionEnvelope"];
type RunEnvelopeResponse = components["schemas"]["RunEnvelopeResponse"];
type ArtifactDocumentEnvelope = components["schemas"]["ArtifactDocumentEnvelope"];
type WorkspaceListResponse = components["schemas"]["WorkspaceListResponse"];
type EnvironmentListResponse = components["schemas"]["EnvironmentListResponse"];
type ToolConnectionListResponse = components["schemas"]["ToolConnectionListResponse"];
type RunbookListResponse = components["schemas"]["RunbookListResponse"];
type TaskTemplateListResponse = components["schemas"]["TaskTemplateListResponse"];
type RunListResponse = components["schemas"]["RunListResponse"];
type ArtifactListResponse = components["schemas"]["ArtifactListResponse"];
type ApprovalListResponse = components["schemas"]["ApprovalListResponse"];
type AuditEventListResponse = components["schemas"]["AuditEventListResponse"];

export interface ClientOptions {
  baseUrl?: string;
  accessToken?: string;
  actorSurface?: string;
  actorAgent?: string;
  actorUser?: string;
  fetch?: typeof globalThis.fetch;
}

type Envelope<T> = { item: T };
type ListEnvelope<T> = { items: T[] };

const DEFAULT_BASE_URL = process.env.EVO_API_BASE_URL || "http://127.0.0.1:8080";

// EvoApiError is thrown for every non-2xx control-plane response. It keeps the
// numeric HTTP status (and the decoded error body) so callers that re-expose
// the API — the ChatGPT companion, future proxies — can map an upstream 401 or
// 404 back onto their own response instead of flattening everything to 500.
// The message keeps the `"<status> <statusText>: <message>"` shape it always
// had, so existing string-matching callers are unaffected.
export class EvoApiError extends Error {
  readonly status: number;
  readonly statusText: string;
  readonly body: unknown;

  constructor(status: number, statusText: string, message: string, body: unknown) {
    super(`${status} ${statusText}: ${message}`);
    this.name = "EvoApiError";
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
}

export class EvoClient {
  readonly baseUrl: string;
  readonly accessToken?: string;
  readonly actorSurface: string;
  readonly actorAgent: string;
  readonly actorUser?: string;
  readonly client: ReturnType<typeof createClient<paths>>;

  constructor(options: ClientOptions = {}) {
    this.baseUrl = options.baseUrl || DEFAULT_BASE_URL;
    this.accessToken = options.accessToken || process.env.EVO_API_TOKEN;
    this.actorSurface = options.actorSurface || process.env.EVO_ACTOR_SURFACE || "cli";
    this.actorAgent = options.actorAgent || process.env.EVO_ACTOR_AGENT || "human";
    this.actorUser = options.actorUser || process.env.EVO_ACTOR_USER;
    this.client = createClient<paths>({
      baseUrl: this.baseUrl,
      fetch: options.fetch
    });
  }

  withAccessToken(accessToken: string): EvoClient {
    return new EvoClient({
      baseUrl: this.baseUrl,
      accessToken,
      actorSurface: this.actorSurface,
      actorAgent: this.actorAgent,
      actorUser: this.actorUser
    });
  }

  async login(input: { email: string; password: string; label?: string }): Promise<AuthSession> {
    const { data, error, response } = await this.client.POST("/v1/auth/login", { body: input });
    return this.unwrapItem<AuthSessionEnvelope, AuthSession>(data, error, response);
  }

  async me(): Promise<AuthIdentity> {
    const { data, error, response } = await this.client.GET("/v1/auth/me", { headers: this.headers() });
    return this.unwrapItem<AuthIdentityEnvelope, AuthIdentity>(data, error, response);
  }

  async createToken(input: { label?: string } = {}): Promise<AuthSession> {
    const { data, error, response } = await this.client.POST("/v1/auth/tokens", {
      body: input,
      headers: this.headers()
    });
    return this.unwrapItem<AuthSessionEnvelope, AuthSession>(data, error, response);
  }

  async listWorkspaces(): Promise<Workspace[]> {
    const { data, error, response } = await this.client.GET("/v1/workspaces", { headers: this.headers() });
    return this.unwrapList<WorkspaceListResponse, Workspace>(data, error, response);
  }

  async createWorkspace(input: { name: string; slug: string; description?: string }): Promise<Workspace> {
    const { data, error, response } = await this.client.POST("/v1/workspaces", {
      body: input,
      headers: this.headers()
    });
    return this.unwrapItem<WorkspaceEnvelope, Workspace>(data, error, response);
  }

  async listEnvironments(workspaceId: string): Promise<Environment[]> {
    const { data, error, response } = await this.client.GET("/v1/environments", {
      params: { query: { workspace_id: workspaceId } },
      headers: this.headers()
    });
    return this.unwrapList<EnvironmentListResponse, Environment>(data, error, response);
  }

  async createEnvironment(input: { workspace_id: string; name: string; slug: string; kind: string }): Promise<Environment> {
    const { data, error, response } = await this.client.POST("/v1/environments", {
      body: input,
      headers: this.headers()
    });
    return this.unwrapItem<EnvironmentEnvelope, Environment>(data, error, response);
  }

  async listToolConnections(workspaceId: string): Promise<ToolConnection[]> {
    const { data, error, response } = await this.client.GET("/v1/tool-connections", {
      params: { query: { workspace_id: workspaceId } },
      headers: this.headers()
    });
    return this.unwrapList<ToolConnectionListResponse, ToolConnection>(data, error, response);
  }

  async createToolConnection(input: { workspace_id: string; environment_id?: string; name: string; kind: string; config?: Record<string, unknown> }): Promise<ToolConnection> {
    const { data, error, response } = await this.client.POST("/v1/tool-connections", {
      body: input,
      headers: this.headers()
    });
    return this.unwrapItem<ToolConnectionEnvelope, ToolConnection>(data, error, response);
  }

  async listRunbooks(): Promise<Runbook[]> {
    const { data, error, response } = await this.client.GET("/v1/runbooks", { headers: this.headers() });
    return this.unwrapList<RunbookListResponse, Runbook>(data, error, response);
  }

  async listTaskTemplates(): Promise<TaskTemplate[]> {
    const { data, error, response } = await this.client.GET("/v1/task-templates", { headers: this.headers() });
    return this.unwrapList<TaskTemplateListResponse, TaskTemplate>(data, error, response);
  }

  async listRuns(workspaceId: string): Promise<TaskRun[]> {
    const { data, error, response } = await this.client.GET("/v1/runs", {
      params: { query: { workspace_id: workspaceId } },
      headers: this.headers()
    });
    return this.unwrapList<RunListResponse, TaskRun>(data, error, response);
  }

  async createRun(input: { workspace_id: string; environment_id?: string; runbook_slug: string; context?: Record<string, unknown> }): Promise<RunEnvelope> {
    const { data, error, response } = await this.client.POST("/v1/runs", {
      body: input,
      headers: this.headers()
    });
    return this.unwrapItem<RunEnvelopeResponse, RunEnvelope>(data, error, response);
  }

  async getRun(runId: string): Promise<RunEnvelope> {
    const { data, error, response } = await this.client.GET("/v1/runs/{runID}", {
      params: { path: { runID: runId } },
      headers: this.headers()
    });
    return this.unwrapItem<RunEnvelopeResponse, RunEnvelope>(data, error, response);
  }

  async listArtifacts(runId: string): Promise<Artifact[]> {
    const { data, error, response } = await this.client.GET("/v1/artifacts", {
      params: { query: { run_id: runId } },
      headers: this.headers()
    });
    return this.unwrapList<ArtifactListResponse, Artifact>(data, error, response);
  }

  async getArtifact(artifactId: string): Promise<ArtifactDocument> {
    const { data, error, response } = await this.client.GET("/v1/artifacts/{artifactID}", {
      params: { path: { artifactID: artifactId } },
      headers: this.headers()
    });
    return this.unwrapItem<ArtifactDocumentEnvelope, ArtifactDocument>(data, error, response);
  }

  async listApprovals(workspaceId: string): Promise<ApprovalRequest[]> {
    const { data, error, response } = await this.client.GET("/v1/approvals", {
      params: { query: { workspace_id: workspaceId } },
      headers: this.headers()
    });
    return this.unwrapList<ApprovalListResponse, ApprovalRequest>(data, error, response);
  }

  async approve(approvalId: string, note = ""): Promise<RunEnvelope> {
    const { data, error, response } = await this.client.POST("/v1/approvals/{approvalID}/approve", {
      params: { path: { approvalID: approvalId } },
      body: { note },
      headers: this.headers()
    });
    return this.unwrapItem<RunEnvelopeResponse, RunEnvelope>(data, error, response);
  }

  async reject(approvalId: string, note = ""): Promise<RunEnvelope> {
    const { data, error, response } = await this.client.POST("/v1/approvals/{approvalID}/reject", {
      params: { path: { approvalID: approvalId } },
      body: { note },
      headers: this.headers()
    });
    return this.unwrapItem<RunEnvelopeResponse, RunEnvelope>(data, error, response);
  }

  async listAuditEvents(workspaceId: string, runId = ""): Promise<AuditEvent[]> {
    const { data, error, response } = await this.client.GET("/v1/audit-events", {
      params: { query: { workspace_id: workspaceId, run_id: runId || undefined } },
      headers: this.headers()
    });
    return this.unwrapList<AuditEventListResponse, AuditEvent>(data, error, response);
  }

  headers(): Record<string, string> {
    const headers: Record<string, string> = {
      "X-Actor-Surface": this.actorSurface,
      "X-Actor-Agent": this.actorAgent
    };
    if (this.accessToken) {
      headers.Authorization = `Bearer ${this.accessToken}`;
    }
    if (this.actorUser) {
      headers["X-Actor-User"] = this.actorUser;
    }
    return headers;
  }

  private unwrapList<TEnvelope extends ListEnvelope<TItem>, TItem>(
    data: TEnvelope | undefined,
    error: unknown,
    response: Response
  ): TItem[] {
    if (response.ok && data) {
      return data.items;
    }
    throw this.toError(response, error);
  }

  private unwrapItem<TEnvelope extends Envelope<TItem>, TItem>(
    data: TEnvelope | undefined,
    error: unknown,
    response: Response
  ): TItem {
    if (response.ok && data) {
      return data.item;
    }
    throw this.toError(response, error);
  }

  private toError(response: Response, error: unknown): EvoApiError {
    return new EvoApiError(response.status, response.statusText, this.errorMessage(error), error);
  }

  private errorMessage(error: unknown): string {
    if (typeof error === "object" && error && "error" in error) {
      const value = (error as { error?: unknown }).error;
      if (typeof value === "string" && value) {
        return value;
      }
    }
    if (typeof error === "string" && error) {
      return error;
    }
    return "request failed";
  }
}
