package steps

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePRBDDLockParserAcceptsUnixAndWindowsLineEndings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktree")
	porcelain := "worktree " + filepath.Join(filepath.Dir(root), "repo") + "\nHEAD abc\n\n" +
		"worktree " + root + "\nHEAD def\nlocked bdd-owner\n"

	for _, test := range []struct {
		name       string
		lineEnding string
	}{
		{name: "LF", lineEnding: "\n"},
		{name: "CRLF", lineEnding: "\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			locked, reason, err := parseSafePRBDDLockState(root, strings.ReplaceAll(porcelain, "\n", test.lineEnding))
			if err != nil {
				t.Fatal(err)
			}
			if !locked || reason != "bdd-owner" {
				t.Fatalf("worktree lock = locked:%t reason:%q", locked, reason)
			}
		})
	}
}
