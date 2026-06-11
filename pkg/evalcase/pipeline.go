package evalcase

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// Pipeline ties classification, extraction, and storage together for a batch of
// completed traces. The zero value is not usable — build one with NewPipeline.
type Pipeline struct {
	// Classifier decides which traces are problematic.
	Classifier ClassifierConfig
	// Extract tunes excerpt extraction.
	Extract ExtractConfig
	// Store is where generated eval cases land.
	Store *FileStore
	// Now stamps GeneratedAt on each case. Defaults to time.Now; overridable for
	// deterministic tests.
	Now func() time.Time
}

// NewPipeline returns a Pipeline writing to store with default classifier and
// extraction settings and the wall clock.
func NewPipeline(store *FileStore) *Pipeline {
	return &Pipeline{
		Classifier: DefaultClassifierConfig(),
		Extract:    ExtractConfig{},
		Store:      store,
		Now:        time.Now,
	}
}

// Result summarises a pipeline run.
type Result struct {
	// Scanned is how many traces were classified.
	Scanned int
	// Problematic is how many traces the classifier flagged.
	Problematic int
	// Generated is how many new eval cases were written.
	Generated int
	// Skipped is how many problematic traces already had a stored case (so were
	// left untouched).
	Skipped int
	// Cases are the eval cases produced this run (both newly generated and the
	// ones that already existed), in input order.
	Cases []EvalCase
}

// Run classifies each trace, extracts an eval case for every problematic one,
// and stores it. Already-stored cases are counted as Skipped and not rewritten.
// Each newly generated case also bumps the agent.eval.cases_generated telemetry
// counter (a no-op until a meter provider is installed), closing the loop with
// the eval-as-span-attribute infra from internal/telemetry.
func (p *Pipeline) Run(ctx context.Context, traces []Trace) (Result, error) {
	if p.Store == nil {
		return Result{}, fmt.Errorf("evalcase: pipeline has no store")
	}
	now := p.Now
	if now == nil {
		now = time.Now
	}

	var res Result
	for _, t := range traces {
		res.Scanned++
		verdict := p.Classifier.Classify(t)
		if !verdict.Problematic {
			continue
		}
		res.Problematic++

		c := Extract(t, verdict, p.Extract, now())
		path, existed, err := p.Store.Save(c)
		if err != nil {
			return res, fmt.Errorf("store eval case for trace %s: %w", t.TraceID, err)
		}
		res.Cases = append(res.Cases, c)
		if existed {
			res.Skipped++
			continue
		}
		res.Generated++
		// Best-effort: record that a production trace became an eval case. The
		// error is only non-nil for an empty trace ID, which we tolerate (the
		// case was still written under the "unknown" ID).
		_ = telemetry.TraceToEvalCase(ctx, t.TraceID)
		_ = path
	}
	return res, nil
}
