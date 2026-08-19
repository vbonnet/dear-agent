package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/vroomgate"
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

func TestOccupiedWorkerIDs(t *testing.T) {
	// Mimics `agm session list` output: arbitrary columns after the session name.
	// worker-ce-cd14.2 is a legacy pre-sanitization session (dotted name); the
	// returned set holds NORMALIZED ids, so it appears as ce-cd14-2.
	lines := []string{
		"NAME                       STATUS   MODEL",
		"worker-ce-bi19             running  opus-200k",
		"worker-ce-cd14.2           running  opus-200k",
		"vroom-orchestrator         running  sonnet-200k",
		"some-other-session         idle     -",
		"", // trailing blank from the split
	}
	ids := occupiedWorkerIDs(lines)
	if !ids["ce-bi19"] {
		t.Error("expected ce-bi19 to be live")
	}
	if !ids["ce-cd14-2"] {
		t.Error("expected sub-bead session worker-ce-cd14.2 to be live under its normalized id ce-cd14-2")
	}
	if ids["ce-cd14"] {
		t.Error("parent ce-cd14 should NOT be considered live from a ce-cd14.2 session")
	}
	if len(ids) != 2 {
		t.Errorf("expected exactly 2 live worker ids, got %d: %v", len(ids), ids)
	}
}

// TestOccupiedWorkerIDsJSONFormat guards dedup against `agm session list`'s
// agent-mode JSON output (the default in the non-TTY dispatch context), where a
// worker name follows a quote — `"name":"worker-ce-24f1"` — not whitespace. A
// regex anchored only to whitespace saw 0 occupied workers, so the dispatcher
// re-selected already-live beads and collision-aborted every tick.
func TestOccupiedWorkerIDsJSONFormat(t *testing.T) {
	// One line, as agm emits it: worker names embedded in JSON, alongside a
	// supervisor session (vroom-overseer), a "subworker-" that must NOT match,
	// and a non-name field ("harness":"worker-harness") that must NOT be misread
	// as a live worker id (the structural parse reads only the name field).
	line := `{"operation":"list_sessions","sessions":[{"name":"worker-ce-24f1","status":"active","harness":"worker-harness"},{"name":"worker-ce-py3x","status":"active"},{"name":"worker-ce-dead","status":"zombie"},{"name":"worker-ce-stopped","status":"stopped"},{"name":"worker-ce-archived","status":"archived"},{"name":"vroom-overseer","status":"active"},{"name":"subworker-ce-nope","status":"active"}]}`
	ids := occupiedWorkerIDs([]string{line})
	if !ids["ce-24f1"] {
		t.Error("expected ce-24f1 to be live from JSON output")
	}
	if !ids["ce-py3x"] {
		t.Error("expected ce-py3x to be live from JSON output")
	}
	if ids["ce-nope"] {
		t.Error("subworker-ce-nope must NOT be read as a live worker")
	}
	if !ids["ce-dead"] {
		t.Error("zombie worker-ce-dead must occupy its non-archived session name")
	}
	if !ids["ce-stopped"] {
		t.Error("stopped worker-ce-stopped must occupy its non-archived session name")
	}
	if ids["ce-archived"] {
		t.Error("archived worker-ce-archived must release its session name")
	}
	if ids["harness"] {
		t.Error(`"harness":"worker-harness" is a non-name field and must NOT be read as a live worker`)
	}
	if len(ids) != 4 {
		t.Errorf("expected exactly 4 occupied worker ids from JSON, got %d: %v", len(ids), ids)
	}
}

func TestOccupiedWorkerIDsLegacyTextIncludesZombie(t *testing.T) {
	ids := occupiedWorkerIDs([]string{
		"worker-ce-live running opus-200k",
		"worker-ce-dead zombie opus-200k",
		"worker-ce-stopped stopped opus-200k",
		"worker-ce-archived archived opus-200k",
	})
	if !ids["ce-live"] || !ids["ce-dead"] || !ids["ce-stopped"] || ids["ce-archived"] {
		t.Fatalf("occupied worker ids = %v, want every non-archived name", ids)
	}
}

