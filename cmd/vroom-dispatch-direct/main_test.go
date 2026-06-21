package main

import (
	"strings"
	"testing"
)

func TestMentionsID(t *testing.T) {
	tests := []struct {
		name string
		text string
		id   string
		want bool
	}{
		{"branch path", "feat/fix-ce-5z0o", "ce-5z0o", true},
		{"title exact", "ce-5z0o: do the thing", "ce-5z0o", true},
		{"plain id end of string", "worker-ce-5z0o", "ce-5z0o", true},
		{"sub-bead does not match parent", "ce-cd14.2 phase B", "ce-cd14", false},
		{"sub-bead matches itself", "ce-cd14.2 phase B", "ce-cd14.2", true},
		{"parent does not match unrelated longer id", "ce-5z0ox", "ce-5z0o", false},
		{"absent", "some other text", "ce-5z0o", false},
		{"id inside slash boundary", "ref/ce-bi19/notes", "ce-bi19", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mentionsID(tt.text, tt.id); got != tt.want {
				t.Errorf("mentionsID(%q, %q) = %v, want %v", tt.text, tt.id, got, tt.want)
			}
		})
	}
}

func TestInFlightInPR(t *testing.T) {
	prs := []pullRequest{
		{Number: 1, HeadRefName: "feat/vroom-prompt-generator", Title: "feat(vroom): prompt-generator (ce-5z0o)"},
		{Number: 2, HeadRefName: "fix/ce-bi19-retro", Title: "retro lenses"},
	}
	if !inFlightInPR("ce-5z0o", prs) {
		t.Error("ce-5z0o should match by PR title")
	}
	if !inFlightInPR("ce-bi19", prs) {
		t.Error("ce-bi19 should match by branch name")
	}
	if inFlightInPR("ce-zzzz", prs) {
		t.Error("ce-zzzz should not match any PR")
	}
}

func TestLiveWorkerIDs(t *testing.T) {
	// Mimics `agm session list` output: arbitrary columns after the session name.
	lines := []string{
		"NAME                       STATUS   MODEL",
		"worker-ce-bi19             running  opus-200k",
		"worker-ce-cd14.2           running  opus-200k",
		"vroom-orchestrator         running  sonnet-200k",
		"some-other-session         idle     -",
		"", // trailing blank from the split
	}
	ids := liveWorkerIDs(lines)
	if !ids["ce-bi19"] {
		t.Error("expected ce-bi19 to be live")
	}
	if !ids["ce-cd14.2"] {
		t.Error("expected sub-bead ce-cd14.2 to be live (full id captured)")
	}
	if ids["ce-cd14"] {
		t.Error("parent ce-cd14 should NOT be considered live from a ce-cd14.2 session")
	}
	if len(ids) != 2 {
		t.Errorf("expected exactly 2 live worker ids, got %d: %v", len(ids), ids)
	}
}

func TestLiveWorkerIDsEmpty(t *testing.T) {
	if ids := liveWorkerIDs(nil); len(ids) != 0 {
		t.Errorf("nil input should yield empty set, got %v", ids)
	}
}

