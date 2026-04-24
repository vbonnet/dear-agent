package wikibrain

import (
	"path/filepath"
	"strings"
)

// PageIndex is a lookup from normalised key → Page.
type PageIndex map[string]*Page

// BuildIndex indexes pages by multiple keys so that both wikilinks
// ([[ADR-013]]) and markdown links (../01-decisions/ADR-013.md) resolve.
func BuildIndex(pages []*Page) PageIndex {
	idx := make(PageIndex, len(pages)*3)
	for _, p := range pages {
		// Key by full rel-path
		idx[p.RelPath] = p
		// Key by basename without extension (covers [[ADR-013]])
		base := strings.TrimSuffix(filepath.Base(p.RelPath), ".md")
		idx[base] = p
		// Key by lower-case basename
		idx[strings.ToLower(base)] = p
	}
	return idx
}

// ComputeInbound tallies how many pages link to each page and sets
// InboundCount on every Page.
func ComputeInbound(pages []*Page, idx PageIndex) {
	for _, src := range pages {
		for _, raw := range src.WikiLinks {
			target := LinkTarget(raw)
			if dst, ok := idx[target]; ok && dst != src {
				dst.InboundCount++
			}
		}
		for _, raw := range src.MarkdownLinks {
			target := resolveRelLink(src.RelPath, raw)
			if dst, ok := idx[target]; ok && dst != src {
				dst.InboundCount++
			}
		}
	}
}

// resolveRelLink converts a relative markdown link from a source file into
// a normalised path relative to the KB root.
func resolveRelLink(srcRel, linkTarget string) string {
	base := filepath.Dir(srcRel)
	joined := filepath.Join(base, linkTarget)
	// Clean away any .. components
	cleaned := filepath.Clean(joined)
	return cleaned
}
