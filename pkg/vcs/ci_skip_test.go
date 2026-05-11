package vcs

import (
	"flag"
	"os"
	"testing"
)

// TestMain sets a deterministic git identity so commits in t.TempDir() repos
// don't fail in CI runners that lack a global user.email / user.name.
func TestMain(m *testing.M) {
	flag.Parse()
	os.Setenv("GIT_AUTHOR_NAME", "dear-agent-tests")
	os.Setenv("GIT_AUTHOR_EMAIL", "tests@dear-agent.local")
	os.Setenv("GIT_COMMITTER_NAME", "dear-agent-tests")
	os.Setenv("GIT_COMMITTER_EMAIL", "tests@dear-agent.local")
	os.Exit(m.Run())
}
