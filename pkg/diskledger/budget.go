package diskledger

import (
	"path/filepath"
	"time"
)

// RootBudget is a byte budget for a long-lived shared directory.
//
// Budgets exist because the shared caches are not owned by any single run, so
// the per-run ledger cannot charge them to anyone. Nobody allocates the shared
// GOCACHE; every run adds to it and no run is responsible for it. A budget is
// the "keep-storage" pattern (docker builder prune --keep-storage, kubelet
// image GC high/low watermark): the root is allowed to be large, but not
// unbounded, and crossing the line is a reportable condition rather than
// something only visible once the volume is full.
type RootBudget struct {
	Name        string
	Path        string
	BudgetBytes int64
}

// RootVerdict is the measured state of one budgeted root.
type RootVerdict struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	AllocatedBytes int64  `json:"allocated_bytes"`
	BudgetBytes    int64  `json:"budget_bytes"`
	OverBudget     bool   `json:"over_budget"`
	Missing        bool   `json:"missing,omitempty"`
	Partial        bool   `json:"partial,omitempty"`
}

// OverBy returns how many bytes the root exceeds its budget by, or 0.
func (v RootVerdict) OverBy() int64 {
	if !v.OverBudget {
		return 0
	}
	return v.AllocatedBytes - v.BudgetBytes
}

// EvaluateRoots measures each budgeted root.
//
// A missing root is never over budget: on a fresh host most of these paths do
// not exist yet, and alarming on absence would train the operator to ignore
// the alarm.
func EvaluateRoots(budgets []RootBudget) []RootVerdict {
	out := make([]RootVerdict, 0, len(budgets))
	for _, b := range budgets {
		u, err := Measure(b.Path)
		if err != nil {
			u.Partial = true
		}
		v := RootVerdict{
			Name: b.Name, Path: b.Path,
			AllocatedBytes: u.AllocatedBytes, BudgetBytes: b.BudgetBytes,
			Missing: u.Missing, Partial: u.Partial,
		}
		v.OverBudget = !u.Missing && b.BudgetBytes > 0 && u.AllocatedBytes > b.BudgetBytes
		out = append(out, v)
	}
	return out
}

// ReclaimStatus classifies the health of the reclaim path.
type ReclaimStatus string

const (
	// StatusOK means no tracked root is over its budget.
	StatusOK ReclaimStatus = "ok"
	// StatusSilentGC means at least one root is over budget and NOTHING was
	// reclaimed in the window. This is the dead-man's-switch: a GC that
	// reports success while collecting nothing is indistinguishable from a
	// healthy idle host unless the over-budget condition is checked too.
	StatusSilentGC ReclaimStatus = "silent_gc"
	// StatusPressure means a root is over budget but the GC is demonstrably
	// reclaiming. The reaper works and is losing the race, which is a
	// different problem with a different fix.
	StatusPressure ReclaimStatus = "pressure"
)

// ReclaimHealth is the verdict on whether reclaim is actually happening.
type ReclaimHealth struct {
	Status         ReclaimStatus `json:"status"`
	OverBudget     []RootVerdict `json:"over_budget,omitempty"`
	BytesReclaimed int64         `json:"bytes_reclaimed"`
	Window         time.Duration `json:"window"`
	Detail         string        `json:"detail,omitempty"`
}

// Alarming reports whether this health state should raise an alarm.
func (h ReclaimHealth) Alarming() bool {
	return h.Status == StatusSilentGC || h.Status == StatusPressure
}

// CheckReclaimHealth decides whether the reclaim path is working.
//
// bytesReclaimed is what the GC actually returned over the window. The
// combination that matters is (over budget) AND (reclaimed nothing): either
// half alone is normal. An idle host reclaims nothing and is fine; a busy host
// can be over budget while the reaper chews through it.
func CheckReclaimHealth(verdicts []RootVerdict, bytesReclaimed int64, window time.Duration) ReclaimHealth {
	h := ReclaimHealth{Status: StatusOK, BytesReclaimed: bytesReclaimed, Window: window}
	for _, v := range verdicts {
		if v.OverBudget {
			h.OverBudget = append(h.OverBudget, v)
		}
	}
	if len(h.OverBudget) == 0 {
		return h
	}
	if bytesReclaimed <= 0 {
		h.Status = StatusSilentGC
		h.Detail = "tracked roots exceed budget and nothing was reclaimed: the collector is running but collecting nothing"
		return h
	}
	h.Status = StatusPressure
	h.Detail = "tracked roots exceed budget while the collector is reclaiming: growth is outpacing reclaim"
	return h
}

// Budget sizes. These are generous on purpose: the goal is to catch runaway
// growth, not to force a rebuild every day. A Go build cache that legitimately
// serves this repo sits in the low single-digit GiB; 10 GiB leaves ample slack
// while still catching the 44 GiB runaway that prompted this.
const (
	budgetGoCache    = 10 << 30
	budgetGoModCache = 8 << 30
	budgetLintCache  = 3 << 30
	budgetScratch    = 4 << 30
)

// DefaultRootBudgets returns the shared roots that leak on this host.
//
// Every path here was measured leaking and NONE of them were scanned by the
// pre-existing build-cache reaper, which only walked /tmp and TMPDIR. That gap
// is why the watchdog could report "0 reaped ... Status: OK" while the shared
// GOCACHE alone held 44 GiB.
func DefaultRootBudgets(home string) []RootBudget {
	return []RootBudget{
		// The macOS default GOCACHE. This is the one that hit 44 GiB.
		{Name: "gocache-darwin", Path: filepath.Join(home, "Library", "Caches", "go-build"), BudgetBytes: budgetGoCache},
		// What agm's tmux spawner actually exports to every agent session.
		{Name: "gocache-xdg", Path: filepath.Join(home, ".cache", "go-build"), BudgetBytes: budgetGoCache},
		{Name: "gomodcache", Path: filepath.Join(home, "go", "pkg", "mod"), BudgetBytes: budgetGoModCache},
		{Name: "golangci-lint", Path: filepath.Join(home, ".cache", "dear-agent", "golangci-lint"), BudgetBytes: budgetLintCache},
		{Name: "preflight-runs", Path: filepath.Join(home, ".cache", "dear-agent", "preflight-runs"), BudgetBytes: budgetScratch},
	}
}
