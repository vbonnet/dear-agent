package main

import "time"

// Severity ranks a single health Issue. The most severe Issue across the
// whole Report determines the process exit code (see Status / ExitCode).
type Severity string

const (
	// SeverityInfo is an observation worth surfacing but never a failure.
	SeverityInfo Severity = "info"
	// SeverityWarn maps to a degraded repo (exit 1).
	SeverityWarn Severity = "warn"
	// SeverityCritical maps to a critical repo (exit 2).
	SeverityCritical Severity = "critical"
)

// Status is the overall verdict for the repo, derived from the Issues.
type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
	StatusCritical Status = "critical"
)

// ExitCode is the process exit code the Status maps to. The contract is
// fixed by the task: 0 healthy, 1 degraded, 2 critical. CI and humans both
// rely on it, so it lives next to the Status it derives from.
func (s Status) ExitCode() int {
	switch s {
	case StatusCritical:
		return 2
	case StatusDegraded:
		return 1
	case StatusHealthy:
		return 0
	}
	return 0
}

// Issue is one actionable finding. Check is the metric family it came from
// (e.g. "lint", "large-files"); Detail is human-readable; Severity drives
// the verdict.
type Issue struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Detail   string   `json:"detail"`
}

// Metric is a single measured value plus whether it could be measured at
// all. Unavailable metrics (missing tool, command failure) never fail the
// verdict — they degrade to an Info note so a missing golangci-lint on a
// laptop does not masquerade as a critical repo. The Note explains *why* a
// metric is unavailable, so the blocked reader can fix the cause.
type Metric struct {
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
}

// CodeQuality groups the code-quality metrics.
type CodeQuality struct {
	Lint              Metric  `json:"lint"`
	LintFindings      int     `json:"lint_findings"`
	Coverage          Metric  `json:"coverage"`
	CoveragePct       float64 `json:"coverage_pct"`
	PackageCoverage   []PkgCoverage `json:"package_coverage,omitempty"`
	TestFiles         int     `json:"test_files"`
	SourceFiles       int     `json:"source_files"`
	TestSourceRatio   float64 `json:"test_source_ratio"`
	AvgComplexity     float64 `json:"avg_cyclomatic_complexity"`
	FunctionsAnalyzed int     `json:"functions_analyzed"`
}

// PkgCoverage is per-package statement coverage.
type PkgCoverage struct {
	Package string  `json:"package"`
	Percent float64 `json:"percent"`
}

// Architecture groups the architecture-health metrics.
type Architecture struct {
	DepGraph         Metric   `json:"dep_graph"`
	MaxDepDepth      int      `json:"max_dependency_depth"`
	CircularDeps     [][]string `json:"circular_dependencies"`
	LargeFiles       []SizedItem `json:"large_files"`
	LongFunctions    []SizedItem `json:"long_functions"`
}

// SizedItem is a file or function flagged for exceeding a size threshold.
type SizedItem struct {
	Name  string `json:"name"`  // file path, or "path:func"
	Lines int    `json:"lines"`
}

// AgentHealth groups the agent-specific health metrics.
type AgentHealth struct {
	Worktrees      Metric `json:"worktrees"`
	WorktreeCount  int    `json:"worktree_count"`
	StaleBranches  Metric `json:"stale_branches"`
	StaleBranchCount int  `json:"stale_branch_count"`
	BDD            Metric `json:"bdd"`
	FeaturesTotal  int    `json:"features_total"`
	FeaturesImpl   int    `json:"features_implemented"`
	EARS           Metric `json:"ears"`
	PackagesTotal  int    `json:"packages_total"`
	PackagesWithSpec int  `json:"packages_with_spec"`
}

// Drift groups the drift-detection metrics.
type Drift struct {
	Chezmoi        Metric   `json:"chezmoi"`
	ChezmoiDrifted []string `json:"chezmoi_drifted_files,omitempty"`
	Hooks          Metric   `json:"hooks"`
	HookDrifted    []string `json:"hook_drifted,omitempty"`
	DocPairing     Metric   `json:"doc_pairing"`
	UnpairedDocs   []string `json:"unpaired_docs,omitempty"`
}

// Report is the full health snapshot serialised to JSON.
type Report struct {
	GeneratedAt  time.Time    `json:"generated_at"`
	Repo         string       `json:"repo"`
	Commit       string       `json:"commit"`
	Status       Status       `json:"status"`
	CodeQuality  CodeQuality  `json:"code_quality"`
	Architecture Architecture `json:"architecture"`
	AgentHealth  AgentHealth  `json:"agent_health"`
	Drift        Drift        `json:"drift"`
	Issues       []Issue      `json:"issues"`
}
