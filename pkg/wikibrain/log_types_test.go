package wikibrain

import (
	"strings"
	"testing"
	"time"
)

// fixedTime is a deterministic timestamp so formatted log lines are stable.
var fixedTime = time.Date(2026, 6, 7, 13, 30, 45, 0, time.UTC)

func TestFormatLogEntry(t *testing.T) {
	got := FormatLogEntry(LogEntry{
		Time:    fixedTime,
		Prefix:  LogPrefixUpdate,
		Message: "hello world",
	})
	want := "2026-06-07T13:30:45Z [UPDATE] hello world"
	if got != want {
		t.Errorf("FormatLogEntry = %q, want %q", got, want)
	}
}

func TestFormatLogEntry_ConvertsToUTC(t *testing.T) {
	// A non-UTC zone must be normalised to UTC in the output.
	loc := time.FixedZone("UTC+5", 5*3600)
	got := FormatLogEntry(LogEntry{
		Time:    time.Date(2026, 6, 7, 18, 30, 45, 0, loc),
		Prefix:  LogPrefixIndex,
		Message: "x",
	})
	want := "2026-06-07T13:30:45Z [INDEX] x"
	if got != want {
		t.Errorf("FormatLogEntry (zone) = %q, want %q", got, want)
	}
}

func TestFormatLintLogEntry(t *testing.T) {
	report := &LintReport{
		RunAt: fixedTime,
		Stats: LintStats{
			TotalPages:   12,
			ErrorCount:   3,
			WarningCount: 2,
			InfoCount:    1,
		},
	}
	got := FormatLintLogEntry(report)
	want := "2026-06-07T13:30:45Z [LINT] pages=12 errors=3 warnings=2 info=1"
	if got != want {
		t.Errorf("FormatLintLogEntry = %q, want %q", got, want)
	}
}

func TestFormatIndexLogEntry(t *testing.T) {
	got := FormatIndexLogEntry(7, fixedTime)
	want := "2026-06-07T13:30:45Z [INDEX] pages=7 index.md regenerated"
	if got != want {
		t.Errorf("FormatIndexLogEntry = %q, want %q", got, want)
	}
}

func TestFormatIngestLogEntry(t *testing.T) {
	got := FormatIngestLogEntry("notes/topic.md", 4, fixedTime)
	want := "2026-06-07T13:30:45Z [INGEST] page=notes/topic.md backlink_suggestions=4"
	if got != want {
		t.Errorf("FormatIngestLogEntry = %q, want %q", got, want)
	}
}

func TestFormatQuerySaveLogEntry(t *testing.T) {
	t.Run("short query preserved", func(t *testing.T) {
		got := FormatQuerySaveLogEntry("what is engram", "out/answer.md", fixedTime)
		want := `2026-06-07T13:30:45Z [QUERY] saved=out/answer.md query="what is engram"`
		if got != want {
			t.Errorf("FormatQuerySaveLogEntry = %q, want %q", got, want)
		}
	})

	t.Run("newlines flattened", func(t *testing.T) {
		got := FormatQuerySaveLogEntry("line1\nline2", "out/a.md", fixedTime)
		if strings.Contains(got, "\n") && strings.Count(got, "\n") > 0 {
			// The trailing-content newline check: only the embedded query newline matters.
			if strings.Contains(strings.SplitN(got, "query=", 2)[1], "\n") {
				t.Errorf("query portion should not contain newline: %q", got)
			}
		}
		if !strings.Contains(got, "line1 line2") {
			t.Errorf("expected flattened query, got %q", got)
		}
	})

	t.Run("long query truncated", func(t *testing.T) {
		longQ := strings.Repeat("a", 200)
		got := FormatQuerySaveLogEntry(longQ, "out/a.md", fixedTime)
		// 77 'a' chars + "..." inside the quoted query.
		want := strings.Repeat("a", 77) + "..."
		if !strings.Contains(got, want) {
			t.Errorf("expected truncated query %q in %q", want, got)
		}
		if strings.Contains(got, strings.Repeat("a", 81)) {
			t.Errorf("query was not truncated: %q", got)
		}
	})
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		s    Severity
		want string
	}{
		{SeverityError, "error"},
		{SeverityWarning, "warning"},
		{SeverityInfo, "info"},
		{Severity(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("Severity(%d).String() = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestSeverityEmoji(t *testing.T) {
	tests := []struct {
		s    Severity
		want string
	}{
		{SeverityError, "🔴"},
		{SeverityWarning, "🟡"},
		{SeverityInfo, "🔵"},
		{Severity(99), "🔵"}, // default branch
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.Emoji(); got != tt.want {
				t.Errorf("Severity(%d).Emoji() = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestLinkTarget(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "notes/topic.md", "notes/topic.md"},
		{"strips anchor", "notes/topic.md#section", "notes/topic.md"},
		{"trims whitespace", "  notes/topic.md  ", "notes/topic.md"},
		{"anchor and whitespace", "  topic#h1 ", "topic"},
		{"anchor only", "#frag", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LinkTarget(tt.raw); got != tt.want {
				t.Errorf("LinkTarget(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestAlreadyLinks(t *testing.T) {
	dst := &Page{RelPath: "notes/target.md"}

	t.Run("wikilink by stem", func(t *testing.T) {
		src := &Page{RelPath: "notes/src.md", WikiLinks: []string{"target"}}
		if !alreadyLinks(src, dst) {
			t.Error("expected wikilink-by-stem to count as already linked")
		}
	})

	t.Run("wikilink by relpath", func(t *testing.T) {
		src := &Page{RelPath: "notes/src.md", WikiLinks: []string{"notes/target.md"}}
		if !alreadyLinks(src, dst) {
			t.Error("expected wikilink-by-relpath to count as already linked")
		}
	})

	t.Run("markdown link resolved", func(t *testing.T) {
		src := &Page{RelPath: "notes/src.md", MarkdownLinks: []string{"target.md"}}
		if !alreadyLinks(src, dst) {
			t.Error("expected resolved markdown link to count as already linked")
		}
	})

	t.Run("no link", func(t *testing.T) {
		src := &Page{RelPath: "notes/src.md", WikiLinks: []string{"other"}}
		if alreadyLinks(src, dst) {
			t.Error("expected no link to be reported")
		}
	})
}

func TestMatchedTerms(t *testing.T) {
	p := &Page{Title: "Engram Substrate", Summary: "A note about memory consolidation."}

	t.Run("matches in title and summary, case-insensitive", func(t *testing.T) {
		got := matchedTerms(p, []string{"engram", "MEMORY", "absent"}, "unrelated")
		if len(got) != 2 {
			t.Fatalf("expected 2 matched terms, got %v", got)
		}
		joined := strings.Join(got, ",")
		if !strings.Contains(joined, "engram") || !strings.Contains(joined, "MEMORY") {
			t.Errorf("unexpected matches: %v", got)
		}
	})

	t.Run("title equals stem adds stem", func(t *testing.T) {
		got := matchedTerms(p, nil, "Engram Substrate")
		if len(got) != 1 || got[0] != "Engram Substrate" {
			t.Errorf("expected stem match, got %v", got)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		got := matchedTerms(p, []string{"nothing"}, "nope")
		if len(got) != 0 {
			t.Errorf("expected no matches, got %v", got)
		}
	})
}
