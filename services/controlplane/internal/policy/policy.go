// Package policy evaluates policy rules against runbook step actions. A step's
// action is the string "<kind>.<slug>" (e.g. "write.execute-rollout"); a rule's
// action_pattern is matched against it to decide whether the step must stop for
// approval. This is the runtime authority behind the platform contract that
// sensitive actions stop for approval — the catalog enforces the same invariant
// structurally at boot, and policy enforces it dynamically at execution time.
package policy

import (
	"strings"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
)

// Action builds the canonical action string for a runbook step.
func Action(kind, slug string) string {
	return kind + "." + slug
}

// Match reports whether action matches pattern. Three forms are supported:
//
//   - "*"          matches any action
//   - "prefix.*"   matches "prefix" and anything under "prefix." (so "write.*"
//     matches both "write" and "write.execute-rollout")
//   - "exact"      matches only an identical action
//
// The wildcard is intentionally limited (no general globbing) so patterns stay
// predictable and auditable.
func Match(pattern, action string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return action == prefix || strings.HasPrefix(action, prefix+".")
	}
	return pattern == action
}

// RequiresApproval reports whether any rule mandates approval for the action,
// returning the first matching rule that requires it. Rules are evaluated in the
// order given (callers pass them in the repository's deterministic created_at,id
// order), so the result is stable. A rule that matches but has
// ApprovalRequired=false does not force approval; only a matching rule with
// ApprovalRequired=true does. When nothing requires approval the second return
// value is nil.
func RequiresApproval(rules []domain.PolicyRule, action string) (bool, *domain.PolicyRule) {
	for i := range rules {
		if rules[i].ApprovalRequired && Match(rules[i].ActionPattern, action) {
			return true, &rules[i]
		}
	}
	return false, nil
}
