package fsguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordingRecorder captures violations in memory for assertions.
type recordingRecorder struct{ got []Violation }

func (r *recordingRecorder) Record(v Violation) error {
	r.got = append(r.got, v)
	return nil
}

func TestInterceptorCheckWrite(t *testing.T) {
	t.Parallel()
	rec := &recordingRecorder{}
	g := &Guard{Home: "/home/tester"} // pure string logic, no symlink resolution
	itc := NewInterceptorWith(g, rec)

	// Allowed: no violation recorded.
	if d := itc.CheckWrite("Write", "~/worktrees/x/f", "/home/tester"); !d.Allowed {
		t.Fatalf("worktree write blocked: %q", d.Message)
	}
	if len(rec.got) != 0 {
		t.Fatalf("allowed write recorded a violation: %+v", rec.got)
	}

	// Blocked: violation recorded, message carries escalation hint.
	d := itc.CheckWrite("Write", "~/src/dear-agent/f", "/home/tester")
	if d.Allowed {
		t.Fatal("src write was allowed")
	}
	if !strings.Contains(d.Message, "PERMISSION_ESCALATION") {
		t.Errorf("message missing escalation hint: %q", d.Message)
	}
	if len(rec.got) != 1 {
		t.Fatalf("got %d violations, want 1", len(rec.got))
	}
	if rec.got[0].Tool != "Write" || rec.got[0].Path != "~/src/dear-agent/f" {
		t.Errorf("violation = %+v", rec.got[0])
	}
	if rec.got[0].Reason == "" {
		t.Error("violation has empty reason")
	}
}

func TestInterceptorCheckCommand(t *testing.T) {
	t.Parallel()
	rec := &recordingRecorder{}
	itc := NewInterceptorWith(&Guard{Home: "/home/tester"}, rec)

	if d := itc.CheckCommand("ls ~/src", "/home/tester"); !d.Allowed {
		t.Fatalf("read command blocked: %q", d.Message)
	}
	d := itc.CheckCommand("rm ~/src/dear-agent/f", "/home/tester")
	if d.Allowed {
		t.Fatal("rm in src allowed")
	}
	if len(rec.got) != 1 || rec.got[0].Tool != "Bash" {
		t.Fatalf("violations = %+v", rec.got)
	}
	if rec.got[0].Command != "rm ~/src/dear-agent/f" {
		t.Errorf("recorded command = %q", rec.got[0].Command)
	}
}

func TestInterceptorNilRecorderSafe(t *testing.T) {
	t.Parallel()
	itc := NewInterceptorWith(&Guard{Home: "/home/tester"}, nil)
	// Must not panic with a nil recorder.
	if d := itc.CheckWrite("Write", "~/src/x", "/home/tester"); d.Allowed {
		t.Fatal("src write allowed")
	}
}

// TestSymlinkEscapeBlocked is the security regression test: a symlink inside
// ~/worktrees that points into ~/src must be classified by its real target and
// blocked, not allowed by its lexical worktree prefix.
func TestSymlinkEscapeBlocked(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	src := filepath.Join(home, "src", "repo")
	worktrees := filepath.Join(home, "worktrees")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktrees, 0o755); err != nil {
		t.Fatal(err)
	}
	// ~/worktrees/escape -> ~/src/repo
	escape := filepath.Join(worktrees, "escape")
	if err := os.Symlink(src, escape); err != nil {
		t.Fatal(err)
	}

	// Resolve home through any symlinks (macOS /var -> /private/var) so prefix
	// checks match, mirroring New().
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit policy with no temp-dir carve-out: t.TempDir() lives under
	// /var/folders, which the default policy treats as always-writable — that
	// would mask the escape we are testing.
	pol := Policy{
		WorktreesDir: filepath.Join(resolvedHome, "worktrees"),
		Protected:    []string{filepath.Join(resolvedHome, "src")},
	}
	g := &Guard{Home: resolvedHome, policy: &pol, resolveSymlinks: true}

	// Lexically this is under ~/worktrees; through the symlink it is ~/src.
	target := filepath.Join(escape, "main.go")
	allowed, msg := g.Classify(target, resolvedHome)
	if allowed {
		t.Fatalf("symlink escape into ~/src was allowed: %q", target)
	}
	if !strings.Contains(msg, "~/src") {
		t.Errorf("block message = %q, want it to mention ~/src", msg)
	}

	// Control: a genuine worktree path (no escaping symlink) is allowed.
	real := filepath.Join(worktrees, "feature", "main.go")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if allowed, msg := g.Classify(real, resolvedHome); !allowed {
		t.Fatalf("genuine worktree path blocked: %q (%q)", real, msg)
	}
}
