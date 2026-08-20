package sessionid_test

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/sessionid"
)

// Every fixture below uses a SYNTHETIC session id. Pasting a real one here
// would commit the exact private reference this package exists to keep out of
// the repository — the test suite must not be the last leak vector.
const (
	synthID  = "session_01AaBbCcDdEeFfGgHhIiJjKk"
	synthID2 = "session_01ZzYyXxWwVvUuTtSsRrQqPp"
)

func TestScanDetectsLeakVectors(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		kind sessionid.Kind
	}{
		{
			// The exact shape found 170 times on origin/main.
			name: "trailer permalink",
			text: "fix(auth): reload the agent\n\nClaude-Session: https://claude.ai/code/" + synthID + "\n",
			kind: sessionid.KindURL,
		},
		{"scheme-relative url", "see claude.ai/code/" + synthID, sessionid.KindURL},
		{"url with trailing path", "https://claude.ai/code/" + synthID + "/turn/3", sessionid.KindURL},
		{"url with query", "https://claude.ai/code/" + synthID + "?tab=diff", sessionid.KindURL},
		{"uppercase host", "HTTPS://CLAUDE.AI/code/" + synthID, sessionid.KindURL},
		{"bare id", "resumed from " + synthID + " earlier", sessionid.KindID},
		{"bare id in body", "## Context\n\n" + synthID + "\n", sessionid.KindID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sessionid.Scan(tc.text)
			if len(got) != 1 {
				t.Fatalf("Scan(%q) = %d findings, want exactly 1: %+v", tc.text, len(got), got)
			}
			if got[0].Kind != tc.kind {
				t.Errorf("kind = %q, want %q", got[0].Kind, tc.kind)
			}
			if !sessionid.Has(tc.text) {
				t.Error("Has() = false, want true")
			}
		})
	}
}

// A permalink contains a bare id. Reporting both would double-count one leak
// and make the error message read as if there were two separate references.
func TestScanReportsPermalinkOnceNotTwice(t *testing.T) {
	got := sessionid.Scan("Claude-Session: https://claude.ai/code/" + synthID)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1 (URL only, id suppressed): %+v", len(got), got)
	}
	if got[0].Kind != sessionid.KindURL {
		t.Errorf("kind = %q, want %q — the URL must win over the nested id", got[0].Kind, sessionid.KindURL)
	}
}

// Precision is the whole design: a guard that fires on ordinary work gets
// bypassed, and a bypassed guard prevents nothing.
func TestScanIgnoresLegitimateText(t *testing.T) {
	for _, clean := range []string{
		"",
		"fix(disk): alarm on a stale sandbox reaper",
		// Bare UUIDs are NOT session references: this repo carries them in
		// worktree paths and fixtures.
		".worktrees/f0ebdc78-9d79-4bb6-885a-754f7a981495/AGENTS.md",
		"see https://claude.ai/code for the product page",
		"the session_ prefix is documented in internal/sessionid",
		"session_id column in the sqlite schema",
		"AGM_SESSION_NAME is exported by the supervisor",
		"docs mention a session identifier without quoting one",
		// Too short to be an Anthropic id.
		"session_abc123",
	} {
		if got := sessionid.Scan(clean); len(got) != 0 {
			t.Errorf("Scan(%q) = %+v, want no findings", clean, got)
		}
	}
}

func TestScanReportsLineNumbers(t *testing.T) {
	text := "title\n\nbody line\n\nClaude-Session: https://claude.ai/code/" + synthID + "\n"
	got := sessionid.Scan(text)
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
	if got[0].Line != 5 {
		t.Errorf("Line = %d, want 5", got[0].Line)
	}
}

func TestScanReportsMultipleDistinctReferencesInOrder(t *testing.T) {
	text := "first " + synthID + "\nsecond https://claude.ai/code/" + synthID2 + "\n"
	got := sessionid.Scan(text)
	if len(got) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(got), got)
	}
	if got[0].Kind != sessionid.KindID || got[0].Line != 1 {
		t.Errorf("first = %+v, want a session-id on line 1", got[0])
	}
	if got[1].Kind != sessionid.KindURL || got[1].Line != 2 {
		t.Errorf("second = %+v, want a session-url on line 2", got[1])
	}
}

func TestRedactRemovesEveryReferenceAndKeepsTheRest(t *testing.T) {
	text := "fix: thing\n\nClaude-Session: https://claude.ai/code/" + synthID + "\nAlso " + synthID2 + " ran.\n"
	got := sessionid.Redact(text)
	if sessionid.Has(got) {
		t.Errorf("Redact left a reference behind: %q", got)
	}
	for _, leaked := range []string{synthID, synthID2} {
		if strings.Contains(got, leaked) {
			t.Errorf("Redact left %q in %q", leaked, got)
		}
	}
	if !strings.Contains(got, "fix: thing") || !strings.Contains(got, "Also") || !strings.Contains(got, "ran.") {
		t.Errorf("Redact damaged surrounding text: %q", got)
	}
	if strings.Count(got, sessionid.Redaction) != 2 {
		t.Errorf("got %d redaction markers, want 2: %q", strings.Count(got, sessionid.Redaction), got)
	}
}

func TestRedactIsIdentityOnCleanText(t *testing.T) {
	const clean = "fix(disk): alarm on a stale sandbox reaper\n\nRefs ADR-038.\n"
	if got := sessionid.Redact(clean); got != clean {
		t.Errorf("Redact(clean) = %q, want unchanged", got)
	}
}

// A trailer repeated across many commits must not produce many near-identical
// bullets; the author needs one line per distinct reference.
func TestDescribeCollapsesRepeatedMatches(t *testing.T) {
	text := synthID + "\nfiller\n" + synthID + "\n"
	got := sessionid.Describe(sessionid.Scan(text))
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("Describe emitted %d lines, want 1 collapsed bullet:\n%s", n, got)
	}
	if !strings.Contains(got, "line 1, 3") {
		t.Errorf("Describe should list both line numbers, got:\n%s", got)
	}
}

func TestDescribeDedupesRepeatedMatchOnSameLine(t *testing.T) {
	text := synthID + " " + synthID + "\n"
	got := sessionid.Describe(sessionid.Scan(text))
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("Describe emitted %d lines, want 1 collapsed bullet:\n%s", n, got)
	}
	if !strings.Contains(got, "(line 1)") {
		t.Errorf("Describe should list the shared line once, got:\n%s", got)
	}
	if strings.Contains(got, "line 1, 1") {
		t.Errorf("Describe should not repeat a line number for the same line, got:\n%s", got)
	}
}

func TestDescribeEmpty(t *testing.T) {
	if got := sessionid.Describe(nil); got != "" {
		t.Errorf("Describe(nil) = %q, want empty", got)
	}
}
