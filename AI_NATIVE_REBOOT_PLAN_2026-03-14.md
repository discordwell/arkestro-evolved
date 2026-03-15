# AI-Native Reboot Plan

Date: March 14, 2026

## Goal

Rebuild this project as an AI-native SaaS:

- easy to operate from Codex, Claude Code, and OpenClaw
- CLI-first and docs-first
- GUI as a review, approval, and visualization layer
- MCP-native at the integration boundary
- designed so agents can safely do real work without custom brittle prompt spaghetti

This is a reset. The current demo should not be treated as the product foundation.

## Research Summary

### 1. Codex has converged on a clear operating model

Relevant signals from current OpenAI docs:

- Codex reads `AGENTS.md` before doing work and layers global plus project-specific guidance.
- Codex skills are reusable folders with `SKILL.md`, optional scripts, references, and assets.
- Codex supports MCP for third-party tools and context.
- Codex has a non-interactive `codex exec` mode with JSONL output for CI and scripts.
- Codex has a TypeScript SDK for programmatic control.
- Codex supports automations and experimental multi-agent workflows.

Implication:

If we want our SaaS to feel native to Codex, we need:

- repository-level guidance files
- reusable skills
- an MCP server
- machine-readable CLI surfaces
- isolated long-running jobs and review queues

### 2. Claude Code uses the same core pattern with different names

Relevant signals from current Anthropic docs:

- Claude Code uses `CLAUDE.md` for project and user memory.
- Claude supports custom slash commands, MCP, hooks, and subagents.
- Claude hooks can validate prompts, block tool calls, run checks, and run asynchronously after writes.
- Claude subagents have separate context windows, focused prompts, and tool restrictions.
- Claude Code can itself act as an MCP server.

Implication:

Claude-native support means:

- a first-class `CLAUDE.md` strategy
- project-shared command packs
- hook-friendly workflows
- tool permissions and post-edit checks
- subagent-friendly decomposition of work

### 3. OpenClaw is broader and less coding-centric

Relevant signals from current OpenClaw docs:

- OpenClaw is a multi-agent runtime built around isolated workspaces, skills, plugins, channels, and routing.
- It has a public skill registry via ClawHub.
- It supports plugins that can add routes, tools, CLI commands, background services, and skills.
- OpenProse provides a portable workflow format for orchestrating explicit multi-agent programs.

Inference:

OpenClaw is not just "another coding assistant." It is closer to a general agent runtime / gateway.

Implication:

We should not make OpenClaw the primary UX model. We should support it through:

- a clean skill/plugin surface
- MCP compatibility where possible
- documented OpenProse-style workflows for advanced users

### 4. The common denominator is not chat

Across all three ecosystems, the common primitives are:

- project memory
- reusable task modules
- tool access via MCP or plugins
- automation / headless execution
- approvals and sandboxing
- separate contexts for specialized workers
- reviewable artifacts

That should drive the product design.

## Product Direction

Build an **agent-operable SaaS control plane** for the product domain, not an "AI feature" bolted onto a normal web app.

The product should be fully operable through:

- a rich CLI
- MCP tools
- structured docs and runbooks
- agent-specific memory and skill packs

The GUI should exist for:

- approvals
- observability
- artifact review
- dashboards
- editing structured configs and policies

The GUI should not be the only place where real work can happen.

## Design Principles

1. MCP-first

Every important action in the SaaS should be representable as a tool call. The GUI should consume the same service layer as the MCP server and CLI.

2. CLI-first

Every operator workflow should have a terminal path first, with structured output and stable commands.

3. Docs as execution surface

Instructions should live in versioned Markdown and schema-backed config, not in hidden prompt strings inside code.

4. Review over magic

Agents can draft, inspect, plan, run, and propose. Humans approve destructive or business-critical actions.

5. Explicit environments

Agents need separate dev, staging, and production contexts with clear credentials, permissions, and audit trails.

6. Thin GUI

The GUI is for state inspection, approvals, and human-friendly editing. It should not contain unique business logic.

## Recommended Architecture

### 1. Core control plane

Build a backend around these first-class entities:

- `workspace`
- `environment`
- `tool`
- `runbook`
- `task`
- `task_run`
- `artifact`
- `approval`
- `credential_binding`
- `policy`
- `audit_event`

This becomes the canonical system. Everything else is an interface to it.

### 2. Interface layer

Expose the same core operations in four ways:

- HTTP/JSON API
- MCP server
- CLI
- GUI

The rule should be:

- no GUI-only actions
- no undocumented agent-only actions
- every action has a stable machine-readable form

### 3. Agent compatibility layer

Support each runtime with native conventions:

#### Codex

- repository `AGENTS.md`
- `.agents/skills/*`
- `codex exec` automation examples
- MCP config snippets
- worktree-friendly task flows

#### Claude Code

- repository `CLAUDE.md`
- `.claude/commands/*`
- `.claude/agents/*`
- `.claude/hooks/*`
- MCP setup docs

#### OpenClaw

- `skills/*` bundles compatible with its skill model
- optional plugin package for richer integration
- OpenProse examples for multi-agent workflows
- workspace setup docs

### 4. Execution model

Long-running operations should run as background jobs with:

