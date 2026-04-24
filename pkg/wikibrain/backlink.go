package wikibrain

import (
	"path/filepath"
	"strings"
)

// AuditBacklinks scans existing pages for mentions of the new page's title
// or stem and returns suggestions for pages that should link back.
// It intentionally avoids semantic analysis — it uses title/filename keywords
// so results are deterministic and fast.
func AuditBacklinks(newPage *Page, existingPages []*Page) []BacklinkSuggestion {
	terms := extractSearchTerms(newPage)
	if len(terms) == 0 {
		return nil
	}

	var suggestions []BacklinkSuggestion
	newPageStem := pageStem(newPage.RelPath)

	for _, p := range existingPages {
		if p.RelPath == newPage.RelPath {
			continue
		}
		// Skip if this page already links to the new page
		if alreadyLinks(p, newPage) {
			continue
		}

		matched := matchedTerms(p, terms, newPageStem)
		if len(matched) > 0 {
			suggestions = append(suggestions, BacklinkSuggestion{
				SourcePage:   p.RelPath,
				TargetPage:   newPage.RelPath,
				MatchedTerms: matched,
			})
		}
	}

	return suggestions
}

// extractSearchTerms derives keywords from a page's title and filename stem.
func extractSearchTerms(p *Page) []string {
	var terms []string

	// From title
	if p.Title != "" && p.Title != titleFromPath(p.RelPath) {
		terms = append(terms, p.Title)
	}

	// From stem (e.g. "ADR-013", "topic-git-worktree-isolation")
	stem := pageStem(p.RelPath)
	terms = append(terms, stem)

	// From stem words (split on hyphens)
	words := strings.Split(stem, "-")
	var meaningful []string
	for _, w := range words {
		if len(w) > 3 {
			meaningful = append(meaningful, w)
		}
	}
	if len(meaningful) >= 2 {
		// Add multi-word phrase from stem
		terms = append(terms, strings.Join(meaningful, " "))
	}

	return dedup(terms)
}

func pageStem(relPath string) string {
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, ".md")
}

func alreadyLinks(src, dst *Page) bool {
	dstStem := pageStem(dst.RelPath)
	for _, wl := range src.WikiLinks {
		if strings.EqualFold(LinkTarget(wl), dstStem) {
			return true
		}
		if strings.EqualFold(LinkTarget(wl), dst.RelPath) {
			return true
		}
	}
	for _, ml := range src.MarkdownLinks {
		target := resolveRelLink(src.RelPath, ml)
		if strings.EqualFold(target, dst.RelPath) {
			return true
		}
	}
	return false
}

// matchedTerms returns which search terms appear verbatim in the page content.
// We search the summary and title only (not the full body — we don't hold it
// in memory) to keep memory usage low. For a more thorough scan the caller
// can re-read files.
func matchedTerms(p *Page, terms []string, newPageStem string) []string {
	corpus := strings.ToLower(p.Title + " " + p.Summary)
	var matched []string
	for _, t := range terms {
		if strings.Contains(corpus, strings.ToLower(t)) {
			matched = append(matched, t)
		}
	}
	// Also check if page title directly matches the stem
	if strings.EqualFold(p.Title, newPageStem) {
		matched = append(matched, newPageStem)
	}
	return dedup(matched)
}

func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := ss[:0]
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
