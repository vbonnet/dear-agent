package session

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()

	// Check git is available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Command(t, dir, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	return dir
}

func commitFile(t *testing.T, dir, filename, content, message string) {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := gittest.Command(t, dir, "add", filename)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = gittest.Command(t, dir, "commit", "-m", message)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func TestVerifyCompletion_NoGitRepo(t *testing.T) {
	dir := t.TempDir()
	result := VerifyCompletion(dir)

	if result.HasCodeChanges {
		t.Error("expected HasCodeChanges=false for non-git dir")
	}
	if result.HasTestChanges {
		t.Error("expected HasTestChanges=false for non-git dir")
	}
	if len(result.DeferralWarnings) != 0 {
		t.Errorf("expected no deferral warnings, got %v", result.DeferralWarnings)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", result.Warnings)
	}
}

func TestVerifyCompletion_EmptyRepo(t *testing.T) {
	dir := setupTestRepo(t)
	result := VerifyCompletion(dir)

	if result.HasCodeChanges {
		t.Error("expected HasCodeChanges=false for empty repo")
	}
	if result.HasTestChanges {
		t.Error("expected HasTestChanges=false for empty repo")
	}
}

func TestVerifyCompletion_WithCodeAndTestChanges(t *testing.T) {
	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "Add main package")
	commitFile(t, dir, "main_test.go", "package main\n", "Add main tests")

	result := VerifyCompletion(dir)

	if !result.HasCodeChanges {
		t.Error("expected HasCodeChanges=true")
	}
	if !result.HasTestChanges {
		t.Error("expected HasTestChanges=true")
	}
	// No deferral language in clean messages
	if len(result.DeferralWarnings) != 0 {
		t.Errorf("expected no deferral warnings, got %v", result.DeferralWarnings)
	}
}

func TestVerifyCompletion_DeferralLanguageDetection(t *testing.T) {
	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "WIP: partial implementation TODO fix later")
	commitFile(t, dir, "hack.go", "package main\n// hack\n", "HACK: temporary workaround")

	result := VerifyCompletion(dir)

	if len(result.DeferralWarnings) != 2 {
		t.Fatalf("expected 2 deferral warnings, got %d: %v", len(result.DeferralWarnings), result.DeferralWarnings)
	}
	// Verify warnings summary mentions deferral
	found := false
	for _, w := range result.Warnings {
		if w == "deferral language detected in commit messages" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected deferral warning in Warnings, got %v", result.Warnings)
	}
}

func TestVerifyCompletion_NoDeferralLanguage(t *testing.T) {
	dir := setupTestRepo(t)
	commitFile(t, dir, "auth.go", "package auth\n", "Add user authentication")
	commitFile(t, dir, "auth_test.go", "package auth\n", "Add auth tests")

	result := VerifyCompletion(dir)

	if len(result.DeferralWarnings) != 0 {
		t.Errorf("expected no deferral warnings, got %v", result.DeferralWarnings)
	}
	if !result.HasCodeChanges {
		t.Error("expected HasCodeChanges=true")
	}
	if !result.HasTestChanges {
		t.Error("expected HasTestChanges=true")
	}
	// No warnings expected when both code and tests exist and no deferral
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", result.Warnings)
	}
}

func TestVerifyCompletion_EmptyDir(t *testing.T) {
	result := VerifyCompletion("")
	if result.HasCodeChanges || result.HasTestChanges {
		t.Error("expected false for empty dir")
	}
}

func TestVerifyCompletion_CodeOnlyNoTests(t *testing.T) {
	dir := setupTestRepo(t)
	commitFile(t, dir, "server.go", "package main\n", "Add server")

	result := VerifyCompletion(dir)

	if !result.HasCodeChanges {
		t.Error("expected HasCodeChanges=true")
	}
	if result.HasTestChanges {
		t.Error("expected HasTestChanges=false")
	}
	// Should warn about missing tests
	found := false
	for _, w := range result.Warnings {
		if w == "no test changes detected in recent commits" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'no test changes' warning, got %v", result.Warnings)
	}
}

// --- Blocking behavior tests ---

