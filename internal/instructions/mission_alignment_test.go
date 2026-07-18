package instructions_test

import (
	"path/filepath"
	"strings"
	"testing"
)

const ownershipContract = "VROOM owns prioritization, dispatch decisions, supervision, and output verification. AGM owns session lifecycle mechanics: session creation, process execution, messaging, monitoring telemetry, and archival."

func TestAlignmentDocumentsUseCanonicalMissionContract(t *testing.T) {
	root := repoRoot(t)
	mission := readFile(t, filepath.Join(root, "docs", "alignment", "MISSION.md"))
	if !strings.Contains(mission, ownershipContract) {
		t.Fatalf("MISSION.md must state the canonical VROOM/AGM ownership contract")
	}

	for _, file := range []string{"VALUES.md", "GOALS.md"} {
		content := readFile(t, filepath.Join(root, "docs", "alignment", file))
		if !strings.Contains(content, "MISSION.md is canonical") {
			t.Fatalf("%s must identify MISSION.md as canonical", file)
		}
	}

	adr := readFile(t, filepath.Join(root, "docs", "adr", "ADR-002-vroom-execution-architecture.md"))
	if !strings.Contains(adr, "MISSION.md is canonical for project purpose and the VROOM/AGM ownership boundary") {
		t.Fatal("ADR-002 must defer project purpose and ownership to MISSION.md")
	}
	context := readFile(t, filepath.Join(root, "CONTEXT.md"))
	if !strings.Contains(context, ownershipContract) {
		t.Fatal("CONTEXT.md must use the canonical VROOM/AGM ownership contract")
	}
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
	}

	for _, file := range []string{"MISSION.md", "VALUES.md", "GOALS.md"} {
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
	for _, file := range []string{"MISSION.md", "VALUES.md", "GOALS.md"} {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, "docs", "alignment", file))
			if !strings.Contains(content, "<!-- Last audited at: 2026-07-18 -->") {
				t.Fatalf("%s must carry the current audit marker", file)
			}
			if lines := len(strings.Split(content, "\n")); lines > 100 {
				t.Fatalf("%s is %d lines; alignment documents must stay at or below 100", file, lines)
			}
		})
	}
}
