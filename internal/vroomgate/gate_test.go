package vroomgate

import (
	"sort"
	"testing"
)

// TestIsHumanGatedKnownEntries pins a representative entry from each rationale
// class rather than mirroring the whole map: an exhaustive copy would have to be
// edited every time a gate is lifted, which is exactly the duplication this
// package exists to remove.
func TestIsHumanGatedKnownEntries(t *testing.T) {
	for _, id := range []string{
		"ce-6as.10", // interactive/HUMAN-ACTION
		"ce-93lw.3", // security surface
		"ce-fmxv",   // core spawn infra
	} {
		if !IsHumanGated(id) {
			t.Errorf("IsHumanGated(%q) = false, want true", id)
		}
	}
}

func TestIsHumanGatedUnknownID(t *testing.T) {
	if IsHumanGated("ce-not-a-real-bead") {
		t.Error("an id absent from the gate list must not be reported as gated")
	}
	if IsHumanGated("") {
		t.Error("the empty id must not be reported as gated")
	}
}

func TestIDsAreSortedAndComplete(t *testing.T) {
	ids := IDs()
	if len(ids) != len(gated) {
		t.Fatalf("IDs() returned %d ids, want %d", len(ids), len(gated))
	}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("IDs() must be sorted, got %v", ids)
	}
	for _, id := range ids {
		if !IsHumanGated(id) {
			t.Errorf("IDs() returned %q which IsHumanGated reports as not gated", id)
		}
	}
}
