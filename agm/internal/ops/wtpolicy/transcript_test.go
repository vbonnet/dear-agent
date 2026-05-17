package wtpolicy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEndsWithQuestion(t *testing.T) {
	for in, want := range map[string]bool{
		"Shall I proceed?":  true,
		"done?\n\t ":        true,
		"All set.":          false,
		"":                  false,
		"a? then more text": false,
	} {
		if got := endsWithQuestion(in); got != want {
			t.Errorf("endsWithQuestion(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMatchesQuestionPhrase(t *testing.T) {
	if !matchesQuestionPhrase("Sure — WOULD YOU LIKE me to continue") {
		t.Error("phrase match must be case-insensitive")
	}
	if !matchesQuestionPhrase("Should I open the PR now") {
		t.Error("'should i' should match")
	}
	if matchesQuestionPhrase("Everything is committed and pushed.") {
		t.Error("non-question text must not match")
	}
}

func TestProjectSlug(t *testing.T) {
	got := projectSlug("/Users/v/worktrees/dear-agent/foo")
	if got != "-Users-v-worktrees-dear-agent-foo" {
		t.Fatalf("slug = %q", got)
	}
	if projectSlug("/a/b.c") != "-a-b-c" {
		t.Fatalf("dot not slugified: %q", projectSlug("/a/b.c"))
	}
}

func TestParseTurn(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantRole string
		wantText string
		wantTool bool
		wantOK   bool
	}{
		{"assistant text", `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi?"}]}}`, "assistant", "hi?", false, true},
		{"assistant AskUserQuestion", `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"AskUserQuestion"}]}}`, "assistant", "", true, true},
		{"user line", `{"type":"user","message":{"role":"user","content":"ok"}}`, "user", "", false, true},
		{"sidechain skipped", `{"type":"assistant","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"x?"}]}}`, "", "", false, false},
		{"summary skipped", `{"type":"summary","content":"foo"}`, "", "", false, false},
		{"garbage skipped", `not json`, "", "", false, false},
		{"assistant bare-string content", `{"type":"assistant","message":{"role":"assistant","content":"plain?"}}`, "assistant", "plain?", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			role, text, tool, ok := parseTurn([]byte(c.line))
			if role != c.wantRole || text != c.wantText || tool != c.wantTool || ok != c.wantOK {
				t.Fatalf("got (%q,%q,%v,%v), want (%q,%q,%v,%v)",
					role, text, tool, ok, c.wantRole, c.wantText, c.wantTool, c.wantOK)
			}
		})
	}
}

// writeTranscript builds $HOME/.claude/projects/<slug>/sess.jsonl for wtPath.
func writeTranscript(t *testing.T, home, wtPath string, lines ...string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", projectSlug(wtPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "sess.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAwaitingInput(t *testing.T) {
	wt := "/Users/x/worktrees/repo/feat"

	asst := func(text string) string {
		return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + text + `"}]}}`
	}
	askTool := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"AskUserQuestion"}]}}`
	userLine := `{"type":"user","message":{"role":"user","content":"my answer"}}`

	t.Run("trailing question ⇒ awaiting", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeTranscript(t, home, wt, userLine, asst("Which option do you want?"))
		a, d := AwaitingInput(wt)
		if !a || d != "trailing-question" {
			t.Fatalf("got (%v,%q), want (true,trailing-question)", a, d)
		}
	})

	t.Run("AskUserQuestion tool ⇒ awaiting", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeTranscript(t, home, wt, asst("Let me ask."), askTool)
		a, d := AwaitingInput(wt)
		if !a || d != "AskUserQuestion" {
			t.Fatalf("got (%v,%q), want (true,AskUserQuestion)", a, d)
		}
	})

	t.Run("question phrase ⇒ awaiting", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeTranscript(t, home, wt, asst("Should I proceed with the rebase"))
		a, d := AwaitingInput(wt)
		if !a || d != "question-phrase" {
			t.Fatalf("got (%v,%q), want (true,question-phrase)", a, d)
		}
	})

	t.Run("user replied after the question ⇒ not awaiting", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeTranscript(t, home, wt, asst("Which one?"), userLine)
		if a, _ := AwaitingInput(wt); a {
			t.Fatal("a user turn after the question means it was answered")
		}
	})

	t.Run("assistant finished normally ⇒ not awaiting", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		writeTranscript(t, home, wt, asst("All done and pushed."))
		if a, _ := AwaitingInput(wt); a {
			t.Fatal("a declarative final turn is not awaiting")
		}
	})

	t.Run("no transcript ⇒ not awaiting (fail-safe toward cleanup)", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		if a, _ := AwaitingInput(wt); a {
			t.Fatal("absent transcript must not pin a worktree")
		}
	})
}