func TestNormalizeSessionID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ce-6as.10", "ce-6as-10"},               // dotted sub-bead (the ce-b1zw trigger)
		{"ce-xulg.14", "ce-xulg-14"},             // another dotted sub-bead
		{"ce-bi19", "ce-bi19"},                   // already safe: unchanged
		{"a.b:c d", "a-b-c-d"},                   // dots, colons, spaces all normalize
		{"worker-ce-6as-10", "worker-ce-6as-10"}, // idempotent on normalized input
	}
	for _, tt := range tests {
		if got := normalizeSessionID(tt.in); got != tt.want {
			t.Errorf("normalizeSessionID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWorkerSessionName(t *testing.T) {
	if got := workerSessionName("ce-6as.10"); got != "worker-ce-6as-10" {
		t.Errorf("workerSessionName(ce-6as.10) = %q, want worker-ce-6as-10", got)
	}
	if got := workerSessionName("ce-bi19"); got != "worker-ce-bi19" {
		t.Errorf("workerSessionName(ce-bi19) = %q, want worker-ce-bi19", got)
	}
}

// TestDottedIDDedupBothDirections proves dedup survives the dot→dash rename in
// BOTH directions (ce-b1zw): a sanitized live session dedups the dotted bead,
// and a legacy dotted live session dedups the same bead.
func TestDottedIDDedupBothDirections(t *testing.T) {
	beads := []bead{{ID: "ce-xyz.3", Title: "dotted sub-bead", Priority: 0}}

	// Direction 1: live session already uses the sanitized name.
	live := occupiedWorkerIDs([]string{"worker-ce-xyz-3   running  opus-200k"})
	if got := selectCandidates(beads, live, nil, 2); len(got) != 0 {
		t.Errorf("sanitized session worker-ce-xyz-3 must dedup bead ce-xyz.3, got candidates %+v", got)
	}

	// Direction 2: legacy live session still carries the dotted name.
	live = occupiedWorkerIDs([]string{"worker-ce-xyz.3   running  opus-200k"})
	if got := selectCandidates(beads, live, nil, 2); len(got) != 0 {
		t.Errorf("legacy dotted session worker-ce-xyz.3 must dedup bead ce-xyz.3, got candidates %+v", got)
	}

	// No live session at all: the bead must remain eligible.
	if got := selectCandidates(beads, nil, nil, 2); len(got) != 1 {
		t.Errorf("with no live worker, bead ce-xyz.3 must stay eligible, got %+v", got)
	}
}

func TestOccupiedWorkerIDsEmpty(t *testing.T) {
	if ids := occupiedWorkerIDs(nil); len(ids) != 0 {
		t.Errorf("nil input should yield empty set, got %v", ids)
	}
}

func TestListSessionsRetrievesEveryPage(t *testing.T) {
	original := listSessionPage
	t.Cleanup(func() { listSessionPage = original })
	var offsets []int
	listSessionPage = func(_ context.Context, offset int) (sessionListPayload, error) {
		offsets = append(offsets, offset)
		if offset == 0 {
			sessions := make([]sessionNameStatus, sessionListPageSize)
			for index := range sessions {
				sessions[index] = sessionNameStatus{Name: fmt.Sprintf("archived-%d", index), Status: "archived"}
			}
			sessions[sessionListPageSize-1] = sessionNameStatus{Name: "worker-ce-first", Status: "stopped"}
			return sessionListPayload{Sessions: sessions}, nil
		}
		return sessionListPayload{Sessions: []sessionNameStatus{{Name: "worker-ce-second", Status: "zombie"}}}, nil
	}

	lines, err := listSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := occupiedWorkerIDs(lines)
	if len(offsets) != 2 || offsets[0] != 0 || offsets[1] != sessionListPageSize {
		t.Fatalf("page offsets = %v, want [0 %d]", offsets, sessionListPageSize)
	}
	if !ids["ce-first"] || !ids["ce-second"] {
		t.Fatalf("occupied ids = %v, want workers from both pages", ids)
	}
}

func TestSessionListArgsRequestStablePagination(t *testing.T) {
	args := sessionListArgs(sessionListPageSize)
	if !slices.Contains(args, "--stable-order") {
		t.Fatalf("session list args = %v, want --stable-order", args)
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

func TestHumanGatedSkipsGmailOAuthBead(t *testing.T) {
	if !vroomgate.IsHumanGated("ce-6as.10") {
		t.Fatal("ce-6as.10 (interactive Gmail OAuth re-consent) must be human-gated")
	}
	beads := []bead{{ID: "ce-6as.10", Title: "Gmail OAuth re-consent", Priority: 0}}
	if got := selectCandidates(beads, nil, nil, 2); len(got) != 0 {
		t.Errorf("human-gated ce-6as.10 must never be a dispatch candidate, got %+v", got)
	}
}

// TestHumanGatedBeadsAreNeverCandidates walks the whole shared gate list rather
// than a hardcoded copy of it: enumerating vroomgate.IDs() covers every gated
// bead (not just a representative one) and, because the list is derived rather
// than duplicated, lifting an individual gate never forces a test edit.
func TestHumanGatedBeadsAreNeverCandidates(t *testing.T) {
	ids := vroomgate.IDs()
	if len(ids) == 0 {
		t.Fatal("the shared human gate list is empty; dispatch would be ungated")
	}
	var beads []bead
	for _, id := range ids {
		beads = append(beads, bead{ID: id, Title: "gated " + id, Priority: 0})
	}
	if got := selectCandidates(beads, nil, nil, 2); len(got) != 0 {
		t.Errorf("human-gated beads must never be dispatch candidates, got %+v", got)
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
		"Wayfinder V2 lifecycle",
		"~/worktrees/dear-agent/ce-test/",
		"~/worktrees/engram-research/ce-test/",
		"do not run `git worktree add`",
		"PR creation is not done",
		"MERGED",
		"DEPLOYED",
		"VERIFIED",
		"deployment: N/A",
		"DONE_WITH_CONCERNS",
		"bd --db ~/beads/context-engine/.beads",
		"NEVER write to ~/src/**",
		"safe-pr create --wayfinder",
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
	for _, providerToken := range []string{"claude-opus", "Anthropic API", "Claude credential"} {
		if strings.Contains(out, providerToken) {
			t.Errorf("worker prompt must be harness-neutral; found %q", providerToken)
		}
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
	args := sessionNewArgs("worker-ce-test", testWorkerLaunchConfig(), "/repo")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"session new worker-ce-test",
		"--detached",
		"--workspace=oss",
		"--harness=claude-code",
		"--model=opus-200k",
		"--mode=auto",
		"--role worker",
		"--directory /repo",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("session args missing %q: %v", want, args)
		}
	}
}

func TestSessionNewArgsSupportsNonClaudeHarness(t *testing.T) {
	cfg := workerLaunchConfig{
		Harness:   "codex-cli",
		Model:     "gpt-5.6-codex",
		Mode:      "auto",
		Workspace: "oss",
	}
	joined := strings.Join(sessionNewArgs("worker-ce-test", cfg, "/repo"), " ")
	for _, want := range []string{"--harness=codex-cli", "--model=gpt-5.6-codex", "--mode=auto"} {
		if !strings.Contains(joined, want) {
			t.Errorf("non-Claude session args missing %q: %s", want, joined)
		}
	}
}

func TestTrustedAddDirsEnvironmentBindsPayloadToSession(t *testing.T) {
	got, err := trustedAddDirsEnvironment("worker-ce-test", []string{"/worktree/one", "/beads"}, "/etc/codex/hooks/worker-guard")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`AGM_TRUSTED_ADD_DIRS_JSON=["/worktree/one","/beads"]`,
		"AGM_TRUSTED_ADD_DIRS_SESSION=worker-ce-test",
		"AGM_TRUSTED_GUARD_PATH=/etc/codex/hooks/worker-guard",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("trustedAddDirsEnvironment() = %v, want %v", got, want)
	}
}

func TestPrepareWorkerWorkspacePrecreatesTaskOwnedGitState(t *testing.T) {
	root := t.TempDir()
	dearRepo := filepath.Join(root, "dear-agent")
	engramRepo := filepath.Join(root, "engram-research")
	initGitRepo(t, dearRepo)
	initGitRepo(t, engramRepo)
	beadsDir := filepath.Join(root, "beads", ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testWorkerLaunchConfig()
	cfg.BeadsDir = beadsDir
	cfg.Worktrees = filepath.Join(root, "worktrees")
	cfg.GitState = filepath.Join(root, "worker-git")
	cfg.EngramRepo = engramRepo

	addDirs, err := prepareWorkerWorkspace(t.Context(), "ce-test.1", cfg, dearRepo)
	if err != nil {
		t.Fatalf("prepareWorkerWorkspace() error: %v", err)
	}
	if _, err := prepareWorkerWorkspace(t.Context(), "ce-test.1", cfg, dearRepo); err != nil {
		t.Fatalf("prepareWorkerWorkspace() idempotent call: %v", err)
	}
	dearCommon, err := gitOutput(t.Context(), dearRepo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	engramCommon, err := gitOutput(t.Context(), engramRepo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{
		filepath.Join(cfg.Worktrees, "dear-agent", "ce-test.1"),
		filepath.Join(cfg.Worktrees, "engram-research", "ce-test.1"),
	}
	for _, target := range targets {
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("prepared worktree %s: %v", target, err)
		}
	}
	for _, forbidden := range []string{
		dearCommon,
		filepath.Join(dearCommon, "config"),
		filepath.Join(dearCommon, "hooks"),
		engramCommon,
	} {
		if slices.Contains(addDirs, forbidden) {
			t.Fatalf("prepared grants include forbidden Git control path %s: %v", forbidden, addDirs)
		}
	}
	for _, want := range []string{
		beadsDir,
		targets[0],
		targets[1],
		filepath.Join(cfg.GitState, "dear-agent", "ce-test.1.git"),
		filepath.Join(cfg.GitState, "engram-research", "ce-test.1.git"),
	} {
		if !slices.Contains(addDirs, want) {
			t.Fatalf("prepared grants missing %s: %v", want, addDirs)
		}
	}
	for _, target := range targets {
		if err := os.WriteFile(filepath.Join(target, "worker.txt"), []byte("worker\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", "worker.txt"}, {"commit", "-m", "worker commit"}} {
			if _, err := gitOutput(t.Context(), target, args...); err != nil {
				t.Fatalf("git %v in prepared worktree: %v", args, err)
			}
		}
		common, err := gitOutput(t.Context(), target, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			t.Fatal(err)
		}
		resolvedBase, err := filepath.EvalSymlinks(cfg.GitState)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(common, resolvedBase+string(os.PathSeparator)) {
			t.Fatalf("prepared worktree common Git dir = %s, want under %s", common, cfg.GitState)
		}
	}
}

func initGitRepo(t *testing.T, path string) {
	t.Helper()
	sandbox := gittest.New(t)
	sandbox.InitRepo(t, path)
	sandbox.Run(t, path, "remote", "add", "origin", "https://example.invalid/"+filepath.Base(path)+".git")
}

func testWorkerLaunchConfig() workerLaunchConfig {
	return workerLaunchConfig{
		Harness:   defaultHarness,
		Model:     defaultModel,
		Mode:      defaultMode,
		Workspace: defaultWorkspace,
	}
}

// TestSessionNewArgsAlwaysPassesDirectory guards against the launchd
// spawn-hang (ce-fmxv): --directory must always be set explicitly so
// `agm session new` never falls back to an inherited (or absent) cwd.
func TestSessionNewArgsAlwaysPassesDirectory(t *testing.T) {
	args := sessionNewArgs("worker-ce-test", testWorkerLaunchConfig(), "/home/user/src/dear-agent")
	for i, a := range args {
		if a == "--directory" {
			if i+1 >= len(args) || args[i+1] != "/home/user/src/dear-agent" {
				t.Fatalf("--directory not followed by repoDir: %v", args)
			}
			return
		}
	}
	t.Fatalf("--directory flag missing from session args: %v", args)
}

// TestDispatch verifies the spawn-then-send ordering and that a spawn failure
// short-circuits before any prompt is sent (no orphaned send to a session that
// never came up).
func TestDispatch(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	var spawned, sent string
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawned = name
		return nil
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error { sent = name; return nil }

	b := bead{ID: "ce-test", Title: "T", Priority: 1}
	if err := dispatch(context.Background(), b, testWorkerLaunchConfig(), "/repo"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if spawned != "worker-ce-test" {
		t.Errorf("spawned = %q, want worker-ce-test", spawned)
	}
	if sent != "worker-ce-test" {
		t.Errorf("sent = %q, want worker-ce-test", sent)
	}
}

func TestDispatchPreparedWorkspaceBecomesSessionDirectory(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	root := t.TempDir()
	repo := filepath.Join(root, "dear-agent")
	initGitRepo(t, repo)
	beadsDir := filepath.Join(root, "beads", ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testWorkerLaunchConfig()
	cfg.BeadsDir = beadsDir
	cfg.Worktrees = filepath.Join(root, "worktrees")
	cfg.GitState = filepath.Join(root, "worker-git")

	var spawnedDir string
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawnedDir = repoDir
		return nil
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	if err := dispatch(context.Background(), bead{ID: "ce-test", Title: "T"}, cfg, repo); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cfg.Worktrees, "dear-agent", "ce-test")
	if spawnedDir != want {
		t.Fatalf("spawned directory = %q, want prepared worktree %q", spawnedDir, want)
	}
}

func TestDispatchSpawnFailureSkipsSend(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error { return errStub }
	sendCalled := false
	sendPrompt = func(ctx context.Context, name, prompt string) error { sendCalled = true; return nil }

	if err := dispatch(context.Background(), bead{ID: "ce-x"}, testWorkerLaunchConfig(), "/repo"); err == nil {
		t.Error("expected error when spawn fails")
	}
	if sendCalled {
		t.Error("send must not be called when spawn fails")
	}
}

// TestDispatchDottedBeadSpawnsSanitizedName verifies a dotted bead id spawns
// (and messages) the tmux-safe session name while the prompt keeps the real
// bead id for bd commands.
func TestDispatchDottedBeadSpawnsSanitizedName(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	var spawned, sent, sentPrompt string
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawned = name
		return nil
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error {
		sent, sentPrompt = name, prompt
		return nil
	}

	b := bead{ID: "ce-xyz.3", Title: "dotted", Priority: 0}
	if err := dispatch(context.Background(), b, testWorkerLaunchConfig(), "/repo"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if spawned != "worker-ce-xyz-3" {
		t.Errorf("spawned = %q, want sanitized worker-ce-xyz-3", spawned)
	}
	if sent != "worker-ce-xyz-3" {
		t.Errorf("sent to = %q, want sanitized worker-ce-xyz-3", sent)
	}
	if !strings.Contains(sentPrompt, "ce-xyz.3") {
		t.Error("prompt must reference the real (dotted) bead id for bd commands")
	}
}

func TestIsDeterministicSpawnFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"circuit breaker backpressure", fmt.Errorf("circuit breaker: spawn refused"), false},
		{"spawn refused", fmt.Errorf("agm: spawn refused (at capacity)"), false},
		{"timeout", fmt.Errorf("agm session new: %w", context.DeadlineExceeded), false},
		{"canceled", fmt.Errorf("agm session new: %w", context.Canceled), false},
		{"unknown error stays fail-closed", fmt.Errorf("exit status 1\nsomething novel"), false},
		{"no TTY (interactive prompt in detached context)",
			fmt.Errorf("exit status 1\ncould not open a new TTY: open /dev/tty: device not configured"), true},
		{"invalid session name", fmt.Errorf("exit status 1\nInvalid session name"), true},
		{"unsafe characters", fmt.Errorf("exit status 1\nsession name contains unsafe characters"), true},
		{"invalid bead", fmt.Errorf("exit status 1\ninvalid bead id"), true},
		{"backpressure wins over signature text",
			fmt.Errorf("circuit breaker: spawn refused (invalid session name mentioned in passing)"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDeterministicSpawnFailure(tt.err); got != tt.want {
				t.Errorf("isDeterministicSpawnFailure(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestDispatchCandidates_SkipsDeterministicSpawnFailure verifies a per-bead
// deterministic spawn failure (bad name / no-TTY prompt) skips that bead and
// keeps the run going — one poisoned bead must not stall the whole queue
// (ce-b1zw: 0/17 dispatched).
func TestDispatchCandidates_SkipsDeterministicSpawnFailure(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	var spawnedNames []string
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawnedNames = append(spawnedNames, name)
		if name == "worker-ce-2" {
			return fmt.Errorf("exit status 1\ncould not open a new TTY")
		}
		return nil
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	candidates := []bead{
		{ID: "ce-1", Title: "one", Priority: 1},
		{ID: "ce-2", Title: "poisoned", Priority: 1},
		{ID: "ce-3", Title: "three", Priority: 1},
	}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", 0, false, &out, &errOut, nil)
	if got != 2 {
		t.Errorf("dispatched = %d, want 2 (skip the poisoned bead, keep going)\nstderr:\n%s", got, errOut.String())
	}
	if len(spawnedNames) != 3 {
		t.Errorf("spawn attempts = %d, want 3 (run continues past the failure): %v", len(spawnedNames), spawnedNames)
	}
	if !strings.Contains(errOut.String(), "skip ce-2") {
		t.Errorf("stderr should log the per-bead skip, got:\n%s", errOut.String())
	}
}

// TestDispatchCandidates_StopsOnBackpressure preserves the original fail-closed
// behavior: a circuit-breaker refusal (capacity exhausted) stops the run early.
func TestDispatchCandidates_StopsOnBackpressure(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	spawnCalls := 0
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawnCalls++
		return fmt.Errorf("circuit breaker: spawn refused")
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	candidates := []bead{
		{ID: "ce-1", Title: "one", Priority: 1},
		{ID: "ce-2", Title: "two", Priority: 1},
	}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", 0, false, &out, &errOut, nil)
	if got != 0 {
		t.Errorf("dispatched = %d, want 0 on backpressure", got)
	}
	if spawnCalls != 1 {
		t.Errorf("spawn attempts = %d, want 1 (backpressure stops the run)", spawnCalls)
	}
}

// TestDispatchCandidates_StopsOnUnknownError keeps unknown spawn errors
// fail-closed: stop the run rather than risk skipping through a systemic fault.
func TestDispatchCandidates_StopsOnUnknownError(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	spawnCalls := 0
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawnCalls++
		return fmt.Errorf("exit status 1\nsomething never seen before")
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	candidates := []bead{
		{ID: "ce-1", Title: "one", Priority: 1},
		{ID: "ce-2", Title: "two", Priority: 1},
	}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", 0, false, &out, &errOut, nil)
	if got != 0 {
		t.Errorf("dispatched = %d, want 0 on unknown error", got)
	}
	if spawnCalls != 1 {
		t.Errorf("spawn attempts = %d, want 1 (unknown errors stop the run)", spawnCalls)
	}
}

// TestDispatchCandidates_CapsSuccessfulDispatches checks the -max-dispatch cap
// bounds the run and defers the rest rather than dispatching them.
func TestDispatchCandidates_CapsSuccessfulDispatches(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	var spawnedNames []string
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawnedNames = append(spawnedNames, name)
		return nil
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	candidates := []bead{
		{ID: "ce-1", Title: "one", Priority: 0},
		{ID: "ce-2", Title: "two", Priority: 0},
		{ID: "ce-3", Title: "three", Priority: 1},
	}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", 2, false, &out, &errOut, nil)
	if got != 2 {
		t.Errorf("dispatched = %d, want 2 (-max-dispatch=2)", got)
	}
	if len(spawnedNames) != 2 || spawnedNames[0] != "worker-ce-1" || spawnedNames[1] != "worker-ce-2" {
		t.Errorf("spawned = %v, want the 2 highest-priority beads only", spawnedNames)
	}
	if !strings.Contains(errOut.String(), "deferring 1 remaining eligible bead") {
		t.Errorf("stderr should report the deferred remainder, got:\n%s", errOut.String())
	}
}

// TestDispatchCandidates_CapCountsOnlySuccesses is the starvation regression:
// with -max-dispatch=1 and a poisoned highest-priority bead, a cap applied by
// truncating the candidate list up front would spend the entire budget on the
// bead that can never spawn, dispatch nothing, and repeat that on every
// scheduled run — permanently starving the queue (the ce-b1zw failure mode).
// The cap must count successful dispatches, so the run walks past the skip.
func TestDispatchCandidates_CapCountsOnlySuccesses(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	var spawnedNames []string
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
		spawnedNames = append(spawnedNames, name)
		if name == "worker-ce-poison" {
			return fmt.Errorf("exit status 1\ncould not open a new TTY")
		}
		return nil
	}
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	candidates := []bead{
		{ID: "ce-poison", Title: "poisoned P0", Priority: 0},
		{ID: "ce-good", Title: "healthy P0", Priority: 0},
		{ID: "ce-later", Title: "P1", Priority: 1},
	}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", 1, false, &out, &errOut, nil)
	if got != 1 {
		t.Fatalf("dispatched = %d, want 1 (the skipped bead must not consume the cap)\nstderr:\n%s", got, errOut.String())
	}
	if len(spawnedNames) != 2 || spawnedNames[1] != "worker-ce-good" {
		t.Errorf("spawned = %v, want the poisoned bead skipped and ce-good dispatched", spawnedNames)
	}
}

// TestDispatchCandidates_UncappedByDefault pins VDD-11: 0 (the flag default)
// and any non-positive value leave dispatch unlimited.
func TestDispatchCandidates_UncappedByDefault(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()

	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error { return nil }
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	candidates := []bead{
		{ID: "ce-1", Title: "one", Priority: 0},
		{ID: "ce-2", Title: "two", Priority: 0},
		{ID: "ce-3", Title: "three", Priority: 1},
	}
	for _, maxDispatch := range []int{0, 5} {
		var out, errOut bytes.Buffer
		if got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", maxDispatch, false, &out, &errOut, nil); got != 3 {
			t.Errorf("maxDispatch=%d: dispatched = %d, want 3", maxDispatch, got)
		}
	}
}

// TestSessionSpawnTimeoutIsGenerous pins the spawn-specific timeout: `agm
// session new` boots the whole harness and legitimately exceeds the blanket
// 60s subprocess bound, so it gets its own, larger budget.
func TestSessionSpawnTimeoutIsGenerous(t *testing.T) {
	if sessionSpawnTimeout <= subprocessTimeout {
		t.Errorf("sessionSpawnTimeout (%v) must exceed subprocessTimeout (%v)",
			sessionSpawnTimeout, subprocessTimeout)
	}
	if sessionSpawnTimeout < 180*time.Second {
		t.Errorf("sessionSpawnTimeout (%v) should be at least 180s for harness boot", sessionSpawnTimeout)
	}
}

func TestDispatchCandidates_DryRunHasNoWorkerCap(t *testing.T) {
	candidates := []bead{
		{ID: "ce-1", Title: "one", Priority: 1},
		{ID: "ce-2", Title: "two", Priority: 1},
		{ID: "ce-3", Title: "three", Priority: 1},
		{ID: "ce-4", Title: "four", Priority: 1},
	}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", 0, true, &out, &errOut, nil)
	if got != len(candidates) {
		t.Fatalf("dry-run dispatched %d candidates, want all %d; output:\n%s", got, len(candidates), out.String())
	}
	if strings.Contains(out.String(), "cap") || strings.Contains(errOut.String(), "cap") {
		t.Fatalf("dispatcher output should not report a worker cap\nstdout:\n%s\nstderr:\n%s", out.String(), errOut.String())
	}
}

func TestDispatchCandidates_StopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	candidates := []bead{
		{ID: "ce-1", Title: "one", Priority: 1},
		{ID: "ce-2", Title: "two", Priority: 1},
	}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(ctx, candidates, testWorkerLaunchConfig(), "/repo", 0, true, &out, &errOut, nil)
	if got != 0 {
		t.Fatalf("dispatchCandidates dispatched %d candidates after context cancellation; output:\n%s", got, out.String())
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("canceled dispatch should not write output\nstdout:\n%s\nstderr:\n%s", out.String(), errOut.String())
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
