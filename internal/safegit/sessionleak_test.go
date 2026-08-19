package safegit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/safegit"
)

// Synthetic, never a real session id — see internal/sessionid tests.
const leakTrailer = "Claude-Session: https://claude.ai/code/session_01AaBbCcDdEeFfGgHhIiJjKk"

// commitRepo builds a repo whose HEAD commits are all "unpushed" (there is no
// origin remote-tracking ref), which is the state a fresh feature branch is in.
func commitRepo(t *testing.T, messages ...string) string {
	t.Helper()
	sb := gittest.Default(t)
	dir := sb.NewRepo(t)
	for i, msg := range messages {
		sb.Run(t, dir, "commit", "--allow-empty", "-m", msg)
		_ = i
	}
	return dir
}

func TestCheckSessionLeaksAllowsCleanHistory(t *testing.T) {
	dir := commitRepo(t, "feat: add a thing", "fix: correct the thing")
	if err := safegit.CheckSessionLeaks(dir); err != nil {
		t.Fatalf("CheckSessionLeaks on clean history = %v, want nil", err)
	}
}

func TestCheckSessionLeaksBlocksLeakedTrailer(t *testing.T) {
	dir := commitRepo(t, "feat: add a thing\n\n"+leakTrailer)
	err := safegit.CheckSessionLeaks(dir)
	if err == nil {
		t.Fatal("CheckSessionLeaks = nil, want a refusal")
	}
	var leak *safegit.SessionLeakError
	if !asSessionLeak(err, &leak) {
		t.Fatalf("error type = %T, want *safegit.SessionLeakError", err)
	}
	if len(leak.Offenders) != 1 {
		t.Errorf("Offenders = %d, want 1", len(leak.Offenders))
	}
	msg := err.Error()
	for _, want := range []string{
		"refusing to push",
		"session-url",
		"git commit --amend",
		"feat: add a thing",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestCheckSessionLeaksReportsEveryOffendingCommit(t *testing.T) {
	dir := commitRepo(t,
		"one: clean",
		"two: leaked\n\n"+leakTrailer,
		"three: clean",
		"four: leaked\n\n"+leakTrailer,
	)
	err := safegit.CheckSessionLeaks(dir)
	if err == nil {
		t.Fatal("CheckSessionLeaks = nil, want a refusal")
	}
	var leak *safegit.SessionLeakError
	if !asSessionLeak(err, &leak) {
		t.Fatalf("error type = %T, want *safegit.SessionLeakError", err)
	}
	if len(leak.Offenders) != 2 {
		t.Fatalf("Offenders = %d, want 2 (the clean commits must not be listed)", len(leak.Offenders))
	}
	if !strings.Contains(err.Error(), "2 unpushed commits carry") {
		t.Errorf("error should count the offenders:\n%s", err.Error())
	}
	// The multi-commit recipe needs a cut point.
	if leak.BaseHint == "" || !strings.HasSuffix(leak.BaseHint, "^") {
		t.Errorf("BaseHint = %q, want a <sha>^ cut point", leak.BaseHint)
	}
	if !strings.Contains(err.Error(), "git reset --soft") {
		t.Errorf("error should teach the multi-commit rewrite:\n%s", err.Error())
	}
}

// A guard that wedges pushes on unrelated git conditions gets routed around.
func TestCheckSessionLeaksFailsOpenOutsideARepository(t *testing.T) {
	if err := safegit.CheckSessionLeaks(t.TempDir()); err != nil {
		t.Fatalf("CheckSessionLeaks outside a repo = %v, want nil (fail open)", err)
	}
}

func TestUnpushedCommitsParsesMultiLineMessages(t *testing.T) {
	dir := commitRepo(t, "subject line\n\nbody paragraph one\n\nbody paragraph two")
	commits, err := safegit.UnpushedCommits(dir)
	if err != nil {
		t.Fatalf("UnpushedCommits: %v", err)
	}
	// NewRepo seeds its own initial commit, so locate ours by subject rather
	// than assuming a commit count.
	var got *safegit.UnpushedCommit
	for i := range commits {
		if commits[i].Subject == "subject line" {
			got = &commits[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("no commit with the expected subject in %+v", commits)
	}
	for _, want := range []string{"body paragraph one", "body paragraph two"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("Message missing %q: %q", want, got.Message)
		}
	}
}

func asSessionLeak(err error, target **safegit.SessionLeakError) bool {
	return errors.As(err, target)
}
