package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// buildRepo creates a repo with a base commit, then a head commit that renames
// a source file and optionally adds new logic — the two shapes the detector
// combines.
func buildRepo(t *testing.T, withFeature bool) (dir, base, head string) {
	t.Helper()
	sb := gittest.Default(t)
	dir = sb.NewRepo(t)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("old.go", "package a\n\nfunc Old() int { return 1 }\n")
	sb.Run(t, dir, "add", "-A")
	sb.Run(t, dir, "commit", "-m", "base")
	base = strings.TrimSpace(sb.Run(t, dir, "rev-parse", "HEAD"))

	sb.Run(t, dir, "mv", "old.go", "new.go")
	if withFeature {
		var b strings.Builder
		b.WriteString("package a\n")
		for range 120 {
			b.WriteString("\n// new logic line\nvar _ = 1\n")
		}
		write("feature.go", b.String())
		sb.Run(t, dir, "add", "-A")
	}
	sb.Run(t, dir, "add", "-A")
	sb.Run(t, dir, "commit", "-m", "head")
	head = strings.TrimSpace(sb.Run(t, dir, "rev-parse", "HEAD"))
	return dir, base, head
}

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb strings.Builder
	code = run(context.Background(), args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestRunRequiresBaseAndHead(t *testing.T) {
	_, stderr, code := runTool(t, "-base", "abc")
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "both -base and -head are required") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunReportsUnresolvableRevisions(t *testing.T) {
	sb := gittest.Default(t)
	dir := sb.NewRepo(t)
	_, stderr, code := runTool(t, "-repo", dir, "-base", "nope1", "-head", "nope2")
	if code != exitUsage {
		t.Errorf("exit = %d, want %d for an unresolvable revision", code, exitUsage)
	}
	if !strings.Contains(stderr, "pr-concern-lint:") {
		t.Errorf("stderr should name the tool: %q", stderr)
	}
}

func TestRunGitHubOutputIsQuietForASingleConcernDiff(t *testing.T) {
	dir, base, head := buildRepo(t, false)
	stdout, _, code := runTool(t, "-repo", dir, "-base", base, "-head", head, "-github-output")
	if code != exitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "mixed_concern=false") {
		t.Errorf("a rename-only diff must not be mixed:\n%s", stdout)
	}
	if !strings.Contains(stdout, "reason=\n") {
		t.Errorf("expected an empty reason line:\n%q", stdout)
	}
}

func TestRunGitHubOutputFlagsAMixedDiff(t *testing.T) {
	dir, base, head := buildRepo(t, true)
	stdout, _, code := runTool(t, "-repo", dir, "-base", base, "-head", head, "-github-output")
	if code != exitOK {
		t.Fatalf("exit = %d, want 0 — this signal is advisory and must never block", code)
	}
	if !strings.Contains(stdout, "mixed_concern=true") {
		t.Fatalf("expected a mixed verdict:\n%s", stdout)
	}
	// The heredoc form is what lets a multi-line reason survive GITHUB_OUTPUT.
	if !strings.Contains(stdout, "reason<<PR_CONCERN_REASON_EOF") {
		t.Errorf("multi-line reason must use the heredoc form:\n%s", stdout)
	}
	if !strings.HasSuffix(strings.TrimSpace(stdout), "PR_CONCERN_REASON_EOF") {
		t.Errorf("heredoc must be terminated:\n%s", stdout)
	}
	if !strings.Contains(stdout, "mixes a mechanical refactor") {
		t.Errorf("reason text missing:\n%s", stdout)
	}
}

func TestRunProseOutput(t *testing.T) {
	dir, base, head := buildRepo(t, false)
	stdout, _, code := runTool(t, "-repo", dir, "-base", base, "-head", head)
	if code != exitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "single-concern") {
		t.Errorf("stdout = %q", stdout)
	}
}

// A threshold of 0 selects the package default rather than flagging every diff.
func TestRunHonoursThresholdFlag(t *testing.T) {
	dir, base, head := buildRepo(t, true)
	stdout, _, _ := runTool(t, "-repo", dir, "-base", base, "-head", head,
		"-min-new-logic", "100000", "-github-output")
	if !strings.Contains(stdout, "mixed_concern=false") {
		t.Errorf("an unreachable threshold must silence the signal:\n%s", stdout)
	}
}
