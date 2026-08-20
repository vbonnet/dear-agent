package main

import "testing"

func TestCleanBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"collapses whitespace", "a\n\tb   c", "a b c"},
		{"trims ends", "  hello world  ", "hello world"},
		{"empty", "", ""},
		{"only whitespace", "  \n\t ", ""},
		{"truncates to preview length", repeat("x", bodyPreviewLen+50), repeat("x", bodyPreviewLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanBody(tt.in); got != tt.want {
				t.Errorf("cleanBody(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanBodyRuneSafe(t *testing.T) {
	// A multibyte string longer than the preview must be cut on a rune
	// boundary, never mid-codepoint, and yield exactly bodyPreviewLen runes.
	in := repeat("é", bodyPreviewLen+10)
	got := cleanBody(in)
	if n := len([]rune(got)); n != bodyPreviewLen {
		t.Fatalf("got %d runes, want %d", n, bodyPreviewLen)
	}
}

func TestFilterThreads(t *testing.T) {
	threads := []thread{
		{ID: "1", IsResolved: false, Author: "gemini-code-assist"},
		{ID: "2", IsResolved: true, Author: "gemini-code-assist"},
		{ID: "3", IsResolved: false, Author: "alice"},
		{ID: "4", IsResolved: false, Author: "gemini-code-assist"},
	}

	t.Run("no author keeps all unresolved", func(t *testing.T) {
		got := filterThreads(threads, "")
		if want := []string{"1", "3", "4"}; !idsEqual(got, want) {
			t.Errorf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("author filter never matches resolved", func(t *testing.T) {
		got := filterThreads(threads, "gemini-code-assist")
		if want := []string{"1", "4"}; !idsEqual(got, want) {
			t.Errorf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("author with no unresolved threads yields empty", func(t *testing.T) {
		if got := filterThreads(threads, "nobody"); len(got) != 0 {
			t.Errorf("got %v, want empty", ids(got))
		}
	})
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for range n {
		out = append(out, s...)
	}
	return string(out)
}

func ids(ts []thread) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

func idsEqual(ts []thread, want []string) bool {
	got := ids(ts)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// mkNode builds a threadNode whose comment authors are the given logins, in
// order, mirroring what the opening/recent GraphQL aliases return: the first
// login opens the thread, the last one had the most recent word.
func mkNode(id string, outdated bool, authors ...string) threadNode {
	var n threadNode
	n.ID = id
	n.IsOutdated = outdated
	n.Path = "pkg/thing.go"
	n.Opening.TotalCount = len(authors)
	if len(authors) == 0 {
		return n
	}
	var open struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Body string `json:"body"`
	}
	open.Author.Login = authors[0]
	open.Body = "comment from " + authors[0]
	n.Opening.Nodes = append(n.Opening.Nodes, open)

	var last struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	last.Author.Login = authors[len(authors)-1]
	n.Recent.Nodes = append(n.Recent.Nodes, last)
	return n
}

// mkLongNode builds a thread whose comment count exceeds any single page, to
// prove the answered check reads the true last commenter rather than the end
// of a fetched page.
func mkLongNode(id string, total int, opening, latest string) threadNode {
	n := mkNode(id, false, opening, latest)
	n.Opening.TotalCount = total
	return n
}

// TestToThreadAnswered pins the evidence gate that resolve-all depends on.
// Answered is the only signal standing between "we addressed the review" and
// a blind bulk resolve, so each transition is asserted explicitly.
func TestToThreadAnswered(t *testing.T) {
	tests := []struct {
		name         string
		node         threadNode
		wantAnswered bool
		wantLast     string
	}{
		{
			name:         "reviewer comment alone is unanswered",
			node:         mkNode("1", false, "gemini-code-assist"),
			wantAnswered: false,
			wantLast:     "gemini-code-assist",
		},
		{
			name:         "reply from someone else answers it",
			node:         mkNode("2", false, "gemini-code-assist", "vbonnet"),
			wantAnswered: true,
			wantLast:     "vbonnet",
		},
		{
			name:         "reviewer replying after us takes the ball back",
			node:         mkNode("3", false, "gemini-code-assist", "vbonnet", "gemini-code-assist"),
			wantAnswered: false,
			wantLast:     "gemini-code-assist",
		},
		{
			name:         "reviewer talking to itself is not an answer",
			node:         mkNode("4", false, "gemini-code-assist", "gemini-code-assist"),
			wantAnswered: false,
			wantLast:     "gemini-code-assist",
		},
		{
			name:         "outdated alone never answers a thread",
			node:         mkNode("5", true, "chatgpt-codex-connector"),
			wantAnswered: false,
			wantLast:     "chatgpt-codex-connector",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toThread(tt.node)
			if got.Answered != tt.wantAnswered {
				t.Errorf("Answered = %t, want %t", got.Answered, tt.wantAnswered)
			}
			if got.LastAuthor != tt.wantLast {
				t.Errorf("LastAuthor = %q, want %q", got.LastAuthor, tt.wantLast)
			}
		})
	}
}

// TestToThreadOutdatedIsReportedNotActedOn guards the trap that motivated this
// change: on dear-agent#1242 three outdated threads were live P1 findings, so
// outdated must survive into output without ever implying resolvability.
func TestToThreadOutdatedIsReportedNotActedOn(t *testing.T) {
	got := toThread(mkNode("x", true, "chatgpt-codex-connector"))
	if !got.IsOutdated {
		t.Fatal("IsOutdated was dropped; refusals could not label the thread")
	}
	if got.Answered {
		t.Fatal("an outdated thread was treated as answered")
	}
	if note := outdatedNote(got); note == "" {
		t.Error("outdatedNote said nothing for an outdated thread")
	}
	if note := outdatedNote(toThread(mkNode("y", false, "bot"))); note != "" {
		t.Errorf("outdatedNote spoke for a current thread: %q", note)
	}
}

// TestToThreadNoComments covers the defensive path: a thread with no comments
// must not panic on the last-comment lookup.
func TestToThreadNoComments(t *testing.T) {
	got := toThread(threadNode{ID: "empty"})
	if got.Author != "unknown" || got.Answered {
		t.Errorf("got %+v, want unknown author and unanswered", got)
	}
}

// TestToThreadLongThreadUsesTrueLastCommenter covers the pagination finding:
// with more comments than one page holds, the verdict must still come from the
// genuinely most recent comment, which the `recent` alias supplies directly.
func TestToThreadLongThreadUsesTrueLastCommenter(t *testing.T) {
	answered := toThread(mkLongNode("long-1", 250, "gemini-code-assist", "vbonnet"))
	if !answered.Answered || answered.LastAuthor != "vbonnet" {
		t.Errorf("250-comment thread ending with our reply: got %+v, want answered by vbonnet", answered)
	}

	// The dangerous direction: the reviewer got the last word far past any
	// page boundary. This must not be treated as answered.
	reopened := toThread(mkLongNode("long-2", 250, "gemini-code-assist", "gemini-code-assist"))
	if reopened.Answered {
		t.Error("reviewer had the last word on a long thread but it counted as answered")
	}
}

// TestToThreadSingleCommentNeverAnswered pins that a lone opening comment is
// unanswered no matter what, since TotalCount is the only length signal.
func TestToThreadSingleCommentNeverAnswered(t *testing.T) {
	n := mkNode("solo", false, "gemini-code-assist")
	if got := toThread(n); got.Answered {
		t.Errorf("single-comment thread counted as answered: %+v", got)
	}
}
