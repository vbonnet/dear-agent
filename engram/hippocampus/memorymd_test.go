package hippocampus

import (
	"strings"
	"testing"
)

const sampleMemoryMD = `# Workspace Memory

## Repo Roles
- **engram-research** = logs ONLY
- **engram** = real code
- **ai-tools** = real code

## Key Files
- Settings: ` + "`~/.claude/settings.json`" + `
- AGM source: ` + "`~/src/ws/oss/repos/ai-tools/`" + `

## Topic Files
- ` + "`workspace-structure.md`" + ` -- directory layout
- ` + "`cost-optimization.md`" + ` -- cost data`

func TestParseMemoryMD_RoundTrip(t *testing.T) {
	doc, err := ParseMemoryMD(sampleMemoryMD)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	rendered := doc.Render()
	if rendered != sampleMemoryMD {
		t.Errorf("round-trip mismatch\n--- want ---\n%s\n--- got ---\n%s", sampleMemoryMD, rendered)
	}

	// Parse again and verify identical structure.
	doc2, err := ParseMemoryMD(rendered)
	if err != nil {
		t.Fatalf("second ParseMemoryMD returned error: %v", err)
	}
	rendered2 := doc2.Render()
	if rendered2 != rendered {
		t.Error("second round-trip produced different output")
	}
}

func TestParseMemoryMD_Sections(t *testing.T) {
	doc, err := ParseMemoryMD(sampleMemoryMD)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	// Top-level: one # section containing three ## children.
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 top-level section, got %d", len(doc.Sections))
	}

	top := doc.Sections[0]
	if top.Level != 1 {
		t.Errorf("expected top-level section level 1, got %d", top.Level)
	}
	if top.Heading != "Workspace Memory" {
		t.Errorf("expected heading 'Workspace Memory', got %q", top.Heading)
	}

	if len(top.Children) != 3 {
		t.Fatalf("expected 3 children under Workspace Memory, got %d", len(top.Children))
	}

	expectedHeadings := []string{"Repo Roles", "Key Files", "Topic Files"}
	for i, want := range expectedHeadings {
		if top.Children[i].Heading != want {
			t.Errorf("child %d: expected heading %q, got %q", i, want, top.Children[i].Heading)
		}
		if top.Children[i].Level != 2 {
			t.Errorf("child %d: expected level 2, got %d", i, top.Children[i].Level)
		}
	}

	// Verify content of Repo Roles section.
	repoRoles := top.Children[0]
	if len(repoRoles.Content) != 4 {
		t.Errorf("Repo Roles: expected 4 content lines, got %d: %v",
			len(repoRoles.Content), repoRoles.Content)
	}
}

func TestParseMemoryMD_NestedSections(t *testing.T) {
	input := `# Top
## Section A
some content
### Subsection A1
detail here
### Subsection A2
more detail
## Section B
other content`

	doc, err := ParseMemoryMD(input)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 top-level section, got %d", len(doc.Sections))
	}

	top := doc.Sections[0]
	if len(top.Children) != 2 {
		t.Fatalf("expected 2 ## children, got %d", len(top.Children))
	}

	secA := top.Children[0]
	if secA.Heading != "Section A" {
		t.Errorf("expected 'Section A', got %q", secA.Heading)
	}
	if len(secA.Content) != 1 {
		t.Errorf("Section A: expected 1 content line, got %d", len(secA.Content))
	}
	if len(secA.Children) != 2 {
		t.Fatalf("Section A: expected 2 ### children, got %d", len(secA.Children))
	}
	if secA.Children[0].Heading != "Subsection A1" {
		t.Errorf("expected 'Subsection A1', got %q", secA.Children[0].Heading)
	}
	if secA.Children[1].Heading != "Subsection A2" {
		t.Errorf("expected 'Subsection A2', got %q", secA.Children[1].Heading)
	}

	secB := top.Children[1]
	if secB.Heading != "Section B" {
		t.Errorf("expected 'Section B', got %q", secB.Heading)
	}
	if len(secB.Children) != 0 {
		t.Errorf("Section B: expected 0 children, got %d", len(secB.Children))
	}

	// Round-trip.
	if doc.Render() != input {
		t.Errorf("nested sections round-trip failed")
	}
}

func TestParseMemoryMD_Preamble(t *testing.T) {
	input := `Some preamble text
Another preamble line

# First Heading
content here`

	doc, err := ParseMemoryMD(input)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	expectedPreamble := "Some preamble text\nAnother preamble line\n"
	if doc.Preamble != expectedPreamble {
		t.Errorf("expected preamble %q, got %q", expectedPreamble, doc.Preamble)
	}

	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Heading != "First Heading" {
		t.Errorf("expected heading 'First Heading', got %q", doc.Sections[0].Heading)
	}

	// Round-trip.
	if doc.Render() != input {
		t.Errorf("preamble round-trip failed\n--- want ---\n%s\n--- got ---\n%s",
			input, doc.Render())
	}
}

func TestParseMemoryMD_EmptyDocument(t *testing.T) {
	doc, err := ParseMemoryMD("")
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	if doc.Preamble != "" {
		t.Errorf("expected empty preamble, got %q", doc.Preamble)
	}
	if len(doc.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(doc.Sections))
	}
	if doc.Render() != "" {
		t.Errorf("expected empty render, got %q", doc.Render())
	}
	if doc.LineCount() != 0 {
		t.Errorf("expected 0 line count, got %d", doc.LineCount())
	}
}

