package review

import (
	"reflect"
	"testing"
)

func TestParseRiskLevel(t *testing.T) {
	tests := []struct {
		in   string
		want RiskLevel
	}{
		{"XS", RiskLevelXS},
		{"xs", RiskLevelXS},
		{" S ", RiskLevelS},
		{"M", RiskLevelM},
		{"L", RiskLevelL},
		{"XL", RiskLevelXL},
		{"", RiskLevelM},        // empty defaults to standard
		{"garbage", RiskLevelM}, // unknown defaults to standard
	}

	for _, tt := range tests {
		if got := ParseRiskLevel(tt.in); got != tt.want {
			t.Errorf("ParseRiskLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestParseRiskLevel_DrivesLiteSkips ties the parser to the profile config: an XS or S
// risk string must resolve to a profile that skips DESIGN/SPEC/PLAN, while M does not.
// This is the contract the session-start wiring relies on (ce-12pl / MVD T2.2).
func TestParseRiskLevel_DrivesLiteSkips(t *testing.T) {
	liteSkips := []string{"DESIGN", "SPEC", "PLAN"}

	for _, risk := range []string{"XS", "S"} {
		cfg := GetProfileConfigForRisk(ParseRiskLevel(risk))
		if !reflect.DeepEqual(cfg.SkipPhases, liteSkips) {
			t.Errorf("risk %q: expected SkipPhases %v, got %v", risk, liteSkips, cfg.SkipPhases)
		}
	}

	if cfg := GetProfileConfigForRisk(ParseRiskLevel("M")); len(cfg.SkipPhases) != 0 {
		t.Errorf("risk M: expected no skipped phases, got %v", cfg.SkipPhases)
	}
}
