// Package docaudit validates freshness metadata on declared, Git-tracked
// living documents and enforces a shrink-only debt baseline.
package docaudit

import (
	"context"
	"time"
)

// FindingKind is a stable baseline identity for a freshness defect.
type FindingKind string

const (
	// MissingMarker means no audit-marker prefix exists in the document.
	MissingMarker FindingKind = "missing-marker"
	// NeedsAudit means the document explicitly carries the debt placeholder.
	NeedsAudit FindingKind = "needs-audit"
	// MalformedMarker means the marker is present but not canonical.
	MalformedMarker FindingKind = "malformed-marker"
	// DuplicateMarker means the document contains more than one marker.
	DuplicateMarker FindingKind = "duplicate-marker"
	// InvalidDate means the canonical-looking date is not a calendar date.
	InvalidDate FindingKind = "invalid-date"
	// FutureDate means the audit date is after the configured as-of date.
	FutureDate FindingKind = "future-date"
	// StaleDate means the audit date exceeds its surface maximum age.
	StaleDate FindingKind = "stale-date"
)

var knownFindingKinds = map[FindingKind]bool{
	MissingMarker:   true,
	NeedsAudit:      true,
	MalformedMarker: true,
	DuplicateMarker: true,
	InvalidDate:     true,
	FutureDate:      true,
	StaleDate:       true,
}

// Finding describes one current document defect.
type Finding struct {
	Kind    FindingKind
	Path    string
	Surface string
}

// ID returns the stable, baseline-compatible identity.
func (f Finding) ID() string { return string(f.Kind) + "\t" + f.Path }

// BaselineEntry is one grandfathered finding identity.
type BaselineEntry struct {
	Kind FindingKind
	Path string
}

// ID returns the serialized baseline entry.
func (e BaselineEntry) ID() string { return string(e.Kind) + "\t" + e.Path }

// Report separates observed debt from integrity failures that block the gate.
type Report struct {
	Documents     int
	Findings      []Finding
	NewFindings   []Finding
	StaleBaseline []BaselineEntry
	AddedBaseline []BaselineEntry
}

// Blocking reports whether the current tree broke the baseline ratchet.
func (r Report) Blocking() bool {
	return len(r.NewFindings) > 0 || len(r.StaleBaseline) > 0 || len(r.AddedBaseline) > 0
}

// Options controls deterministic date and historical baseline comparison.
type Options struct {
	AsOf        time.Time
	BaselineRef string
}

// CheckRepository validates every declared, tracked living document.
func CheckRepository(ctx context.Context, root string, opts Options) (Report, error) {
	return checkRepository(ctx, root, opts)
}
