package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
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
	cmd := exec.Command("git", "-C", dir, "add", filename)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
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

func TestVerifyCompletion_UnmergedBranchBlocks(t *testing.T) {
	dir := setupTestRepo(t)
	commitFile(t, dir, "main.go", "package main\n", "Initial commit")

	// Create main branch, then a feature branch with extra commits
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
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
	if !result.Critical() {
		t.Error("expected Critical()=true with unmerged commits")
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
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
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
