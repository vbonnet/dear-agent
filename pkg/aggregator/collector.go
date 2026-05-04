package aggregator

import (
	"context"
	"errors"
	"fmt"
)

// Collector produces signals of a single Kind. Implementations are
// independent of the store — the Aggregator owns the persistence
// step. See ADR-015 §D3.
type Collector interface {
	// Name returns a stable dot-separated identifier
	// (e.g. "dear-agent.git"). Used as the key in Report.Collected.
	Name() string

	// Kind returns the signal kind this collector emits. The
	// aggregator validates that every Signal returned by Collect
	// matches this Kind before persisting.
	Kind() Kind

	// Collect runs the collector once and returns the produced
	// signals. A returned error does not fail the whole Aggregator
	// run; it is recorded in the Report.
	Collect(ctx context.Context) ([]Signal, error)
}

// ErrToolMissing is returned by collectors that depend on an external
// command (golangci-lint, govulncheck, …) when the tool is not on
// $PATH. The aggregator surfaces it in the per-run report so operators
// see "this collector is misconfigured" rather than "everything works
// silently".
type ErrToolMissing struct {
	Collector string
	Tool      string
}

// Error implements the error interface.
func (e *ErrToolMissing) Error() string {
	return fmt.Sprintf("aggregator: collector %s requires %s on $PATH",
		e.Collector, e.Tool)
}

// IsToolMissing reports whether err (or anything wrapped inside) is
// an *ErrToolMissing. Convenience for callers that want to skip the
// "tool not installed" case without parsing strings.
func IsToolMissing(err error) bool {
	var target *ErrToolMissing
	return errors.As(err, &target)
}
