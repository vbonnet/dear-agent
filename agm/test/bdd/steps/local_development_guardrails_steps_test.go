package steps

import (
	"path/filepath"
	"slices"
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

func TestMissingNamedGoTestRuns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "all exact top-level tests ran",
			output: "=== RUN   TestCommandRegression\n" +
				"--- PASS: TestCommandRegression (0.00s)\n" +
				"=== RUN   TestInternalRegression\r\n" +
				"--- PASS: TestInternalRegression (0.00s)\r\n",
		},
		{
			name:   "missing regression is reported",
			output: "=== RUN   TestCommandRegression\n",
			want:   []string{"TestInternalRegression"},
		},
		{
			name:   "subtest marker is not an exact regression run",
			output: "=== RUN   TestInternalRegression/subcase\n",
			want:   []string{"TestCommandRegression", "TestInternalRegression"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := missingNamedGoTestRuns(tt.output, "TestCommandRegression", "TestInternalRegression")
			if !slices.Equal(got, tt.want) {
				t.Fatalf("missingNamedGoTestRuns() = %v, want %v", got, tt.want)
			}
		})
	}
}
