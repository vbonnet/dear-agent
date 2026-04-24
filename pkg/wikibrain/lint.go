package wikibrain

import (
	"fmt"
	"time"
)

const (
	staleThreshold   = 6 * 30 * 24 * time.Hour // ~6 months
	coverageMinLinks = 2                         // pages with fewer outbound links are flagged
)

// Lint runs all checks against the scanned pages and returns a report.
func Lint(kbPath string) (*LintReport, error) {
	pages, err := ScanKB(kbPath)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	idx := BuildIndex(pages)
	ComputeInbound(pages, idx)

	report := &LintReport{
		KBPath: kbPath,
		RunAt:  time.Now(),
	}
	report.Stats.TotalPages = len(pages)

	for _, p := range pages {
		report.Stats.TotalLinks += len(p.WikiLinks) + len(p.MarkdownLinks)
		report.Issues = append(report.Issues, checkBrokenLinks(p, idx)...)
		report.Issues = append(report.Issues, checkOrphan(p)...)
		report.Issues = append(report.Issues, checkStale(p)...)
		report.Issues = append(report.Issues, checkMissingMeta(p)...)
		report.Issues = append(report.Issues, checkCoverageGap(p)...)
	}

	for _, iss := range report.Issues {
		switch iss.Severity {
		case SeverityError:
			report.Stats.ErrorCount++
		case SeverityWarning:
			report.Stats.WarningCount++
		case SeverityInfo:
			report.Stats.InfoCount++
		}
	}

	return report, nil
}

func checkBrokenLinks(p *Page, idx PageIndex) []LintIssue {
	var issues []LintIssue
	seen := map[string]bool{}

	for _, raw := range p.WikiLinks {
		target := LinkTarget(raw)
		if seen[target] {
			continue
		}
		seen[target] = true
		if _, ok := idx[target]; !ok {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Code:     CodeBrokenLink,
				File:     p.RelPath,
				Message:  fmt.Sprintf("broken wikilink [[%s]] — target not found", raw),
			})
		}
	}

	for _, raw := range p.MarkdownLinks {
		target := resolveRelLink(p.RelPath, raw)
		if seen[target] {
			continue
		}
		seen[target] = true
		if _, ok := idx[target]; !ok {
			issues = append(issues, LintIssue{
				Severity: SeverityError,
				Code:     CodeBrokenLink,
				File:     p.RelPath,
				Message:  fmt.Sprintf("broken markdown link (%s) — target not found", raw),
			})
		}
	}

	return issues
}

func checkOrphan(p *Page) []LintIssue {
	if p.InboundCount == 0 {
		return []LintIssue{{
			Severity: SeverityWarning,
			Code:     CodeOrphanPage,
			File:     p.RelPath,
			Message:  "orphan page — no other pages link here",
		}}
	}
	return nil
}

func checkStale(p *Page) []LintIssue {
	if !p.HasLastUpdated {
		return nil // handled by checkMissingMeta
	}
	age := time.Since(p.LastUpdated)
	if age > staleThreshold {
		months := int(age.Hours() / 24 / 30)
		return []LintIssue{{
			Severity: SeverityWarning,
			Code:     CodeStalePage,
			File:     p.RelPath,
			Message:  fmt.Sprintf("stale page — last updated %d months ago (%s)", months, p.LastUpdated.Format("2006-01-02")),
		}}
	}
	return nil
}

func checkMissingMeta(p *Page) []LintIssue {
	if !p.HasLastUpdated {
		return []LintIssue{{
			Severity: SeverityInfo,
			Code:     CodeMissingMeta,
			File:     p.RelPath,
			Message:  "missing 'Last updated:' field — add date to enable staleness tracking",
		}}
	}
	return nil
}

func checkCoverageGap(p *Page) []LintIssue {
	total := len(p.WikiLinks) + len(p.MarkdownLinks)
	if total < coverageMinLinks {
		return []LintIssue{{
			Severity: SeverityInfo,
			Code:     CodeCoverageGap,
			File:     p.RelPath,
			Message:  fmt.Sprintf("coverage gap — only %d outbound link(s); consider cross-referencing related pages", total),
		}}
	}
	return nil
}
