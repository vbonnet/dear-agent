package tableutil

import (
	"strings"
	"testing"
)

func TestRenderPlainMarkdown_Empty(t *testing.T) {
	t.Parallel()
	tr := NewTable("My Table", []string{"A", "B"}, nil)
	got := tr.RenderPlainMarkdown()
	if got != "" {
		t.Errorf("RenderPlainMarkdown with no rows = %q, want empty string", got)
	}
}

func TestRenderPlainMarkdown_Basic(t *testing.T) {
	t.Parallel()
	rows := [][]string{
		{"r1c1", "r1c2"},
		{"r2c1", "r2c2"},
	}
	tr := NewTable("Test", []string{"Col1", "Col2"}, rows)
	got := tr.RenderPlainMarkdown()

	if !strings.Contains(got, "Test:") {
		t.Errorf("missing title; got %q", got)
	}
	if !strings.Contains(got, "| Col1 | Col2 |") {
		t.Errorf("missing header row; got %q", got)
	}
	if !strings.Contains(got, "| r1c1 | r1c2 |") {
		t.Errorf("missing first data row; got %q", got)
	}
	if !strings.Contains(got, "| r2c1 | r2c2 |") {
		t.Errorf("missing second data row; got %q", got)
	}
	// Separator row
	if !strings.Contains(got, "| --- | --- |") {
		t.Errorf("missing separator row; got %q", got)
	}
}

func TestRenderPlainMarkdown_NoTitle(t *testing.T) {
	t.Parallel()
	tr := NewTable("", []string{"X"}, [][]string{{"val"}})
	got := tr.RenderPlainMarkdown()
	if strings.Contains(got, ":") && strings.HasPrefix(got, ":") {
		t.Errorf("unexpected colon prefix for empty title; got %q", got)
	}
	if !strings.Contains(got, "| X |") {
		t.Errorf("missing header; got %q", got)
	}
}

func TestFormatCSV_Basic(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	FormatCSV(
		[]string{"Name", "Age"},
		[][]string{{"Alice", "30"}, {"Bob", "25"}},
		&sb,
	)
	got := sb.String()

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("FormatCSV: got %d lines, want 3; output = %q", len(lines), got)
	}
	if lines[0] != "Name,Age" {
		t.Errorf("header = %q, want %q", lines[0], "Name,Age")
	}
	if lines[1] != "Alice,30" {
		t.Errorf("row 1 = %q, want %q", lines[1], "Alice,30")
	}
	if lines[2] != "Bob,25" {
		t.Errorf("row 2 = %q, want %q", lines[2], "Bob,25")
	}
}

func TestFormatCSV_Empty(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	FormatCSV([]string{"A"}, nil, &sb)
	got := sb.String()
	// Header only.
	if !strings.HasPrefix(got, "A\n") {
		t.Errorf("FormatCSV with no data rows = %q, want header line only", got)
	}
}

func TestFormatJSON_ReturnsString(t *testing.T) {
	t.Parallel()
	got, err := FormatJSON(42)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	if got == "" {
		t.Error("FormatJSON returned empty string")
	}
}
