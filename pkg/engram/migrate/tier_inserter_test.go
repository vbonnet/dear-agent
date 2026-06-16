package migrate_test

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/engram/migrate"
)

func TestInsertTierMarkers_WithFrontmatter(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		"---",
		"name: test-engram",
		"type: instruction",
		"---",
		"",
		"This is the first paragraph describing the engram.",
		"",
		"## Section One",
		"",
		"Details of section one.",
		"",
		"## Section Two",
		"",
		"Details of section two.",
	}, "\n")

	out, err := migrate.InsertTierMarkers(input)
	if err != nil {
		t.Fatalf("InsertTierMarkers error: %v", err)
	}
	if !strings.Contains(out, "[!T0]") {
		t.Error("output missing [!T0] marker")
	}
	if !strings.Contains(out, "[!T1]") {
		t.Error("output missing [!T1] marker")
	}
	if !strings.Contains(out, "[!T2]") {
		t.Error("output missing [!T2] marker")
	}
	// Frontmatter must be preserved verbatim.
	if !strings.Contains(out, "name: test-engram") {
		t.Error("output missing frontmatter field 'name: test-engram'")
	}
}

func TestInsertTierMarkers_NoFrontmatter(t *testing.T) {
	t.Parallel()
	input := "A simple document without frontmatter.\n\n## Header\n\nSome content."

	out, err := migrate.InsertTierMarkers(input)
	if err != nil {
		t.Fatalf("InsertTierMarkers error: %v", err)
	}
	if !strings.Contains(out, "[!T0]") {
		t.Errorf("output missing [!T0] marker; got: %q", out)
	}
	if !strings.Contains(out, "[!T2]") {
		t.Errorf("output missing [!T2] marker; got: %q", out)
	}
}

func TestInsertTierMarkers_EmptyContent(t *testing.T) {
	t.Parallel()
	// Empty input should not panic and should produce T-markers.
	out, err := migrate.InsertTierMarkers("")
	if err != nil {
		t.Fatalf("InsertTierMarkers error on empty input: %v", err)
	}
	// Even empty content gets the tier scaffold.
	if !strings.Contains(out, "[!T0]") {
		t.Errorf("empty input: output missing [!T0]; got: %q", out)
	}
}

func TestInsertTierMarkers_T0ContentFromFirstParagraph(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		"The first paragraph is the executive summary.",
		"",
		"## Details",
		"",
		"More detailed content here.",
	}, "\n")

	out, err := migrate.InsertTierMarkers(input)
	if err != nil {
		t.Fatalf("InsertTierMarkers error: %v", err)
	}
	// T0 block should contain text from the first paragraph.
	if !strings.Contains(out, "first paragraph") {
		t.Errorf("T0 block missing first-paragraph content; output:\n%s", out)
	}
}

func TestInsertTierMarkers_T2ContainsAllContent(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		"## A",
		"Content A.",
		"",
		"## B",
		"Content B.",
		"",
		"## C",
		"Content C.",
	}, "\n")

	out, err := migrate.InsertTierMarkers(input)
	if err != nil {
		t.Fatalf("InsertTierMarkers error: %v", err)
	}
	// T2 should contain the last section (everything).
	t2Idx := strings.Index(out, "[!T2]")
	if t2Idx < 0 {
		t.Fatal("no [!T2] block")
	}
	t2Block := out[t2Idx:]
	if !strings.Contains(t2Block, "Content C") {
		t.Errorf("T2 block missing 'Content C'; T2 block:\n%s", t2Block)
	}
}

func TestInsertTierMarkers_MarkerOrder(t *testing.T) {
	t.Parallel()
	out, err := migrate.InsertTierMarkers("Some content.\n\n## Section\n\nDetails.")
	if err != nil {
		t.Fatalf("InsertTierMarkers: %v", err)
	}
	t0 := strings.Index(out, "[!T0]")
	t1 := strings.Index(out, "[!T1]")
	t2 := strings.Index(out, "[!T2]")
	if t0 < 0 || t1 < 0 || t2 < 0 {
		t.Fatalf("missing tier markers; t0=%d t1=%d t2=%d", t0, t1, t2)
	}
	if t0 >= t1 || t1 >= t2 {
		t.Errorf("tier markers out of order: T0=%d T1=%d T2=%d", t0, t1, t2)
	}
}
