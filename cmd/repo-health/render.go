package main

import (
	"fmt"
	"sort"
	"strings"
)

// statusEmoji gives a quick visual marker for the markdown header.
func statusEmoji(s Status) string {
	switch s {
	case StatusCritical:
		return "🔴"
	case StatusDegraded:
		return "🟡"
	case StatusHealthy:
		return "🟢"
	}
	return "🟢"
}

// renderMarkdown produces the human-readable summary. It leads with the
// verdict and the issues that drive it, then lists every metric so the
// report is useful even when the repo is healthy.
func renderMarkdown(r Report) string {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	p("# Repo Health: %s %s\n\n", statusEmoji(r.Status), strings.ToUpper(string(r.Status)))
	p("- Repo: `%s`\n", r.Repo)
	p("- Commit: `%s`\n", r.Commit)
	p("- Generated: %s\n\n", r.GeneratedAt.UTC().Format("2006-01-02 15:04:05 UTC"))

	// Issues, worst first.
	if len(r.Issues) > 0 {
		p("## Issues\n\n")
		order := map[Severity]int{SeverityCritical: 0, SeverityWarn: 1, SeverityInfo: 2}
		sorted := append([]Issue(nil), r.Issues...)
		sort.SliceStable(sorted, func(i, j int) bool { return order[sorted[i].Severity] < order[sorted[j].Severity] })
		for _, is := range sorted {
			p("- %s **%s** — %s\n", sevMarker(is.Severity), is.Check, is.Detail)
		}
		p("\n")
	}

	cq := r.CodeQuality
	p("## Code Quality\n\n")
	p("| Metric | Value |\n|---|---|\n")
	p("| Lint findings | %s |\n", availInt(cq.Lint, cq.LintFindings))
	if cq.Coverage.Available {
		p("| Coverage (mean of pkgs) | %.1f%% |\n", cq.CoveragePct)
	} else {
		p("| Coverage (mean of pkgs) | n/a |\n")
	}
	p("| Source files | %d |\n", cq.SourceFiles)
	p("| Test files | %d |\n", cq.TestFiles)
	p("| Test:source ratio | %.2f |\n", cq.TestSourceRatio)
	p("| Avg cyclomatic complexity | %.2f (%d funcs) |\n\n", cq.AvgComplexity, cq.FunctionsAnalyzed)

	arch := r.Architecture
	p("## Architecture\n\n")
	p("| Metric | Value |\n|---|---|\n")
	p("| Max dependency depth | %s |\n", availInt(arch.DepGraph, arch.MaxDepDepth))
	p("| Circular dependencies | %d |\n", len(arch.CircularDeps))
	p("| Files > %d lines | %d |\n", 500, len(arch.LargeFiles))
	p("| Functions > %d lines | %d |\n\n", 50, len(arch.LongFunctions))
	if len(arch.CircularDeps) > 0 {
		p("**Import cycles:**\n\n")
		for _, c := range arch.CircularDeps {
			p("- `%s`\n", strings.Join(c, " → "))
		}
		p("\n")
	}
	if top := topN(arch.LargeFiles, 10); len(top) > 0 {
		p("<details><summary>Largest files</summary>\n\n")
		for _, f := range top {
			p("- `%s` — %d lines\n", f.Name, f.Lines)
		}
		p("\n</details>\n\n")
	}

	ah := r.AgentHealth
	p("## Agent Health\n\n")
	p("| Metric | Value |\n|---|---|\n")
	p("| Linked worktrees | %s |\n", availInt(ah.Worktrees, ah.WorktreeCount))
	p("| Stale branches (>%dd) | %s |\n", 30, availInt(ah.StaleBranches, ah.StaleBranchCount))
	p("| BDD @implemented | %s |\n", availRatio(ah.BDD, ah.FeaturesImpl, ah.FeaturesTotal))
	p("| EARS SPEC.md coverage | %s |\n\n", availRatio(ah.EARS, ah.PackagesWithSpec, ah.PackagesTotal))

	dr := r.Drift
	p("## Drift\n\n")
	p("| Metric | Value |\n|---|---|\n")
	p("| Chezmoi drifted files | %s |\n", availInt(dr.Chezmoi, len(dr.ChezmoiDrifted)))
	p("| Hook drift | %s |\n", availInt(dr.Hooks, len(dr.HookDrifted)))
	p("| Unpaired .ai/.why docs | %s |\n\n", availInt(dr.DocPairing, len(dr.UnpairedDocs)))
	for _, h := range dr.HookDrifted {
		p("- hook: %s\n", h)
	}

	return b.String()
}

func sevMarker(s Severity) string {
	switch s {
	case SeverityCritical:
		return "🔴"
	case SeverityWarn:
		return "🟡"
	case SeverityInfo:
		return "ℹ️"
	}
	return "ℹ️"
}

func availInt(m Metric, v int) string {
	if !m.Available {
		return "n/a"
	}
	return fmt.Sprintf("%d", v)
}

func availRatio(m Metric, n, total int) string {
	if !m.Available {
		return "n/a"
	}
	pct := 0.0
	if total > 0 {
		pct = float64(n) / float64(total) * 100
	}
	return fmt.Sprintf("%d/%d (%.0f%%)", n, total, pct)
}

func topN(items []SizedItem, n int) []SizedItem {
	if len(items) <= n {
		return items
	}
	return items[:n]
}
