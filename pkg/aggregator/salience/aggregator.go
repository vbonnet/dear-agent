package salience

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Outcome is the result of Aggregator.Ingest. Notify and Suppressed are
// not strictly redundant: Suppressed=true tags signals that were dropped
// for an explicit reason (Reason is set), while Notify=false with
// Suppressed=false only happens for invalid input.
type Outcome struct {
	Signal     Signal `json:"signal"`
	Notify     bool   `json:"notify"`
	Suppressed bool   `json:"suppressed"`
	Reason     string `json:"reason,omitempty"`
}

// Reasons returned in Outcome.Reason. Stable strings so downstream
// dashboards can group on them.
const (
	ReasonBudgetExhausted = "budget_exhausted"
	ReasonNoiseDropped    = "noise_dropped"
	ReasonInvalidKind     = "invalid_kind"
)

// Aggregator routes drift signals through a Classifier and a
// NotificationBudget. The zero value is not useful — use New.
type Aggregator struct {
	Classifier Classifier
	Budget     *NotificationBudget

	// DropNoise, when true, suppresses TierNoise signals before they
	// reach the budget. This keeps cosmetic/formatting events from
	// even touching the slot counter. Defaulted to true by New.
	DropNoise bool

	// Now is overridable for tests; nil falls back to time.Now and is
	// only used to stamp Signal.ObservedAt on signals that arrive
	// without one.
	Now func() time.Time
}

// New returns an Aggregator with default policy: DefaultClassifier, no
// budget (every signal allowed), and DropNoise enabled.
func New() *Aggregator {
	return &Aggregator{
		Classifier: DefaultClassifier(),
		DropNoise:  true,
	}
}

// Ingest classifies the signal (filling Salience when zero), then asks
// the budget whether to notify. The returned Outcome carries the
// (possibly mutated) Signal so callers can log a single record.
func (a *Aggregator) Ingest(s Signal) Outcome {
	if err := s.Validate(); err != nil {
		return Outcome{
			Signal:     s,
			Notify:     false,
			Suppressed: true,
			Reason:     ReasonInvalidKind,
		}
	}
	if s.ObservedAt.IsZero() {
		s.ObservedAt = a.now()
	}
	if s.Salience == TierNoise {
		s.Salience = a.classifier().Classify(s.Kind)
	}

	if a.DropNoise && s.Salience == TierNoise {
		return Outcome{
			Signal:     s,
			Notify:     false,
			Suppressed: true,
			Reason:     ReasonNoiseDropped,
		}
	}

	if a.Budget != nil && !a.Budget.Allow(s.Salience) {
		return Outcome{
			Signal:     s,
			Notify:     false,
			Suppressed: true,
			Reason:     ReasonBudgetExhausted,
		}
	}

	return Outcome{Signal: s, Notify: true}
}

// LoadJSONL reads one Signal per line from r, ingests each, and returns
// the per-line Outcomes in order. Blank lines are skipped. A malformed
// line is reported as an error and stops the load: partial results are
// returned alongside the error so callers can decide whether to keep
// what was processed.
func (a *Aggregator) LoadJSONL(r io.Reader) ([]Outcome, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var out []Outcome
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var s Signal
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return out, fmt.Errorf("salience: load JSONL: line %d: %w", line, err)
		}
		out = append(out, a.Ingest(s))
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("salience: load JSONL: scan: %w", err)
	}
	return out, nil
}

// LoadJSONLString is a convenience for tests and one-off CLI input.
func (a *Aggregator) LoadJSONLString(s string) ([]Outcome, error) {
	return a.LoadJSONL(strings.NewReader(s))
}

// Summary aggregates a slice of Outcomes for reporting.
type Summary struct {
	Total       int            `json:"total"`
	Notified    int            `json:"notified"`
	Suppressed  int            `json:"suppressed"`
	ByTier      map[Tier]int   `json:"-"`
	ByKind      map[Kind]int   `json:"-"`
	ByReason    map[string]int `json:"-"`
	NotifyRatio float64        `json:"notifyRatio"`
}

// Summarize folds outcomes into a Summary for reporting.
func Summarize(outcomes []Outcome) Summary {
	sum := Summary{
		Total:    len(outcomes),
		ByTier:   map[Tier]int{},
		ByKind:   map[Kind]int{},
		ByReason: map[string]int{},
	}
	for _, o := range outcomes {
		sum.ByTier[o.Signal.Salience]++
		sum.ByKind[o.Signal.Kind]++
		switch {
		case o.Notify:
			sum.Notified++
		case o.Suppressed:
			sum.Suppressed++
			sum.ByReason[o.Reason]++
		}
	}
	if sum.Total > 0 {
		sum.NotifyRatio = float64(sum.Notified) / float64(sum.Total)
	}
	return sum
}

// ErrEmptyInput is returned by helpers that expect at least one signal
// but receive nothing.
var ErrEmptyInput = errors.New("salience: no signals in input")

func (a *Aggregator) classifier() Classifier {
	if a.Classifier != nil {
		return a.Classifier
	}
	return DefaultClassifier()
}

func (a *Aggregator) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}