func TestFirstParagraph(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"one line", "one line"},
		{"first para\n\nsecond para", "first para"},
		{"  spaced \n line  \n\n more", "spaced line"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := firstParagraph(tt.in); got != tt.want {
			t.Errorf("firstParagraph(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSelectCandidates(t *testing.T) {
	beads := []bead{
		{ID: "ce-aaaa", Title: "fresh", Priority: 1},
		{ID: "ce-bbbb", Title: "live worker", Priority: 1},
		{ID: "ce-cccc", Title: "in PR", Priority: 1},
		{ID: "ce-cd14", Title: "human gated", Priority: 0},
		{ID: "", Title: "empty id", Priority: 1},
	}
	live := map[string]bool{"ce-bbbb": true}
	prs := []pullRequest{{Number: 9, HeadRefName: "feat/ce-cccc-x", Title: "x"}}

	got := selectCandidates(beads, live, prs, 2)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %+v", len(got), got)
	}
	if got[0].ID != "ce-aaaa" {
		t.Errorf("expected ce-aaaa, got %s", got[0].ID)
	}
}

func TestSelectCandidatesPriorityBand(t *testing.T) {
	beads := []bead{
		{ID: "ce-p0", Title: "crit", Priority: 0},
		{ID: "ce-p1", Title: "imp", Priority: 1},
		{ID: "ce-p2", Title: "nice", Priority: 2},
	}
	// max-priority 0 → only P0 survives.
	if got := selectCandidates(beads, nil, nil, 0); len(got) != 1 || got[0].ID != "ce-p0" {
		t.Errorf("max-priority 0: expected [ce-p0], got %+v", got)
	}
	// max-priority 1 → P0 and P1.
	if got := selectCandidates(beads, nil, nil, 1); len(got) != 2 {
		t.Errorf("max-priority 1: expected 2 candidates, got %+v", got)
	}
	// max-priority 2 → all three.
	if got := selectCandidates(beads, nil, nil, 2); len(got) != 3 {
		t.Errorf("max-priority 2: expected 3 candidates, got %+v", got)
	}
}

func TestSelectCandidatesPriorityOrder(t *testing.T) {
	beads := []bead{
		{ID: "ce-zzzz", Title: "z", Priority: 2},
		{ID: "ce-aaaa", Title: "a", Priority: 2},
		{ID: "ce-mmmm", Title: "m", Priority: 0},
		{ID: "ce-nnnn", Title: "n", Priority: 1},
	}
	got := selectCandidates(beads, nil, nil, 2)
	want := []string{"ce-mmmm", "ce-nnnn", "ce-aaaa", "ce-zzzz"} // P0, P1, then P2 by id
	if len(got) != len(want) {
		t.Fatalf("expected %d candidates, got %d", len(want), len(got))
	}
	for i, c := range got {
		if c.ID != want[i] {
			t.Errorf("position %d: got %s, want %s", i, c.ID, want[i])
		}
	}
}

func TestRenderPrompt(t *testing.T) {
	b := bead{
		ID:          "ce-test",
		Title:       "Do the thing",
		Description: "Make it work.\n\nMore detail here that should appear in the goal but not the summary.",
		Priority:    1,
	}
	out := renderPrompt(b)

	for _, want := range []string{
		"# Worker: ce-test — Do the thing",
		"assigned to bead ce-test (P1): Make it work.",
		"/wayfinder",                                // worker drives the bead through the SDLC workflow
		"~/worktrees/dear-agent/ce-test/",           // isolated worktree off read-only ~/src
		"A bead is Done ONLY when its PR is MERGED", // merged-PR DoD
		"bd --db ~/beads/context-engine/.beads",
		"NEVER write to ~/src/**",
		"claude-opus-4-8",
		"More detail here", // full description in the Goal block
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered prompt missing %q\n---\n%s", want, out)
		}
	}
	summaryLine := strings.Split(out, "\n")[2]
	if strings.Contains(summaryLine, "More detail here") {
		t.Errorf("summary line should not contain second paragraph: %q", summaryLine)
	}
}

func TestRenderPromptEmptyDescription(t *testing.T) {
	b := bead{ID: "ce-x", Title: "Title only", Priority: 2}
	out := renderPrompt(b)
	if !strings.Contains(out, "assigned to bead ce-x (P2): Title only") {
		t.Errorf("empty description should fall back to title in summary:\n%s", out)
	}
}

func TestSessionNewArgs(t *testing.T) {
	args := sessionNewArgs("worker-ce-test", "opus-200k")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"session new worker-ce-test",
		"--detached",
		"--workspace=oss",
		"--harness=claude-code",
		"--model=opus-200k",
		"--mode=auto",
		"--role worker",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("session args missing %q: %v", want, args)
		}
	}
}

// TestDispatch verifies the spawn-then-send ordering and that a spawn failure
// short-circuits before any prompt is sent (no orphaned send to a session that
// never came up).
func TestDispatch(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	var spawned, sent string
	spawnSession = func(name, model string) error { spawned = name; return nil }
	sendPrompt = func(name, prompt string) error { sent = name; return nil }

	b := bead{ID: "ce-test", Title: "T", Priority: 1}
	if err := dispatch(b, "opus-200k"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if spawned != "worker-ce-test" {
		t.Errorf("spawned = %q, want worker-ce-test", spawned)
	}
	if sent != "worker-ce-test" {
		t.Errorf("sent = %q, want worker-ce-test", sent)
	}
}

func TestDispatchSpawnFailureSkipsSend(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	spawnSession = func(name, model string) error { return errStub }
	sendCalled := false
	sendPrompt = func(name, prompt string) error { sendCalled = true; return nil }

	if err := dispatch(bead{ID: "ce-x"}, "opus-200k"); err == nil {
		t.Error("expected error when spawn fails")
	}
	if sendCalled {
		t.Error("send must not be called when spawn fails")
	}
}

func TestExpandHome(t *testing.T) {
	home := "/home/user"
	tests := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/foo/bar", "/home/user/foo/bar"},
		{"/abs/path", "/abs/path"},
		{"relative", "relative"},
	}
	for _, tt := range tests {
		if got := expandHome(tt.in, home); got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestScrubAPIKey(t *testing.T) {
	in := []string{"PATH=/bin", "ANTHROPIC_API_KEY=secret", "HOME=/h"}
	out := scrubAPIKey(in)
	for _, e := range out {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			t.Error("ANTHROPIC_API_KEY should be scrubbed")
		}
	}
	if len(out) != 2 {
		t.Errorf("expected 2 vars after scrub, got %d", len(out))
	}
}

var errStub = stubErr("stub failure")

type stubErr string

func (e stubErr) Error() string { return string(e) }
