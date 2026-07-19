package backlog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RankWeights weights the three ranking axes. The zero value is treated as
// DefaultRankWeights, mirroring the configurable-weights pattern in
// aggregator.Scorer without importing it (ADR-022 §D3).
type RankWeights struct {
	Priority float64 `json:"priority"`
	Leverage float64 `json:"leverage"`
	Effort   float64 `json:"effort"`
}

// DefaultRankWeights favors priority, then unblocking leverage, then
// preferring small effort as a tiebreaker.
var DefaultRankWeights = RankWeights{Priority: 0.5, Leverage: 0.3, Effort: 0.2}

func (w RankWeights) orDefault() RankWeights {
	if w == (RankWeights{}) {
		return DefaultRankWeights
	}
	return w
}

// Suggestion is one ranked item plus the reasoning behind its rank.
type Suggestion struct {
	Item       Item     `json:"item"`
	Eligible   bool     `json:"eligible"`
	Score      float64  `json:"score"`
	Dependents int      `json:"dependents"`
	Reason     string   `json:"reason"`
	Blockers   []string `json:"blockers,omitempty"`
}

// Ranker scores backlog items by the VROOM Orchestrator dispatch rules
// (ADR-022): dependency+status eligibility, priority
// ordering, unblocking leverage, then effort.
type Ranker struct {
	Weights RankWeights
}

// Rank returns one Suggestion per input item. Eligible items (Pending with
// every dependency Done) come first, ordered by blended Score; the rest
// follow with their Blockers populated. Order is deterministic.
func (r Ranker) Rank(items []Item) []Suggestion {
	w := r.Weights.orDefault()
	byID := make(map[string]Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	maxPhase := maxPhaseOf(items)
	dependents := dependentCounts(items)
	maxDeps := maxIntValue(dependents)

	out := make([]Suggestion, 0, len(items))
	for _, it := range items {
		blockers := unmetDeps(it, byID, items)
		eligible := it.Status == StatusPending && len(blockers) == 0
		pScore := priorityScore(it, maxPhase)
		lScore := ratio(dependents[it.ID], maxDeps)
		eScore := effortScore(it.Effort)
		score := w.Priority*pScore + w.Leverage*lScore + w.Effort*eScore
		out = append(out, Suggestion{
			Item:       it,
			Eligible:   eligible,
			Score:      score,
			Dependents: dependents[it.ID],
			Reason: fmt.Sprintf("priority=%s, unblocks %d, effort=%s",
				it.Priority, dependents[it.ID], it.Effort),
			Blockers: blockers,
		})
	}
	sortSuggestions(out)
	return out
}

// sortSuggestions orders eligible-first, then Score desc, then a
// deterministic phase/ID tiebreak.
func sortSuggestions(s []Suggestion) {
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if a.Eligible != b.Eligible {
			return a.Eligible
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Item.Phase != b.Item.Phase {
			return phaseLess(a.Item.Phase, b.Item.Phase)
		}
		return compareID(a.Item.ID, b.Item.ID) < 0
	})
}

// unmetDeps returns the dependency references that are not satisfied. A
// "N.*" wildcard requires every phase-N item to be Done. An unknown
// dependency ID is unmet (fail safe).
func unmetDeps(it Item, byID map[string]Item, all []Item) []string {
	var unmet []string
	for _, dep := range it.Deps {
		if IsPhaseWildcard(dep) {
			if u := unmetWildcard(dep, all); u != "" {
				unmet = append(unmet, u)
			}
			continue
		}
		d, ok := byID[dep]
		if !ok {
			unmet = append(unmet, dep+" (unknown)")
			continue
		}
		if d.Status != StatusDone {
			unmet = append(unmet, fmt.Sprintf("%s (%s)", dep, d.Status))
		}
	}
	return unmet
}

// unmetWildcard reports a phase wildcard as unmet if any item in that
// phase is not Done. It returns "" when the wildcard is satisfied.
func unmetWildcard(dep string, all []Item) string {
	prefix := strings.TrimSuffix(dep, "*") // "0.*" -> "0."
	pending := 0
	matched := 0
	for _, x := range all {
		if !strings.HasPrefix(x.ID, prefix) {
			continue
		}
		matched++
		if x.Status != StatusDone {
			pending++
		}
	}
	if matched == 0 || pending == 0 {
		return ""
	}
	return fmt.Sprintf("%s (%d not done)", dep, pending)
}

// dependentCounts maps each item ID to the number of items that directly
// depend on it (a "N.*" dependent counts for every phase-N item).
func dependentCounts(items []Item) map[string]int {
	counts := make(map[string]int, len(items))
	for _, it := range items {
		for _, dep := range it.Deps {
			if IsPhaseWildcard(dep) {
				prefix := strings.TrimSuffix(dep, "*")
				for _, x := range items {
					if strings.HasPrefix(x.ID, prefix) {
						counts[x.ID]++
					}
				}
				continue
			}
			counts[dep]++
		}
	}
	return counts
}

// priorityScore is the normalized 0..1 priority. An explicit Priority
// dominates; absent it, earlier phases score higher because phases are
// sequential. Cross-phase (phase<0) defaults low ("triage as they come up").
func priorityScore(it Item, maxPhase int) float64 {
	switch it.Priority {
	case PriorityHigh:
		return 1.0
	case PriorityMed:
		return 0.66
	case PriorityLow:
		return 0.33
	case PriorityUnset:
		return phaseDerivedPriority(it.Phase, maxPhase)
	}
	return phaseDerivedPriority(it.Phase, maxPhase)
}

// phaseDerivedPriority scores an item with no explicit Priority by phase
// order: earlier phases rank higher; cross-phase (phase<0) defaults low.
func phaseDerivedPriority(phase, maxPhase int) float64 {
	if phase < 0 {
		return 0.1
	}
	if maxPhase <= 0 {
		return 0.5
	}
	return 1.0 - float64(phase)/float64(maxPhase+1)
}

// effortScore rewards smaller items: S=1.0, M=0.5, L=0.0. Unknown is
// treated as Medium (ADR-022 §D1).
func effortScore(e Effort) float64 {
	switch e {
	case EffortSmall:
		return 1.0
	case EffortMedium, EffortUnknown:
		return 0.5
	case EffortLarge:
		return 0.0
	}
	return 0.5
}

func maxPhaseOf(items []Item) int {
	m := 0
	for _, it := range items {
		if it.Phase > m {
			m = it.Phase
		}
	}
	return m
}

func maxIntValue(m map[string]int) int {
	hi := 0
	for _, v := range m {
		if v > hi {
			hi = v
		}
	}
	return hi
}

func ratio(n, denom int) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(n) / float64(denom)
}

// phaseLess orders phases ascending but pushes cross-phase (-1) last.
func phaseLess(a, b int) bool {
	if a < 0 {
		return false
	}
	if b < 0 {
		return true
	}
	return a < b
}

// compareID compares dotted/dashed IDs numerically where the segments are
// numbers ("0.2" < "0.10"), lexicographically otherwise. Returns -1, 0, 1.
func compareID(a, b string) int {
	as := strings.FieldsFunc(a, func(r rune) bool { return r == '.' || r == '-' })
	bs := strings.FieldsFunc(b, func(r rune) bool { return r == '.' || r == '-' })
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		if aerr == nil && berr == nil {
			if an != bn {
				return sign(an - bn)
			}
			continue
		}
		if as[i] != bs[i] {
			return strings.Compare(as[i], bs[i])
		}
	}
	return sign(len(as) - len(bs))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
