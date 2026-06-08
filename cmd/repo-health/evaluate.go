package main

import "fmt"

// Evaluation thresholds. These are deliberately separated from the
// structural thresholds in Options: Options decides what counts as a "large
// file" (a fixed style rule), while these decide when an aggregate becomes a
// degraded or critical *verdict*. They are tuned so a large, healthy repo is
// not perpetually red: structural facts (file/function sizes, depth, spec
// coverage) are surfaced as info and only escalate at clearly-bad levels;
// the verdict is driven mainly by actionable failures (lint, cycles, drift).
const (
	lintCritical = 100 // findings at/above this are critical, else warn if >0

	complexityWarn     = 10.0
	complexityCritical = 15.0

	coverageWarn    = 40.0 // overall % below this warns (only when measured)
	testRatioWarn   = 0.20 // test:source file ratio below this warns
	depDepthWarn    = 20   // dependency chain longer than this warns

	largeFilesWarn = 40  // more flagged files than this warns (else info)
	longFuncsWarn  = 150 // more flagged functions than this warns (else info)

	worktreeWarn     = 10
	worktreeCritical = 50 // 78 stranded worktrees was the documented failure
	staleBranchWarn  = 20
)

// verdict reduces the Issues to an overall Status: critical if any critical
// issue, degraded if any warn, healthy otherwise.
func verdict(issues []Issue) Status {
	worst := StatusHealthy
	for _, is := range issues {
		switch is.Severity {
		case SeverityCritical:
			return StatusCritical
		case SeverityWarn:
			worst = StatusDegraded
		}
	}
	return worst
}

// evaluate turns a populated Report into the ordered list of Issues that
// drives the verdict. Unavailable metrics contribute an info note (so the
// reader sees what was not measured) but never a failure.
func evaluate(r *Report, opts Options) []Issue {
	var out []Issue
	add := func(check string, sev Severity, format string, args ...any) {
		out = append(out, Issue{Check: check, Severity: sev, Detail: fmt.Sprintf(format, args...)})
	}
	unavailable := func(check string, m Metric) bool {
		if m.Available {
			return false
		}
		if m.Note != "" {
			add(check, SeverityInfo, "not measured: %s", m.Note)
		}
		return true
	}

	cq := r.CodeQuality
	if !unavailable("lint", cq.Lint) {
		switch {
		case cq.LintFindings >= lintCritical:
			add("lint", SeverityCritical, "%d golangci-lint findings (>= %d)", cq.LintFindings, lintCritical)
		case cq.LintFindings > 0:
			add("lint", SeverityWarn, "%d golangci-lint findings (should be 0)", cq.LintFindings)
		}
	}
	if !unavailable("coverage", cq.Coverage) && cq.CoveragePct < coverageWarn {
		add("coverage", SeverityWarn, "overall coverage %.1f%% below %.0f%%", cq.CoveragePct, coverageWarn)
	}
	if cq.SourceFiles > 0 && cq.TestSourceRatio < testRatioWarn {
		add("test-ratio", SeverityWarn, "test:source file ratio %.2f below %.2f", cq.TestSourceRatio, testRatioWarn)
	}
	if cq.FunctionsAnalyzed > 0 {
		switch {
		case cq.AvgComplexity >= complexityCritical:
			add("complexity", SeverityCritical, "avg cyclomatic complexity %.1f (>= %.0f)", cq.AvgComplexity, complexityCritical)
		case cq.AvgComplexity >= complexityWarn:
			add("complexity", SeverityWarn, "avg cyclomatic complexity %.1f (>= %.0f)", cq.AvgComplexity, complexityWarn)
		}
	}

	arch := r.Architecture
	if !unavailable("dep-graph", arch.DepGraph) {
		if len(arch.CircularDeps) > 0 {
			add("circular-deps", SeverityCritical, "%d import cycle(s) detected", len(arch.CircularDeps))
		}
		sev := SeverityInfo
		if arch.MaxDepDepth > depDepthWarn {
			sev = SeverityWarn
		}
		add("dep-depth", sev, "max dependency chain depth %d", arch.MaxDepDepth)
	}
	if n := len(arch.LargeFiles); n > 0 {
		sev := SeverityInfo
		if n > largeFilesWarn {
			sev = SeverityWarn
		}
		add("large-files", sev, "%d file(s) over %d lines", n, opts.maxFileLines)
	}
	if n := len(arch.LongFunctions); n > 0 {
		sev := SeverityInfo
		if n > longFuncsWarn {
			sev = SeverityWarn
		}
		add("long-functions", sev, "%d function(s) over %d lines", n, opts.maxFuncLines)
	}

	ah := r.AgentHealth
	if !unavailable("worktrees", ah.Worktrees) {
		switch {
		case ah.WorktreeCount > worktreeCritical:
			add("worktrees", SeverityCritical, "%d linked worktrees (>= %d — reap stranded ones)", ah.WorktreeCount, worktreeCritical)
		case ah.WorktreeCount > worktreeWarn:
			add("worktrees", SeverityWarn, "%d linked worktrees (> %d)", ah.WorktreeCount, worktreeWarn)
		}
	}
	if !unavailable("stale-branches", ah.StaleBranches) && ah.StaleBranchCount > staleBranchWarn {
		add("stale-branches", SeverityWarn, "%d branches older than %d days", ah.StaleBranchCount, opts.staleDays)
	}
	if !unavailable("bdd", ah.BDD) {
		add("bdd", SeverityInfo, "%d/%d features tagged @implemented", ah.FeaturesImpl, ah.FeaturesTotal)
	}
	if !unavailable("ears", ah.EARS) {
		add("ears", SeverityInfo, "%d/%d packages have a SPEC.md", ah.PackagesWithSpec, ah.PackagesTotal)
	}

	dr := r.Drift
	if !unavailable("chezmoi-drift", dr.Chezmoi) && len(dr.ChezmoiDrifted) > 0 {
		add("chezmoi-drift", SeverityWarn, "%d dotfile(s) drifted from chezmoi source", len(dr.ChezmoiDrifted))
	}
	if !unavailable("hook-drift", dr.Hooks) && len(dr.HookDrifted) > 0 {
		add("hook-drift", SeverityWarn, "%d git hook(s) drifted or undeployed", len(dr.HookDrifted))
	}
	// Doc-pairing: orphaned rationale (.why.md without content) warns; a
	// missing .why.md beside a .ai.md is the common case and stays info.
	if !unavailable("doc-pairing", dr.DocPairing) {
		orphaned := 0
		for _, u := range dr.UnpairedDocs {
			if !hasSuffixInfo(u) {
				orphaned++
			}
		}
		if orphaned > 0 {
			add("doc-pairing", SeverityWarn, "%d orphaned rationale doc(s) (.why.md without content)", orphaned)
		}
		if info := len(dr.UnpairedDocs) - orphaned; info > 0 {
			add("doc-pairing", SeverityInfo, "%d .ai.md without a .why.md rationale", info)
		}
	}

	return out
}

// hasSuffixInfo reports whether an unpaired-doc entry is the info-only
// variant (tagged "[info]" by docPairingDrift).
func hasSuffixInfo(entry string) bool {
	const tag = "[info]"
	return len(entry) >= len(tag) && entry[len(entry)-len(tag):] == tag
}
