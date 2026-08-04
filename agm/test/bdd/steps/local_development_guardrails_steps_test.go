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

func TestWorkflowJobTimeoutMinutesStaysWithinNamedJob(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		want     int
		wantErr  bool
	}{
		{
			name: "reads named job timeout",
			workflow: "jobs:\n" +
				"  integration-tests:\n" +
				"    runs-on: ubuntu-latest\n" +
				"    timeout-minutes: 100\n" +
				"  next-job:\n" +
				"    timeout-minutes: 200\n",
			want: 100,
		},
		{
			name: "does not borrow next job timeout",
			workflow: "jobs:\n" +
				"  integration-tests:\n" +
				"    runs-on: ubuntu-latest\n" +
				"  next-job:\n" +
				"    timeout-minutes: 200\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := workflowJobTimeoutMinutes([]byte(tc.workflow), "integration-tests")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("workflowJobTimeoutMinutes = %d, want missing-timeout error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("workflowJobTimeoutMinutes = %d, want %d", got, tc.want)
			}
		})
	}
}
