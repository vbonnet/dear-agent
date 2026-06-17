package orphan

import (
	"context"
	"log/slog"

	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

// Reclaimer adapts the orphan.Reap function to the supervisor.ResourceReclaimer
// interface. When called, it finds orphaned processes (PPID==1) matching the
// configured targets and kills them.
type Reclaimer struct {
	lister  ProcLister
	killer  ProcessKiller
	targets []string
	logger  *slog.Logger
}

// ReclaimerOption configures a Reclaimer.
type ReclaimerOption func(*Reclaimer)

// WithReclaimerTargets overrides the default process targets (gopls, agm-mcp-server).
func WithReclaimerTargets(targets []string) ReclaimerOption {
	return func(r *Reclaimer) { r.targets = targets }
}

// WithReclaimerLogger sets the logger for reap operations.
func WithReclaimerLogger(l *slog.Logger) ReclaimerOption {
	return func(r *Reclaimer) { r.logger = l }
}

// NewReclaimer constructs a Reclaimer that kills orphaned processes.
func NewReclaimer(lister ProcLister, killer ProcessKiller, opts ...ReclaimerOption) *Reclaimer {
	r := &Reclaimer{
		lister:  lister,
		killer:  killer,
		targets: DefaultTargets,
		logger:  slog.Default(),
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Reclaim implements supervisor.ResourceReclaimer by finding and killing
// orphaned target processes.
func (r *Reclaimer) Reclaim(_ context.Context) (supervisor.ReclaimResult, error) {
	res, err := Reap(r.lister, r.killer, r.targets, false, r.logger)
	if err != nil {
		return supervisor.ReclaimResult{}, err
	}
	return supervisor.ReclaimResult{
		OrphansFound:  len(res.Orphans),
		OrphansKilled: len(res.Killed),
		OrphansFailed: len(res.Failed),
	}, nil
}
