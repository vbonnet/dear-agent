package manifest

import "testing"

func TestParseSessionLifecycle(t *testing.T) {
	for _, value := range []string{"", "reaping", "archived"} {
		t.Run(value, func(t *testing.T) {
			lifecycle, err := ParseSessionLifecycle(value)
			if err != nil {
				t.Fatalf("ParseSessionLifecycle(%q) error: %v", value, err)
			}
			if string(lifecycle) != value || !lifecycle.Valid() {
				t.Errorf("lifecycle = %q (valid=%t), want accepted %q", lifecycle, lifecycle.Valid(), value)
			}
		})
	}
	if _, err := ParseSessionLifecycle("unknown"); err == nil {
		t.Fatal("ParseSessionLifecycle accepted an unknown value")
	}
}

func TestParseSessionOutcome(t *testing.T) {
	for _, value := range []string{"", "completed", "crashed", "killed", "gc-stale"} {
		t.Run(value, func(t *testing.T) {
			outcome, err := ParseSessionOutcome(value)
			if err != nil {
				t.Fatalf("ParseSessionOutcome(%q) error: %v", value, err)
			}
			if string(outcome) != value || !outcome.Valid() {
				t.Errorf("outcome = %q (valid=%t), want accepted %q", outcome, outcome.Valid(), value)
			}
		})
	}
	if _, err := ParseSessionOutcome("unknown"); err == nil {
		t.Fatal("ParseSessionOutcome accepted an unknown value")
	}
}

func TestManifestValidateRejectsUnknownLifecycleAndOutcome(t *testing.T) {
	valid := &Manifest{
		SchemaVersion: "2.0",
		SessionID:     "session",
		Name:          "session",
		Context:       Context{Project: "/project"},
		Tmux:          Tmux{SessionName: "session"},
	}
	for _, test := range []struct {
		name   string
		mutate func(*Manifest)
	}{
		{name: "lifecycle", mutate: func(m *Manifest) { m.Lifecycle = "unknown" }},
		{name: "outcome", mutate: func(m *Manifest) { m.Outcome = SessionOutcome("unknown") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest := *valid
			test.mutate(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatalf("Validate accepted unknown %s", test.name)
			}
		})
	}
}
