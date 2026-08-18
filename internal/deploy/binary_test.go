package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestShortRev(t *testing.T) {
	if got := shortRev("0123456789abcdef0123"); got != "0123456789ab" {
		t.Fatalf("shortRev long = %q, want 0123456789ab", got)
	}
	if got := shortRev("abc"); got != "abc" {
		t.Fatalf("shortRev short = %q, want abc", got)
	}
}

func TestSameRev(t *testing.T) {
	full := "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		a, b string
		want bool
	}{
		{full, full, true},
		{full, "0123456789ab", true}, // b is an abbreviation of a
		{"0123456789ab", full, true}, // a is an abbreviation of b
		{full, "fedcba", false},      // different
		{"", full, false},            // empty never matches
		{full, "", false},            // empty never matches
	}
	for _, c := range cases {
		if got := sameRev(c.a, c.b); got != c.want {
			t.Errorf("sameRev(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// gitRepo creates a temp git repo with two commits and returns the repo path
// plus the old and HEAD commit shas.
func gitRepo(t *testing.T) (repo, oldSha, headSha string) {
	t.Helper()
	repo = t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := gittest.Output(t, repo, args...)
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return out
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "one")
	oldSha = rev(t, repo, "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "f"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "two")
	headSha = rev(t, repo, "HEAD")
	return repo, oldSha, headSha
}

func rev(t *testing.T, repo, ref string) string {
	t.Helper()
	out, err := gittest.Command(t, repo, "rev-parse", "--verify", ref).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return string(out[:40])
}

// stubVersion swaps readBinaryVersion for the duration of a test.
func stubVersion(t *testing.T, rev string, modified bool, err error) {
	t.Helper()
	orig := readBinaryVersion
	readBinaryVersion = func(string) (string, bool, error) { return rev, modified, err }
	t.Cleanup(func() { readBinaryVersion = orig })
}

func TestBinaryStatus_Current(t *testing.T) {
	repo, _, head := gitRepo(t)
	stubVersion(t, head, false, nil)

	a := Artifact{Name: "mergeloop", Kind: KindBinary, Source: "cmd/mergeloop", Deployed: "~/go/bin/mergeloop"}
	s := Status(a, Options{RepoRoot: repo, Home: t.TempDir()})
	if s.State != StateOK {
		t.Fatalf("state = %q, want ok (%s)", s.State, s.Error)
	}
	if s.SourceVersion != shortRev(head) || s.DeployedVersion != shortRev(head) {
		t.Fatalf("versions deployed=%q source=%q", s.DeployedVersion, s.SourceVersion)
	}
}

func TestBinaryStatus_Stale(t *testing.T) {
	repo, old, head := gitRepo(t)
	stubVersion(t, old, false, nil)

	a := Artifact{Name: "mergeloop", Kind: KindBinary, Source: "cmd/mergeloop", Deployed: "~/go/bin/mergeloop"}
	s := Status(a, Options{RepoRoot: repo, Home: t.TempDir()})
	if s.State != StateDrift {
		t.Fatalf("state = %q, want drift", s.State)
	}
	if s.DeployedVersion != shortRev(old) || s.SourceVersion != shortRev(head) {
		t.Fatalf("versions deployed=%q source=%q", s.DeployedVersion, s.SourceVersion)
	}
	if s.Detail == "" || s.Detail[:5] != "stale" {
		t.Fatalf("detail = %q, want stale ...", s.Detail)
	}
}

func TestBinaryStatus_Divergent(t *testing.T) {
	repo, _, _ := gitRepo(t)
	// A commit sha that is not in the repo at all.
	stubVersion(t, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", false, nil)

	a := Artifact{Name: "mergeloop", Kind: KindBinary, Source: "cmd/mergeloop", Deployed: "~/go/bin/mergeloop"}
	s := Status(a, Options{RepoRoot: repo, Home: t.TempDir()})
	if s.State != StateDrift {
		t.Fatalf("state = %q, want drift", s.State)
	}
	if s.Detail == "" || s.Detail[:9] != "divergent" {
		t.Fatalf("detail = %q, want divergent ...", s.Detail)
	}
}

func TestBinaryStatus_NotDeployed(t *testing.T) {
	repo, _, _ := gitRepo(t)
	stubVersion(t, "", false, os.ErrNotExist)

	a := Artifact{Name: "mergeloop", Kind: KindBinary, Source: "cmd/mergeloop", Deployed: "~/go/bin/mergeloop"}
	s := Status(a, Options{RepoRoot: repo, Home: t.TempDir()})
	if s.State != StateMissing {
		t.Fatalf("state = %q, want missing", s.State)
	}
}

func TestBinaryStatus_Unstamped(t *testing.T) {
	repo, _, _ := gitRepo(t)
	stubVersion(t, "", false, nil) // binary present but no vcs.revision

	a := Artifact{Name: "mergeloop", Kind: KindBinary, Source: "cmd/mergeloop", Deployed: "~/go/bin/mergeloop"}
	s := Status(a, Options{RepoRoot: repo, Home: t.TempDir()})
	if s.State != StateError {
		t.Fatalf("state = %q, want error", s.State)
	}
	if s.DeployedVersion != "unstamped" {
		t.Fatalf("deployed version = %q, want unstamped", s.DeployedVersion)
	}
}

func TestBinaryStatus_NoSourceRepo(t *testing.T) {
	stubVersion(t, "abc", false, nil)
	a := Artifact{Name: "mergeloop", Kind: KindBinary, Source: "cmd/mergeloop", Deployed: "~/go/bin/mergeloop"}
	// RepoRoot is a non-repo temp dir: HEAD cannot be resolved.
	s := Status(a, Options{RepoRoot: t.TempDir(), Home: t.TempDir()})
	if s.State != StateError {
		t.Fatalf("state = %q, want error", s.State)
	}
}

// A binary artifact is never written by Deploy — it is status-only.
func TestDeploy_BinaryIsSkipped(t *testing.T) {
	a := Artifact{Name: "mergeloop", Kind: KindBinary, Source: "cmd/mergeloop", Deployed: "~/go/bin/mergeloop"}
	res, err := Deploy(a, Options{RepoRoot: t.TempDir(), Home: t.TempDir()})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if res.Action != ActionSkipped {
		t.Fatalf("action = %q, want skipped", res.Action)
	}
}

// binaryVersion must error cleanly on a file that is not a Go binary.
func TestBinaryVersion_NotABinary(t *testing.T) {
	f := filepath.Join(t.TempDir(), "notbin")
	if err := os.WriteFile(f, []byte("hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := binaryVersion(f); err == nil {
		t.Fatal("expected error reading build info from non-binary")
	}
}