- a queued/in-progress/completed/failed lifecycle
- logs and machine-readable events
- artifacts captured on completion
- approval checkpoints before sensitive actions

### 5. Artifact model

Agents should produce explicit artifacts, not just messages:

- plans
- diffs
- SQL migrations
- release notes
- risk reports
- incident summaries
- sourcing analyses
- audit packs

Artifacts should be addressable, reviewable, downloadable, and attributable to a run.

## What to Build

### Phase 0: Product reset

Stop extending the current demo code.

Keep only:

- domain learnings
- any useful naming
- research artifacts

Do not preserve the current architecture.

### Phase 1: Foundational platform

Build these first:

1. New backend service
2. New CLI
3. MCP server
4. Auth, orgs, workspaces, environments
5. Audit/event log
6. Job runner
7. Artifact store
8. Policy and approval engine

Deliverable:

- a usable control plane even before the domain-specific GUI is rich

### Phase 2: Agent-native operating surfaces

Build:

1. `AGENTS.md` and Codex skill pack for the repo
2. `CLAUDE.md`, slash commands, hooks, and subagent templates
3. OpenClaw skill pack and optional plugin
4. Example CI flows using `codex exec` and Claude headless mode
5. Runbook docs that map common tasks to CLI, MCP, and agent workflows

Deliverable:

- a new operator can clone the repo and run real workflows through their preferred agent

### Phase 3: GUI

Build the GUI as an operations console, not as the product brain.

Core screens:

- workspace overview
- task runs
- approvals inbox
- artifacts browser
- environment/config editor
- tool registry
- runbook catalog
- audit timeline

Nice-to-have later:

- live agent session monitor
- prompt/memory debugger
- workflow graph visualizer

### Phase 4: Domain modules

After the control plane exists, rebuild the product domain on top of it.

For this repo’s likely domain direction, that means:

- sourcing or procurement workflows expressed as tasks, runbooks, tools, and artifacts
- agent-readable category, supplier, event, and policy context
- repeatable reports and counterfactual analyses

The domain logic should plug into the platform, not reinvent it.

## CLI Plan

The CLI is a product, not a dev convenience.

It should support:

- human-readable output
- JSON output
- streaming events
- non-interactive mode
- shell completion
- stable resource naming

Suggested command areas:

- `evo login`
- `evo workspace`
- `evo env`
- `evo tool`
- `evo task`
- `evo runbook`
- `evo run`
- `evo artifact`
- `evo approval`
- `evo audit`
- `evo docs`
- `evo mcp`

Important:

- every write operation should support `--json`
- long tasks should support `watch`
- approvals should be scriptable

## Documentation Plan

Documentation needs to do three jobs:

1. Teach humans
2. Feed agents
3. Define policy

Documentation structure:

- `docs/intro/`
- `docs/runbooks/`
- `docs/tools/`
- `docs/policies/`
- `docs/agent-guides/`
- `docs/api/`

Agent-specific docs:

- `AGENTS.md`
- `CLAUDE.md`
- `.agents/skills/*`
- `.claude/commands/*`
- `.claude/agents/*`
- `openclaw/skills/*`

Every runbook should have:

- purpose
- required permissions
- CLI path
- MCP tool path
- expected artifacts
- rollback or recovery notes

## GUI Plan

The GUI should optimize for:

- trust
- visibility
- approvals
- configuration

Not for:

- hiding system behavior
- trapping users in the browser

The GUI should be able to show:

- what task ran
- with what instructions
- against which workspace/environment
- using which tools
- what artifacts were produced
- what approvals were requested
- who approved them

## Technology Recommendation

### Backend

- TypeScript or Go for the control plane
- PostgreSQL for canonical state
- object storage for artifacts
- queue/job system for long-running runs

### Agent orchestration

- use the vendor-native surfaces where available
- do not build a large custom agent loop first
- for OpenAI-native internal workflows, use Responses API plus background mode when you need your own backend-run agents

### Frontend

- a thin React app is fine
- prioritize operational clarity over marketing UI

## Key Decisions

### Decision 1

Primary abstraction is `task run`, not `chat`.

### Decision 2

Primary integration protocol is MCP, not a bespoke plugin API.

### Decision 3

Primary user surface is CLI + docs, not GUI-only.

### Decision 4

Codex and Claude Code are tier-1 integration targets.

### Decision 5

OpenClaw support is important, but should be built through skills/plugins as a compatibility layer, not as the main product mental model.

## Near-Term Build Order

1. Write the new product spec and domain boundaries
2. Define the core resource schema for workspaces, tasks, runs, artifacts, approvals, and tools
3. Build the CLI skeleton against a stubbed backend
4. Build the MCP server on the same service layer
5. Build auth, workspace management, and audit logging
6. Add background task execution and artifact persistence
7. Add Codex-native repo files and skill packs
8. Add Claude-native repo files, commands, hooks, and subagents
9. Add the first thin GUI
10. Rebuild the domain workflows on top

## Final Recommendation

The right reboot is not "make the app smarter."

It is:

- make the product operable by agents
- make all important workflows explicit and machine-readable
- make documentation part of the runtime
- make approvals and artifacts first-class
- make the GUI a control tower rather than the only interface

That gives us something structurally aligned with Codex, Claude Code, and OpenClaw instead of trying to imitate them badly inside a conventional SaaS.
