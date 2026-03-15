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

## Tests

```bash
make test
```
