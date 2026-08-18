package instructions_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAlignmentDocumentsUseCanonicalMissionLanguage(t *testing.T) {
	root := repoRoot(t)
	mission := readFile(t, filepath.Join(root, "docs", "alignment", "MISSION.md"))
	for _, required := range []string{"## Ownership", "VROOM owns", "AGM owns", "final decision"} {
		if !strings.Contains(mission, required) {
			t.Fatalf("MISSION.md must define the canonical ownership boundary: missing %q", required)
		}
	}

	for _, file := range []string{"VISION.md", "VALUES.md", "GOALS.md"} {
		content := readFile(t, filepath.Join(root, "docs", "alignment", file))
		if !strings.Contains(content, "MISSION.md is canonical") {
			t.Fatalf("%s must identify MISSION.md as canonical", file)
		}
	}

	adr := readFile(t, filepath.Join(root, "docs", "adr", "ADR-002-vroom-execution-architecture.md"))
	if !strings.Contains(normalizeWhitespace(adr), "MISSION.md is canonical for project purpose and the VROOM/AGM ownership boundary") {
		t.Fatal("ADR-002 must defer project purpose and ownership to MISSION.md")
	}
	context := readFile(t, filepath.Join(root, "CONTEXT.md"))
	if !strings.Contains(context, "docs/alignment/MISSION.md") || !strings.Contains(context, "canonical") {
		t.Fatal("CONTEXT.md must defer the ownership contract to the canonical mission")
	}
	rootGoal := readFile(t, filepath.Join(root, "GOAL.md"))
	if !strings.Contains(rootGoal, "docs/alignment/MISSION.md") || !strings.Contains(rootGoal, "canonical") {
		t.Fatal("root GOAL.md must defer project purpose to the canonical mission")
	}
}

func TestAlignmentDocumentsPreservePrivacyAndVerificationBoundaries(t *testing.T) {
	root := repoRoot(t)
	values := strings.ToLower(normalizeWhitespace(readFile(t, filepath.Join(root, "docs", "alignment", "VALUES.md"))))
	for _, required := range []string{
		"persistent records",
		"callers must redact or omit secrets",
		"personally identifiable information (pii)",
		"does not provide automatic redaction",
	} {
		if !strings.Contains(values, required) {
			t.Fatalf("VALUES.md must preserve privacy boundary %q", required)
		}
	}

	mission := normalizeWhitespace(readFile(t, filepath.Join(root, "docs", "alignment", "MISSION.md")))
	for _, required := range []string{"session or batch checks as `VERIFIED`", "evidence for VROOM", "not an independent acceptance decision"} {
		if !strings.Contains(mission, required) {
			t.Fatalf("MISSION.md must distinguish AGM verification mechanics from VROOM acceptance: missing %q", required)
		}
	}
}

func normalizeWhitespace(content string) string {
	return strings.Join(strings.Fields(content), " ")
}

func TestAlignmentDocumentsUseGovernedFrontMatter(t *testing.T) {
	root := repoRoot(t)
	allowedKeys := map[string][]string{
		"MISSION.md": {"title", "version", "status", "date", "adr_ref", "context_ref", "scope"},
		"VISION.md":  {"title", "version", "status", "date", "mission_ref", "adr_ref", "context_ref", "horizon"},
		"VALUES.md":  {"title", "version", "status", "date", "mission_ref"},
		"GOALS.md":   {"title", "version", "status", "date", "mission_ref"},
	}

	for file, allowed := range allowedKeys {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, "docs", "alignment", file))
			metadata := parseAlignmentFrontMatter(t, content)
			got := make([]string, 0, len(metadata))
			for key := range metadata {
				got = append(got, key)
			}
			sort.Strings(got)
			sort.Strings(allowed)
			if fmt.Sprint(got) != fmt.Sprint(allowed) {
				t.Fatalf("%s front matter keys = %v, want %v", file, got, allowed)
			}
			if file != "MISSION.md" && metadata["mission_ref"] != "docs/alignment/MISSION.md" {
				t.Fatalf("%s mission_ref = %q, want canonical mission", file, metadata["mission_ref"])
			}
		})
	}
}

func parseAlignmentFrontMatter(t *testing.T, content string) map[string]string {
	t.Helper()
	if !strings.HasPrefix(content, "---\n") {
		t.Fatal("alignment document must start with YAML front matter")
	}
	rest := strings.TrimPrefix(content, "---\n")
	frontMatter, _, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		t.Fatal("alignment document must close YAML front matter")
	}
	var metadata map[string]string
	if err := yaml.Unmarshal([]byte(frontMatter), &metadata); err != nil {
		t.Fatalf("parse alignment front matter: %v", err)
	}
	return metadata
}

func TestAlignmentDocumentsCarryAuditMarker(t *testing.T) {
	root := repoRoot(t)
	auditMarker := regexp.MustCompile(`<!-- Last audited at: \d{4}-\d{2}-\d{2} -->`)
	for _, file := range []string{"MISSION.md", "VISION.md", "VALUES.md", "GOALS.md"} {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, "docs", "alignment", file))
			if !auditMarker.MatchString(content) {
				t.Fatalf("%s must carry a valid audit marker", file)
			}
		})
	}
}
