package salience

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTierOrdering(t *testing.T) {
	tiers := []Tier{TierNoise, TierLow, TierMedium, TierHigh, TierCritical}
	for i := 1; i < len(tiers); i++ {
		if tiers[i-1] >= tiers[i] {
			t.Errorf("expected %s < %s, got %d vs %d",
				tiers[i-1], tiers[i], tiers[i-1], tiers[i])
		}
	}
}

func TestTierString(t *testing.T) {
	cases := map[Tier]string{
		TierNoise:    "noise",
		TierLow:      "low",
		TierMedium:   "medium",
		TierHigh:     "high",
		TierCritical: "critical",
	}
	for tier, want := range cases {
		if got := tier.String(); got != want {
			t.Errorf("Tier(%d).String() = %q, want %q", tier, got, want)
		}
	}
	if got := Tier(99).String(); !strings.Contains(got, "99") {
		t.Errorf("unknown tier should include numeric, got %q", got)
	}
}

func TestParseTier(t *testing.T) {
	cases := map[string]Tier{
		"":         TierNoise,
		"noise":    TierNoise,
		"NOISE":    TierNoise,
		"low":      TierLow,
		"medium":   TierMedium,
		"med":      TierMedium,
		"high":     TierHigh,
		"critical": TierCritical,
		"crit":     TierCritical,
	}
	for in, want := range cases {
		got, err := ParseTier(in)
		if err != nil {
			t.Errorf("ParseTier(%q) unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseTier(%q) = %s, want %s", in, got, want)
		}
	}
	if got, err := ParseTier("  high\t"); err != nil || got != TierHigh {
		t.Errorf("ParseTier should trim whitespace, got %s, %v", got, err)
	}
	if _, err := ParseTier("urgent"); err == nil {
		t.Error("ParseTier(urgent) should error")
	}
}

func TestTierJSONRoundTrip(t *testing.T) {
	for _, tier := range []Tier{TierNoise, TierLow, TierMedium, TierHigh, TierCritical} {
		data, err := json.Marshal(tier)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", tier, err)
		}
		var got Tier
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(%s): %v", data, err)
		}
		if got != tier {
			t.Errorf("round-trip mismatch: %s -> %s", tier, got)
		}
	}
}

func TestTierUnmarshalNumeric(t *testing.T) {
	// Numeric form is a transitional convenience; lock in that it works.
	var got Tier
	if err := json.Unmarshal([]byte("3"), &got); err != nil {
		t.Fatalf("unmarshal 3: %v", err)
	}
	if got != TierHigh {
		t.Errorf("Unmarshal(3) = %s, want high", got)
	}
}

func TestTierUnmarshalInvalid(t *testing.T) {
	var got Tier
	if err := json.Unmarshal([]byte(`"urgent"`), &got); err == nil {
		t.Error("Unmarshal(\"urgent\") should error")
	}
	if err := json.Unmarshal([]byte(`{}`), &got); err == nil {
		t.Error("Unmarshal({}) should error")
	}
}

func TestKindValidate(t *testing.T) {
	for _, k := range AllKinds() {
		if err := k.Validate(); err != nil {
			t.Errorf("AllKinds() %s should validate, got %v", k, err)
		}
	}
	if err := Kind("not_a_kind").Validate(); err == nil {
		t.Error("unknown kind should not validate")
	}
}

func TestAllKindsCount(t *testing.T) {
	// Lock in that we have exactly the nine drift kinds the design promises.
	if got, want := len(AllKinds()), 9; got != want {
		t.Errorf("AllKinds() len = %d, want %d", got, want)
	}
}

func TestSignalValidate(t *testing.T) {
	good := Signal{Kind: KindBuildFailure, Subject: "make"}
	if err := good.Validate(); err != nil {
		t.Errorf("good signal should validate: %v", err)
	}
	bad := Signal{Kind: "weather"}
	if err := bad.Validate(); err == nil {
		t.Error("bad kind should fail validation")
	}
}

func TestSignalJSONRoundTrip(t *testing.T) {
	want := Signal{
		ID:         "sig-1",
		Kind:       KindBuildFailure,
		Subject:    "go build ./...",
		Salience:   TierCritical,
		Source:     "ci",
		Note:       "exit 2",
		ObservedAt: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"salience":"critical"`) {
		t.Errorf("expected salience as string in %s", data)
	}
	var got Signal
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != want {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}
