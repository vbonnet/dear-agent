package pricing

import "testing"

func TestLookup_Alias(t *testing.T) {
	cases := []struct {
		model string
		in    float64
		out   float64
	}{
		{"opus", 15.00, 75.00},
		{"sonnet", 3.00, 15.00},
		{"haiku", 1.00, 5.00},
		{"Opus", 15.00, 75.00},  // case-insensitive
		{"SONNET", 3.00, 15.00}, // case-insensitive
	}
	for _, c := range cases {
		p := Lookup(c.model)
		if p.InputPerMillion != c.in || p.OutputPerMillion != c.out {
			t.Errorf("Lookup(%q) = {%f, %f}, want {%f, %f}",
				c.model, p.InputPerMillion, p.OutputPerMillion, c.in, c.out)
		}
	}
}

func TestLookup_FullName(t *testing.T) {
	// Full names should resolve via substring match.
	cases := []struct {
		model string
		want  string // expected alias
	}{
		{"claude-opus-4-6[1m]", "opus"},
		{"claude-sonnet-4-6", "sonnet"},
		{"claude-haiku-4-5", "haiku"},
	}
	for _, c := range cases {
		p := Lookup(c.model)
		expected := Lookup(c.want)
		if p != expected {
			t.Errorf("Lookup(%q) = %+v, want same as Lookup(%q) = %+v",
				c.model, p, c.want, expected)
		}
	}
}

func TestLookup_Unknown(t *testing.T) {
	p := Lookup("nonexistent-model-xyz")
	if p != UnknownModel {
		t.Errorf("Lookup(unknown) = %+v, want UnknownModel", p)
	}
	p = Lookup("")
	if p != UnknownModel {
		t.Errorf("Lookup(\"\") = %+v, want UnknownModel", p)
	}
}

func TestEstimate(t *testing.T) {
	// 1M opus input tokens = $15
	got := Estimate("opus", 1_000_000, 0)
	if got < 14.99 || got > 15.01 {
		t.Errorf("Estimate(opus, 1M, 0) = %f, want ~15.00", got)
	}
	// 1M sonnet output tokens = $15
	got = Estimate("sonnet", 0, 1_000_000)
	if got < 14.99 || got > 15.01 {
		t.Errorf("Estimate(sonnet, 0, 1M) = %f, want ~15.00", got)
	}
	// Unknown model → 0
	got = Estimate("nonexistent", 1_000_000, 1_000_000)
	if got != 0 {
		t.Errorf("Estimate(unknown) = %f, want 0", got)
	}
}

func TestIsKnown(t *testing.T) {
	if !IsKnown("opus") {
		t.Error("opus should be known")
	}
	if !IsKnown("claude-sonnet-4-6[1m]") {
		t.Error("full Claude name should resolve")
	}
	if IsKnown("nonexistent-model") {
		t.Error("nonexistent should not be known")
	}
}

// Opus should be meaningfully more expensive than Sonnet — regression guard
// for the "flip default to sonnet" decision.
func TestOpusCostsMoreThanSonnet(t *testing.T) {
	opus := Lookup("opus")
	sonnet := Lookup("sonnet")
	if opus.InputPerMillion <= sonnet.InputPerMillion*2 {
		t.Errorf("expected opus input price to be >2x sonnet, got opus=%f sonnet=%f",
			opus.InputPerMillion, sonnet.InputPerMillion)
	}
	if opus.OutputPerMillion <= sonnet.OutputPerMillion*2 {
		t.Errorf("expected opus output price to be >2x sonnet, got opus=%f sonnet=%f",
			opus.OutputPerMillion, sonnet.OutputPerMillion)
	}
}