func TestMemoryDocument_AddEntry(t *testing.T) {
	doc, err := ParseMemoryMD(sampleMemoryMD)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	// Add to existing section.
	doc.AddEntry("Repo Roles", "- **new-repo** = experimental")

	sec := doc.FindSection("Repo Roles")
	if sec == nil {
		t.Fatal("FindSection returned nil for 'Repo Roles'")
	}

	lastLine := sec.Content[len(sec.Content)-1]
	if lastLine != "- **new-repo** = experimental" {
		t.Errorf("expected added entry as last content line, got %q", lastLine)
	}

	// Add to non-existent section (creates new ## section).
	doc.AddEntry("New Section", "- first entry")

	newSec := doc.FindSection("New Section")
	if newSec == nil {
		t.Fatal("FindSection returned nil for newly created 'New Section'")
	}
	if newSec.Level != 2 {
		t.Errorf("expected new section level 2, got %d", newSec.Level)
	}
	if len(newSec.Content) != 1 {
		t.Fatalf("expected 1 content line, got %d", len(newSec.Content))
	}
	if newSec.Content[0] != "- first entry" {
		t.Errorf("expected '- first entry', got %q", newSec.Content[0])
	}
}

func TestMemoryDocument_RemoveEntry(t *testing.T) {
	doc, err := ParseMemoryMD(sampleMemoryMD)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	// Remove existing entry.
	removed := doc.RemoveEntry("Repo Roles", "- **engram** = real code")
	if !removed {
		t.Error("RemoveEntry returned false for existing entry")
	}

	sec := doc.FindSection("Repo Roles")
	if sec == nil {
		t.Fatal("FindSection returned nil")
	}
	for _, line := range sec.Content {
		if line == "- **engram** = real code" {
			t.Error("entry still present after removal")
		}
	}

	// Remove non-existent entry.
	removed = doc.RemoveEntry("Repo Roles", "- nonexistent entry")
	if removed {
		t.Error("RemoveEntry returned true for non-existent entry")
	}

	// Remove from non-existent section.
	removed = doc.RemoveEntry("No Such Section", "- anything")
	if removed {
		t.Error("RemoveEntry returned true for non-existent section")
	}
}

func TestMemoryDocument_LineCount(t *testing.T) {
	doc, err := ParseMemoryMD(sampleMemoryMD)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	// Count lines in the sample directly.
	expectedLines := strings.Count(sampleMemoryMD, "\n") + 1
	got := doc.LineCount()
	if got != expectedLines {
		t.Errorf("expected %d lines, got %d", expectedLines, got)
	}
}

func TestMemoryDocument_FindSection(t *testing.T) {
	doc, err := ParseMemoryMD(sampleMemoryMD)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	// Find top-level section.
	top := doc.FindSection("Workspace Memory")
	if top == nil {
		t.Fatal("FindSection returned nil for 'Workspace Memory'")
	}
	if top.Level != 1 {
		t.Errorf("expected level 1, got %d", top.Level)
	}

	// Find nested section.
	keyFiles := doc.FindSection("Key Files")
	if keyFiles == nil {
		t.Fatal("FindSection returned nil for 'Key Files'")
	}
	if keyFiles.Level != 2 {
		t.Errorf("expected level 2, got %d", keyFiles.Level)
	}

	// Non-existent section.
	missing := doc.FindSection("Does Not Exist")
	if missing != nil {
		t.Error("FindSection returned non-nil for non-existent section")
	}
}

func TestParseMemoryMD_ConsecutiveHeadings(t *testing.T) {
	input := `## Section One
## Section Two
## Section Three`

	doc, err := ParseMemoryMD(input)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	if len(doc.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(doc.Sections))
	}

	for i, want := range []string{"Section One", "Section Two", "Section Three"} {
		if doc.Sections[i].Heading != want {
			t.Errorf("section %d: expected %q, got %q", i, want, doc.Sections[i].Heading)
		}
		if len(doc.Sections[i].Content) != 0 {
			t.Errorf("section %d: expected 0 content lines, got %d",
				i, len(doc.Sections[i].Content))
		}
	}

	// Round-trip.
	if doc.Render() != input {
		t.Errorf("consecutive headings round-trip failed")
	}
}

func TestParseMemoryMD_HeadingWithoutSpace(t *testing.T) {
	// "#word" is not a heading (no space after #).
	input := `#notaheading
## Real Heading
content`

	doc, err := ParseMemoryMD(input)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	if doc.Preamble != "#notaheading" {
		t.Errorf("expected preamble '#notaheading', got %q", doc.Preamble)
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Heading != "Real Heading" {
		t.Errorf("expected 'Real Heading', got %q", doc.Sections[0].Heading)
	}
}

func TestParseMemoryMD_BlankLinesPreserved(t *testing.T) {
	input := `## Section

- item 1

- item 2

## Next`

	doc, err := ParseMemoryMD(input)
	if err != nil {
		t.Fatalf("ParseMemoryMD returned error: %v", err)
	}

	sec := doc.FindSection("Section")
	if sec == nil {
		t.Fatal("FindSection returned nil")
	}

	// Content should include blank lines.
	if len(sec.Content) != 5 {
		t.Errorf("expected 5 content lines (with blanks), got %d: %v",
			len(sec.Content), sec.Content)
	}

	// Round-trip.
	if doc.Render() != input {
		t.Errorf("blank lines round-trip failed\n--- want ---\n%s\n--- got ---\n%s",
			input, doc.Render())
	}
}
