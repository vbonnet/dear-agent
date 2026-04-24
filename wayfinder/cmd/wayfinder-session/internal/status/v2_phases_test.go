package status

import (
	"testing"
)

// TestAllPhasesV2_Count verifies correct number of v2 phases
func TestAllPhasesV2_Count(t *testing.T) {
	phases := AllPhasesV2()

	// V2 has 9 consolidated phases (down from v1's 13)
	expectedCount := 9
	if len(phases) != expectedCount {
		t.Errorf("AllPhasesV2() length = %d, want %d", len(phases), expectedCount)
	}
}

// TestAllPhasesV2_Sequence verifies exact v2 phase sequence
func TestAllPhasesV2_Sequence(t *testing.T) {
	phases := AllPhasesV2()

	// V2 uses descriptive names with 9-phase consolidation
	expected := []string{
		"CHARTER",  // Project Framing
		"PROBLEM",  // Discovery: Problem
		"RESEARCH", // Discovery: Solutions
		"DESIGN",   // Discovery: Approach
		"SPEC",     // Discovery: Requirements
		"PLAN",     // Design (consolidated: tech-lead, security, QA)
		"SETUP",    // Roadmap (optional, consolidated: planning, breakdown, dependencies)
		"BUILD",    // Build (consolidated: implement, test, integrate)
		"RETRO",    // Retrospective
	}

	for i, phase := range phases {
		if i >= len(expected) {
			t.Errorf("AllPhasesV2() has extra phase at index %d: %q", i, phase)
			continue
		}
		if phase != expected[i] {
			t.Errorf("AllPhasesV2()[%d] = %q, want %q", i, phase, expected[i])
		}
	}

	if len(phases) != len(expected) {
		t.Errorf("AllPhasesV2() length = %d, want %d", len(phases), len(expected))
	}
}

// TestAllPhases_VersionSwitch verifies version-aware phase selection
func TestAllPhases_VersionSwitch(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		expectedFirst string
		expectedLast  string
		expectedCount int
	}{
		{
			name:          "v1 explicit",
			version:       WayfinderV1,
			expectedFirst: "W0",
			expectedLast:  "S11",
			expectedCount: 13,
		},
		{
			name:          "v2 explicit",
			version:       WayfinderV2,
			expectedFirst: "CHARTER",
			expectedLast:  "RETRO",
			expectedCount: 9,
		},
		{
			name:          "empty defaults to v1",
			version:       "",
			expectedFirst: "W0",
			expectedLast:  "S11",
			expectedCount: 13,
		},
		{
			name:          "no args defaults to v1",
			version:       "",
			expectedFirst: "W0",
			expectedLast:  "S11",
			expectedCount: 13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var phases []string
			if tt.version == "" && tt.name == "no args defaults to v1" {
				phases = AllPhases() // no args
			} else {
				phases = AllPhases(tt.version)
			}

			if len(phases) != tt.expectedCount {
				t.Errorf("AllPhases(%q) length = %d, want %d", tt.version, len(phases), tt.expectedCount)
			}

			if len(phases) > 0 && phases[0] != tt.expectedFirst {
				t.Errorf("AllPhases(%q)[0] = %q, want %q", tt.version, phases[0], tt.expectedFirst)
			}

			if len(phases) > 0 && phases[len(phases)-1] != tt.expectedLast {
				t.Errorf("AllPhases(%q)[last] = %q, want %q", tt.version, phases[len(phases)-1], tt.expectedLast)
			}
		})
	}
}

