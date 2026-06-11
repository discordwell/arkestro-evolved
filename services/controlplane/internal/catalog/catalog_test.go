package catalog_test

import (
	"strings"
	"testing"

	"github.com/discordwell/evo-control-plane/services/controlplane/internal/catalog"
	"github.com/discordwell/evo-control-plane/services/controlplane/internal/domain"
)

func validRunbook() domain.Runbook {
	return domain.Runbook{
		Slug:             "demo",
		Title:            "Demo",
		ApprovalRequired: true,
		Steps: []domain.RunbookStep{
			{Slug: "collect", Kind: "read"},
			{Slug: "draft", Kind: "artifact"},
			{Slug: "gate", Kind: "approval"},
			{Slug: "apply", Kind: "write"},
		},
	}
}

func TestShippedCatalogLoadsAndValidates(t *testing.T) {
	cat, err := catalog.Load("../../../../catalog/runbooks.json")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cat.Runbooks) == 0 {
		t.Fatalf("expected runbooks in the shipped catalog")
	}
	if _, ok := cat.Runbook("release-coordination"); !ok {
		t.Fatalf("expected release-coordination runbook")
	}
	templates := cat.TaskTemplates()
	wantTemplates := 0
	for _, runbook := range cat.Runbooks {
		wantTemplates += len(runbook.Steps)
	}
	if len(templates) != wantTemplates {
		t.Fatalf("expected %d task templates, got %d", wantTemplates, len(templates))
	}
}

func TestValidateRejectsBrokenCatalogs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*catalog.Catalog)
		wantErr string
	}{
		{
			name:    "version zero",
			mutate:  func(c *catalog.Catalog) { c.Version = 0 },
			wantErr: "version",
		},
		{
			name: "duplicate runbook slug",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks = append(c.Runbooks, c.Runbooks[0])
			},
			wantErr: "duplicate runbook slug",
		},
		{
			name: "runbook slug with unsafe characters",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Slug = "../escape"
			},
			wantErr: "lowercase letters, digits, and hyphens",
		},
		{
			name: "step slug with path separator",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Steps[0].Slug = "nested/step"
			},
			wantErr: "lowercase letters, digits, and hyphens",
		},
		{
			name: "uppercase step slug",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Steps[0].Slug = "Collect"
			},
			wantErr: "lowercase letters, digits, and hyphens",
		},
		{
			name: "missing title",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Title = ""
			},
			wantErr: "title is required",
		},
		{
			name: "no steps",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Steps = nil
			},
			wantErr: "at least one step",
		},
		{
			name: "duplicate step slug",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Steps[1].Slug = c.Runbooks[0].Steps[0].Slug
			},
			wantErr: "duplicate step slug",
		},
		{
			name: "unknown step kind",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Steps[0].Kind = "yolo"
			},
			wantErr: "unknown kind",
		},
		{
			name: "approval flag without approval step",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].Steps[2].Kind = "read"
				c.Runbooks[0].Steps[2].Slug = "extra-read"
			},
			wantErr: "approval_required",
		},
		{
			name: "approval step without approval flag",
			mutate: func(c *catalog.Catalog) {
				c.Runbooks[0].ApprovalRequired = false
			},
			wantErr: "approval_required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := catalog.Catalog{Version: 1, Runbooks: []domain.Runbook{validRunbook()}}
			tc.mutate(&cat)
			err := cat.Validate()
			if err == nil {
				t.Fatalf("expected validation error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestValidateAcceptsValidCatalog(t *testing.T) {
	cat := catalog.Catalog{Version: 1, Runbooks: []domain.Runbook{validRunbook()}}
	if err := cat.Validate(); err != nil {
		t.Fatalf("expected valid catalog, got %v", err)
	}
}
