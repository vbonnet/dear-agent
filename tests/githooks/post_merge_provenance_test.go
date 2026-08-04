package githooks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := gittest.Command(t, dir, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func pairProvenanceAt(t *testing.T, repo, ref string) (string, string) {
	t.Helper()
	commit := gitOutput(t, repo, "rev-parse", ref)
	if len(commit) < 12 {
		t.Fatalf("resolved commit %q is shorter than 12 characters", commit)
	}
	revision := commit[:12]
	commitTime := gitOutput(t, repo, "show", "-s", "--format=%cI", ref)
	parsedCommitTime, err := time.Parse(time.RFC3339, commitTime)
	if err != nil {
		t.Fatalf("parse pinned commit time %q: %v", commitTime, err)
	}
	buildDate := parsedCommitTime.UTC().Format(time.RFC3339)
	ldflags := strings.Join([]string{
		"-X github.com/vbonnet/dear-agent/pkg/version.Version=dev-" + revision,
		"-X github.com/vbonnet/dear-agent/pkg/version.GitCommit=" + revision,
		"-X github.com/vbonnet/dear-agent/pkg/version.BuildDate=" + buildDate,
		"-X github.com/vbonnet/dear-agent/pkg/version.BuiltBy=post-merge-hook",
	}, " ")
	return commit, ldflags
}

func stubGitArgumentFailure(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "$STUB_GIT_FAIL_ARG" ]; then
    [ -n "$STUB_GIT_OUTPUT" ] && printf '%s\n' "$STUB_GIT_OUTPUT"
    exit "${STUB_GIT_STATUS:-1}"
  fi
done
exec "$REAL_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRebuild_AGMPairBuildsWithCompletePinnedProvenance(t *testing.T) {
	origin := newRebuildRepo(t)
	wantCommit, wantFlags := pairProvenanceAt(t, origin, "HEAD")
	local := cloneRepo(t, origin)
	mergeBranchChanging(t, local, map[string]string{
		"agm/internal/tmux/prompt.go": "package tmux // divergent local provenance\n",
	})
	localHead := revParse(t, local, "HEAD")
	if localHead == wantCommit {
		t.Fatal("test setup: local HEAD did not diverge from origin trunk")
	}

	records := installRecords(t, runRebuildRecord(t, local))
	if len(records) != 2 {
		t.Fatalf("pair rebuild recorded %d builds, want exactly two: %+v", len(records), records)
	}
	seenPackages := make(map[string]bool, len(records))
	for _, record := range records {
		if record.pkg != "./agm/cmd/agm" && record.pkg != "./agm/cmd/agm-reaper" {
			t.Errorf("unexpected package in pair rebuild: %+v", record)
		}
		seenPackages[record.pkg] = true
		if record.commit != wantCommit {
			t.Errorf("%s built from %s, want origin trunk %s; local HEAD was %s",
				record.pkg, record.commit, wantCommit, localHead)
		}
		if record.ldflags != wantFlags {
			t.Errorf("%s ldflags = %q, want origin-trunk profile %q", record.pkg, record.ldflags, wantFlags)
		}
	}
	for _, pkg := range []string{"./agm/cmd/agm", "./agm/cmd/agm-reaper"} {
		if !seenPackages[pkg] {
			t.Errorf("pair rebuild did not record %s: %+v", pkg, records)
		}
	}
	if records[0].ldflags != records[1].ldflags {
		t.Errorf("pair builds received different provenance profiles: %+v", records)
	}
}

