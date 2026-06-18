# Evo Control Plane

Evo Control Plane is an AI-native SaaS operations platform built for agents first.

It is designed to be operated from:

- Codex
- Claude Code
- OpenClaw-compatible skill/plugin flows
- ChatGPT Apps
- a structured CLI and web console

The current repository is a monorepo with:

- `services/controlplane`: Go API and worker
- `packages/sdk`: shared TypeScript SDK
- `packages/cli`: `evo` CLI
- `packages/mcp`: MCP server
- `apps/console`: React control tower
- `apps/chatgpt-app`: ChatGPT/App SDK host
- `catalog`: runbook catalog shared across surfaces
- `docs`: human and agent-facing docs
- `legacy/arkessro-demo`: archived procurement demo

## Core concepts

- `workspace`
- `environment`
- `tool_connection`
- `runbook`
- `task_template`
- `task_run`
- `artifact`
- `approval_request`
- `policy_rule`
- `audit_event`

## Local development

Install dependencies:

```bash
pnpm install
```

Start infra:

```bash
docker compose up -d postgres
```

Or run the services individually:

```bash
make api
```

Run the Go worker:

```bash
make worker
```

Run the console:

```bash
make console
```

Run the MCP server:

```bash
make mcp
```

Run the remote MCP HTTP surface:

```bash
make mcp-http
```

Run the ChatGPT app host:

```bash
make chatgpt
```

## Auth and bootstrap access

The API now requires bearer auth for all `/v1/*` routes except `POST /v1/auth/login`.

Default local bootstrap credentials:

- email: `admin@evo.local`
- password: `changeme`

Login with the CLI and persist a local token:

```bash
node packages/cli/dist/index.js auth login \
  --email admin@evo.local \
  --password changeme
```

Check the current identity:

```bash
node packages/cli/dist/index.js auth whoami --json
```

Print local MCP connection metadata:

```bash
node packages/cli/dist/index.js mcp print-config --json
```

For smoke tests, prefer the isolated script instead of writing auth state into your normal config directory:

```bash
make smoke-cli
```

That script uses a temporary `XDG_CONFIG_HOME` and deletes the local token on exit.

## Remote agent surfaces

- Remote MCP HTTP listens on `http://127.0.0.1:3301/mcp` by default.
- The ChatGPT companion host listens on `http://127.0.0.1:3200` by default.
- The companion advertises MCP connection metadata at `GET /api/connect`.
- Both remote surfaces accept `Authorization: Bearer <token>` and target the same control-plane API.

## Policy rules

Policy rules govern whether a step's action must stop for approval. Each rule has
an `action_pattern` (`*`, `prefix.*`, or an exact `<kind>.<slug>`), an
`approval_required` flag, and an optional workspace scope (empty = global).
Bootstrap seeds one global rule, `approval-required-for-write` (`write.*`), so by
default every write needs approval.

Enforcement happens in two layers: the catalog rejects any runbook whose write is
not structurally preceded by an approval step (at boot), and the worker re-checks
at run time that a policy-required write actually cleared an approved gate before
executing it. A well-formed run is unaffected; an unguarded write is refused with
a `policy.violation` audit event rather than executed. Approval gates record the
policy that mandated them, so the trail explains *why* sign-off was required.

List the rules in force for a workspace:

```bash
node packages/cli/dist/index.js policy list --workspace-id <id> --json
```

They are also available at `GET /v1/policies?workspace_id=<id>` and through the
`policy.list` MCP tool.

## Tests

```bash
make test
```
