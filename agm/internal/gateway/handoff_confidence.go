package gateway

import (
	"fmt"
	"strings"
)

// HandoffConfidenceLevel is the qualitative trust a handing-off agent
// places in the context it is serializing. It exists so a fresh agent
// picking up the handoff cold knows how much to lean on the prior work
// versus re-deriving it.
type HandoffConfidenceLevel string

// Confidence level values, ordered low → high.
const (
	// ConfidenceLow means the context is known-incomplete or unverified:
	// the receiving agent should re-establish the important facts itself.
	ConfidenceLow HandoffConfidenceLevel = "low"
	// ConfidenceMedium means the context is broadly trustworthy but has
	// gaps the receiving agent should be aware of.
	ConfidenceMedium HandoffConfidenceLevel = "medium"
	// ConfidenceHigh means the handing-off agent vouches for the context:
	// it is complete and verified to the best of its knowledge.
	ConfidenceHigh HandoffConfidenceLevel = "high"
)

// Confidence band boundaries on the 0.0–1.0 score axis. A score on a
// boundary rounds up to the higher band (>= is inclusive at the floor).
const (
	confidenceMediumFloor = 0.40
	confidenceHighFloor   = 0.75
)

// IsValid reports whether l is one of the three recognized levels.
func (l HandoffConfidenceLevel) IsValid() bool {
	switch l {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
		return true
	default:
		return false
	}
}

// LevelForScore maps a 0.0–1.0 score onto the qualitative band. Scores
// outside [0,1] are clamped so the function is total — callers that need
// to reject out-of-range scores should use Validate.
func LevelForScore(score float64) HandoffConfidenceLevel {
	switch {
	case score >= confidenceHighFloor:
		return ConfidenceHigh
	case score >= confidenceMediumFloor:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

// HandoffConfidence is the handing-off agent's self-assessment of the
// completeness and accuracy of the context it is transferring. It travels
// inside HandoffContext so the assessment is serialized alongside the
// payload it describes — a receiving agent never sees the context without
// also seeing how much the sender trusted it.
type HandoffConfidence struct {
	// Level is the qualitative band. It is always consistent with Score
	// (NewHandoffConfidence derives it; Validate enforces it).
	Level HandoffConfidenceLevel `json:"level"`
	// Score is the fine-grained confidence on a 0.0–1.0 axis. It mirrors
	// the convention used by RoutingDecision.Confidence so the two can be
	// compared and rolled up together.
	Score float64 `json:"score"`
	// Rationale explains *why* the confidence is what it is. This is the
	// single most useful field for a cold receiver: "low because the
	// integration suite was never run" tells it exactly what to distrust.
	Rationale string `json:"rationale"`
	// Gaps enumerates known unknowns / incomplete areas in the handed-off
	// context. Empty means "no gaps the sender is aware of" — which is not
	// the same as "no gaps".
	Gaps []string `json:"gaps,omitempty"`
}

// NewHandoffConfidence builds a confidence assessment from a score and a
// rationale, deriving the qualitative level from the score so the two can
// never disagree. It returns an error for an out-of-range score or an
// empty rationale: an unexplained confidence number is worse than none,
// because a cold receiver cannot reason about it.
func NewHandoffConfidence(score float64, rationale string, gaps ...string) (*HandoffConfidence, error) {
	hc := &HandoffConfidence{
		Level:     LevelForScore(score),
		Score:     score,
		Rationale: strings.TrimSpace(rationale),
		Gaps:      gaps,
	}
	if err := hc.Validate(); err != nil {
		return nil, err
	}
	return hc, nil
}

// Validate checks the assessment is internally consistent: score in
// [0,1], a recognized level, the level matching the score's band, and a
// non-empty rationale. It is called by NewHandoffConfidence and again on
// deserialization so a tampered or hand-written handoff file is rejected
// rather than silently trusted.
func (hc *HandoffConfidence) Validate() error {
	if hc.Score < 0.0 || hc.Score > 1.0 {
		return fmt.Errorf("handoff confidence: score %.3f out of range [0.0, 1.0]", hc.Score)
	}
	if !hc.Level.IsValid() {
		return fmt.Errorf("handoff confidence: unknown level %q (want one of: low, medium, high)", hc.Level)
	}
	if want := LevelForScore(hc.Score); hc.Level != want {
		return fmt.Errorf("handoff confidence: level %q inconsistent with score %.3f (score maps to %q)", hc.Level, hc.Score, want)
	}
	if strings.TrimSpace(hc.Rationale) == "" {
		return fmt.Errorf("handoff confidence: rationale must not be empty")
	}
	return nil
}