func TestVerifyCompletion_UncommittedChangesBlock(t *testing.T) {
	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "Initial commit")

	// Create an uncommitted file
	if err := os.WriteFile(filepath.Join(dir, "dirty.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := VerifyCompletion(dir)

	if len(result.UncommittedFiles) == 0 {
		t.Fatal("expected UncommittedFiles to be non-empty")
	}
	if !result.Critical() {
		t.Error("expected Critical()=true with uncommitted files")
	}
	errs := result.CriticalErrors()
	found := false
	for _, e := range errs {
		if e == "uncommitted changes in 1 file(s)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected uncommitted changes error, got %v", errs)
	}
}

// stubOpenPR overrides the package-level openPRForBranch seam for the
// duration of the test so PR-aware checks never shell out to a real `gh`
// (no network/auth dependency, deterministic, fast).
func stubOpenPR(t *testing.T, fn func(repoPath, branch string) (int, bool)) {
	t.Helper()
	original := openPRForBranch
	openPRForBranch = fn
	t.Cleanup(func() { openPRForBranch = original })
}

// stubUnpushed drives the pushed-state probe. Test repositories have no
// remote, so the real probe reports every commit as unpushed; a test that
// wants to exercise the open-PR downgrade must say the branch is pushed.
func stubUnpushed(t *testing.T, fn func(worktreePath, branch string) (bool, error)) {
	t.Helper()
	original := hasUnpushedCommits
	hasUnpushedCommits = fn
	t.Cleanup(func() { hasUnpushedCommits = original })
}

func TestVerifyCompletion_UnmergedBranchBlocks(t *testing.T) {
	stubOpenPR(t, func(string, string) (int, bool) { return 0, false })

	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "Initial commit")

	// Create main branch, then a feature branch with extra commits
	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Command(t, dir, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Rename default branch to main
	run("branch", "-M", "main")
	// Create feature branch
	run("checkout", "-b", "feature")
	commitFile(t, dir, "feature.go", "package main\n", "Feature work")
	commitFile(t, dir, "feature_test.go", "package main\n", "Feature tests")

	result := VerifyCompletion(dir)

	if len(result.UnmergedCommits) == 0 {
		t.Fatal("expected UnmergedCommits to be non-empty")
	}
	if result.HasOpenPR {
		t.Error("expected HasOpenPR=false when no PR is known for the branch")
	}
	if !result.Critical() {
		t.Error("expected Critical()=true with unmerged commits and no open PR")
	}
	errs := result.CriticalErrors()
	found := false
	for _, e := range errs {
		if e == "branch has 2 unmerged commit(s)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unmerged commits error, got %v", errs)
	}
}

// TestVerifyCompletion_UnmergedWithOpenPRNotCritical covers gap #2 from the
// worktree-sprawl retro (ce-93lw.27): unmerged commits on a branch that has
// a confirmed open PR are tracked, in-flight work — not abandoned — so they
// must not hard-block archive.
func TestVerifyCompletion_UnmergedWithOpenPRNotCritical(t *testing.T) {
	stubOpenPR(t, func(_, branch string) (int, bool) {
		if branch == "feature" {
			return 7, true
		}
		return 0, false
	})
	// Fully pushed: the PR demonstrably covers every local commit.
	stubUnpushed(t, func(string, string) (bool, error) { return false, nil })

	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "Initial commit")

	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Command(t, dir, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("branch", "-M", "main")
	run("checkout", "-b", "feature")
	commitFile(t, dir, "feature.go", "package main\n", "Feature work")
	commitFile(t, dir, "feature_test.go", "package main\n", "Feature tests")

	result := VerifyCompletion(dir)

	if len(result.UnmergedCommits) == 0 {
		t.Fatal("expected UnmergedCommits to be non-empty")
	}
	if !result.HasOpenPR {
		t.Fatal("expected HasOpenPR=true when the branch has a confirmed open PR")
	}
	if result.OpenPRNumber != 7 {
		t.Errorf("expected OpenPRNumber=7, got %d", result.OpenPRNumber)
	}
	if result.Critical() {
		t.Errorf("expected Critical()=false when unmerged commits are covered by an open PR, got errors: %v", result.CriticalErrors())
	}
	if len(result.CriticalErrors()) != 0 {
		t.Errorf("expected no critical errors, got %v", result.CriticalErrors())
	}
	foundWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "open PR #7") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected a non-blocking warning mentioning the open PR, got %v", result.Warnings)
	}
}

