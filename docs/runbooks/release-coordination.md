# Release Coordination

- Family: `release`
- Approval required: yes
- Tool connections: `deploy`, `observability`, `repo`
- Expected artifacts: `release-note`, `risk-report`, `deployment-summary`
- Failure modes: `staging-check-failed`, `approval-rejected`, `deployment-failed`

Steps:

1. Collect context
2. Draft release notes
3. Request rollout approval
4. Execute rollout
5. Publish summary

