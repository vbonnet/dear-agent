package backlog

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func indexByID(items []Item) map[string]Item {
	m := make(map[string]Item, len(items))
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

func TestMarkdownSourceItems(t *testing.T) {
	src := NewMarkdownSource(filepath.Join("testdata", "sample_backlog.md"))
	if !strings.Contains(src.Name(), "sample_backlog.md") {
		t.Errorf("Name() = %q", src.Name())
	}
	items, err := src.Items(context.Background())
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 12 {
		t.Fatalf("len(items) = %d, want 12", len(items))
	}
	by := indexByID(items)

	want := []struct {
		id     string
		phase  int
		status Status
		prio   Priority
		effort Effort
		deps   []string
	}{
		{"0.1", 0, StatusDone, PriorityUnset, EffortMedium, nil},
		{"0.2", 0, StatusDone, PriorityUnset, EffortSmall, []string{"0.1"}},
		{"1.1", 1, StatusPending, PriorityUnset, EffortMedium, []string{"0.*"}},
		{"1.3", 1, StatusInFlight, PriorityUnset, EffortLarge, []string{"1.1"}},
		{"1.5", 1, StatusPending, PriorityUnset, EffortMedium, []string{"9.9"}},
		{"6.1", 6, StatusUnknown, PriorityHigh, EffortUnknown, nil},
		{"6.3", 6, StatusDone, PriorityLow, EffortUnknown, nil},
		{"X.1", -1, StatusUnknown, PriorityUnset, EffortUnknown, nil},
		{"DEAR-X.5", -1, StatusDone, PriorityUnset, EffortUnknown, nil},
	}
	for _, w := range want {
		it, ok := by[w.id]
		if !ok {
			t.Errorf("missing item %s", w.id)
			continue
		}
		if it.Phase != w.phase || it.Status != w.status ||
			it.Priority != w.prio || it.Effort != w.effort {
			t.Errorf("%s = {phase %d status %v prio %v effort %v}, want {%d %v %v %v}",
				w.id, it.Phase, it.Status, it.Priority, it.Effort,
				w.phase, w.status, w.prio, w.effort)
		}
		if strings.Join(it.Deps, ",") != strings.Join(w.deps, ",") {
			t.Errorf("%s deps = %v, want %v", w.id, it.Deps, w.deps)
		}
	}
}

func TestMarkdownSourceMissingFileSkipped(t *testing.T) {
	real := filepath.Join("testdata", "sample_backlog.md")
	src := NewMarkdownSource("does/not/exist.md", real)
	items, err := src.Items(context.Background())
	if err != nil {
		t.Fatalf("missing file should be skipped, got err: %v", err)
	}
	if len(items) != 12 {
		t.Errorf("len = %d, want 12 (missing file ignored)", len(items))
	}

	only := NewMarkdownSource("nope.md")
	items, err = only.Items(context.Background())
	if err != nil || len(items) != 0 {
		t.Errorf("all-missing: items=%d err=%v, want 0 items nil err", len(items), err)
	}
}

func TestParseMarkdownNoTables(t *testing.T) {
	items, err := parseMarkdown(strings.NewReader("# Title\n\nJust prose | with a pipe.\n"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len = %d, want 0", len(items))
	}
}

func TestParseMarkdownAlignmentSeparator(t *testing.T) {
	doc := `## P
| # | Title | Status |
|:--|:-----:|------:|
| 2.1 | Aligned row | pending |
`
	items, err := parseMarkdown(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(items) != 1 || items[0].ID != "2.1" || items[0].Status != StatusPending {
		t.Errorf("got %+v, want one pending item 2.1", items)
	}
	if items[0].Section != "P" {
		t.Errorf("Section = %q, want P", items[0].Section)
	}
}
