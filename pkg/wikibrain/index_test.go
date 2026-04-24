package wikibrain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

func TestGenerateIndex_ContainsExpectedSections(t *testing.T) {
	pages := []*wikibrain.Page{
		{
			RelPath:        "01-decisions/ADR-001.md",
			Title:          "Go as Primary Language",
			Summary:        "Go is the primary language.",
			HasLastUpdated: true,
			LastUpdated:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			RelPath: "02-research-index/topic-foo.md",
			Title:   "Foo Research",
			Summary: "About foo.",
		},
	}

	content := wikibrain.GenerateIndex(pages, time.Now())

	if !strings.Contains(content, "# engram-kb") {
		t.Error("missing title heading")
	}
	if !strings.Contains(content, "01-decisions") {
		t.Error("missing 01-decisions section")
	}
	if !strings.Contains(content, "02-research-index") {
		t.Error("missing 02-research-index section")
	}
	if !strings.Contains(content, "Go as Primary Language") {
		t.Error("missing page title")
	}
	if !strings.Contains(content, "2026-01-15") {
		t.Error("missing last-updated date")
	}
	if !strings.Contains(content, "Total pages:** 2") {
		t.Error("missing page count")
	}
}

func TestGenerateIndex_LongSummaryTruncated(t *testing.T) {
	long := strings.Repeat("x", 200)
	pages := []*wikibrain.Page{
		{
			RelPath: "01-decisions/ADR-001.md",
			Title:   "Title",
			Summary: long,
		},
	}

	content := wikibrain.GenerateIndex(pages, time.Now())
	if strings.Contains(content, long) {
		t.Error("long summary should be truncated in the index")
	}
	if !strings.Contains(content, "...") {
		t.Error("truncated summary should end with ...")
	}
}
