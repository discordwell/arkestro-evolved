# Architecture

Evo Control Plane is a monorepo for an AI-native SaaS operations platform: agents
(Claude Code, Codex, ChatGPT apps) and humans drive versioned runbooks through a
single Go control plane, with approvals as mandatory checkpoints before write
steps execute.

## Components

| Path | What it is |
| --- | --- |
| `services/controlplane` | Go API server (`cmd/api`) and queue worker (`cmd/worker`) |
| `packages/sdk` | TypeScript client generated from `openapi/controlplane.openapi.yaml` |
| `packages/cli` | `evo` CLI built on the SDK |
| `packages/mcp` | MCP server (stdio + streamable HTTP) exposing control-plane tools |
| `apps/console` | React console for visibility, review, and approvals only |
| `apps/chatgpt-app` | ChatGPT companion host proxying the API |
| `catalog/runbooks.json` | Shared runbook catalog consumed by every surface |
| `legacy/arkessro-demo` | Archived procurement demo (not part of the platform) |

Business logic lives in the Go control plane; every other surface is a thin
client over the HTTP API.

## Control plane layout (`services/controlplane/internal`)

- `api` — chi router, bearer-token auth middleware, JSON handlers, SSE run stream.
- `service` — the `ControlPlane` orchestrator: auth, tenancy checks, run
  lifecycle, approval decisions, artifact creation, audit events.
- `repo` — `Repository` interface with two implementations: `Memory` (tests)
  and `Postgres` (pgx, schema auto-created under an advisory lock). Both wrap
  `repo.ErrNotFound` for missing records; the API maps it to HTTP 404.
- `storage` — artifact object store: filesystem (default) or S3-compatible.
- `catalog` — loads the runbook catalog JSON used to validate and drive runs.
- `worker` — polling loop that claims queued runs and executes runbook steps.
- `config` — `EVO_*` environment configuration with local-dev defaults.

## Run lifecycle

A run executes the steps of its runbook (`read`, `artifact`, `approval`,
`write`) in order, tracked by `current_step`:

```
queued -> running -> [awaiting_approval -> queued] -> completed | failed | rejected
```

- `CreateRun` validates the runbook slug, workspace, and environment, then
  enqueues the run and records a `run.created` audit event.
- The worker claims the oldest queued run (`FOR UPDATE SKIP LOCKED` in
  Postgres; the memory repo mirrors the same mark-running-on-claim semantics)
  and advances steps, emitting audit events per step.
- An `approval` step creates an `approval_request`, parks the run in
  `awaiting_approval`, and stops. Approving re-queues the run; rejecting
  terminates it as `rejected`.
- `artifact` steps write markdown documents to the object store and register
  them with the run.

Approval decisions are tenant-checked against the owning run's org **before**
any state is persisted (`service.DecideApproval`).

## Auth model

- `POST /v1/auth/login` (bootstrap admin from `EVO_DEFAULT_ADMIN_*`) returns a
  bearer token; all other `/v1/*` routes require one.
- Tokens are stored as SHA-256 hashes; only a short preview is kept readable.
  Passwords are bcrypt-hashed.
- Every request resolves the token to an org + user principal; list/get
  operations are scoped through `authorizeWorkspace` / org-ID comparisons.
- Actor attribution (`X-Actor-Surface`, `X-Actor-Agent`, `X-Actor-User`)
  flows into runs, artifacts, and audit events.

## Error mapping

Handlers return Go errors; `api.statusForError` maps them: `ErrUnauthorized` /
`ErrInvalidCredentials` → 401, `ErrForbidden` → 403, `repo.ErrNotFound` → 404,
anything else → 400.

## Testing

- `make test` runs both suites; `make test-go` / `make test-ts` individually.
- Go: service-level lifecycle tests (`internal/service`), HTTP tests against
  an `httptest` server (`internal/api`), repo semantics (`internal/repo`).
- TS: `node:test` suites per package (`pnpm -r test`); the SDK must be built
  first (the root `pnpm test` script handles ordering).
- `make smoke-cli` exercises a running API end-to-end through the CLI.
