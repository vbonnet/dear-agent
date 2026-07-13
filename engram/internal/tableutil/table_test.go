package tableutil

import (
	"strings"
	"testing"
)

// --- RenderPlainMarkdown ---

func TestRenderPlainMarkdown_Empty(t *testing.T) {
	tbl := NewTable("title", []string{"A", "B"}, nil)
	got := tbl.RenderPlainMarkdown()
	if got != "" {
		t.Errorf("expected empty string for no rows, got %q", got)
	}
}

func TestRenderPlainMarkdown_Structure(t *testing.T) {
	rows := [][]string{
		{"foo", "bar"},
		{"baz", "qux"},
	}
	tbl := NewTable("My Table", []string{"Col1", "Col2"}, rows)
	got := tbl.RenderPlainMarkdown()

	if !strings.HasPrefix(got, "My Table:\n") {
		t.Errorf("output should start with title")
	}
	if !strings.Contains(got, "| Col1 | Col2 |") {
		t.Errorf("missing header row in: %q", got)
	}
	if !strings.Contains(got, "| --- |") {
		t.Errorf("missing separator row in: %q", got)
	}
	if !strings.Contains(got, "| foo | bar |") {
		t.Errorf("missing data row in: %q", got)
	}
	if !strings.Contains(got, "| baz | qux |") {
		t.Errorf("missing data row in: %q", got)
	}
}

func TestRenderPlainMarkdown_NoTitle(t *testing.T) {
	tbl := NewTable("", []string{"X"}, [][]string{{"v"}})
	got := tbl.RenderPlainMarkdown()
	if strings.HasPrefix(got, ":\n") {
		t.Errorf("empty title should not prefix output with ':', got: %q", got[:10])
	}
	if !strings.Contains(got, "| X |") {
		t.Errorf("missing header in: %q", got)
	}
}

// --- FormatCSV ---

func TestFormatCSV_Headers(t *testing.T) {
	var sb strings.Builder
	FormatCSV([]string{"a", "b", "c"}, nil, &sb)
	got := sb.String()
	if got != "a,b,c\n" {
		t.Errorf("FormatCSV headers = %q, want %q", got, "a,b,c\n")
	}
}

func TestFormatCSV_DataRows(t *testing.T) {
	var sb strings.Builder
	rows := [][]string{{"1", "2"}, {"3", "4"}}
	FormatCSV([]string{"x", "y"}, rows, &sb)
	got := sb.String()
	if !strings.Contains(got, "1,2\n") {
		t.Errorf("missing first row in: %q", got)
	}
	if !strings.Contains(got, "3,4\n") {
		t.Errorf("missing second row in: %q", got)
	}
}

func TestFormatCSV_EscapesStructuredValues(t *testing.T) {
	var sb strings.Builder
	FormatCSV([]string{"name", "note"}, [][]string{{"alpha,beta", "quoted \"value\""}}, &sb)
	if got, want := sb.String(), "name,note\n\"alpha,beta\",\"quoted \"\"value\"\"\"\n"; got != want {
		t.Fatalf("FormatCSV() = %q, want %q", got, want)
	}
}

func TestFormatJSON_UsesJSONEncoding(t *testing.T) {
	got, err := FormatJSON(map[string]string{"name": "engram"})
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"name":"engram"}` {
		t.Fatalf("FormatJSON() = %q", got)
	}
}
