# Control Plane Skill

Use this skill when operating Evo Control Plane through Codex.

## Primary commands

- `evo workspace list`
- `evo runbook list`
- `evo run create --workspace-id <id> --runbook-slug <slug>`
- `evo run watch <run-id>`
- `evo approval list --workspace-id <id>`
- `evo approval approve <approval-id>`
- `evo approval reject <approval-id>`

## Rules

- Prefer the shared runbook catalog in `catalog/runbooks.json`.
- Do not execute approval-gated work without an explicit approval step.
- Treat the CLI and MCP server as the operator surface; the console is for visibility.

