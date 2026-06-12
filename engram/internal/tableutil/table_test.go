package tableutil_test

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/tableutil"
)

func TestRenderPlainMarkdown_Basic(t *testing.T) {
	t.Parallel()
	tbl := tableutil.NewTable("", []string{"A", "B"}, [][]string{{"x", "y"}, {"p", "q"}})
	out := tbl.RenderPlainMarkdown()
	if !strings.Contains(out, "| A | B |") {
		t.Errorf("output missing header row; got:\n%s", out)
	}
	if !strings.Contains(out, "| x | y |") {
		t.Errorf("output missing data row; got:\n%s", out)
	}
	if !strings.Contains(out, "| --- |") {
		t.Errorf("output missing separator; got:\n%s", out)
	}
}

func TestRenderPlainMarkdown_WithTitle(t *testing.T) {
	t.Parallel()
	tbl := tableutil.NewTable("My Table", []string{"Col"}, [][]string{{"val"}})
	out := tbl.RenderPlainMarkdown()
	if !strings.HasPrefix(out, "My Table:\n") {
		t.Errorf("output should start with title; got:\n%s", out)
	}
}

func TestRenderPlainMarkdown_EmptyRows(t *testing.T) {
	t.Parallel()
	tbl := tableutil.NewTable("T", []string{"A"}, [][]string{})
	out := tbl.RenderPlainMarkdown()
	if out != "" {
		t.Errorf("empty rows should produce empty output; got: %q", out)
	}
}

func TestFormatCSV_Basic(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	tableutil.FormatCSV([]string{"Name", "Value"}, [][]string{{"alpha", "1"}, {"beta", "2"}}, &sb)
	got := sb.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 data), got %d: %q", len(lines), got)
	}
	if lines[0] != "Name,Value" {
		t.Errorf("header line = %q, want %q", lines[0], "Name,Value")
	}
	if lines[1] != "alpha,1" {
		t.Errorf("data line 1 = %q, want %q", lines[1], "alpha,1")
	}
}

func TestFormatCSV_EmptyRows(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	tableutil.FormatCSV([]string{"H"}, [][]string{}, &sb)
	got := sb.String()
	// Should have the header but no data rows.
	if got != "H\n" {
		t.Errorf("empty rows: got %q, want %q", got, "H\n")
	}
}

func TestFormatCSV_MultipleColumns(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	tableutil.FormatCSV([]string{"A", "B", "C"}, [][]string{{"1", "2", "3"}}, &sb)
	got := sb.String()
	if !strings.Contains(got, "A,B,C") {
		t.Errorf("header missing in output: %q", got)
	}
	if !strings.Contains(got, "1,2,3") {
		t.Errorf("data row missing in output: %q", got)
	}
}
