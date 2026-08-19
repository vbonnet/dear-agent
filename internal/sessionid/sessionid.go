// Package sessionid detects Claude Code session identifiers and session URLs
// in text that is about to become permanent, public GitHub state.
//
// # Why this exists
//
// A Claude Code session id addresses a private transcript. It is not a secret
// in the credential sense — it grants no access on its own — but it is private
// user data with no reviewer value, and publishing it correlates a public
// commit with a private conversation. AGENTS.md and the review protocol have
// long said not to publish them. That instruction did not hold: on
// 2026-08-18 a scan of `origin/main` found 160 distinct session ids across 170
// commit messages, carried in a `Claude-Session: https://claude.ai/code/...`
// trailer that squash-merge then folds into main's history.
//
// The instruction tier demonstrably failed here, so this is the deterministic
// tier — the same escalation `.claude/hooks/pretool-pr-guard` made for untraced
// PRs. The two sanctioned publish paths (`safe-pr` for PR title/body/comment,
// `safe-push` for commit messages) call Scan and refuse to publish text that
// carries a match.
//
// # Scope: precision over recall, deliberately
//
// Only shapes unique to Claude session references are matched. Bare UUIDs are
// NOT matched even though Claude Desktop names local sessions with them: this
// repository legitimately carries UUIDs in worktree paths (`.worktrees/<uuid>`)
// and test fixtures, so a UUID rule would block ordinary work. A guard that
// cries wolf gets bypassed, and a bypassed guard prevents nothing.
package sessionid

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Redaction replaces a matched reference when text is stripped rather than
// rejected. It is deliberately conspicuous: a reader of the resulting commit
// or PR body should be able to tell that something was removed.
const Redaction = "[redacted-claude-session]"

// Kind names the shape a Finding matched, so the caller's error message can
// name the exact thing to delete.
type Kind string

const (
	// KindURL is a full session permalink, e.g.
	// https://claude.ai/code/session_01XsStv64mq84DrweqajsZLK — the form the
	// leaked `Claude-Session:` trailers used.
	KindURL Kind = "session-url"
	// KindID is a bare session identifier, e.g. session_01XsStv64mq84DrweqajsZLK,
	// which identifies the same private transcript without the URL wrapper.
	KindID Kind = "session-id"
)

// patterns are ordered longest-match-first: the URL rule runs before the bare
// id rule so a permalink is reported once as a URL rather than twice (once as
// a URL and again as the id embedded in it).
//
//   - Session ids are Anthropic-style: the literal `session_` followed by a
//     base62 body (observed: `01` plus 22 characters). The {16,} floor accepts
//     the observed length with room for format drift while staying long enough
//     that ordinary prose cannot reach it.
//   - The URL rule accepts any scheme, any claude.ai path prefix, and an
//     optional trailing path or query, because the leak vector is whatever the
//     harness happens to render, not one blessed URL layout.
var patterns = []struct {
	kind Kind
	re   *regexp.Regexp
}{
	{KindURL, regexp.MustCompile(`(?i)\b(?:https?://)?claude\.ai/[A-Za-z0-9._~/-]*session_[A-Za-z0-9]{16,}[A-Za-z0-9._~/?=&%-]*`)},
	{KindID, regexp.MustCompile(`\bsession_[A-Za-z0-9]{16,}\b`)},
}

// Finding is one detected reference.
type Finding struct {
	// Kind is the shape that matched.
	Kind Kind
	// Match is the exact matched text, so the caller can point at it.
	Match string
	// Line is the 1-based line number the match starts on, so an author can
	// jump straight to it in a multi-paragraph PR body or commit message.
	Line int
	// start and end are byte offsets into the scanned text. They are unexported
	// because they are an implementation detail of overlap suppression and
	// redaction, not part of the reported result.
	start, end int
}

// Scan reports every session reference in text, ordered by position. It never
// reports two overlapping matches: a permalink yields one KindURL finding, not
// a URL plus the id nested inside it.
func Scan(text string) []Finding {
	var found []Finding
	for _, p := range patterns {
		for _, loc := range p.re.FindAllStringIndex(text, -1) {
			found = append(found, Finding{
				Kind:  p.kind,
				Match: text[loc[0]:loc[1]],
				start: loc[0],
				end:   loc[1],
			})
		}
	}
	// Sort by position, then by descending length so the longer (URL) match at
	// a given offset is kept and the nested id match is dropped below.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].start != found[j].start {
			return found[i].start < found[j].start
		}
		return found[i].end > found[j].end
	})

	out := make([]Finding, 0, len(found))
	prevEnd := -1
	for _, f := range found {
		if f.start < prevEnd {
			continue // nested inside an already-reported match
		}
		f.Line = 1 + strings.Count(text[:f.start], "\n")
		out = append(out, f)
		prevEnd = f.end
	}
	return out
}

// Has reports whether text carries any session reference. It is the cheap
// predicate for callers that only need a yes/no.
func Has(text string) bool { return len(Scan(text)) > 0 }

// Redact replaces every session reference in text with Redaction. It is the
// auto-strip half of the contract, for callers that own the text they are
// rewriting. Callers that are relaying an author's words should reject instead,
// so the author — not the tool — decides what the text should say.
func Redact(text string) string {
	findings := Scan(text)
	if len(findings) == 0 {
		return text
	}
	var b strings.Builder
	prev := 0
	for _, f := range findings {
		b.WriteString(text[prev:f.start])
		b.WriteString(Redaction)
		prev = f.end
	}
	b.WriteString(text[prev:])
	return b.String()
}

// Describe renders findings as an indented, deduplicated bullet list for an
// error message. Identical matches on different lines collapse to one bullet
// listing every line, so a trailer repeated across ten commits does not
// produce ten near-identical lines of output.
func Describe(findings []Finding) string {
	type key struct {
		kind  Kind
		match string
	}
	var order []key
	lines := map[key][]int{}
	for _, f := range findings {
		k := key{f.Kind, f.Match}
		if _, seen := lines[k]; !seen {
			order = append(order, k)
		}
		lines[k] = append(lines[k], f.Line)
	}
	var b strings.Builder
	for _, k := range order {
		nums := make([]string, 0, len(lines[k]))
		for _, n := range lines[k] {
			nums = append(nums, fmt.Sprintf("%d", n))
		}
		fmt.Fprintf(&b, "  - %s %q (line %s)\n", k.kind, k.match, strings.Join(nums, ", "))
	}
	return b.String()
}
