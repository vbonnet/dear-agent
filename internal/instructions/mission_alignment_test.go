package instructions_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const ownershipContract = "VROOM owns prioritization, dispatch decisions, supervision, and output verification. AGM owns session lifecycle mechanics: session creation, process execution, messaging, monitoring telemetry, and archival."

func TestAlignmentDocumentsUseCanonicalMissionContract(t *testing.T) {
	root := repoRoot(t)
	mission := readFile(t, filepath.Join(root, "docs", "alignment", "MISSION.md"))
	if !strings.Contains(normalizeWhitespace(mission), ownershipContract) {
		t.Fatalf("MISSION.md must state the canonical VROOM/AGM ownership contract")
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
	if !strings.Contains(normalizeWhitespace(context), ownershipContract) {
		t.Fatal("CONTEXT.md must use the canonical VROOM/AGM ownership contract")
	}
}

func normalizeWhitespace(content string) string {
	return strings.Join(strings.Fields(content), " ")
}

func TestAlignmentDocumentsRejectSupersededOptimizationModel(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{
		"lexicographic_hierarchy:",
		"optimization_weights:",
		"weight_sum:",
		"review_cadence:",
		"strict lexicographic",
		"weights sum to",
		"reviewed quarterly",
		"AGM governs the lifecycle",
		"target state for AGM",
		"more than 2x",
	}

	for _, file := range []string{"MISSION.md", "VISION.md", "VALUES.md", "GOALS.md"} {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, "docs", "alignment", file))
			lower := strings.ToLower(content)
			for _, token := range forbidden {
				if strings.Contains(lower, strings.ToLower(token)) {
					t.Fatalf("%s contains superseded model token %q", file, token)
				}
			}
		})
	}
}

func TestAlignmentDocumentsStayFreshAndFocused(t *testing.T) {
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
