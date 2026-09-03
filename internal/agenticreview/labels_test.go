package agenticreview

import "testing"

func TestLabelRendersFamilyAndPhase(t *testing.T) {
	if got, want := Label(FamilyClaude, PhaseStarted), "agentic-review:claude:started"; got != want {
		t.Fatalf("Label = %q, want %q", got, want)
	}
	if got, want := Label(FamilyCodex, PhaseChangesRequested), "agentic-review:codex:changes-requested"; got != want {
		t.Fatalf("Label = %q, want %q", got, want)
	}
}

func TestParseLabelRoundTrips(t *testing.T) {
	for _, f := range DefaultFamilies {
		for _, p := range AllPhases {
			name := Label(f, p)
			gotF, gotP, ok := ParseLabel(name)
			if !ok {
				t.Fatalf("ParseLabel(%q) not recognized", name)
			}
			if gotF != f || gotP != p {
				t.Fatalf("ParseLabel(%q) = %q/%q, want %q/%q", name, gotF, gotP, f, p)
			}
		}
	}
}

func TestParseLabelRejectsForeignLabels(t *testing.T) {
	for _, name := range []string{
		"needs-security-review",
		"agentic-review:claude",
		"agentic-review:claude:started:extra",
		"agentic-review::started",
		"agentic-review:claude:launched",
		"ai-review:override",
		"",
	} {
		if _, _, ok := ParseLabel(name); ok {
			t.Errorf("ParseLabel(%q) = ok, want not recognized", name)
		}
	}
}

// A phase label for a family outside the configured set must still parse: the
// gate has to be able to see a stray family rather than silently treating its
// label as unrelated repository noise.
func TestParseLabelAcceptsUnconfiguredFamily(t *testing.T) {
	f, p, ok := ParseLabel("agentic-review:llama:approved")
	if !ok || f != Family("llama") || p != PhaseApproved {
		t.Fatalf("ParseLabel = %q/%q/%v, want llama/approved/true", f, p, ok)
	}
}

func TestManagedLabelsCoversEveryFamilyPhase(t *testing.T) {
	got := ManagedLabels(DefaultFamilies)
	if want := len(DefaultFamilies) * len(AllPhases); len(got) != want {
		t.Fatalf("ManagedLabels returned %d labels, want %d", len(got), want)
	}
	seen := make(map[string]bool, len(got))
	for _, name := range got {
		if seen[name] {
			t.Fatalf("ManagedLabels repeated %q", name)
		}
		seen[name] = true
		if _, _, ok := ParseLabel(name); !ok {
			t.Fatalf("ManagedLabels produced unparseable %q", name)
		}
	}
}
