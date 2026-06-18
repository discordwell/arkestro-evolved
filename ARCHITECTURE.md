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
  consistent with the presence of approval steps, and every `write` step gated
  by a preceding `approval` step), so a broken catalog cannot fail runs
  mid-execution. The write-gating rule enforces the platform contract that
  external writes stop for approval: a runbook cannot ship a write that would
  execute unguarded, regardless of its `approval_required` flag. Step slugs
  become artifact file names, so the slug charset is what keeps storage keys
  path-safe.
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
  re-queues the run; rejecting terminates it as `rejected`. The decision is
  attributed to whoever made it: the deciding actor's identity is recorded on
  the approval as `decided_by` (its user, falling back to its agent) and in the
  `approval.approved` / `approval.rejected` audit event, so the trail answers
  "who approved this write?". A runbook with several approval steps gates on
  each one — a decision never satisfies a later gate. (Approvals persisted
  before step scoping carry index 0 and are honored for the runbook's first
  approval step.)
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
- Actor attribution flows from the `X-Actor-Surface` / `X-Actor-Agent` /
  `X-Actor-User` headers (the user defaults to the authenticated principal).
  Surface and agent are stored on runs and artifacts; the human user is kept in
  the audit trail — the `run.created` event records the initiating user, and an
  approval decision records the deciding user as `decided_by` on the approval
  and in its audit event.

## Error mapping

Handlers return Go errors; `api.statusForError` maps them: `ErrUnauthorized` /
`ErrInvalidCredentials` → 401, `ErrForbidden` → 403, `repo.ErrNotFound` → 404,
anything else → 400.

The TypeScript SDK preserves that status on the way back out: a non-2xx
response is thrown as an `EvoApiError` carrying the numeric `status` (plus the
decoded body), not just a formatted message string. Surfaces that re-expose the
API rely on this — the ChatGPT companion maps `EvoApiError.status` straight onto
its own response, so an upstream 401 (bad token) or 404 (missing run) reaches
the caller as that status instead of being flattened to 500; only genuinely
unexpected (non-API) errors become 500.

The companion also validates the approval decision (`approve`/`reject`) in its
`/api/approvals/{id}/{decision}` route before proxying, mirroring the control
plane's own check. A malformed decision is answered with a local `400` rather
than silently falling through to reject — which would terminate the run — so the
same approve-or-reject invariant the Go service enforces holds at every surface.

## Testing

- `make test` runs both suites; `make test-go` / `make test-ts` individually.
- Go: service-level lifecycle tests (`internal/service`), HTTP tests against
  an `httptest` server (`internal/api`), repo semantics (`internal/repo`).
- TS: `node:test` suites per package (`pnpm -r test`); the SDK must be built
  first (the root `pnpm test` script handles ordering).
- `make smoke-cli` exercises a running API end-to-end through the CLI.
