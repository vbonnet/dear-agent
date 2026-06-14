// Package prlinkify converts bare PR references (e.g. "PR #464") to markdown links.
package prlinkify

import (
	"fmt"
	"regexp"
	"strings"
)

// Config controls which GitHub repo PR links point to.
type Config struct {
	DefaultRepo string
}

// PRRef is a PR reference found in text.
type PRRef struct {
	Number string
	Repo   string
	URL    string
}

// Linkify replaces bare PR references with markdown links, skipping
// already-linked refs, inline code, and fenced code blocks.
func Linkify(text string, cfg Config) string {
	cfg = withDefaults(cfg)

	segs := splitProtected(text)
	var pass1 strings.Builder
	for _, seg := range segs {
		if seg.protected {
			pass1.WriteString(seg.text)
		} else {
			pass1.WriteString(linkifyPlural(seg.text, cfg))
		}
	}

	segs = splitProtected(pass1.String())
	var pass2 strings.Builder
	for _, seg := range segs {
		if seg.protected {
			pass2.WriteString(seg.text)
		} else {
			pass2.WriteString(linkifySingular(seg.text, cfg))
		}
	}

	return pass2.String()
}

// FindRefs extracts deduplicated PR references from text without modifying it.
func FindRefs(text string, cfg Config) []PRRef {
	cfg = withDefaults(cfg)
	seen := make(map[string]bool)
	var refs []PRRef

	segs := splitProtected(text)
	for _, seg := range segs {
		if seg.protected {
			continue
		}
		for _, m := range numRe.FindAllStringSubmatch(seg.text, -1) {
			if contextRe.MatchString(seg.text) && !seen[m[1]] {
				seen[m[1]] = true
				refs = append(refs, PRRef{
					Number: m[1],
					Repo:   cfg.DefaultRepo,
					URL:    prURL(cfg.DefaultRepo, m[1]),
				})
			}
		}
	}
	return refs
}

func withDefaults(cfg Config) Config {
	if cfg.DefaultRepo == "" {
		cfg.DefaultRepo = "vbonnet/dear-agent"
	}
	return cfg
}

func prURL(repo, num string) string {
	return fmt.Sprintf("https://github.com/%s/pull/%s", repo, num)
}

type segment struct {
	text      string
	protected bool
}

var (
	// Order matters: fenced code blocks first (greedy), then inline code, then existing links.
	protectedRe = regexp.MustCompile(
		"(?s)```.*?```" +
			"|`[^`\n]+`" +
			"|\\[[^\\]]*#\\d+[^\\]]*\\]\\([^)]*\\)",
	)

	// "PRs #412, #419 and #423" — plural with comma/and separators.
	pluralRe = regexp.MustCompile(`\bPRs\s+#\d+(?:\s*(?:,\s*(?:and\s+)?|and\s+)#\d+)+`)

	// "PR #464" — singular.
	singleRe = regexp.MustCompile(`\bPR\s+#(\d+)`)

	// Extract all #NNN from a matched region.
	numRe = regexp.MustCompile(`#(\d+)`)

	// Detects PR-referencing context (used by FindRefs).
	contextRe = regexp.MustCompile(`\bPRs?\b`)
)

func splitProtected(text string) []segment {
	var segments []segment
	lastEnd := 0

	for _, loc := range protectedRe.FindAllStringIndex(text, -1) {
		if loc[0] > lastEnd {
			segments = append(segments, segment{text: text[lastEnd:loc[0]]})
		}
		segments = append(segments, segment{text: text[loc[0]:loc[1]], protected: true})
		lastEnd = loc[1]
	}

	if lastEnd < len(text) {
		segments = append(segments, segment{text: text[lastEnd:]})
	}

	return segments
}

func linkifyPlural(text string, cfg Config) string {
	return pluralRe.ReplaceAllStringFunc(text, func(match string) string {
		nums := numRe.FindAllStringSubmatch(match, -1)
		links := make([]string, 0, len(nums))
		for _, n := range nums {
			links = append(links, fmt.Sprintf("[PR #%s](%s)", n[1], prURL(cfg.DefaultRepo, n[1])))
		}
		return strings.Join(links, ", ")
	})
}

func linkifySingular(text string, cfg Config) string {
	return singleRe.ReplaceAllStringFunc(text, func(match string) string {
		sub := singleRe.FindStringSubmatch(match)
		return fmt.Sprintf("[PR #%s](%s)", sub[1], prURL(cfg.DefaultRepo, sub[1]))
	})
}