func TestRebuild_DetachedSourceSurvivesClosedStderr(t *testing.T) {
	origin := newRebuildRepo(t)
	wantCommit := revParse(t, origin, "HEAD")
	local := cloneRepo(t, origin)
	mergeBranchChanging(t, local, map[string]string{
		"cmd/vroom-dispatch/main.go": "package main // divergent local source\n",
	})
	localHead := revParse(t, local, "HEAD")
	if localHead == wantCommit {
		t.Fatal("test setup: local HEAD did not diverge from origin trunk")
	}

	recordFile := filepath.Join(t.TempDir(), "builds")
	goDir := stubGo(t, recordFile)
	cmd := exec.Command("bash", "-c", `exec 2>&-; exec bash "$1"`, "closed-stderr", hookPath(t))
	cmd.Dir = local
	cmd.Env = append(gittest.Env(t),
		"HOME="+t.TempDir(),
		"PATH="+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEAR_AGENT_MANAGED_REPO_ROOTS="+local,
		"AGM_POST_MERGE_SWEEP=0",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook must remain fail safe with closed stderr, got %v\n%s", err, output)
	}
	records := installRecords(t, recordFile)
	if len(records) != 1 || records[0].pkg != "./cmd/vroom-dispatch" {
		t.Fatalf("closed-stderr rebuild records = %+v, want only vroom-dispatch", records)
	}
	if records[0].commit != wantCommit {
		t.Fatalf("closed-stderr build used %s, want detached origin trunk %s; local HEAD was %s",
			records[0].commit, wantCommit, localHead)
	}
}

func TestRebuild_AGMPairAdmissionFailurePreservesInstalledPair(t *testing.T) {
	tests := []struct {
		name        string
		failArg     string
		output      string
		status      string
		wantWarning string
	}{
		{
			name:        "detached checkout unavailable",
			failArg:     "worktree",
			status:      "1",
			wantWarning: "cannot create an immutable detached AGM pair source",
		},
		{
			name:        "malformed revision",
			failArg:     "--short=12",
			output:      "zzzzzzzzzzzz",
			status:      "0",
			wantWarning: "cannot resolve complete AGM pair provenance",
		},
		{
			name:        "overlength uniqueness revision",
			failArg:     "--short=12",
			output:      "0123456789abc",
			status:      "0",
			wantWarning: "cannot resolve complete AGM pair provenance",
		},
		{
			name:        "commit date unavailable",
			failArg:     "--date=format-local:%Y-%m-%dT%H:%M:%SZ",
			status:      "1",
			wantWarning: "cannot resolve complete AGM pair provenance",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRebuildRepo(t)
			mergeBranchChanging(t, repo, map[string]string{
				"agm/internal/tmux/prompt.go": "package tmux // provenance failure\n",
			})
			gobin := t.TempDir()
			wantInstalled := map[string]string{
				"agm":        "old-agm\n",
				"agm-reaper": "old-reaper\n",
			}
			for name, content := range wantInstalled {
				if err := os.WriteFile(filepath.Join(gobin, name), []byte(content), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			recordFile := filepath.Join(t.TempDir(), "builds")
			goDir := stubGo(t, recordFile)
			gitDir := stubGitArgumentFailure(t)
			realGit, err := exec.LookPath("git")
			if err != nil {
				t.Skip("git not available")
			}
			cmd := exec.Command("bash", hookPath(t))
			cmd.Dir = repo
			cmd.Env = append(gittest.Env(t),
				"HOME="+t.TempDir(),
				"GOBIN="+gobin,
				"PATH="+gitDir+string(os.PathListSeparator)+goDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"REAL_GIT="+realGit,
				"STUB_GIT_FAIL_ARG="+tc.failArg,
				"STUB_GIT_OUTPUT="+tc.output,
				"STUB_GIT_STATUS="+tc.status,
				"DEAR_AGENT_MANAGED_REPO_ROOTS="+repo,
				"AGM_POST_MERGE_SWEEP=0",
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("hook must fail safe, got %v\n%s", err, output)
			}
			if !strings.Contains(string(output), tc.wantWarning) {
				t.Fatalf("hook did not explain %s:\n%s", tc.name, output)
			}
			if records := installRecords(t, recordFile); len(records) != 0 {
				t.Fatalf("%s ran %d pair builds, want none: %+v", tc.name, len(records), records)
			}
			for name, want := range wantInstalled {
				got, err := os.ReadFile(filepath.Join(gobin, name))
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != want {
					t.Fatalf("%s changed after %s: got %q, want %q", name, tc.name, got, want)
				}
			}
		})
	}
}
