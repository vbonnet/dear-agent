// Package openviking is the placeholder for the future OpenViking
// graph-database adapter (Phase 5.3). The interface lands now so
// downstream callers can compile against it; the real implementation
// ships when an enterprise customer asks. The body is deliberately a
// stub that errors on every call.
//
// To unblock that future work today, this package ships:
//
//   - The Config shape — connection string + auth.
//   - An Adapter type that satisfies pkg/source.Adapter by returning
//     ErrNotImplemented from every method.
//   - Tests that pin the stub's error contract so the eventual real
//     implementation can drop in without breaking callers (the
//     registry treats the stub's HealthCheck failure as a feature
//     flag — present but not yet usable).
package openviking

import (
	"context"
	"errors"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// Name is the backend identifier the MCP layer matches against
// FetchQuery.Filters.Backend.
const Name = "openviking"

// ErrNotImplemented is returned by every method on the stub adapter
// until the real implementation lands. Callers can errors.Is against
// this to detect the unbuilt-feature case explicitly.
var ErrNotImplemented = errors.New("source/openviking: graph backend not implemented (compile with -tags openviking and configure)")

// Config holds the configuration the eventual implementation will
// need: a connection URL (for example bolt://host:7687) plus auth
// credentials. Defined now so the rest of the codebase can wire
// configuration through without waiting on the engineering work.
type Config struct {
	// URL is the OpenViking connection URL. bolt://, neo4j://, or any
	// future graph protocol.
	URL string

	// User and Password are the basic-auth pair. Empty means
	// unauthenticated (only valid for embedded/local instances).
	User     string
	Password string

	// Database, when non-empty, scopes operations to a named
	// database. Useful in multi-tenant deployments.
	Database string
}

// Adapter is the placeholder. Every method returns ErrNotImplemented;
// HealthCheck does the same so callers can detect the feature gap.
//
// Once the real implementation lands, this struct holds the driver
// session and the methods route through pkg/source.Source ↔ graph
// node/edge translations. Until then, the type satisfies the interface
// contract so other code can compile and link.
type Adapter struct {
	cfg Config
}

// Open returns a stub Adapter for the given config. Validation of the
// config happens at the real implementation; for now we accept any
// shape so callers can wire configuration without a special branch.
func Open(cfg Config) (*Adapter, error) {
	return &Adapter{cfg: cfg}, nil
}

// Name returns the backend identifier.
func (a *Adapter) Name() string { return Name }

// HealthCheck always returns ErrNotImplemented. Callers using the
// stub should treat the registry's "openviking" entry as a feature
// flag — if HealthCheck errs, the rest of the workflow should fall
// back to a different backend or skip the feature entirely.
func (a *Adapter) HealthCheck(_ context.Context) error {
	return ErrNotImplemented
}

// Fetch returns ErrNotImplemented.
func (a *Adapter) Fetch(_ context.Context, _ source.FetchQuery) ([]source.Source, error) {
	return nil, ErrNotImplemented
}

// Add returns ErrNotImplemented.
func (a *Adapter) Add(_ context.Context, _ source.Source) (source.Ref, error) {
	return source.Ref{}, ErrNotImplemented
}

// Close releases the stub. No resources to free; included for symmetry.
func (a *Adapter) Close() error {
	return nil
}

// Config returns the underlying configuration. Useful for tests and
// for the registry to log what it would connect to once the real
// implementation lands.
func (a *Adapter) Config() Config { return a.cfg }
