package fsguard

import (
	"path/filepath"
	"testing"
)

// FuzzClassify ensures Classify never panics on arbitrary input.
// Run with: go test -fuzz=FuzzClassify -fuzztime=60s ./internal/fsguard/
func FuzzClassify(f *testing.F) {
	home := f.TempDir()
	g := &Guard{Home: home}

	// Seed corpus: interesting path shapes.
	seeds := []string{
		filepath.Join(home, "worktrees", "repo", "file.go"),
		filepath.Join(home, "src", "repo", "file.go"),
		filepath.Join(home, ".config", "settings.json"),
		"/dev/null",
		"/tmp/scratch",
		"/var/tmp/x",
		"",
		"relative/path",
		"~/worktrees/repo",
		"$HOME/src/repo",
		filepath.Join(home, "worktrees"),
		"../../etc/passwd",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, path string) {
		// Must not panic.
		_, _ = g.Classify(path, "")
	})
}

// FuzzExpand ensures expand never panics on arbitrary input.
func FuzzExpand(f *testing.F) {
	home := f.TempDir()
	g := &Guard{Home: home}

	f.Add("~/worktrees/x")
	f.Add("$HOME/src/y")
	f.Add("/absolute/path")
	f.Add("")
	f.Add("../../etc/passwd")
	f.Add("relative/path")

	f.Fuzz(func(t *testing.T, path string) {
		_ = g.expand(path, "")
	})
}

// FuzzUnder ensures under never panics on arbitrary input and upholds
// the reflexivity invariant: under(x, x) == true for any non-empty x.
func FuzzUnder(f *testing.F) {
	f.Add("/home/user/worktrees/repo", "/home/user/worktrees")
	f.Add("/home/user/src/repo", "/home/user/src")
	f.Add("", "")
	f.Add("/a", "/b")

	f.Fuzz(func(t *testing.T, path, base string) {
		_ = under(path, base)
		// Reflexivity: under(x, x) must always be true.
		if path != "" {
			if !under(path, path) {
				t.Errorf("under(%q, %q) = false, want true (reflexivity)", path, path)
			}
		}
	})
}
