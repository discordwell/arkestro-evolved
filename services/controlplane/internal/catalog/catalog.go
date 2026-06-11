package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
)

type Catalog struct {
	Version  int              `json:"version"`
	Runbooks []domain.Runbook `json:"runbooks"`
}

func Load(path string) (Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, fmt.Errorf("read catalog: %w", err)
	}

	var c Catalog
	if err := json.Unmarshal(raw, &c); err != nil {
		return Catalog{}, fmt.Errorf("decode catalog: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Catalog{}, fmt.Errorf("invalid catalog: %w", err)
	}
	return c, nil
}

var validStepKinds = map[string]bool{
	"read":     true,
	"artifact": true,
	"approval": true,
	"write":    true,
}

// slugPattern constrains runbook and step slugs to URL- and filesystem-safe
// names: step slugs become artifact file names under the object-store root,
// so path separators, "..", and uppercase are all unsafe.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Validate checks the structural invariants the worker relies on, so a broken
// catalog fails at boot instead of failing runs mid-execution.
func (c Catalog) Validate() error {
	if c.Version < 1 {
		return fmt.Errorf("catalog version must be >= 1, got %d", c.Version)
	}
	slugs := make(map[string]bool, len(c.Runbooks))
	for _, runbook := range c.Runbooks {
		if runbook.Slug == "" {
			return errors.New("runbook slug is required")
		}
		if !slugPattern.MatchString(runbook.Slug) {
			return fmt.Errorf("runbook slug %q must be lowercase letters, digits, and hyphens", runbook.Slug)
		}
		if slugs[runbook.Slug] {
			return fmt.Errorf("duplicate runbook slug %q", runbook.Slug)
		}
		slugs[runbook.Slug] = true
		if runbook.Title == "" {
			return fmt.Errorf("runbook %q: title is required", runbook.Slug)
		}
		if len(runbook.Steps) == 0 {
			return fmt.Errorf("runbook %q: at least one step is required", runbook.Slug)
		}
		stepSlugs := make(map[string]bool, len(runbook.Steps))
		hasApproval := false
		for i, step := range runbook.Steps {
			if step.Slug == "" {
				return fmt.Errorf("runbook %q: step %d: slug is required", runbook.Slug, i)
			}
			if !slugPattern.MatchString(step.Slug) {
				return fmt.Errorf("runbook %q: step slug %q must be lowercase letters, digits, and hyphens", runbook.Slug, step.Slug)
			}
			if stepSlugs[step.Slug] {
				return fmt.Errorf("runbook %q: duplicate step slug %q", runbook.Slug, step.Slug)
			}
			stepSlugs[step.Slug] = true
			if !validStepKinds[step.Kind] {
				return fmt.Errorf("runbook %q: step %q: unknown kind %q", runbook.Slug, step.Slug, step.Kind)
			}
			if step.Kind == "approval" {
				hasApproval = true
			}
		}
		if runbook.ApprovalRequired != hasApproval {
			return fmt.Errorf("runbook %q: approval_required=%t does not match approval steps present=%t", runbook.Slug, runbook.ApprovalRequired, hasApproval)
		}
	}
	return nil
}

func (c Catalog) Runbook(slug string) (domain.Runbook, bool) {
	for _, runbook := range c.Runbooks {
		if runbook.Slug == slug {
			return runbook, true
		}
	}
	return domain.Runbook{}, false
}

func (c Catalog) TaskTemplates() []domain.TaskTemplate {
	out := make([]domain.TaskTemplate, 0)
	for _, runbook := range c.Runbooks {
		for _, step := range runbook.Steps {
			out = append(out, domain.TaskTemplate{
				ID:          runbook.Slug + ":" + step.Slug,
				RunbookSlug: runbook.Slug,
				StepSlug:    step.Slug,
				Kind:        step.Kind,
			})
		}
	}
	return out
}