// TestIsValidV2Phase tests phase name validation
func TestIsValidV2Phase(t *testing.T) {
	tests := []struct {
		phase string
		valid bool
	}{
		// Valid V2 descriptive names
		{"CHARTER", true},
		{"PROBLEM", true},
		{"RESEARCH", true},
		{"DESIGN", true},
		{"SPEC", true},
		{"PLAN", true},
		{"SETUP", true},
		{"BUILD", true},
		{"RETRO", true},

		// Valid V1 phases (also match pattern)
		{"W0", true},
		{"D1", true},
		{"S4", true},
		{"S5", true},
		{"S9", true},
		{"S10", true},
		{"S11", true},

		// Invalid: lowercase
		{"charter", false},
		{"problem", false},
		{"retro", false},

		// Invalid: dotted names (not used in actual V2 implementation)
		{"discovery.problem", false},
		{"discovery.solutions", false},
		{"build.implement", false},
		{"design.tech-lead", false},

		// Invalid: single lowercase words
		{"definition", false},
		{"specification", false},
		{"deploy", false},
		{"retrospective", false},

		// Invalid: multiple dots
		{"discovery.problem.detail", false},

		// Invalid: leading/trailing dots
		{".discovery", false},
		{"discovery.", false},

		// Invalid: spaces
		{"discovery problem", false},
		{"W 0", false},

		// Invalid: underscores
		{"discovery_problem", false},
		{"W_0", false},

		// Invalid: empty
		{"", false},

		// Invalid: wrong letter prefix
		{"X0", false},
		{"A1", false},
		{"Z99", false},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := IsValidV2Phase(tt.phase)
			if result != tt.valid {
				t.Errorf("IsValidV2Phase(%q) = %v, want %v", tt.phase, result, tt.valid)
			}
		})
	}
}

// TestPhaseToFileName tests phase name to filename conversion
func TestPhaseToFileName(t *testing.T) {
	tests := []struct {
		phase    string
		expected string
	}{
		// V2 descriptive names convert directly
		{"CHARTER", "CHARTER.md"},
		{"PROBLEM", "PROBLEM.md"},
		{"RESEARCH", "RESEARCH.md"},
		{"DESIGN", "DESIGN.md"},
		{"SPEC", "SPEC.md"},
		{"PLAN", "PLAN.md"},
		{"SETUP", "SETUP.md"},
		{"BUILD", "BUILD.md"},
		{"RETRO", "RETRO.md"},

		// Dotted names (for mapping purposes only) convert with hyphens
		{"discovery.problem", "discovery-problem.md"},
		{"discovery.solutions", "discovery-solutions.md"},
		{"build.implement", "build-implement.md"},
		{"design.tech-lead", "design-tech-lead.md"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := PhaseToFileName(tt.phase)
			if result != tt.expected {
				t.Errorf("PhaseToFileName(%q) = %q, want %q", tt.phase, result, tt.expected)
			}
		})
	}
}

// TestFileNameToPhase tests filename to phase name conversion
func TestFileNameToPhase(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		// V2 descriptive names convert directly
		{"CHARTER.md", "CHARTER"},
		{"PROBLEM.md", "PROBLEM"},
		{"RESEARCH.md", "RESEARCH"},
		{"DESIGN.md", "DESIGN"},
		{"SPEC.md", "SPEC"},
		{"PLAN.md", "PLAN"},
		{"SETUP.md", "SETUP"},
		{"BUILD.md", "BUILD"},
		{"RETRO.md", "RETRO"},

		// Dotted names (for mapping purposes) convert from hyphens
		{"discovery-problem.md", "discovery.problem"},
		{"discovery-solutions.md", "discovery.solutions"},
		{"build-implement.md", "build.implement"},
		{"design-tech-lead.md", "design.tech-lead"},

		// Invalid formats
		{"invalid", ""},                // no .md extension
		{"too-many-parts-here.md", ""}, // 4 parts
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := FileNameToPhase(tt.filename)
			if result != tt.expected {
				t.Errorf("FileNameToPhase(%q) = %q, want %q", tt.filename, result, tt.expected)
			}
		})
	}
}

// TestPhaseMapping_V1ToV2 tests v1 to v2 phase mapping
func TestPhaseMapping_V1ToV2(t *testing.T) {
	tests := []struct {
		v1Phase string
		v2Phase string
	}{
		{"D1", "discovery.problem"},
		{"D2", "discovery.solutions"},
		{"D3", "discovery.approach"},
		{"D4", "discovery.requirements"},
		{"S4", "definition"},
		{"S6", "design.tech-lead"}, // S6 maps to first design sub-phase
		{"S7", "roadmap.planning"},
		{"S8", "build.implement"},
		{"S9", "build.test"},
		{"S10", "deploy"},
		{"S11", "retrospective"},
		{"W0", ""}, // W0 removed in v2
		{"S5", ""}, // S5 (Research) merged into discovery/design
	}

	for _, tt := range tests {
		t.Run(tt.v1Phase, func(t *testing.T) {
			result := V1ToV2PhaseMap[tt.v1Phase]
			if result != tt.v2Phase {
				t.Errorf("V1ToV2PhaseMap[%q] = %q, want %q", tt.v1Phase, result, tt.v2Phase)
			}
		})
	}
}

