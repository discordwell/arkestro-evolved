package policy_test

import (
	"testing"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/policy"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		action  string
		want    bool
	}{
		{"*", "write.execute-rollout", true},
		{"*", "read.collect", true},
		{"write.*", "write.execute-rollout", true},
		{"write.*", "write", true},
		{"write.*", "writes.execute", false}, // prefix boundary must be a dot
		{"write.*", "read.collect", false},
		{"write.execute-rollout", "write.execute-rollout", true},
		{"write.execute-rollout", "write.execute-mitigation", false},
	}
	for _, tc := range cases {
		if got := policy.Match(tc.pattern, tc.action); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.action, got, tc.want)
		}
	}
}

func TestAction(t *testing.T) {
	if got := policy.Action("write", "execute-rollout"); got != "write.execute-rollout" {
		t.Fatalf("Action = %q, want write.execute-rollout", got)
	}
}

func TestRequiresApproval(t *testing.T) {
	rules := []domain.PolicyRule{
		{Name: "log-reads", ActionPattern: "read.*", ApprovalRequired: false},
		{Name: "approval-required-for-write", ActionPattern: "write.*", ApprovalRequired: true},
	}

	required, rule := policy.RequiresApproval(rules, "write.execute-rollout")
	if !required {
		t.Fatalf("expected write action to require approval")
	}
	if rule == nil || rule.Name != "approval-required-for-write" {
		t.Fatalf("expected the write rule to be returned, got %+v", rule)
	}

	// A read action matches only a rule that does not require approval, so it is
	// not gated.
	if required, rule := policy.RequiresApproval(rules, "read.collect"); required || rule != nil {
		t.Fatalf("read action must not require approval, got required=%v rule=%+v", required, rule)
	}

	// An action matched by no rule is not gated.
	if required, _ := policy.RequiresApproval(rules, "artifact.publish"); required {
		t.Fatalf("unmatched action must not require approval")
	}
}

// The first approval-requiring match wins, so attribution is deterministic.
func TestRequiresApprovalReturnsFirstMatch(t *testing.T) {
	rules := []domain.PolicyRule{
		{Name: "specific", ActionPattern: "write.execute-rollout", ApprovalRequired: true},
		{Name: "broad", ActionPattern: "write.*", ApprovalRequired: true},
	}
	_, rule := policy.RequiresApproval(rules, "write.execute-rollout")
	if rule == nil || rule.Name != "specific" {
		t.Fatalf("expected first matching rule 'specific', got %+v", rule)
	}
}
