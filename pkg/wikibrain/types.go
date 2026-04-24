// Package wikibrain provides lint, index, and backlink analysis for the engram-kb
// markdown knowledge base.
package wikibrain

import (
	"strings"
	"time"
)

// Severity classifies the urgency of a lint issue.
type Severity int

const (
	SeverityError   Severity = iota // 🔴 must fix — structural breakage
	SeverityWarning                 // 🟡 should fix — quality degradation
	SeverityInfo                    // 🔵 informational — coverage signal
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

func (s Severity) Emoji() string {
	switch s {
	case SeverityError:
		return "🔴"
	case SeverityWarning:
		return "🟡"
	default:
		return "🔵"
	}
}

// LintCode identifies the specific check that fired.
type LintCode string

const (
	CodeBrokenLink  LintCode = "BROKEN_LINK"  // target page does not exist
	CodeOrphanPage  LintCode = "ORPHAN_PAGE"   // page has no inbound links
	CodeStalePage   LintCode = "STALE_PAGE"    // last-updated > 6 months ago
	CodeMissingMeta LintCode = "MISSING_META"  // no last-updated or status field
	CodeCoverageGap LintCode = "COVERAGE_GAP"  // page has very few outbound links (likely stub)
)

// LintIssue is a single finding from the linter.
type LintIssue struct {
	Severity Severity
	Code     LintCode
	File     string // repo-relative path
	Line     int    // 1-based; 0 means "whole file"
	Message  string
}

// LintReport is the complete output of a lint run.
type LintReport struct {
	KBPath  string
	RunAt   time.Time
	Issues  []LintIssue
	Stats   LintStats
}

// LintStats summarises a lint run.
type LintStats struct {
	TotalPages   int
	TotalLinks   int
	ErrorCount   int
	WarningCount int
	InfoCount    int
}

// Page represents a single markdown document in the knowledge base.
type Page struct {
	// AbsPath is the absolute filesystem path.
	AbsPath string
	// RelPath is the path relative to the KB root.
	RelPath string
	// Title extracted from the first H1 heading (or filename if absent).
	Title string
	// Summary is the first non-heading, non-empty paragraph (best-effort).
	Summary string
	// WikiLinks contains link targets from [[…]] syntax.
	WikiLinks []string
	// MarkdownLinks contains targets from [text](target) syntax.
	MarkdownLinks []string
	// LastUpdated is parsed from "Last updated:" or "last-updated:" frontmatter.
	LastUpdated time.Time
	// HasLastUpdated indicates whether LastUpdated was found.
	HasLastUpdated bool
	// InboundCount is populated during graph analysis, not during scanning.
	InboundCount int
}

// LinkTarget strips anchors and normalises a raw link string to a rel-path
// key that can be looked up in the page index.
func LinkTarget(raw string) string {
	// Drop URL anchor
	if idx := strings.Index(raw, "#"); idx != -1 {
		raw = raw[:idx]
	}
	raw = strings.TrimSpace(raw)
	return raw
}

// BacklinkSuggestion is produced by the backlink auditor when an existing page
// could be linked to a newly ingested page but currently isn't.
type BacklinkSuggestion struct {
	// SourcePage is the existing page that should receive the new link.
	SourcePage string
	// TargetPage is the newly ingested page that should be linked to.
	TargetPage string
	// MatchedTerms are the words/phrases that triggered the suggestion.
	MatchedTerms []string
}

// IndexEntry is one row in the generated index.md catalog.
type IndexEntry struct {
	RelPath     string
	Title       string
	Summary     string
	LastUpdated time.Time
	HasDate     bool
}
