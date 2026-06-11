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

- `api` — chi router, bearer-token auth middleware, JSON handlers, SSE run
  stream (dedupes by audit-event ID, so reordered polls never drop or repeat
  events; a run that is already terminal closes the stream immediately
  instead of waiting out a poll tick).
- `service` — the `ControlPlane` orchestrator: auth, tenancy checks, run
  lifecycle, approval decisions, artifact creation, audit events.
- `repo` — `Repository` interface with two implementations: `Memory` (tests)
  and `Postgres` (pgx, schema auto-created under an advisory lock). Both wrap
  `repo.ErrNotFound` for missing records; the API maps it to HTTP 404. List
  results order deterministically by `(created_at, id)` — runs and approvals
  newest first, everything else oldest first — identically in both backends.
- `storage` — artifact object store: filesystem (default) or S3-compatible.
  The filesystem store rejects keys that are empty, absolute, or escape the
  artifact root via `..`.
- `catalog` — loads the runbook catalog JSON and validates it at boot (unique
  lowercase-kebab runbook/step slugs, known step kinds, `approval_required`
  consistent with the presence of approval steps), so a broken catalog cannot
  fail runs mid-execution. Step slugs become artifact file names, so the slug
  charset is what keeps storage keys path-safe.
- `worker` — polling loop that claims queued runs and executes runbook steps.
  The queue drains without waiting while claims succeed; after an error or an
  idle poll the worker waits one poll interval, so a persistent failure (e.g.
  the database being down) cannot spin the loop hot.
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
- An `approval` step creates an `approval_request` scoped to that step
  (`step_index`), parks the run in `awaiting_approval`, and stops. Approving
  re-queues the run; rejecting terminates it as `rejected`. A runbook with
  several approval steps gates on each one — a decision never satisfies a
  later gate. (Approvals persisted before step scoping carry index 0 and are
  honored for the runbook's first approval step.)
- `artifact` steps write markdown documents to the object store and register
  them with the run.

Approval decisions are tenant-checked against the owning run's org **before**
any state is persisted (`service.DecideApproval`), and the pending→decided
transition is compare-and-swap in the repository, so concurrent decisions
resolve to exactly one winner (the rest get `repo.ErrNotPending` → HTTP 400).

## Auth model

- `POST /v1/auth/login` (bootstrap admin from `EVO_DEFAULT_ADMIN_*`) returns a
  bearer token; all other `/v1/*` routes require one.
- Tokens are stored as SHA-256 hashes; only a short preview is kept readable.
  Passwords are bcrypt-hashed. Login burns a dummy bcrypt compare for unknown
  emails so response timing cannot enumerate accounts, and `last_used_at`
  refreshes at most once per minute instead of writing on every request.
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
