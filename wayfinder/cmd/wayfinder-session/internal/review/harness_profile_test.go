package review

import (
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestGetProfileForRisk_LiteForXSAndS(t *testing.T) {
	tests := []struct {
		risk     RiskLevel
		expected HarnessProfile
	}{
		{RiskLevelXS, ProfileLite},
		{RiskLevelS, ProfileLite},
	}

	for _, tt := range tests {
		got := GetProfileForRisk(tt.risk)
		if got != tt.expected {
			t.Errorf("GetProfileForRisk(%s) = %s, want %s", tt.risk, got, tt.expected)
		}
	}
}

func TestGetProfileForRisk_StandardForM(t *testing.T) {
	got := GetProfileForRisk(RiskLevelM)
	if got != ProfileStandard {
		t.Errorf("GetProfileForRisk(M) = %s, want %s", got, ProfileStandard)
	}
}

func TestGetProfileForRisk_DeepForLAndXL(t *testing.T) {
	tests := []struct {
		risk     RiskLevel
		expected HarnessProfile
	}{
		{RiskLevelL, ProfileDeep},
		{RiskLevelXL, ProfileDeep},
	}

	for _, tt := range tests {
		got := GetProfileForRisk(tt.risk)
		if got != tt.expected {
			t.Errorf("GetProfileForRisk(%s) = %s, want %s", tt.risk, got, tt.expected)
		}
	}
}

func TestLiteProfileSkipsCorrectPhases(t *testing.T) {
	cfg := GetProfileConfig(ProfileLite)

	expectedSkips := []string{"DESIGN", "SPEC", "PLAN"}
	for _, phase := range expectedSkips {
		if !cfg.ShouldSkipPhase(phase) {
			t.Errorf("Lite profile should skip phase %s", phase)
		}
	}

	// Should NOT skip IMPLEMENT or REVIEW
	for _, phase := range []string{"IMPLEMENT", "REVIEW", "TEST"} {
		if cfg.ShouldSkipPhase(phase) {
			t.Errorf("Lite profile should not skip phase %s", phase)
		}
	}
}

func TestLiteProfileSettings(t *testing.T) {
	cfg := GetProfileConfig(ProfileLite)

	if cfg.EvaluatorRequired {
		t.Error("Lite profile should not require evaluator")
	}
	if cfg.ReviewPersonas != 2 {
		t.Errorf("Lite profile should have 2 review personas, got %d", cfg.ReviewPersonas)
	}
	if cfg.MaxRetries != 0 {
		t.Errorf("Lite profile should have 0 retries, got %d", cfg.MaxRetries)
	}
}

func TestStandardProfileSettings(t *testing.T) {
	cfg := GetProfileConfig(ProfileStandard)

	if cfg.EvaluatorRequired {
		t.Error("Standard profile should not require evaluator")
	}
	if cfg.ReviewPersonas != 3 {
		t.Errorf("Standard profile should have 3 review personas, got %d", cfg.ReviewPersonas)
	}
	if cfg.MaxRetries != 1 {
		t.Errorf("Standard profile should have 1 retry, got %d", cfg.MaxRetries)
	}
	if len(cfg.SkipPhases) != 0 {
		t.Errorf("Standard profile should skip no phases, got %v", cfg.SkipPhases)
	}
}

func TestDeepProfileRequiresEvaluator(t *testing.T) {
	cfg := GetProfileConfig(ProfileDeep)

	if !cfg.EvaluatorRequired {
		t.Error("Deep profile must require evaluator")
	}
	if cfg.ReviewPersonas != 5 {
		t.Errorf("Deep profile should have 5 review personas, got %d", cfg.ReviewPersonas)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("Deep profile should have 3 retries, got %d", cfg.MaxRetries)
	}
	if len(cfg.SkipPhases) != 0 {
		t.Errorf("Deep profile should skip no phases, got %v", cfg.SkipPhases)
	}
}

func TestOverrideForcesSpecificProfile(t *testing.T) {
	// Even with XS risk, override to deep should return deep config
	deep := ProfileDeep
	cfg := GetProfileConfigWithOverride(RiskLevelXS, &deep)

	if cfg.Profile != ProfileDeep {
		t.Errorf("Override should force deep profile, got %s", cfg.Profile)
	}
	if !cfg.EvaluatorRequired {
		t.Error("Overridden deep profile must require evaluator")
	}
	if cfg.ReviewPersonas != 5 {
		t.Errorf("Overridden deep profile should have 5 personas, got %d", cfg.ReviewPersonas)
	}
}

func TestOverrideNilUsesRisk(t *testing.T) {
	cfg := GetProfileConfigWithOverride(RiskLevelXL, nil)

	if cfg.Profile != ProfileDeep {
		t.Errorf("Nil override with XL risk should return deep profile, got %s", cfg.Profile)
	}
}

func TestOverrideLiteOnHighRisk(t *testing.T) {
	lite := ProfileLite
	cfg := GetProfileConfigWithOverride(RiskLevelXL, &lite)

	if cfg.Profile != ProfileLite {
		t.Errorf("Override should force lite profile even on XL risk, got %s", cfg.Profile)
	}
	if cfg.EvaluatorRequired {
		t.Error("Overridden lite profile should not require evaluator")
	}
}

func TestGetProfileConfigForRisk(t *testing.T) {
	tests := []struct {
		risk            RiskLevel
		expectedProfile HarnessProfile
	}{
		{RiskLevelXS, ProfileLite},
		{RiskLevelS, ProfileLite},
		{RiskLevelM, ProfileStandard},
		{RiskLevelL, ProfileDeep},
		{RiskLevelXL, ProfileDeep},
	}

	for _, tt := range tests {
		cfg := GetProfileConfigForRisk(tt.risk)
		if cfg.Profile != tt.expectedProfile {
			t.Errorf("GetProfileConfigForRisk(%s) = %s, want %s",
				tt.risk, cfg.Profile, tt.expectedProfile)
		}
	}
}

func TestClassifyRiskReturnsProfileConfig(t *testing.T) {
	config := DefaultReviewConfig()
	adapter := NewRiskAdapter(config)

	tmpDir := t.TempDir()

	task := &status.Task{
		ID:           "test-task",
		Description:  "fix typo in docs",
		Deliverables: []string{},
	}

	risk, profile := adapter.ClassifyRisk(task, tmpDir)

	// With no deliverables and a low-risk description, risk should be low
	expectedProfile := GetProfileForRisk(risk)
	if profile.Profile != expectedProfile {
		t.Errorf("ClassifyRisk returned profile %s, expected %s for risk %s",
			profile.Profile, expectedProfile, risk)
	}
}

func TestShouldSkipPhase(t *testing.T) {
	cfg := ProfileConfig{
		SkipPhases: []string{"DESIGN", "SPEC"},
	}

	if !cfg.ShouldSkipPhase("DESIGN") {
		t.Error("Should skip DESIGN")
	}
	if !cfg.ShouldSkipPhase("SPEC") {
		t.Error("Should skip SPEC")
	}
	if cfg.ShouldSkipPhase("IMPLEMENT") {
		t.Error("Should not skip IMPLEMENT")
	}
}

func TestUnknownProfileDefaultsToStandard(t *testing.T) {
	cfg := GetProfileConfig(HarnessProfile("unknown"))
	if cfg.Profile != ProfileStandard {
		t.Errorf("Unknown profile should default to standard, got %s", cfg.Profile)
	}
}