// TestNextPhase_V2Progression tests v2 phase progression
func TestNextPhase_V2Progression(t *testing.T) {
	// V2 uses descriptive names with 9-phase consolidation
	expectedSequence := []string{
		"CHARTER",  // Project Framing
		"PROBLEM",  // Discovery: Problem
		"RESEARCH", // Discovery: Solutions
		"DESIGN",   // Discovery: Approach
		"SPEC",     // Discovery: Requirements
		"PLAN",     // Design (consolidated)
		"SETUP",    // Roadmap (optional, consolidated)
		"BUILD",    // Build (consolidated)
		"RETRO",    // Retrospective
	}

	s := &Status{
		Version:      WayfinderV2,
		CurrentPhase: "",
		Phases:       []Phase{},
	}

	for i, expected := range expectedSequence {
		next, err := s.NextPhase()
		if err != nil {
			t.Fatalf("unexpected error at step %d: %v", i, err)
		}
		if next != expected {
			t.Errorf("step %d: expected %q, got %q", i, expected, next)
		}

		// Advance to next phase and mark current phase as completed
		if i < len(expectedSequence)-1 {
			s.CurrentPhase = next
			// Mark the phase as completed so NextPhase() will advance
			s.UpdatePhase(next, PhaseStatusCompleted, OutcomeSuccess)
		}
	}

	// Verify RETRO (retrospective) is truly final
	s.CurrentPhase = "RETRO"
	s.UpdatePhase("RETRO", PhaseStatusCompleted, OutcomeSuccess)
	_, err := s.NextPhase()
	if err == nil {
		t.Error("expected error when advancing from RETRO, got none")
		return
	}
	if err.Error() != "already at final phase RETRO" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestGetVersion tests version detection with default
func TestGetVersion(t *testing.T) {
	tests := []struct {
		name             string
		wayfinderVersion string
		expected         string
	}{
		{"explicit v1", WayfinderV1, WayfinderV1},
		{"explicit v2", WayfinderV2, WayfinderV2},
		{"empty defaults to v1", "", WayfinderV1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Status{
				Version: tt.wayfinderVersion,
			}
			result := s.GetVersion()
			if result != tt.expected {
				t.Errorf("GetVersion() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestPhaseIndex_V2 tests phase indexing for v2
func TestPhaseIndex_V2(t *testing.T) {
	tests := []struct {
		phase    string
		expected int
	}{
		// V2 descriptive names (9-phase consolidation)
		{"CHARTER", 0},
		{"PROBLEM", 1},
		{"RESEARCH", 2},
		{"DESIGN", 3},
		{"SPEC", 4},
		{"PLAN", 5},
		{"SETUP", 6},
		{"BUILD", 7},
		{"RETRO", 8},
		{"unknown", 999}, // Unknown phase
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := phaseIndex(tt.phase, WayfinderV2)
			if result != tt.expected {
				t.Errorf("phaseIndex(%q, v2) = %d, want %d", tt.phase, result, tt.expected)
			}
		})
	}
}

// TestNextPhase_V2WithSkipRoadmap tests v2 phase progression with roadmap skipping
// NOTE: V2 uses short names (W0, D1, S6, S7, S8, S11), not dotted names
// S7 is the roadmap phase (consolidated planning+breakdown+dependencies)
func TestNextPhase_V2WithSkipRoadmap(t *testing.T) {
	tests := []struct {
		name          string
		currentPhase  string
		skipRoadmap   bool
		expectedNext  string
		markCompleted bool
	}{
		{
			name:          "PLAN to SETUP (default behavior, roadmap included)",
			currentPhase:  "PLAN",
			skipRoadmap:   false,
			expectedNext:  "SETUP",
			markCompleted: true,
		},
		{
			name:          "PLAN to BUILD (skip roadmap SETUP)",
			currentPhase:  "PLAN",
			skipRoadmap:   true,
			expectedNext:  "BUILD",
			markCompleted: true,
		},
		{
			name:          "SETUP to BUILD (default behavior)",
			currentPhase:  "SETUP",
			skipRoadmap:   false,
			expectedNext:  "BUILD",
			markCompleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Status{
				Version:      WayfinderV2,
				CurrentPhase: tt.currentPhase,
				SkipRoadmap:  tt.skipRoadmap,
				Phases:       []Phase{},
			}

			if tt.markCompleted {
				s.UpdatePhase(tt.currentPhase, PhaseStatusCompleted, OutcomeSuccess)
			}

			next, err := s.NextPhase()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if next != tt.expectedNext {
				t.Errorf("expected %q, got %q", tt.expectedNext, next)
			}
		})
	}
}

// TestNextPhase_V2FullProgressionWithSkipRoadmap tests complete v2 workflow with roadmap skipping
func TestNextPhase_V2FullProgressionWithSkipRoadmap(t *testing.T) {
	// V2 sequence with SETUP (roadmap) skipped
	expectedSequence := []string{
		"CHARTER",  // Project Framing
		"PROBLEM",  // Discovery: Problem
		"RESEARCH", // Discovery: Solutions
		"DESIGN",   // Discovery: Approach
		"SPEC",     // Discovery: Requirements
		"PLAN",     // Design (consolidated)
		// SETUP (roadmap) skipped
		"BUILD", // Build (consolidated)
		"RETRO", // Retrospective
	}

	s := &Status{
		Version:      WayfinderV2,
		SkipRoadmap:  true, // Enable roadmap skipping
		CurrentPhase: "",
		Phases:       []Phase{},
	}

	for i, expected := range expectedSequence {
		next, err := s.NextPhase()
		if err != nil {
			t.Fatalf("unexpected error at step %d: %v", i, err)
		}
		if next != expected {
			t.Errorf("step %d: expected %q, got %q", i, expected, next)
		}

		// Verify SETUP (roadmap) phase is never returned
		if next == "SETUP" {
			t.Errorf("step %d: roadmap phase SETUP should have been skipped", i)
		}

		// Advance to next phase and mark current phase as completed
		if i < len(expectedSequence)-1 {
			s.CurrentPhase = next
			s.UpdatePhase(next, PhaseStatusCompleted, OutcomeSuccess)
		}
	}

	// Verify RETRO (retrospective) is truly final
	s.CurrentPhase = "RETRO"
	s.UpdatePhase("RETRO", PhaseStatusCompleted, OutcomeSuccess)
	_, err := s.NextPhase()
	if err == nil {
		t.Error("expected error when advancing from RETRO, got none")
		return
	}
	if err.Error() != "already at final phase RETRO" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestNextPhase_V2DefaultIncludesRoadmap verifies default behavior includes roadmap phases
func TestNextPhase_V2DefaultIncludesRoadmap(t *testing.T) {
	s := &Status{
		Version:      WayfinderV2,
		SkipRoadmap:  false,  // Default: roadmap included
		CurrentPhase: "PLAN", // Design phase
		Phases:       []Phase{},
	}

	s.UpdatePhase("PLAN", PhaseStatusCompleted, OutcomeSuccess)

	next, err := s.NextPhase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if next != "SETUP" {
		t.Errorf("default behavior should include roadmap phase SETUP, got %q instead", next)
	}
}

// TestSkipRoadmap_V1Ignored verifies skip-roadmap flag is ignored for v1
func TestSkipRoadmap_V1Ignored(t *testing.T) {
	s := &Status{
		Version:      WayfinderV1,
		SkipRoadmap:  true, // Should be ignored for v1
		CurrentPhase: "S6",
		Phases:       []Phase{},
	}

	s.UpdatePhase("S6", PhaseStatusCompleted, OutcomeSuccess)

	next, err := s.NextPhase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// v1 should follow normal sequence regardless of SkipRoadmap flag
	if next != "S7" {
		t.Errorf("v1 should ignore SkipRoadmap flag, expected S7, got %q", next)
	}
}
