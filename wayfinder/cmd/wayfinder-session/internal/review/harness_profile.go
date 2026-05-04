// Package review provides review-related functionality.
package review

// HarnessProfile defines the process depth profile for a task based on its risk level.
// Profiles control which phases run, whether an evaluator is required, and how many
// review personas participate.
type HarnessProfile string

// HarnessProfile values selected based on task risk.
const (
	ProfileLite     HarnessProfile = "lite"     // XS/S risk — minimal scaffolding
	ProfileStandard HarnessProfile = "standard" // M risk — full phase sequence
	ProfileDeep     HarnessProfile = "deep"     // L/XL risk — full sequence with mandatory evaluation
)

// ProfileConfig holds the concrete settings associated with a HarnessProfile.
type ProfileConfig struct {
	Profile           HarnessProfile
	SkipPhases        []string // Phases to skip (e.g. "DESIGN", "SPEC", "PLAN")
	EvaluatorRequired bool     // Whether adversarial evaluator is mandatory
	ReviewPersonas    int      // Number of review personas
	MaxRetries        int      // Max evaluator retries
	Description       string   // Human-readable description
}

// GetProfileForRisk maps a RiskLevel to a HarnessProfile.
func GetProfileForRisk(risk RiskLevel) HarnessProfile {
	switch risk {
	case RiskLevelXS, RiskLevelS:
		return ProfileLite
	case RiskLevelM:
		return ProfileStandard
	case RiskLevelL, RiskLevelXL:
		return ProfileDeep
	default:
		return ProfileStandard
	}
}

// GetProfileConfig returns the configuration for a given profile.
func GetProfileConfig(profile HarnessProfile) ProfileConfig {
	switch profile {
	case ProfileLite:
		return ProfileConfig{
			Profile:           ProfileLite,
			SkipPhases:        []string{"DESIGN", "SPEC", "PLAN"},
			EvaluatorRequired: false,
			ReviewPersonas:    2,
			MaxRetries:        0,
			Description:       "Lightweight process for low-risk changes — skips design/spec/plan phases",
		}
	case ProfileStandard:
		return ProfileConfig{
			Profile:           ProfileStandard,
			SkipPhases:        nil,
			EvaluatorRequired: false,
			ReviewPersonas:    3,
			MaxRetries:        1,
			Description:       "Standard process with full phase sequence and optional evaluation",
		}
	case ProfileDeep:
		return ProfileConfig{
			Profile:           ProfileDeep,
			SkipPhases:        nil,
			EvaluatorRequired: true,
			ReviewPersonas:    5,
			MaxRetries:        3,
			Description:       "Deep process for high-risk changes — mandatory evaluator with retries",
		}
	default:
		return GetProfileConfig(ProfileStandard)
	}
}

// GetProfileConfigForRisk is a convenience function that maps a RiskLevel directly
// to its ProfileConfig without an intermediate profile lookup.
func GetProfileConfigForRisk(risk RiskLevel) ProfileConfig {
	return GetProfileConfig(GetProfileForRisk(risk))
}

// GetProfileConfigWithOverride returns the profile configuration for the given risk
// level, unless an explicit override is provided. When override is non-nil the
// returned config corresponds to the overridden profile regardless of risk.
func GetProfileConfigWithOverride(risk RiskLevel, override *HarnessProfile) ProfileConfig {
	if override != nil {
		return GetProfileConfig(*override)
	}
	return GetProfileConfigForRisk(risk)
}

// ShouldSkipPhase reports whether the given phase should be skipped under this profile.
func (pc ProfileConfig) ShouldSkipPhase(phase string) bool {
	for _, p := range pc.SkipPhases {
		if p == phase {
			return true
		}
	}
	return false
}
