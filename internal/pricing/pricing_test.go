package pricing

import "testing"

func TestLookup_Alias(t *testing.T) {
	cases := []struct {
		model string
		in    float64
		out   float64
	}{
		{"fable", 10.00, 50.00},
		{"claude-fable-5", 10.00, 50.00}, // full name resolves via substring
		{"opus", 15.00, 75.00},
		{"sonnet", 3.00, 15.00},
		{"haiku", 1.00, 5.00},
		{"fable", 10.00, 50.00},
		{"Opus", 15.00, 75.00},  // case-insensitive
		{"SONNET", 3.00, 15.00}, // case-insensitive
		{"FABLE", 10.00, 50.00}, // case-insensitive
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
		{"claude-fable-5", "fable"},
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

func TestOpenRouterFamilyDefaultsHaveSourcedPricing(t *testing.T) {
	models := []string{
		"z-ai/glm-5.2",
		"deepseek/deepseek-v4-pro",
		"nvidia/nemotron-3-ultra-550b-a55b",
		"qwen/qwen3.6-max-preview",
	}
	for _, model := range models {
		price := Lookup(model)
		if price == UnknownModel || price.InputPerMillion <= 0 || price.OutputPerMillion <= 0 {
			t.Errorf("Lookup(%q) = %+v, want priced model", model, price)
		}
		if price.Source == "" || price.AsOf == "" {
			t.Errorf("Lookup(%q) lacks pricing provenance: %+v", model, price)
		}
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

// TestLookup_NewModels2026_09 pins the rate cards for the two models wired in
// 2026-09. Fable 5.1 shares Fable 5's base input/output rates (they differ only
// in cache-read multiplier, which this table does not model); Gemini 3.8 Flash
// is a new rate that previously fell through to UnknownModel.
func TestLookup_NewModels2026_09(t *testing.T) {
	cases := []struct {
		model string
		in    float64
		out   float64
	}{
		{"claude-fable-5-1", 10.00, 50.00},
		{"3.8-flash", 0.75, 3.75},
		{"gemini-3.8-flash", 0.75, 3.75},
	}
	for _, c := range cases {
		p := Lookup(c.model)
		if p.InputPerMillion != c.in || p.OutputPerMillion != c.out {
			t.Errorf("Lookup(%q) = {%f, %f}, want {%f, %f}",
				c.model, p.InputPerMillion, p.OutputPerMillion, c.in, c.out)
		}
	}
}

// TestGemini38FlashDoesNotShadowOlderFlashRates guards the substring-match
// fallback: "gemini-3.8-flash" must not collide with the older "3-flash" or
// "2.5-flash" entries, which are priced differently.
func TestGemini38FlashDoesNotShadowOlderFlashRates(t *testing.T) {
	if got := Lookup("gemini-3.5-flash"); got.InputPerMillion == 0.75 {
		t.Errorf("gemini-3.5-flash wrongly resolved to the 3.8 Flash rate: %+v", got)
	}
	if got := Lookup("gemini-3-flash-preview"); got.InputPerMillion != 0.30 {
		t.Errorf("gemini-3-flash-preview = %+v, want the 3-flash rate 0.30", got)
	}
}

// TestNewModelRatesAreSourced enforces PRICING-07's auditability rule for the
// entries added in 2026-09: a published rate card and an as-of date.
func TestNewModelRatesAreSourced(t *testing.T) {
	for _, model := range []string{"claude-fable-5-1", "3.8-flash"} {
		p := Lookup(model)
		if p.Source == "" || p.AsOf == "" {
			t.Errorf("Lookup(%q) = %+v, want a non-empty Source and AsOf", model, p)
		}
	}
}
