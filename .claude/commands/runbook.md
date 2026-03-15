# /runbook

Use this command flow when Claude needs to inspect or launch a runbook-backed workflow.

1. List runbooks through MCP or `evo runbook list`
2. Select the runbook by slug
3. Launch the run with explicit `workspace_id`
4. Watch the run until it reaches a terminal or approval state
5. If approval is pending, stop and ask for approval

