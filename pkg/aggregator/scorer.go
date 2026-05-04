package aggregator

import "sort"

// DefaultWeights are the per-Kind weights the scorer uses when none
// are configured. See ADR-015 §D6 for the rationale; in short,
// security dominates and git_activity is a tiebreaker.
var DefaultWeights = map[Kind]float64{
	KindSecurityAlerts: 1.0,
	KindLintTrend:      0.4,
	KindTestCoverage:   0.5,
	KindDepFreshness:   0.3,
	KindGitActivity:    0.2,
}

// kindCeilings are the per-Kind "this many is plenty" ceilings used to
// normalize a raw value into 0..1. They live here as constants so v1
// tuning is a one-file change; ADR-015 §D6 commits to revisiting them
// once we have real-world signals.
var kindCeilings = map[Kind]float64{
	KindGitActivity:    100, // 100 commits/week saturates "active"
	KindLintTrend:      200, // 200 findings on one file is already a fire
	KindTestCoverage:   100, // percent
	KindDepFreshness:   50,  // 50 outdated modules saturates "stale"
	KindSecurityAlerts: 10,  // 10 vulns is already an emergency
}

// Scorer computes weighted priority across signal kinds.
type Scorer struct {
	// Weights override DefaultWeights when non-nil. Missing keys fall
	// back to the default.
	Weights map[Kind]float64
}

// Score is one row of a Scorer.Score result.
type Score struct {
	Kind     Kind    `json:"kind"`
	Subject  string  `json:"subject,omitempty"`
	Raw      float64 `json:"raw"`
	Norm     float64 `json:"norm"`
	Weight   float64 `json:"weight"`
	Weighted float64 `json:"weighted"`
}

// Score takes a flat list of signals (typically the most recent
// observation per kind, or a per-subject slice) and returns one Score
// per signal, normalized and weighted. Output is sorted by Weighted
// descending so callers can take the top N directly.
//
// Normalization rules:
//
//   - test_coverage: a coverage drop is the pressure, so Norm =
//     1 - clamp(value/100, 0, 1).
//   - everything else: Norm = clamp(value/ceiling, 0, 1).
//
// Subjects are preserved on the output so per-package callers
// (recommendation engine) can attribute the score back to the file
// or package that produced it.
func (s Scorer) Score(signals []Signal) []Score {
	out := make([]Score, 0, len(signals))
	for _, sig := range signals {
		w := s.weight(sig.Kind)
		norm := normalize(sig.Kind, sig.Value)
		out = append(out, Score{
			Kind:     sig.Kind,
			Subject:  sig.Subject,
			Raw:      sig.Value,
			Norm:     norm,
			Weight:   w,
			Weighted: norm * w,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Weighted > out[j].Weighted
	})
	return out
}

// Total sums the Weighted column. The recommendation engine uses this
// as a single per-snapshot priority number when ranking repos.
func (s Scorer) Total(scores []Score) float64 {
	var total float64
	for _, sc := range scores {
		total += sc.Weighted
	}
	return total
}

// weight returns the configured weight for k, falling back to
// DefaultWeights when Weights is nil or missing the key.
func (s Scorer) weight(k Kind) float64 {
	if s.Weights != nil {
		if w, ok := s.Weights[k]; ok {
			return w
		}
	}
	if w, ok := DefaultWeights[k]; ok {
		return w
	}
	return 0
}

// normalize maps a raw collector value into 0..1 using kindCeilings.
// Coverage is inverted so a *drop* in coverage produces a *higher*
// pressure score (matches operator intuition: "more coverage is good").
func normalize(k Kind, v float64) float64 {
	ceiling, ok := kindCeilings[k]
	if !ok || ceiling == 0 {
		return 0
	}
	if k == KindTestCoverage {
		return 1 - clamp01(v/ceiling)
	}
	return clamp01(v / ceiling)
}

// clamp01 clamps x to the closed interval [0, 1].
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
