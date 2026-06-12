package tableutil

import (
	"strings"
	"testing"
)

func TestNewTable_NotNil(t *testing.T) {
	tr := NewTable("Title", []string{"A", "B"}, [][]string{{"1", "2"}})
	if tr == nil {
		t.Fatal("NewTable returned nil")
	}
}

func TestRenderPlainMarkdown_EmptyRows_ReturnsEmpty(t *testing.T) {
	tr := NewTable("T", []string{"Col"}, nil)
	if got := tr.RenderPlainMarkdown(); got != "" {
		t.Errorf("RenderPlainMarkdown empty rows = %q, want \"\"", got)
	}
}

func TestRenderPlainMarkdown_ContainsHeaders(t *testing.T) {
	tr := NewTable("", []string{"Name", "Value"}, [][]string{{"foo", "bar"}})
	got := tr.RenderPlainMarkdown()
	if !strings.Contains(got, "Name") || !strings.Contains(got, "Value") {
		t.Errorf("RenderPlainMarkdown missing headers, got: %q", got)
	}
}

func TestRenderPlainMarkdown_ContainsRow(t *testing.T) {
	tr := NewTable("", []string{"Col"}, [][]string{{"hello"}})
	got := tr.RenderPlainMarkdown()
	if !strings.Contains(got, "hello") {
		t.Errorf("RenderPlainMarkdown missing row data, got: %q", got)
	}
}

func TestRenderPlainMarkdown_WithTitle(t *testing.T) {
	tr := NewTable("My Title", []string{"Col"}, [][]string{{"v"}})
	got := tr.RenderPlainMarkdown()
	if !strings.Contains(got, "My Title") {
		t.Errorf("RenderPlainMarkdown missing title, got: %q", got)
	}
}

func TestFormatCSV_HeadersAndRows(t *testing.T) {
	var sb strings.Builder
	FormatCSV([]string{"a", "b"}, [][]string{{"1", "2"}, {"3", "4"}}, &sb)
	got := sb.String()
	if !strings.Contains(got, "a,b") {
		t.Errorf("FormatCSV missing headers: %q", got)
	}
	if !strings.Contains(got, "1,2") || !strings.Contains(got, "3,4") {
		t.Errorf("FormatCSV missing row data: %q", got)
	}
}

func TestFormatJSON_ReturnsString(t *testing.T) {
	got, err := FormatJSON(map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}
	if got == "" {
		t.Error("FormatJSON returned empty string")
	}
}
