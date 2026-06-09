package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/claudeui"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

// fakeInspector lets the safety logic be tested without a real repo or gh.
type fakeInspector struct {
	wts      []gitpkg.Worktree
	dirty    map[string]bool
	dirtyErr map[string]error
	unmerged map[string]string // path -> detail ("" = none)
	openPR   map[string]int    // branch -> PR number (presence == known)
}

func (f fakeInspector) Worktrees(string) ([]gitpkg.Worktree, error) { return f.wts, nil }
func (f fakeInspector) Dirty(p string) (bool, error)                { return f.dirty[p], f.dirtyErr[p] }
func (f fakeInspector) Unmerged(p string) (bool, string)            { d := f.unmerged[p]; return d != "", d }
func (f fakeInspector) OpenPR(_, b string) (int, bool)              { n, ok := f.openPR[b]; return n, ok }

func TestAwaitingInputReason(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"All done, archived 3 sessions.", false},
		{"Does this look right?", true},
		{"  trailing whitespace then question?\n\n", true},
		{"Would you like me to proceed with the refactor", true},
		{"I think we SHOULD I-beam... no wait", true}, // "should i" substring (lowercased)
		{"What would you prefer here, A or B", true},
		{"Running tests now.", false},
		{"", false},
	}
	for _, c := range cases {
		if _, got := awaitingInputReason(c.text); got != c.want {
			t.Errorf("awaitingInputReason(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestAssistantTextOf(t *testing.T) {
	// assistant with text blocks (skips thinking/tool_use)
	line := `{"type":"assistant","message":{"role":"assistant","content":[` +
		`{"type":"thinking","thinking":"x"},{"type":"text","text":"Hello "},` +
		`{"type":"tool_use","name":"Bash"},{"type":"text","text":"world?"}]}}`
	if got := assistantTextOf(line); got != "Hello world?" {
		t.Errorf("text blocks: got %q want %q", got, "Hello world?")
	}
	// assistant with plain string content
	if got := assistantTextOf(`{"type":"assistant","message":{"role":"assistant","content":"just a string"}}`); got != "just a string" {
		t.Errorf("string content: got %q", got)
	}
	// tool-only assistant turn -> empty
	if got := assistantTextOf(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`); got != "" {
		t.Errorf("tool-only should be empty, got %q", got)
	}
	// user line -> empty
	if got := assistantTextOf(`{"type":"user","message":{"role":"user","content":"hi assistant"}}`); got != "" {
		t.Errorf("user line should be empty, got %q", got)
	}
	// junk -> empty
	if got := assistantTextOf(`not json`); got != "" {
		t.Errorf("junk should be empty, got %q", got)
	}
}

func TestLastAssistantText_ReadsTail(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "-Users-x-w")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(proj, "cli-123.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"go"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Should I proceed?"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	scan := realTranscriptScanner(dir)
	got, found := scan("cli-123")
	if !found || got != "Should I proceed?" {
		t.Fatalf("scan = (%q,%v), want last assistant text", got, found)
	}
	if _, found := scan("nonexistent"); found {
		t.Error("missing transcript should report not found")
	}
}

func TestEvalSafetyWarnings(t *testing.T) {
	cwd := t.TempDir() // must exist for worktree association
	s := &claudeui.Session{
		SessionID:    "local_x",
		CliSessionID: "cli-x",
		Cwd:          cwd,
	}

	t.Run("clean session has no warnings", func(t *testing.T) {
		insp := fakeInspector{wts: []gitpkg.Worktree{{Path: cwd, Branch: "b"}}}
		if w := evalSafetyWarnings(s, insp, func(string) (string, bool) { return "all done.", true }); len(w) != 0 {
			t.Fatalf("expected no warnings, got %+v", w)
		}
	})

	t.Run("dirty + open PR + awaiting input", func(t *testing.T) {
		insp := fakeInspector{
			wts:    []gitpkg.Worktree{{Path: cwd, Branch: "feature/z"}},
			dirty:  map[string]bool{cwd: true},
			openPR: map[string]int{"feature/z": 77},
		}
		w := evalSafetyWarnings(s, insp, func(string) (string, bool) { return "Would you like me to continue", true })
		kinds := map[string]string{}
		for _, x := range w {
			kinds[x.Kind] = x.Detail
		}
		if _, ok := kinds[warnUncommitted]; !ok {
			t.Errorf("missing uncommitted-work warning: %+v", w)
		}
		if d := kinds[warnOpenPR]; d == "" || !strings.Contains(d, "#77") {
			t.Errorf("missing/!#77 open-pr warning: %q", d)
		}
		if _, ok := kinds[warnAwaitingInput]; !ok {
			t.Errorf("missing awaiting-input warning: %+v", w)
		}
	})

	t.Run("unmerged when not dirty", func(t *testing.T) {
		insp := fakeInspector{
			wts:      []gitpkg.Worktree{{Path: cwd, Branch: "b"}},
			unmerged: map[string]string{cwd: "3 commit(s) not in origin/main"},
		}
		w := evalSafetyWarnings(s, insp, nil)
		if len(w) != 1 || w[0].Kind != warnUncommitted || !strings.Contains(w[0].Detail, "3 commit") {
			t.Fatalf("expected unmerged warning, got %+v", w)
		}
	})

	t.Run("gone worktree -> only transcript heuristic applies", func(t *testing.T) {
		gone := &claudeui.Session{CliSessionID: "cli-g", Cwd: "/no/such/dir"}
		insp := fakeInspector{dirty: map[string]bool{"/no/such/dir": true}}
		w := evalSafetyWarnings(gone, insp, func(string) (string, bool) { return "what next?", true })
		if len(w) != 1 || w[0].Kind != warnAwaitingInput {
			t.Fatalf("expected only awaiting-input, got %+v", w)
		}
	})
}

func TestArchiveUI_SafetyGate(t *testing.T) {
	mkReq := func() *ArchiveUISessionsRequest {
		req := uiFixture(t, map[string]string{
			"local_w.json": uiSession("w", nowMs-30*dayMs, "false", "/gone/wt"),
		}, nil)
		req.inspector = fakeInspector{} // no worktrees -> only transcript matters
		req.transcript = func(string) (string, bool) { return "Should I proceed?", true }
		return req
	}

	t.Run("dry-run surfaces warning and would-skip", func(t *testing.T) {
		r, err := ArchiveUISessions(&OpContext{}, mkReq())
		if err != nil {
			t.Fatal(err)
		}
		oc, _ := outcomeBySession(r, "w")
		if oc.Action != "skip" || oc.Reason != uiSkipSafetyWarnings {
			t.Fatalf("want skip:safety-warnings, got %s:%s", oc.Action, oc.Reason)
		}
		if len(oc.Warnings) != 1 || oc.Warnings[0].Kind != warnAwaitingInput {
			t.Fatalf("want awaiting-input warning, got %+v", oc.Warnings)
		}
		if r.Warned != 1 || r.Changed != 0 {
			t.Fatalf("want warned=1 changed=0, got warned=%d changed=%d", r.Warned, r.Changed)
		}
	})

	t.Run("apply without --force skips warned, mutates nothing", func(t *testing.T) {
		req := mkReq()
		req.Apply = true
		r, err := ArchiveUISessions(&OpContext{}, req)
		if err != nil {
			t.Fatal(err)
		}
		oc, _ := outcomeBySession(r, "w")
		if oc.Action != "skip" || oc.Reason != uiSkipSafetyWarnings {
			t.Fatalf("want skip:safety-warnings, got %s:%s", oc.Action, oc.Reason)
		}
		got, _ := os.ReadFile(filepath.Join(r.Store, "local_w.json"))
		if string(got) != uiSession("w", nowMs-30*dayMs, "false", "/gone/wt") {
			t.Fatal("warned session was mutated without --force")
		}
	})

	t.Run("--force archives despite warning, keeps warning attached", func(t *testing.T) {
		req := mkReq()
		req.Apply = true
		req.Force = true
		r, err := ArchiveUISessions(&OpContext{}, req)
		if err != nil {
			t.Fatal(err)
		}
		oc, _ := outcomeBySession(r, "w")
		if oc.Action != "archived" {
			t.Fatalf("want archived under --force, got %s:%s", oc.Action, oc.Reason)
		}
		if len(oc.Warnings) != 1 || r.Warned != 1 || r.Changed != 1 {
			t.Fatalf("force must retain warning: warnings=%+v warned=%d changed=%d",
				oc.Warnings, r.Warned, r.Changed)
		}
	})

	t.Run("unarchive direction is not safety-gated", func(t *testing.T) {
		req := uiFixture(t, map[string]string{
			"local_a.json": uiSession("a", nowMs-30*dayMs, "true", "/gone/wt"),
		}, nil)
		req.inspector = fakeInspector{}
		req.transcript = func(string) (string, bool) { return "Should I proceed?", true }
		req.Apply = true
		req.Unarchive = true
		req.Status = "all"
		r, err := ArchiveUISessions(&OpContext{}, req)
		if err != nil {
			t.Fatal(err)
		}
		oc, _ := outcomeBySession(r, "a")
		if oc.Action != "unarchived" || len(oc.Warnings) != 0 || r.Warned != 0 {
			t.Fatalf("unarchive must not be gated: action=%s warnings=%+v warned=%d",
				oc.Action, oc.Warnings, r.Warned)
		}
	})
}

func TestAssociatedWorktrees_NameMatch(t *testing.T) {
	cwd := t.TempDir()
	s := &claudeui.Session{CliSessionID: "abc123", Cwd: cwd}
	insp := fakeInspector{wts: []gitpkg.Worktree{
		{Path: "/other/unrelated", Branch: "main"},
		{Path: "/wt/abc123-feature", Branch: "claude/abc123"}, // matches by name + branch
		{Path: cwd, Branch: "x"},                              // matches by cwd
	}}
	got := associatedWorktrees(s, insp)
	if len(got) != 2 {
		t.Fatalf("expected 2 associated (cwd + name match), got %d: %+v", len(got), got)
	}
}
