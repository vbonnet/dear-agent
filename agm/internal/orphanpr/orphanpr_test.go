package orphanpr

import (
	"reflect"
	"testing"
)

func TestExtractBeadRefs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "Closes ce-vkvk", []string{"ce-vkvk"}},
		{"subtask suffix", "feat(agm): thing (ce-s2tn.6)", []string{"ce-s2tn.6"}},
		{"dedup preserves order", "ce-dpa6 and again ce-dpa6 then ce-r81r", []string{"ce-dpa6", "ce-r81r"}},
		{"multiple distinct", "tracks ce-mbgq ce-dpa6", []string{"ce-mbgq", "ce-dpa6"}},
		{"none", "no bead here", nil},
		{"embedded prefix does not match", "abce-vkvk is not a bead ref", nil},
		{"real ref among prose", "see ce-vkvk for details", []string{"ce-vkvk"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBeadRefs(tt.in)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractBeadRefs(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// fixedStatus builds a StatusFunc from a static map; unknown ids resolve to
// BeadUnknown.
func fixedStatus(m map[string]BeadStatus) StatusFunc {
	return func(beadID string) BeadStatus {
		if s, ok := m[beadID]; ok {
			return s
		}
		return BeadUnknown
	}
}

func TestScan_FlagsClosedBeadPR(t *testing.T) {
	prs := []PR{
		{Number: 579, Title: "fix thing (ce-mbgq)", Body: "Closes ce-mbgq"},
	}
	got := Scan(prs, fixedStatus(map[string]BeadStatus{"ce-mbgq": BeadClosed}))
	if len(got) != 1 {
		t.Fatalf("expected 1 orphaned PR, got %d: %+v", len(got), got)
	}
	if got[0].Number != 579 {
		t.Errorf("expected PR #579 flagged, got #%d", got[0].Number)
	}
	if !reflect.DeepEqual(got[0].ClosedBeads, []string{"ce-mbgq"}) {
		t.Errorf("ClosedBeads = %v, want [ce-mbgq]", got[0].ClosedBeads)
	}
}

func TestScan_OnlyClosedBeadPRFlagged(t *testing.T) {
	prs := []PR{
		{Number: 579, Title: "closed-bead work", Body: "Closes ce-mbgq"},
		{Number: 600, Title: "open-bead work", Body: "Closes ce-live"},
	}
	status := fixedStatus(map[string]BeadStatus{
		"ce-mbgq": BeadClosed,
		"ce-live": BeadOpen,
	})
	got := Scan(prs, status)
	if len(got) != 1 {
		t.Fatalf("expected only 1 orphaned PR, got %d: %+v", len(got), got)
	}
	if got[0].Number != 579 {
		t.Errorf("expected only PR #579 flagged, got #%d", got[0].Number)
	}
}

func TestScan_MixedRefsSinglePRFlaggedWithClosedOnly(t *testing.T) {
	// A single PR referencing both a closed and an open bead is flagged, but
	// ClosedBeads must contain only the closed one.
	prs := []PR{
		{Number: 581, Title: "spans two beads ce-dpa6", Body: "also relates to ce-live"},
	}
	status := fixedStatus(map[string]BeadStatus{
		"ce-dpa6": BeadClosed,
		"ce-live": BeadOpen,
	})
	got := Scan(prs, status)
	if len(got) != 1 {
		t.Fatalf("expected 1 orphaned PR, got %d", len(got))
	}
	if !reflect.DeepEqual(got[0].ClosedBeads, []string{"ce-dpa6"}) {
		t.Errorf("ClosedBeads = %v, want [ce-dpa6]", got[0].ClosedBeads)
	}
	if len(got[0].BeadRefs) != 2 {
		t.Errorf("expected 2 bead refs recorded, got %d", len(got[0].BeadRefs))
	}
}

func TestScan_NoBeadRefsNotFlagged(t *testing.T) {
	prs := []PR{{Number: 1, Title: "chore: tidy", Body: "no bead ref"}}
	if got := Scan(prs, fixedStatus(nil)); len(got) != 0 {
		t.Fatalf("expected no orphans, got %+v", got)
	}
}

func TestScan_UnknownBeadNotFlagged(t *testing.T) {
	// A bead whose status can't be resolved must NOT be treated as closed.
	prs := []PR{{Number: 2, Title: "refs ce-ghost", Body: ""}}
	if got := Scan(prs, fixedStatus(nil)); len(got) != 0 {
		t.Fatalf("unknown-status bead should not be flagged, got %+v", got)
	}
}

func TestParseBeadStatus(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want BeadStatus
	}{
		{
			"closed header",
			"✓ ce-mbgq · P0: Deploy thing   [● P0 · CLOSED]\nOwner: vbonnet",
			BeadClosed,
		},
		{
			"open header",
			"○ ce-vkvk · DEAR retro   [● P1 · OPEN]\nOwner: vbonnet",
			BeadOpen,
		},
		{
			"in-progress header maps to open",
			"◐ ce-xyz1 · thing   [● P2 · IN_PROGRESS]",
			BeadOpen,
		},
		{
			"no status token is unknown",
			"some unrelated output without a bracketed status",
			BeadUnknown,
		},
		{
			"empty is unknown",
			"",
			BeadUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseBeadStatus(tt.in); got != tt.want {
				t.Errorf("parseBeadStatus(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
