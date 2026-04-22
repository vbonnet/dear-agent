package wikibrain_test

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

func TestAuditBacklinks_FindsMatchingPage(t *testing.T) {
	newPage := &wikibrain.Page{
		RelPath: "02-research-index/topic-ecphory-retrieval.md",
		Title:   "Ecphory Retrieval",
		Summary: "How ecphory-based retrieval works in engram.",
	}
	existing := []*wikibrain.Page{
		{
			RelPath: "01-decisions/ADR-003.md",
			Title:   "Three-Tier Ecphory",
			Summary: "Ecphory retrieval underpins the three-tier memory architecture.",
		},
		{
			RelPath: "02-research-index/topic-unrelated.md",
			Title:   "Something Else",
			Summary: "Not related to retrieval at all.",
		},
	}

	suggestions := wikibrain.AuditBacklinks(newPage, existing)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one backlink suggestion")
	}
	if suggestions[0].SourcePage != "01-decisions/ADR-003.md" {
		t.Errorf("expected suggestion for ADR-003, got %s", suggestions[0].SourcePage)
	}
}

func TestAuditBacklinks_SkipsAlreadyLinked(t *testing.T) {
	newPage := &wikibrain.Page{
		RelPath: "02-research-index/topic-ecphory-retrieval.md",
		Title:   "Ecphory Retrieval",
		Summary: "Ecphory-based retrieval.",
	}
	existing := []*wikibrain.Page{
		{
			RelPath:   "01-decisions/ADR-003.md",
			Title:     "Three-Tier Ecphory",
			Summary:   "Ecphory retrieval system.",
			WikiLinks: []string{"topic-ecphory-retrieval"}, // already linked
		},
	}

	suggestions := wikibrain.AuditBacklinks(newPage, existing)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions (already linked), got %d", len(suggestions))
	}
}

func TestAuditBacklinks_NoFalsePositives(t *testing.T) {
	newPage := &wikibrain.Page{
		RelPath: "02-research-index/topic-bdd-framework.md",
		Title:   "BDD Framework Evolution",
		Summary: "How BDD testing evolved.",
	}
	existing := []*wikibrain.Page{
		{
			RelPath: "01-decisions/ADR-001.md",
			Title:   "Go as Primary Language",
			Summary: "Go is fast and compiled.",
		},
	}

	suggestions := wikibrain.AuditBacklinks(newPage, existing)
	if len(suggestions) != 0 {
		t.Errorf("unexpected false-positive suggestion: %+v", suggestions)
	}
}
