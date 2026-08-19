package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/vroomgate"
	"github.com/vbonnet/dear-agent/internal/vroomprompt"
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

func TestExistingPromptIDs(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"ce-bi19.md", "ce-5vje.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := existingPromptIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ids["ce-bi19"] || !ids["ce-5vje"] {
		t.Errorf("expected ce-bi19 and ce-5vje, got %v", ids)
	}
	if ids["notes"] {
		t.Error("non-.md file should be ignored")
	}
}

func TestExistingPromptIDsMissingDir(t *testing.T) {
	ids, err := existingPromptIDs(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir should not error, got %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty set, got %v", ids)
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
		{ID: "ce-bbbb", Title: "already prompted", Priority: 1},
		{ID: "ce-cccc", Title: "in PR", Priority: 1},
		{ID: "ce-cd14", Title: "human gated", Priority: 0},
		{ID: "", Title: "empty id", Priority: 1},
	}
	existing := map[string]bool{"ce-bbbb": true}
	prs := []pullRequest{{Number: 9, HeadRefName: "feat/ce-cccc-x", Title: "x"}}

	got := selectCandidates(beads, existing, prs, "/tmp/prompts")
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %+v", len(got), got)
	}
	if got[0].bead.ID != "ce-aaaa" {
		t.Errorf("expected ce-aaaa, got %s", got[0].bead.ID)
	}
	if got[0].path != "/tmp/prompts/ce-aaaa.md" {
		t.Errorf("unexpected path %s", got[0].path)
	}
}

// TestSelectCandidatesHonoursSharedHumanGate is the drift regression: this
// generator used to keep its own copy of the human-gated list, so a bead gated
// in vroom-dispatch-direct still got a prompt file materialised here and stayed
// dispatchable through the orchestrator. Both binaries now read the one list in
// internal/vroomgate, and this walks all of it rather than a hardcoded sample.
func TestSelectCandidatesHonoursSharedHumanGate(t *testing.T) {
	ids := vroomgate.IDs()
	if len(ids) == 0 {
		t.Fatal("the shared human gate list is empty; prompt generation would be ungated")
	}
	var beads []bead
	for _, id := range ids {
		beads = append(beads, bead{ID: id, Title: "gated " + id, Priority: 0})
	}
	if got := selectCandidates(beads, nil, nil, "/tmp/prompts"); len(got) != 0 {
		t.Errorf("human-gated beads must never get a generated prompt, got %+v", got)
	}
}

func TestSelectCandidatesSorted(t *testing.T) {
	beads := []bead{
		{ID: "ce-zzzz", Title: "z"},
		{ID: "ce-aaaa", Title: "a"},
		{ID: "ce-mmmm", Title: "m"},
	}
	got := selectCandidates(beads, nil, nil, "/tmp")
	want := []string{"ce-aaaa", "ce-mmmm", "ce-zzzz"}
	for i, c := range got {
		if c.bead.ID != want[i] {
			t.Errorf("position %d: got %s, want %s", i, c.bead.ID, want[i])
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
		"Bead ce-test (P1). Make it work.",
		"provider-visible PR created through safe-pr",
		"Never arm auto-merge",
		"bd --db ~/beads/context-engine/.beads --dolt-auto-commit on",
		"NEVER write to ~/src/**",
		"claude-opus-4-8",
		"More detail here", // full description in the Goal block
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered prompt missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "auto-merge armed") {
		t.Fatalf("rendered prompt retained retired safe-pr auto-arming guidance:\n%s", out)
	}
	// The summary line should be just the first paragraph, not the whole desc.
	summaryLine := strings.Split(out, "\n")[2]
	if strings.Contains(summaryLine, "More detail here") {
		t.Errorf("summary line should not contain second paragraph: %q", summaryLine)
	}
}

func TestRenderPromptEmptyDescription(t *testing.T) {
	b := bead{ID: "ce-x", Title: "Title only", Priority: 2}
	out := renderPrompt(b)
	if !strings.Contains(out, "Bead ce-x (P2). Title only") {
		t.Errorf("empty description should fall back to title in summary:\n%s", out)
	}
}

func TestRenderPromptForRoute(t *testing.T) {
	got := renderPromptForRoute(bead{ID: "ce-route", Title: "route"}, vroomprompt.Route{
		Harness: "opencode-cli", Model: "qwen", Mode: "auto", Workspace: "oss",
	})
	for _, want := range []string{"harness=opencode-cli", "model=qwen", "--mode=auto", "--workspace=oss"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "safe-push") || strings.Contains(got, "git push") {
		t.Fatalf("prompt must require safe-push without raw git-push guidance:\n%s", got)
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
