package catalog

import (
	"encoding/json"
	"fmt"
	"os"

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
	return c, nil
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
