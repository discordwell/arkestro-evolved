# CLAUDE.md

## Product

This repository is an AI-native SaaS control plane. Claude-facing automation should operate through:

- versioned runbooks in `catalog/runbooks.json`
- the public HTTP API
- the MCP server in `packages/mcp`
- approval-aware workflows only

## Claude workflow conventions

- Prefer documented runbooks over one-off shell plans.
- Treat approvals as mandatory checkpoints for external writes.
- Use the console only for visibility, review, and approvals; business logic belongs in the control plane.
- Keep agent-facing docs synchronized with the shared runbook catalog.

