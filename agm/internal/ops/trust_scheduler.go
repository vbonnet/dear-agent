package ops

import (
	"sort"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// TrustTier classifies an agent's trust level for scheduling decisions.
type TrustTier string

const (
	// TrustTierPreferred — agent has a strong track record, gets priority
	// for complex tasks.
	TrustTierPreferred TrustTier = "preferred"

	// TrustTierNormal — agent is above min dispatch but below preferred.
	TrustTierNormal TrustTier = "normal"

	// TrustTierProbation — agent is unreliable, only receives XS tasks.
	TrustTierProbation TrustTier = "probation"

	// TrustTierBlocked — agent is below min dispatch score, no unsupervised
	// work.
	TrustTierBlocked TrustTier = "blocked"
)

// AgentCandidate represents an available agent for work dispatch.
type AgentCandidate struct {
	SessionName string `json:"session_name"`
	TrustScore  int    `json:"trust_score"`
	TrustTier   TrustTier `json:"trust_tier"`
}

// DispatchResult is the output of TrustAwareDispatch.
type DispatchResult struct {
	Ranked   []AgentCandidate `json:"ranked"`
	Blocked  []AgentCandidate `json:"blocked,omitempty"`
}

// TrustThresholds returns the current scheduling thresholds from contracts.
func TrustThresholds() (minDispatch, preferred, probation int) {
	slo := contracts.Load()
	return slo.TrustProtocol.MinDispatchScore,
		slo.TrustProtocol.PreferredScore,
		slo.TrustProtocol.ProbationScore
}

// ClassifyTrust maps a trust score to its scheduling tier.
func ClassifyTrust(score int) TrustTier {
	minDispatch, preferred, probation := TrustThresholds()

	switch {
	case score >= preferred:
		return TrustTierPreferred
	case score >= minDispatch:
		return TrustTierNormal
	case score >= probation:
		return TrustTierProbation
	default:
		return TrustTierBlocked
	}
}

// TrustAwareDispatch ranks available agents by trust score and separates
// blocked agents. Agents above minDispatchScore are returned in descending
// score order; agents below are returned in the Blocked list.
func TrustAwareDispatch(sessionNames []string) (*DispatchResult, error) {
	minDispatch, _, _ := TrustThresholds()

	candidates := make([]AgentCandidate, 0, len(sessionNames))
	for _, name := range sessionNames {
		result, err := TrustScore(nil, &TrustScoreRequest{SessionName: name})
		if err != nil {
			// If we can't read trust data, use base score (new agent).
			result = &TrustScoreResult{
				SessionName: name,
				Score:       contracts.Load().TrustProtocol.BaseScore,
			}
		}
		candidates = append(candidates, AgentCandidate{
			SessionName: name,
			TrustScore:  result.Score,
			TrustTier:   ClassifyTrust(result.Score),
		})
	}

	// Sort by score descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TrustScore > candidates[j].TrustScore
	})

	dr := &DispatchResult{}
	for _, c := range candidates {
		if c.TrustScore >= minDispatch {
			dr.Ranked = append(dr.Ranked, c)
		} else {
			dr.Blocked = append(dr.Blocked, c)
		}
	}

	return dr, nil
}

// TrustPenalty returns the scheduling restriction for an agent at the given
// score. Returns true if the agent should be restricted, along with a
// human-readable reason.
func TrustPenalty(score int) (restricted bool, reason string) {
	tier := ClassifyTrust(score)
	switch tier {
	case TrustTierBlocked:
		return true, "trust score below minimum dispatch threshold — no unsupervised work"
	case TrustTierProbation:
		return true, "trust score in probation range — only XS tasks allowed"
	default:
		return false, ""
	}
}
