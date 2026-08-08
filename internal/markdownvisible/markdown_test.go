package markdownvisible

import (
	"strings"
	"testing"
)

func TestLinesExcludesCommonMarkCodeAndHTMLComments(t *testing.T) {
	document := strings.Join([]string{
		"visible before <!-- hidden --> visible after",
		"<!--",
		"**HIDDEN-01** When hidden, the system shall ignore it.",
		"-->",
		"```markdown",
		"**FENCED-01** When copied, the system shall ignore it.",
		"```",
		"",
		"    **INDENTED-01** When copied, the system shall ignore it.",
		"",
		"**VISIBLE-01** When checked, the system shall report it.",
	}, "\n")
	lines := Lines([]byte(document))
	if len(lines) != 11 {
		t.Fatalf("line count = %d, want 11", len(lines))
	}
	if got := strings.Join(strings.Fields(lines[0].Text), " "); got != "visible before visible after" || !lines[0].Visible {
		t.Fatalf("inline comment masking = (%q, %t)", lines[0].Text, lines[0].Visible)
	}
	for _, index := range []int{1, 2, 3, 4, 5, 8} {
		if lines[index].Visible {
			t.Errorf("excluded line %d remained visible: %q", index, lines[index].Text)
		}
	}
	if !lines[7].Visible || !lines[10].Visible || lines[10].Text != "**VISIBLE-01** When checked, the system shall report it." {
		t.Fatalf("visible prose was lost: %#v", lines)
	}
}

func TestLinesUsesCommonMarkFenceRules(t *testing.T) {
	tests := []struct {
		name         string
		document     string
		visibleToken string
		hiddenToken  string
	}{
		{
			name:         "four-space delimiter is indented code, not an open fence",
			document:     "    ```text\n**VISIBLE-01** When checked, the system shall report it.\n",
			visibleToken: "VISIBLE-01",
			hiddenToken:  "```text",
		},
		{
			name:         "three-space fence",
			document:     "   ```text\n**HIDDEN-01** When copied, the system shall ignore it.\n   ```\n**VISIBLE-01** When checked, the system shall report it.\n",
			visibleToken: "VISIBLE-01",
			hiddenToken:  "HIDDEN-01",
		},
		{
			name:         "backtick in backtick info rejects fence",
			document:     "```bad`info\n**VISIBLE-01** When checked, the system shall report it.\n",
			visibleToken: "VISIBLE-01",
		},
		{
			name:         "backtick in tilde info remains a fence",
			document:     "~~~bad`info\n**HIDDEN-01** When copied, the system shall ignore it.\n~~~\n**VISIBLE-01** When checked, the system shall report it.\n",
			visibleToken: "VISIBLE-01",
			hiddenToken:  "HIDDEN-01",
		},
		{
			name:         "nested list fence",
			document:     "- item\n\n  ```text\n  **HIDDEN-01** When copied, the system shall ignore it.\n  ```\n\n**VISIBLE-01** When checked, the system shall report it.\n",
			visibleToken: "VISIBLE-01",
			hiddenToken:  "HIDDEN-01",
		},
		{
			name:         "root closing fence allows at most three spaces",
			document:     "  ```text\n  **HIDDEN-01** When copied, the system shall ignore it.\n    ```\n  ```\n**VISIBLE-01** When checked, the system shall report it.\n",
			visibleToken: "VISIBLE-01",
			hiddenToken:  "HIDDEN-01",
		},
		{
			name:        "unclosed fence",
			document:    "```text\n**HIDDEN-01** When copied, the system shall ignore it.\n",
			hiddenToken: "HIDDEN-01",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := Lines([]byte(test.document))
			visible := visibleText(lines)
			if test.visibleToken != "" && !strings.Contains(visible, test.visibleToken) {
				t.Fatalf("visible text %q lacks %q", visible, test.visibleToken)
			}
			if test.hiddenToken != "" && strings.Contains(visible, test.hiddenToken) {
				t.Fatalf("visible text %q contains excluded %q", visible, test.hiddenToken)
			}
		})
	}
}

