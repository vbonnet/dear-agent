package backlog

import (
	"context"
	"fmt"
)

// Context describes the situation a suggestion is made in — the inputs the
// the ranking policy weighs before a suggestion is returned (ADR-022).
type Context struct {
	// Phase restricts suggestions to one phase. -1 means any phase.
	Phase int
	// Capacity caps how many items to suggest (the Orchestrator's
	// max_parallel). <= 0 means DefaultCapacity.
	Capacity int
	// MaxEffort drops eligible items larger than this. EffortUnknown (0)
	// means no cap.
	MaxEffort Effort
}

// DefaultCapacity is the number of suggestions returned when
// Context.Capacity is unset.
const DefaultCapacity = 3

// Result is the outcome of a suggestion run.
type Result struct {
	// Suggested is the ranked shortlist to pick up next.
	Suggested []Suggestion `json:"suggested"`
	// Blocked is every Pending item that cannot start yet, with the
	// dependencies that block it, so the operator sees why work is stuck.
	Blocked []Suggestion `json:"blocked,omitempty"`
	// Total is how many items the source yielded (post phase filter).
	Total int `json:"total"`
}

// Suggester ties a Source to a Ranker and applies the current Context.
type Suggester struct {
	Source Source
	Ranker Ranker
}

// NewSuggester builds a Suggester with the default Ranker weights.
func NewSuggester(src Source) *Suggester {
	return &Suggester{Source: src, Ranker: Ranker{Weights: DefaultRankWeights}}
}

// Suggest loads the source, ranks every item, and returns the top-Capacity
// eligible items plus an explanation of what is blocked.
func (s *Suggester) Suggest(ctx context.Context, c Context) (Result, error) {
	items, err := s.Source.Items(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load items: %w", err)
	}
	if c.Phase >= 0 {
		items = filterPhase(items, c.Phase)
	}
	ranked := s.Ranker.Rank(items)

	capacity := c.Capacity
	if capacity <= 0 {
		capacity = DefaultCapacity
	}

	res := Result{Total: len(items)}
	for _, sg := range ranked {
		switch {
		case sg.Eligible:
			if c.MaxEffort != EffortUnknown && sg.Item.Effort > c.MaxEffort {
				continue
			}
			if len(res.Suggested) < capacity {
				res.Suggested = append(res.Suggested, sg)
			}
		case sg.Item.Status == StatusPending:
			// Pending but not eligible == blocked by a dependency.
			res.Blocked = append(res.Blocked, sg)
		}
	}
	return res, nil
}

// filterPhase keeps only items in the requested phase.
func filterPhase(items []Item, phase int) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.Phase == phase {
			out = append(out, it)
		}
	}
	return out
}
