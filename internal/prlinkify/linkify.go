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
//
// It runs a single replacement pass over the unprotected segments using
// combinedRe, which matches both plural ("PRs #1, #2 and #3") and singular
// ("PR #464") forms.
func Linkify(text string, cfg Config) string {
	cfg = withDefaults(cfg)

	segs := splitProtected(text)
	var sb strings.Builder
	for _, seg := range segs {
		if seg.protected {
			sb.WriteString(seg.text)
			continue
		}
		sb.WriteString(combinedRe.ReplaceAllStringFunc(seg.text, func(match string) string {
			sub := combinedRe.FindStringSubmatch(match)
			if sub[1] != "" {
				// Plural form: expand each #NNN to its own link.
				nums := numRe.FindAllStringSubmatch(sub[1], -1)
				links := make([]string, 0, len(nums))
				for _, n := range nums {
					links = append(links, prLink(cfg.DefaultRepo, n[1]))
				}
				return strings.Join(links, ", ")
			}
			// Singular form.
			return prLink(cfg.DefaultRepo, sub[2])
		}))
	}
	return sb.String()
}

// FindRefs extracts deduplicated PR references from text without modifying it.
// Only numbers that are part of an actual PR reference (matched by combinedRe)
// are returned; unrelated "#NNN" tokens such as issue numbers are ignored.
func FindRefs(text string, cfg Config) []PRRef {
	cfg = withDefaults(cfg)
	seen := make(map[string]bool)
	var refs []PRRef

	add := func(num string) {
		if seen[num] {
			return
		}
		seen[num] = true
		refs = append(refs, PRRef{
			Number: num,
			Repo:   cfg.DefaultRepo,
			URL:    prURL(cfg.DefaultRepo, num),
		})
	}

	segs := splitProtected(text)
	for _, seg := range segs {
		if seg.protected {
			continue
		}
		for _, match := range combinedRe.FindAllStringSubmatch(seg.text, -1) {
			if match[1] != "" {
				// Plural form: pull every #NNN out of the matched region.
				for _, n := range numRe.FindAllStringSubmatch(match[1], -1) {
					add(n[1])
				}
			} else if match[2] != "" {
				add(match[2])
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

func prLink(repo, num string) string {
	return fmt.Sprintf("[PR #%s](%s)", num, prURL(repo, num))
}

type segment struct {
	text      string
	protected bool
}

var (
	// Order matters: fenced code blocks first (greedy), then inline code, then
	// existing markdown links. The link branch protects both inline links
	// ("[PR #123](url)") and reference-style / shortcut links ("[PR #123][ref]",
	// "[PR #123]") so we never nest a link inside an existing one.
	protectedRe = regexp.MustCompile(
		"(?s)```.*?```" +
			"|`[^`\n]+`" +
			`|\[[^\]]*#\d+[^\]]*\](?:\([^)]*\)|\[[^\]]*\])?`,
	)

	// combinedRe matches both PR reference forms in a single pattern.
	//   Group 1 (plural): "PRs #412, #419 and #423"
	//   Group 2 (singular number): "PR #464" -> "464"
	// The plural alternative is listed first so it wins over the singular
	// alternative on plural inputs.
	combinedRe = regexp.MustCompile(
		`(\bPRs\s+#\d+(?:\s*(?:,\s*(?:and\s+)?|and\s+)#\d+)+)` +
			`|\bPR\s+#(\d+)`,
	)

	// Extract all #NNN from a matched plural region.
	numRe = regexp.MustCompile(`#(\d+)`)
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
