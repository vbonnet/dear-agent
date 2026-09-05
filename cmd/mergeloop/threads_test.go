package main

import (
	"testing"
)

func TestIsKnownBotAuthor(t *testing.T) {
	cases := []struct {
		login string
		want  bool
	}{
		{"gemini-code-assist", true},
		{"gemini-code-assist[bot]", true}, // "[bot]" suffix normalizes off
		{"chatgpt-codex-connector", true},
		{"alice", false},           // human
		{"dependabot[bot]", false}, // a bot, but not one we auto-resolve
		{"", false},                // missing author
	}
	for _, c := range cases {
		if got := isKnownBotAuthor(c.login); got != c.want {
			t.Errorf("isKnownBotAuthor(%q) = %v, want %v", c.login, got, c.want)
		}
	}
}

func TestAllCommentsFromKnownBots(t *testing.T) {
	cases := []struct {
		name   string
		logins []string
		want   bool
	}{
		{"empty", nil, false},
		{"single bot comment", []string{"gemini-code-assist"}, true},
		{"single human comment", []string{"alice"}, false},
		{"all known bots", []string{"gemini-code-assist", "chatgpt-codex-connector"}, true},
		{"bot opens, human replies", []string{"gemini-code-assist", "alice"}, false},
		{"bot opens, unknown bot replies", []string{"gemini-code-assist", "dependabot[bot]"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := allCommentsFromKnownBots(c.logins); got != c.want {
				t.Errorf("allCommentsFromKnownBots(%v) = %v, want %v", c.logins, got, c.want)
			}
		})
	}
}

