package main

import "testing"

// TestDetailedFlag_ReenablesIDs verifies that --detailed (detailed=true) brings
// IDs back into agent-mode output even when the caller did not request the "id"
// field via --fields.
func TestDetailedFlag_ReenablesIDs(t *testing.T) {
	if !includeID(nil, true, ModeAgent) {
		t.Error("includeID(nil, detailed=true, ModeAgent) = false, want true (--detailed must re-enable IDs)")
	}
	// --detailed also wins regardless of an unrelated field mask.
	if !includeID([]string{"name", "status"}, true, ModeAgent) {
		t.Error("includeID([name,status], detailed=true, ModeAgent) = false, want true")
	}
}

// TestDetailedFlag_ReenablesFullPaths verifies that --detailed restores full
// sandbox/project paths in agent-mode (which otherwise basenames them).
func TestDetailedFlag_ReenablesFullPaths(t *testing.T) {
	if !useFullPath(true, ModeAgent) {
		t.Error("useFullPath(detailed=true, ModeAgent) = false, want true (--detailed must restore full paths)")
	}
	if !includeHints(true, ModeAgent) {
		t.Error("includeHints(detailed=true, ModeAgent) = false, want true (--detailed must restore hints)")
	}
}

// TestDetailedFlag_AbsentByDefault verifies that without --detailed, agent-mode
// stays terse: IDs are stripped, paths are basenamed, hints are dropped — unless
// the caller explicitly requests the "id" field via --fields.
func TestDetailedFlag_AbsentByDefault(t *testing.T) {
	if includeID(nil, false, ModeAgent) {
		t.Error("includeID(nil, detailed=false, ModeAgent) = true, want false (IDs stripped by default)")
	}
	if useFullPath(false, ModeAgent) {
		t.Error("useFullPath(detailed=false, ModeAgent) = true, want false (paths basenamed by default)")
	}
	if includeHints(false, ModeAgent) {
		t.Error("includeHints(detailed=false, ModeAgent) = true, want false (hints dropped by default)")
	}
	// An explicit --fields id still re-enables IDs without --detailed.
	if !includeID([]string{"id"}, false, ModeAgent) {
		t.Error("includeID([id], detailed=false, ModeAgent) = false, want true (--fields id opts in)")
	}
}

// TestDetailedFlag_HumanMode_Unchanged verifies that human-mode output is fully
// verbose regardless of --detailed: IDs, full paths, and hints are always
// present.
func TestDetailedFlag_HumanMode_Unchanged(t *testing.T) {
	for _, detailed := range []bool{false, true} {
		if !includeID(nil, detailed, ModeHuman) {
			t.Errorf("includeID(nil, detailed=%v, ModeHuman) = false, want true", detailed)
		}
		if !useFullPath(detailed, ModeHuman) {
			t.Errorf("useFullPath(detailed=%v, ModeHuman) = false, want true", detailed)
		}
		if !includeHints(detailed, ModeHuman) {
			t.Errorf("includeHints(detailed=%v, ModeHuman) = false, want true", detailed)
		}
	}
}