func TestLinesMasksContainerNestedFenceLines(t *testing.T) {
	tests := []struct {
		name     string
		document string
		hidden   []int
	}{
		{
			name: "list indentation exceeds root fence allowance",
			document: strings.Join([]string{
				"- item",
				"",
				"    ```text",
				"    **HIDDEN-01** When copied, the system shall ignore it.",
				"    ```",
				"",
				"**VISIBLE-01** When checked, the system shall report it.",
			}, "\n"),
			hidden: []int{2, 3, 4},
		},
		{
			name: "nested blockquote prefixes",
			document: strings.Join([]string{
				"> outer",
				">",
				"> > ```text",
				"> > **HIDDEN-01** When copied, the system shall ignore it.",
				"> > ```",
				"",
				"**VISIBLE-01** When checked, the system shall report it.",
			}, "\n"),
			hidden: []int{2, 3, 4},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := Lines([]byte(test.document))
			visible := visibleText(lines)
			if strings.Contains(visible, "HIDDEN-01") || strings.Contains(visible, "```") {
				t.Fatalf("container-nested fence leaked into visible text: %q", visible)
			}
			if !strings.Contains(visible, "VISIBLE-01") {
				t.Fatalf("prose after container-nested fence was lost: %q", visible)
			}
			for _, index := range test.hidden {
				if lines[index].Visible || strings.TrimSpace(lines[index].Text) != "" {
					t.Errorf("excluded physical line %d = (%q, %t), want blank and hidden", index, lines[index].Text, lines[index].Visible)
				}
			}
		})
	}
}

func TestLinesMasksOpeningAndClosingFenceDelimiters(t *testing.T) {
	document := "```markdown\n**HIDDEN-01** When copied, the system shall ignore it.\n````\n**VISIBLE-01** When checked, the system shall report it.\n"
	visible := visibleText(Lines([]byte(document)))
	for _, token := range []string{"HIDDEN-01", "```", "````"} {
		if strings.Contains(visible, token) {
			t.Fatalf("visible text %q retains fenced token %q", visible, token)
		}
	}
	if !strings.Contains(visible, "VISIBLE-01") {
		t.Fatalf("visible text %q lost prose after closing fence", visible)
	}
}

func TestLinesMasksEmptyFenceDelimiters(t *testing.T) {
	document := "```\n```\n**VISIBLE-01** When checked, the system shall report it.\n"
	visible := visibleText(Lines([]byte(document)))
	if strings.Contains(visible, "```") || !strings.Contains(visible, "VISIBLE-01") {
		t.Fatalf("empty fence visibility = %q", visible)
	}
}

func TestLinesExcludesAllCommonMarkHTMLBlockForms(t *testing.T) {
	forms := []string{
		"<script>\n**HIDDEN-01** When script, the system shall ignore it.\n</script>",
		"<!--\n**HIDDEN-02** When comment, the system shall ignore it.\n-->",
		"<?processing\n**HIDDEN-03** When processing, the system shall ignore it.\n?>",
		"<!DOCTYPE html\n**HIDDEN-04** When declaration, the system shall ignore it.\n>",
		"<![CDATA[\n**HIDDEN-05** When cdata, the system shall ignore it.\n]]>",
		"<div>\n**HIDDEN-06** When block tag, the system shall ignore it.\n</div>",
		"<custom-element attr=\"x\">\n**HIDDEN-07** When complete tag, the system shall ignore it.",
	}
	for index, form := range forms {
		visible := visibleText(Lines([]byte(form + "\n\n**VISIBLE-01** When checked, the system shall report it.\n")))
		if strings.Contains(visible, "HIDDEN-") {
			t.Fatalf("HTML form %d leaked hidden prose: %q", index+1, visible)
		}
		if !strings.Contains(visible, "VISIBLE-01") {
			t.Fatalf("HTML form %d masked following prose: %q", index+1, visible)
		}
	}
}

func TestLinesDoesNotTreatCommentMarkersInsideCodeSpansAsComments(t *testing.T) {
	document := "`<!--` visible after\n``inside <!-- code`` still visible\nvisible last <!-- hidden --> tail\n"
	lines := Lines([]byte(document))
	visible := visibleText(lines)
	for _, token := range []string{"visible after", "still visible", "visible last", "tail"} {
		if !strings.Contains(visible, token) {
			t.Fatalf("visible text %q lacks %q; lines=%#v", visible, token, lines)
		}
	}
	if strings.Contains(visible, "hidden") {
		t.Fatalf("HTML comment content remained visible: %q", visible)
	}
}

func visibleText(lines []Line) string {
	visible := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Visible {
			visible = append(visible, line.Text)
		}
	}
	return strings.Join(visible, "\n")
}