func TestNormalizeBotLogin(t *testing.T) {
	cases := map[string]string{
		"some-bot":      "some-bot",
		"some-bot[bot]": "some-bot",
		"alice":         "alice",
	}
	for in, want := range cases {
		if got := normalizeBotLogin(in); got != want {
			t.Errorf("normalizeBotLogin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitOwnerRepo(t *testing.T) {
	cases := []struct {
		in          string
		owner, name string
		ok          bool
	}{
		{"vbonnet/dear-agent", "vbonnet", "dear-agent", true},
		{"owner/repo/extra", "owner", "repo/extra", true}, // SplitN keeps the tail
		{"noslash", "", "", false},
		{"/repo", "", "", false},
		{"owner/", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		owner, name, ok := splitOwnerRepo(c.in)
		if owner != c.owner || name != c.name || ok != c.ok {
			t.Errorf("splitOwnerRepo(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.in, owner, name, ok, c.owner, c.name, c.ok)
		}
	}
}

func TestClassifyCommentSeverity(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		want     threadSeverity
		blocking bool
	}{
		// No badge → advisory by default; existing auto-resolve behaviour preserved.
		{"empty body", "", severityNone, false},
		{"plain suggestion no badge", "Consider extracting this into a helper.", severityNone, false},

		// Explicit advisory markers.
		{"nit marker", "**nit:** missing newline at end of file", severityAdvisory, false},
		{"P3 badge", "[P3] this could be cleaner", severityAdvisory, false},
		{"P4 badge", "[P4] minor style issue", severityAdvisory, false},
		{"P5 badge", "[P5] cosmetic", severityAdvisory, false},
		{"advisory word", "advisory: prefer sync.Once here", severityAdvisory, false},
		{"info word", "info: this pattern is deprecated", severityAdvisory, false},
		{"note word", "note: this changes external API", severityAdvisory, false},
		{"low word", "severity: low \u2014 consider renaming", severityAdvisory, false},
		{"suggestion word", "suggestion: extract constant", severityAdvisory, false},
		{"style word", "style: inconsistent casing", severityAdvisory, false},
		{"cosmetic word", "cosmetic change only", severityAdvisory, false},

		// Blocking markers.
		{"P0 badge uppercase", "[P0] data loss on shutdown", severityP0, true},
		{"P0 inline", "This is a P0 regression.", severityP0, true},
		{"P1 badge", "**[P1]** nil pointer dereference on empty slice", severityP1, true},
		{"P1 inline", "P1: this breaks the auth flow", severityP1, true},
		{"P2 badge", "[P2] race condition under load", severityP2, true},
		{"P2 Codex style", "**Severity: P2** \u2014 mutex not held across goroutine", severityP2, true},
		{"critical word", "critical: SQL injection via unescaped input", severityP2, true},
		{"blocker word", "blocker: this must be fixed before merge", severityP2, true},
		{"security word", "security: token exposed in logs", severityP2, true},
		{"vuln word", "vuln: use of deprecated crypto primitive", severityP2, true},

		// Both P1 and advisory in body: P1 wins (max severity).
		{"p1 overrides nit", "nit: also fix style. P1: but this is a real bug.", severityP1, true},

		// Advisory badge overrides incidental blocking keyword in the same body.
		// "[P3] security option" must not fire as P2 just because "security" is
		// a blocking keyword: the explicit badge is authoritative (ce-lr7j Codex P1).
		{"advisory badge beats security keyword", "[P3] Rename the security option", severityAdvisory, false},
		{"advisory badge beats critical keyword", "[P4] critical path refactor (style)", severityAdvisory, false},
		// Blocking badge wins over advisory badge when both are present.
		{"p1 badge beats advisory badge", "P1: real bug. Also [P3] nit.", severityP1, true},
		// Word-boundary prevents false positives: "HEAP0" must not trigger P0
		// even when a blocking keyword also appears (Gemini high finding).
		{"HEAP0 substring not P0", "HEAP0 critical regression", severityP2, true},
		// temp0 does not contain P0; critical keyword dominates (Gemini suggestion).
		{"temp0 no P0 false positive", "critical: check temp0", severityP2, true},
		// P10 must not be confused with P1 (no word boundary after the 1 in "P10").
		// reSeverityMarker uses \bP[0-9]\b, so "P10" is not detected as a badge at all.
		{"P10 not P1", "P10: some badge", severityNone, false},

		// Fail-closed: severity-like pattern present but not in vocabulary.
		// P6/P7/P8/P9 are in the severity-marker regex but not in blocking or safe lists.
		{"unknown P6", "P6: unrecognised badge", severityUnknown, true},
		{"unknown P9", "[P9] some future severity", severityUnknown, true},
		// Unknown badge (P6) must not be overridden by a safe keyword in the body
		// (Codex P2: check unknown badge before safe-keyword fallback).
		{"unknown P6 with suggestion not advisory", "P6 suggestion: revise this", severityUnknown, true},

		// Advisory badge must be a structured label, not a bare prose mention.
		// "only P3 if" in a blocking comment must not downgrade severity
		// (Codex P1: anchor advisory badges to bracket/image form).
		{"P3 in prose does not downgrade blocking", "Critical: this corrupts data; it would only be P3 if validation had run", severityP2, true},

		// Textual high-severity labels from bots that don't use Px badges.
		// Gemini image format: ![high](.../gstatic.com/...).
		{"Gemini high image badge", "![high](https://www.gstatic.com/codereviewagent/high-priority.svg) SQL injection", severityP2, true},
		// Generic text label format.
		{"text Severity: high label", "Severity: high \u2014 potential data loss", severityP2, true},
		{"text Priority: high label", "Priority: high fix needed", severityP2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyCommentSeverity(c.body)
			if got != c.want {
				t.Errorf("classifyCommentSeverity(%q) = %v, want %v", c.body, got, c.want)
			}
			if got.blocking() != c.blocking {
				t.Errorf("classifyCommentSeverity(%q).blocking() = %v, want %v", c.body, got.blocking(), c.blocking)
			}
		})
	}
}

func TestMaxSeverity(t *testing.T) {
	cases := []struct {
		name   string
		bodies []string
		want   threadSeverity
	}{
		{"empty", nil, severityNone},
		{"all advisory", []string{"nit: style", "suggestion: rename"}, severityAdvisory},
		{"mixed P1 and nit", []string{"nit: formatting", "P1: data loss"}, severityP1},
		{"P2 only", []string{"[P2] race condition"}, severityP2},
		{"P0 wins over P1", []string{"P1: real bug", "P0: data corruption"}, severityP0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := maxSeverity(c.bodies); got != c.want {
				t.Errorf("maxSeverity(%v) = %v, want %v", c.bodies, got, c.want)
			}
		})
	}
}

func TestThreadSeverityString(t *testing.T) {
	cases := map[threadSeverity]string{
		severityNone:     "none",
		severityAdvisory: "advisory",
		severityP2:       "P2",
		severityP1:       "P1",
		severityP0:       "P0",
		severityUnknown:  "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("threadSeverity(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}
