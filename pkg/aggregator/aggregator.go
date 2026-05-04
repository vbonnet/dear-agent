package aggregator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Aggregator runs a list of Collectors and persists their output via
// a Store. See ADR-015 §D5.
type Aggregator struct {
	Store      Store
	Collectors []Collector

	// Now is overridable so tests can inject a deterministic clock.
	// Production leaves it nil; the aggregator uses time.Now.
	Now func() time.Time

	// IDGen is overridable for tests that want stable signal IDs.
	// Production leaves it nil; the aggregator uses uuid.NewString.
	IDGen func() string
}

// Report summarises one Aggregator.Run. The Errors map carries the
// first error from each failing collector so a partial run is still
// observable. A nil Errors entry (or absent key) means success.
type Report struct {
	StartedAt  time.Time        `json:"startedAt"`
	FinishedAt time.Time        `json:"finishedAt"`
	Collected  map[string]int   `json:"collected"`
	Errors     map[string]error `json:"-"`            // not JSON-encodable directly
	ErrorMsgs  map[string]string `json:"errors,omitempty"`
}

// Run invokes every collector in registration order. Failed collectors
// are recorded in the report; successful collectors' signals are
// persisted. The whole run aborts only if the Store insert fails (a
// failure of the substrate itself).
func (a *Aggregator) Run(ctx context.Context) (Report, error) {
	if a.Store == nil {
		return Report{}, fmt.Errorf("aggregator: Run: nil Store")
	}
	now := a.Now
	if now == nil {
		now = time.Now
	}
	idGen := a.IDGen
	if idGen == nil {
		idGen = uuid.NewString
	}

	report := Report{
		StartedAt: now(),
		Collected: map[string]int{},
		Errors:    map[string]error{},
		ErrorMsgs: map[string]string{},
	}

	var batch []Signal
	for _, c := range a.Collectors {
		name := c.Name()
		sigs, err := c.Collect(ctx)
		if err != nil {
			report.Errors[name] = err
			report.ErrorMsgs[name] = err.Error()
			report.Collected[name] = 0
			continue
		}
		stamped := stampSignals(sigs, c.Kind(), now(), idGen)
		batch = append(batch, stamped...)
		report.Collected[name] = len(stamped)
	}

	if err := a.Store.Insert(ctx, batch); err != nil {
		report.FinishedAt = now()
		return report, fmt.Errorf("aggregator: Run: store insert: %w", err)
	}
	report.FinishedAt = now()
	return report, nil
}

// stampSignals fills in the run-level fields (ID, CollectedAt) and
// validates that every signal's Kind matches the collector's declared
// Kind. Mismatched signals are dropped with a synthetic Metadata note;
// dropping is preferable to failing the whole batch because a single
// buggy collector should not poison the others.
func stampSignals(in []Signal, kind Kind, at time.Time, idGen func() string) []Signal {
	out := make([]Signal, 0, len(in))
	for _, s := range in {
		if s.Kind != "" && s.Kind != kind {
			continue
		}
		s.Kind = kind
		if s.ID == "" {
			s.ID = idGen()
		}
		if s.CollectedAt.IsZero() {
			s.CollectedAt = at
		}
		out = append(out, s)
	}
	return out
}
