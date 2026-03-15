# AGENTS.md

## Product

This repository is an AI-native SaaS control plane for running operational workflows through CLI, MCP, ChatGPT, Codex, Claude Code, and a thin web console.

## Core rules

- Treat the Go control plane API as the system of record.
- Prefer using runbooks from `catalog/runbooks.json` instead of inventing ad hoc workflow names.
- Sensitive actions must stop for approval; do not bypass approval rules in code or docs.
- Keep all operator-visible actions available via HTTP API, CLI, and MCP.
- Do not add GUI-only business logic.

## Primary surfaces

- CLI: `packages/cli`
- MCP server: `packages/mcp`
- ChatGPT app host: `apps/chatgpt-app`
- Console: `apps/console`
- Control plane API and worker: `services/controlplane`

## Runbook catalog

Shared workflow definitions live in `catalog/runbooks.json`. Human docs and agent docs should derive from the same workflow semantics.