// TestVerifyCompletion_FailClosedOnUnresolvableBase covers gap #1 from the
// retro: the old implementation hard-coded `git log main..HEAD` and silently
// returned a clean result on any git error (fail OPEN). A branch whose base
// cannot be resolved at all (no origin, no local "main") must fail CLOSED —
// treated as unmerged — rather than silently reporting no unmerged commits.
func TestVerifyCompletion_FailClosedOnUnresolvableBase(t *testing.T) {
	stubOpenPR(t, func(string, string) (int, bool) { return 0, false })

	dir := setupTestRepo(t)
	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Command(t, dir, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	// Switch off the default branch before the first commit, so no "main"
	// branch is ever created and there is no origin remote to fall back to
	// — ResolveBaseRef has nothing to resolve.
	run("checkout", "-b", "solo-work")
	commitFile(t, dir, "server.go", "package main\n", "Add server")
	commitFile(t, dir, "server_test.go", "package main\n", "Add server tests")

	result := VerifyCompletion(dir)

	if len(result.VerificationFailures) == 0 {
		t.Fatal("expected a VerificationFailures entry when the base ref cannot be resolved")
	}
	if len(result.UnmergedCommits) != 0 {
		t.Errorf("a probe failure must not be reported as a commit count, got %v", result.UnmergedCommits)
	}
	if !result.Critical() {
		t.Error("expected Critical()=true when the unmerged check fails closed")
	}
	// The distinction is the point: the operator must be told that base
	// resolution failed and that fetching is the recovery, not sent looking
	// for a commit that does not exist.
	errs := result.CriticalErrors()
	joined := strings.Join(errs, " | ")
	if !strings.Contains(joined, "could not verify branch state") {
		t.Errorf("expected the failure to be labelled as a verification failure, got %v", errs)
	}
	if !strings.Contains(joined, "base branch is unresolvable") {
		t.Errorf("expected the failure to name base resolution as the cause, got %v", errs)
	}
	if strings.Contains(joined, "unmerged commit(s)") {
		t.Errorf("a base-resolution failure must not masquerade as a commit count, got %v", errs)
	}
}

// TestVerifyCompletion_OpenPRDoesNotCoverUnpushedCommits is the branch-leak
// regression: an open PR is evidence about the commits it contains, not about
// commits made after it was opened. Archive cleanup force-removes the
// worktree once this gate passes, so unpushed work must keep it closed.
func TestVerifyCompletion_OpenPRDoesNotCoverUnpushedCommits(t *testing.T) {
	stubOpenPR(t, func(_, branch string) (int, bool) {
		if branch == "feature" {
			return 7, true
		}
		return 0, false
	})
	stubUnpushed(t, func(string, string) (bool, error) { return true, nil })

	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "Initial commit")
	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Command(t, dir, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("branch", "-M", "main")
	run("checkout", "-b", "feature")
	commitFile(t, dir, "feature.go", "package main\n", "Feature work")
	commitFile(t, dir, "feature_test.go", "package main\n", "Feature tests")

	result := VerifyCompletion(dir)

	if !result.HasOpenPR {
		t.Fatal("expected HasOpenPR=true")
	}
	if !result.HasUnpushedCommits {
		t.Fatal("expected HasUnpushedCommits=true")
	}
	if !result.Critical() {
		t.Fatal("expected Critical()=true: an open PR does not cover commits that exist on no remote")
	}
	joined := strings.Join(result.CriticalErrors(), " | ")
	if !strings.Contains(joined, "exist on no remote") {
		t.Errorf("expected the error to name unpushed commits as the reason, got %q", joined)
	}
}

// TestVerifyCompletion_UnpushedProbeFailureFailsClosed proves a broken
// pushed-state probe keeps the block rather than granting the downgrade.
func TestVerifyCompletion_UnpushedProbeFailureFailsClosed(t *testing.T) {
	stubOpenPR(t, func(_, branch string) (int, bool) {
		if branch == "feature" {
			return 9, true
		}
		return 0, false
	})
	stubUnpushed(t, func(string, string) (bool, error) {
		return false, errors.New("rev-list exploded")
	})

	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "Initial commit")
	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Command(t, dir, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("branch", "-M", "main")
	run("checkout", "-b", "feature")
	commitFile(t, dir, "feature.go", "package main\n", "Feature work")

	result := VerifyCompletion(dir)

	if !result.HasUnpushedCommits {
		t.Error("a failed pushed-state probe must be treated as unpushed work")
	}
	if !result.Critical() {
		t.Error("expected Critical()=true when the pushed-state probe fails")
	}
}

// TestVerifyCompletion_EmptyRepoNotFailClosed proves the "no commits yet"
// case is not conflated with a real resolution failure: a freshly
// initialized repo/worktree with zero commits has nothing that could be
// unmerged, so it must not fail closed.
func TestVerifyCompletion_EmptyRepoNotFailClosed(t *testing.T) {
	dir := setupTestRepo(t)
	result := VerifyCompletion(dir)

	if len(result.UnmergedCommits) != 0 {
		t.Errorf("expected no UnmergedCommits for a repo with zero commits, got %v", result.UnmergedCommits)
	}
	if result.Critical() {
		t.Errorf("expected Critical()=false for an empty repo, got errors: %v", result.CriticalErrors())
	}
}

func TestVerifyCompletion_MissingTestsBlock(t *testing.T) {
	dir := setupTestRepo(t)
	commitFile(t, dir, "server.go", "package main\n", "Add server code")

	result := VerifyCompletion(dir)

	if !result.MissingTests {
		t.Error("expected MissingTests=true when code changes have no test changes")
	}
	if !result.Critical() {
		t.Error("expected Critical()=true with missing tests")
	}
	errs := result.CriticalErrors()
	found := false
	for _, e := range errs {
		if e == "code changes detected without corresponding test changes" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing tests error, got %v", errs)
	}
}

func TestVerifyCompletion_CleanStateNotCritical(t *testing.T) {
	dir := setupTestRepo(t)

	run := func(args ...string) {
		t.Helper()
		out, err := gittest.Command(t, dir, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Set up main branch with code + tests, stay on main
	run("branch", "-M", "main")
	commitFile(t, dir, "app.go", "package main\n", "Add app")
	commitFile(t, dir, "app_test.go", "package main\n", "Add app tests")

	result := VerifyCompletion(dir)

	if result.Critical() {
		t.Errorf("expected Critical()=false for clean state, got errors: %v", result.CriticalErrors())
	}
	if len(result.UncommittedFiles) != 0 {
		t.Errorf("expected no uncommitted files, got %v", result.UncommittedFiles)
	}
	if len(result.UnmergedCommits) != 0 {
		t.Errorf("expected no unmerged commits, got %v", result.UnmergedCommits)
	}
	if result.MissingTests {
		t.Error("expected MissingTests=false")
	}
}

func TestVerifyCompletion_CriticalErrorsFormat(t *testing.T) {
	v := &CompletionVerification{
		UncommittedFiles: []string{"M file1.go", "?? file2.go", "M file3.go"},
		UnmergedCommits:  []string{"abc1234 some commit"},
		MissingTests:     true,
	}

	if !v.Critical() {
		t.Fatal("expected Critical()=true")
	}

	errs := v.CriticalErrors()
	if len(errs) != 3 {
		t.Fatalf("expected 3 critical errors, got %d: %v", len(errs), errs)
	}
	if errs[0] != "uncommitted changes in 3 file(s)" {
		t.Errorf("unexpected error[0]: %s", errs[0])
	}
	if errs[1] != "branch has 1 unmerged commit(s)" {
		t.Errorf("unexpected error[1]: %s", errs[1])
	}
	if errs[2] != "code changes detected without corresponding test changes" {
		t.Errorf("unexpected error[2]: %s", errs[2])
	}
}

func TestVerifyCompletion_NoCriticalWhenEmpty(t *testing.T) {
	v := &CompletionVerification{}
	if v.Critical() {
		t.Error("expected Critical()=false for zero-value verification")
	}
	if len(v.CriticalErrors()) != 0 {
		t.Error("expected no critical errors for zero-value verification")
	}
}
