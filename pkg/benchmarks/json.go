package benchmarks

import (
	"encoding/json"
	"math"
	"time"
)

// summaryJSON is the on-the-wire shape of Summary. CostPerSolved is a
// pointer so the +Inf sentinel encodes as null instead of breaking
// json.Marshal.
type summaryJSON struct {
	Total          int           `json:"total"`
	Solved         int           `json:"solved"`
	Failed         int           `json:"failed"`
	Errored        int           `json:"errored"`
	SolveRate      float64       `json:"solve_rate"`
	TotalCostUSD   float64       `json:"total_cost_usd"`
	CostPerSolved  *float64      `json:"cost_per_solved_usd"`
	TotalTokensIn  int           `json:"total_tokens_in"`
	TotalTokensOut int           `json:"total_tokens_out"`
	AvgDuration    time.Duration `json:"avg_duration_ns"`
}

// MarshalJSON serializes Summary, encoding non-finite CostPerSolved as null.
func (s Summary) MarshalJSON() ([]byte, error) {
	view := summaryJSON{
		Total:          s.Total,
		Solved:         s.Solved,
		Failed:         s.Failed,
		Errored:        s.Errored,
		SolveRate:      s.SolveRate,
		TotalCostUSD:   s.TotalCostUSD,
		TotalTokensIn:  s.TotalTokensIn,
		TotalTokensOut: s.TotalTokensOut,
		AvgDuration:    s.AvgDuration,
	}
	if !math.IsInf(s.CostPerSolved, 0) && !math.IsNaN(s.CostPerSolved) {
		v := s.CostPerSolved
		view.CostPerSolved = &v
	}
	return json.Marshal(view)
}

// UnmarshalJSON is the inverse of MarshalJSON: a null cost_per_solved_usd
// becomes +Inf so the in-memory invariant ("no solved => Inf") is preserved
// through a write-then-read round trip.
func (s *Summary) UnmarshalJSON(data []byte) error {
	var view summaryJSON
	if err := json.Unmarshal(data, &view); err != nil {
		return err
	}
	s.Total = view.Total
	s.Solved = view.Solved
	s.Failed = view.Failed
	s.Errored = view.Errored
	s.SolveRate = view.SolveRate
	s.TotalCostUSD = view.TotalCostUSD
	s.TotalTokensIn = view.TotalTokensIn
	s.TotalTokensOut = view.TotalTokensOut
	s.AvgDuration = view.AvgDuration
	if view.CostPerSolved == nil {
		s.CostPerSolved = math.Inf(1)
	} else {
		s.CostPerSolved = *view.CostPerSolved
	}
	return nil
}
