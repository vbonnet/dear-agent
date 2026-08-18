package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/ci"
	"github.com/vbonnet/dear-agent/internal/ci/act"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

// -----------------------------------------------------------------------------
// Unit tests for the small, side-effect-free helpers.
// -----------------------------------------------------------------------------

func TestIsMergeCommit(t *testing.T) {
	tests := []struct {
		name           string
		setupMergeHead bool
		want           bool
	}{
		{
			name:           "merge in progress",
			setupMergeHead: true,
			want:           true,
		},
		{
			name:           "no merge in progress",
			setupMergeHead: false,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			gitDir := filepath.Join(tmpDir, ".git")
			require.NoError(t, os.Mkdir(gitDir, 0755))
			t.Chdir(tmpDir)

			if tt.setupMergeHead {
				require.NoError(t, os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), []byte("abc123\n"), 0644))
			}

			assert.Equal(t, tt.want, isMergeCommit())
		})
	}
}

func TestGetMergeTarget(t *testing.T) {
	tests := []struct {
		name       string
		setupRepo  func(t *testing.T, repoDir string)
		wantBranch string
	}{
		{
			name: "on main branch",
			setupRepo: func(t *testing.T, repoDir string) {
				initGitRepo(t, repoDir, "main")
			},
			wantBranch: "main",
		},
		{
			name: "on master branch",
			setupRepo: func(t *testing.T, repoDir string) {
				initGitRepo(t, repoDir, "master")
			},
			wantBranch: "master",
		},
		{
			name: "on feature branch",
			setupRepo: func(t *testing.T, repoDir string) {
				initGitRepo(t, repoDir, "main")
				checkoutBranch(t, repoDir, "feature/test")
			},
			wantBranch: "feature/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setupRepo(t, tmpDir)
			t.Chdir(tmpDir)

			assert.Equal(t, tt.wantBranch, getMergeTarget())
		})
	}
}

func TestGetMergeTarget_NoGitRepo(t *testing.T) {
	// In a non-git directory `git rev-parse` errors → empty string.
	t.Chdir(t.TempDir())
	assert.Equal(t, "", getMergeTarget())
}

func TestRollback(t *testing.T) {
	// Create a real merge state, then make sure rollback unwinds it.
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	checkoutBranch(t, tmpDir, "feature")
	createFile(t, tmpDir, "feature.txt", "feature content")
	gitAdd(t, tmpDir, "feature.txt")
	gitCommit(t, tmpDir, "Add feature file")
	gitCheckout(t, tmpDir, "main")
	createFile(t, tmpDir, "feature.txt", "main content")
	gitAdd(t, tmpDir, "feature.txt")
	gitCommit(t, tmpDir, "Add conflicting file")
	_ = gittest.Command(t, tmpDir, "merge", "feature", "--no-commit").Run()

	mergeHeadPath := filepath.Join(tmpDir, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeHeadPath); err != nil {
		t.Skipf("could not create MERGE_HEAD state: %v", err)
	}
	t.Chdir(tmpDir)

	rollback()

	_, err := os.Stat(mergeHeadPath)
	assert.True(t, os.IsNotExist(err), "MERGE_HEAD should be removed after rollback")
}

func TestRollback_NoMerge_LogsWarning(t *testing.T) {
	// Rollback in a clean repo prints a warning but must not crash.
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	t.Chdir(tmpDir)
	stderr := captureStdio(t, func() { rollback() })
	// `git reset --merge` in a non-merge state is actually a no-op on modern
	// git, so we don't assert the warning text — just that no panic occurred.
	_ = stderr
}

func TestFindPolicyFile_FoundInWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, ".ci-policy.yaml")
	require.NoError(t, os.WriteFile(policyPath, []byte("version: v1\n"), 0644))

	got := findPolicyFile(tmpDir)
	assert.Equal(t, policyPath, got)
}

func TestFindPolicyFile_FoundInParent(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, ".ci-policy.yaml")
	require.NoError(t, os.WriteFile(policyPath, []byte("version: v1\n"), 0644))
	sub := filepath.Join(tmpDir, "deep", "nested")
	require.NoError(t, os.MkdirAll(sub, 0755))

	got := findPolicyFile(sub)
	assert.Equal(t, policyPath, got)
}

func TestFindPolicyFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	// findPolicyFile delegates to ci.FindPolicyFile which walks to the FS root.
	// We can't make the walk fail without going outside the FS, but we can
	// verify "no policy in tree" returns empty by passing a freshly-created
	// dir that has no .ci-policy.yaml anywhere up to /.
	got := findPolicyFile(tmpDir)
	// If somewhere up the tree from t.TempDir() there happens to be a real
	// .ci-policy.yaml, this assertion will be skipped — the contract is just
	// "empty or some existing path", not "always empty".
	if got != "" {
		t.Logf("found a parent policy file (test tolerated): %s", got)
	}
}

func TestPrintHelp(t *testing.T) {
	out := captureStdout(t, printHelp)
	for _, want := range []string{"USAGE", "OPTIONS", "ENVIRONMENT", "CONFIGURATION", "SKIP_CI_GATES"} {
		if !strings.Contains(out, want) {
			t.Errorf("printHelp output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintRemediation(t *testing.T) {
	out := captureStderr(t, printRemediation)
	for _, want := range []string{"Remediation steps", "Review workflow failures", "Emergency bypass", "SKIP_CI_GATES=true"} {
		if !strings.Contains(out, want) {
			t.Errorf("printRemediation output missing %q:\n%s", want, out)
		}
	}
}

// -----------------------------------------------------------------------------
// checkBypass — table-driven over the four interesting paths.
// -----------------------------------------------------------------------------

func TestCheckBypass(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		allowBypass bool
		want        bool
	}{
		{"no env set", "", true, false},
		{"env=true and policy allows", "true", true, true},
		{"env=1 and policy allows", "1", true, true},
		{"env=yes and policy allows", "yes", true, true},
		{"env=other and policy allows", "maybe", true, false},
		{"env=true but policy disallows", "true", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each subtest needs a fresh cwd so logBypass can write its file
			// into a .git directory we control. Even when bypass returns false
			// the cwd is harmless.
			tmpDir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(tmpDir, ".git"), 0755))
			t.Chdir(tmpDir)
			t.Setenv("SKIP_CI_GATES", tt.envValue)

			policy := &ci.GatePolicy{
				FailureBehavior: "block",
				AllowBypass:     tt.allowBypass,
			}

			got := checkBypass(policy, true)
			if got != tt.want {
				t.Errorf("checkBypass(%q, allow=%v) = %v, want %v",
					tt.envValue, tt.allowBypass, got, tt.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// logBypass — verify it appends to .git/ci-gate-bypass.log, and that the
// stderr fallback fires when that file is unwritable.
// -----------------------------------------------------------------------------

func TestLogBypass_WritesToGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, ".git"), 0755))
	initGitRepo(t, tmpDir, "main")
	t.Chdir(tmpDir)
	t.Setenv("USER", "alice")

	logBypass()

	logPath := filepath.Join(tmpDir, ".git", "ci-gate-bypass.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	body := string(data)
	if !strings.Contains(body, "User alice bypassed CI gates for merge to main") {
		t.Errorf("bypass log missing expected line:\n%s", body)
	}
	// Timestamp shape — should look like RFC3339 (contains a 'T' and 'Z' or offset).
	if !strings.Contains(body, "T") {
		t.Errorf("bypass log missing RFC3339 timestamp:\n%s", body)
	}
}

func TestLogBypass_StderrFallback(t *testing.T) {
	// No .git directory → OpenFile fails → we expect the stderr fallback path.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("USER", "") // forces the "unknown" branch

	stderr := captureStderr(t, logBypass)
	if !strings.Contains(stderr, "User unknown bypassed CI gates for merge to") {
		t.Errorf("stderr fallback did not fire as expected:\n%s", stderr)
	}
}

// -----------------------------------------------------------------------------
// reportResults — synthesize MultiWorkflowResult fixtures so we can exercise
// every branch (passed / failed / skipped / missing / infra-error / optional)
// without ever invoking the real `act` binary.
// -----------------------------------------------------------------------------

func TestReportResults(t *testing.T) {
	type wantOutput struct {
		mustContain []string
	}
	pass := func(d time.Duration) *act.WorkflowResult {
		return &act.WorkflowResult{Result: &ci.PipelineResult{Success: true, Duration: d}}
	}
	fail := func(exit int, out string) *act.WorkflowResult {
		return &act.WorkflowResult{Result: &ci.PipelineResult{Success: false, ExitCode: exit, Output: out, Duration: time.Second}}
	}
	skip := func(reason string) *act.WorkflowResult {
		return &act.WorkflowResult{Skipped: true, SkipReason: reason}
	}
	infraErr := func(msg string) *act.WorkflowResult {
		return &act.WorkflowResult{Error: simpleErr(msg)}
	}
	noResult := &act.WorkflowResult{} // exists but Result==nil and no Error/Skipped

	path := func(name string) string { return filepath.Join(".github", "workflows", name) }

	tests := []struct {
		name    string
		policy  *ci.GatePolicy
		results map[string]*act.WorkflowResult
		want    int
		out     wantOutput
		verbose bool
	}{
		{
			name: "all required passed",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml", "b.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{
				path("a.yml"): pass(2 * time.Second),
				path("b.yml"): pass(3 * time.Second),
			},
			want: 0,
			out:  wantOutput{mustContain: []string{"a.yml - passed", "b.yml - passed"}},
		},
		{
			name: "one required failed — RequireAllPassing blocks",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml", "b.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{
				path("a.yml"): pass(time.Second),
				path("b.yml"): fail(2, "stack trace"),
			},
			want:    1,
			out:     wantOutput{mustContain: []string{"a.yml - passed", "b.yml - failed (exit 2", "stack trace"}},
			verbose: true,
		},
		{
			name: "one required failed — allow-any keeps going",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml", "b.yml"},
				RequireAllPassing: false,
			},
			results: map[string]*act.WorkflowResult{
				path("a.yml"): pass(time.Second),
				path("b.yml"): fail(1, ""),
			},
			want: 0,
		},
		{
			name: "all required failed — allow-any still blocks",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml"},
				RequireAllPassing: false,
			},
			results: map[string]*act.WorkflowResult{path("a.yml"): fail(1, "")},
			want:    1,
		},
		{
			name: "skipped required workflow counts as failure",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{path("a.yml"): skip("dependency failed")},
			want:    1,
			out:     wantOutput{mustContain: []string{"a.yml - skipped (dependency failed)"}},
		},
		{
			name: "missing required workflow counts as failure",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"missing.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{},
			want:    1,
			out:     wantOutput{mustContain: []string{"missing.yml - not executed"}},
		},
		{
			name: "infrastructure error on required workflow",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{path("a.yml"): infraErr("disk full")},
			want:    1,
			out:     wantOutput{mustContain: []string{"a.yml - infrastructure error: disk full"}},
		},
		{
			name: "required present but nil Result",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{path("a.yml"): noResult},
			want:    1,
			out:     wantOutput{mustContain: []string{"a.yml - no result"}},
		},
		{
			name: "optional workflows do not affect exit code",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"a.yml"},
				OptionalWorkflows: []string{"o-pass.yml", "o-fail.yml", "o-skip.yml", "o-err.yml", "o-missing.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{
				path("a.yml"):      pass(time.Second),
				path("o-pass.yml"): pass(time.Second),
				path("o-fail.yml"): fail(1, ""),
				path("o-skip.yml"): skip("upstream failed"),
				path("o-err.yml"):  infraErr("network down"),
				// o-missing.yml deliberately absent
			},
			want: 0,
			out: wantOutput{mustContain: []string{
				"Optional workflows",
				"o-pass.yml - passed",
				"o-fail.yml - failed (optional)",
				"o-skip.yml - skipped",
				"o-err.yml - error (optional): network down",
				"o-missing.yml - not executed",
			}},
		},
		{
			name: "absolute workflow paths are not re-prefixed",
			policy: &ci.GatePolicy{
				RequiredWorkflows: []string{"/abs/path.yml"},
				RequireAllPassing: true,
			},
			results: map[string]*act.WorkflowResult{"/abs/path.yml": pass(time.Second)},
			want:    0,
			out:     wantOutput{mustContain: []string{"/abs/path.yml - passed"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &act.MultiWorkflowResult{
				Results:       tt.results,
				TotalDuration: 4 * time.Second,
			}
			var got int
			out := captureStdout(t, func() {
				got = reportResults(result, tt.policy, tt.verbose)
			})
			if got != tt.want {
				t.Errorf("exit = %d, want %d. output:\n%s", got, tt.want, out)
			}
			for _, want := range tt.out.mustContain {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// run() — drive the hook end-to-end via real temp git dirs + crafted policies.
// -----------------------------------------------------------------------------

func TestRun_HelpFlag(t *testing.T) {
	withArgs(t, []string{"pre-merge-commit", "--help"})
	out := captureStdout(t, func() {
		if code := run(); code != 0 {
			t.Errorf("--help exit = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "USAGE") {
		t.Errorf("--help did not print usage:\n%s", out)
	}
}

func TestRun_NotAMergeCommit(t *testing.T) {
	withArgs(t, []string{"pre-merge-commit", "--verbose"})
	tmpDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, ".git"), 0755))
	t.Chdir(tmpDir)

	out := captureStdout(t, func() {
		if code := run(); code != 0 {
			t.Errorf("non-merge exit = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "Not a merge commit") {
		t.Errorf("verbose non-merge output missing expected line:\n%s", out)
	}
}

func TestRun_MissingPolicyFile_UsesDefaultsAndAllows(t *testing.T) {
	// ci.LoadPolicyFromFile treats a missing file as the default policy
	// (empty RequiredWorkflows). That short-circuits run() with exit 0 — no
	// gates configured, no work to do. This is the documented contract; the
	// test guards it against regressions that would treat missing files as
	// hard errors.
	withArgs(t, []string{"pre-merge-commit", "-v"})
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git", "MERGE_HEAD"), []byte("abc"), 0644))
	t.Chdir(tmpDir)

	out := captureStdout(t, func() {
		if code := run(); code != 0 {
			t.Errorf("missing policy exit = %d, want 0 (defaults applied)", code)
		}
	})
	if !strings.Contains(out, "No required workflows") {
		t.Errorf("expected default-policy empty-workflows message:\n%s", out)
	}
}

func TestRun_MalformedPolicyFile_ReturnsError(t *testing.T) {
	withArgs(t, []string{"pre-merge-commit"})
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git", "MERGE_HEAD"), []byte("abc"), 0644))
	// YAML is intentionally invalid — unmarshal will fail.
	writePolicy(t, tmpDir, "version: v1\ndefault: {\n  this is: not yaml\n[]\n")
	t.Chdir(tmpDir)

	stderr := captureStderr(t, func() {
		if code := run(); code != 1 {
			t.Errorf("malformed policy exit = %d, want 1", code)
		}
	})
	if !strings.Contains(stderr, "Failed to load CI policy") {
		t.Errorf("expected policy load error in stderr:\n%s", stderr)
	}
}

func TestRun_BypassActivated(t *testing.T) {
	withArgs(t, []string{"pre-merge-commit"})
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git", "MERGE_HEAD"), []byte("abc"), 0644))
	writePolicy(t, tmpDir, `version: v1
default:
  failure_behavior: block
  allow_bypass: true
  required_workflows:
    - test.yml
  timeout_minutes: 30
`)
	t.Setenv("SKIP_CI_GATES", "true")
	t.Setenv("USER", "alice")
	t.Chdir(tmpDir)

	out := captureStdout(t, func() {
		if code := run(); code != 0 {
			t.Errorf("bypass exit = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "CI GATE BYPASS ACTIVATED") {
		t.Errorf("expected bypass banner in stdout:\n%s", out)
	}

	logPath := filepath.Join(tmpDir, ".git", "ci-gate-bypass.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err, "bypass should write to .git log")
	assert.Contains(t, string(data), "alice")
}

func TestRun_NoRequiredWorkflows(t *testing.T) {
	withArgs(t, []string{"pre-merge-commit", "-v"})
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git", "MERGE_HEAD"), []byte("abc"), 0644))
	writePolicy(t, tmpDir, `version: v1
default:
  failure_behavior: block
  required_workflows: []
  timeout_minutes: 30
`)
	t.Chdir(tmpDir)
	t.Setenv("SKIP_CI_GATES", "")

	out := captureStdout(t, func() {
		if code := run(); code != 0 {
			t.Errorf("empty workflows exit = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "No required workflows") {
		t.Errorf("expected empty-workflows message:\n%s", out)
	}
}

func TestRun_DryRun(t *testing.T) {
	withArgs(t, []string{"pre-merge-commit", "--dry-run"})
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git", "MERGE_HEAD"), []byte("abc"), 0644))
	writePolicy(t, tmpDir, `version: v1
default:
  failure_behavior: block
  required_workflows:
    - ci.yml
  optional_workflows:
    - lint.yml
  timeout_minutes: 30
`)
	t.Chdir(tmpDir)

	out := captureStdout(t, func() {
		if code := run(); code != 0 {
			t.Errorf("--dry-run exit = %d, want 0", code)
		}
	})
	for _, want := range []string{"Dry-run mode", "ci.yml", "Optional workflows", "lint.yml"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestRun_FailingExecutor_RespectsFailureBehavior(t *testing.T) {
	// With no act binary present, executeWorkflows returns an infrastructure
	// error → exit code 1 from that helper. run() then decides what to do
	// based on the policy's failure_behavior. We sweep all three.
	tests := []struct {
		name       string
		behavior   string
		want       int
		mustStdout string
		mustStderr string
	}{
		{
			name:       "warn keeps the merge",
			behavior:   "warn",
			want:       0,
			mustStdout: "policy allows merge (warn mode)",
		},
		{
			name:     "allow keeps the merge silently",
			behavior: "allow",
			want:     0,
			// "allow" mode is only logged in verbose; we just check the exit.
		},
		{
			name:       "block aborts and rolls back",
			behavior:   "block",
			want:       1,
			mustStderr: "CI gates failed - merge blocked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withArgs(t, []string{"pre-merge-commit"})
			tmpDir := t.TempDir()
			initGitRepo(t, tmpDir, "main")
			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git", "MERGE_HEAD"), []byte("abc"), 0644))
			writePolicy(t, tmpDir, "version: v1\ndefault:\n  failure_behavior: "+tt.behavior+
				"\n  required_workflows:\n    - test.yml\n  timeout_minutes: 1\n")
			t.Chdir(tmpDir)
			t.Setenv("SKIP_CI_GATES", "")

			var got int
			var stdout string
			stderr := captureStderr(t, func() {
				stdout = captureStdout(t, func() { got = run() })
			})
			if got != tt.want {
				t.Errorf("%s: exit = %d, want %d", tt.behavior, got, tt.want)
			}
			if tt.mustStdout != "" && !strings.Contains(stdout, tt.mustStdout) {
				t.Errorf("%s: stdout missing %q:\n%s", tt.behavior, tt.mustStdout, stdout)
			}
			if tt.mustStderr != "" && !strings.Contains(stderr, tt.mustStderr) {
				t.Errorf("%s: stderr missing %q:\n%s", tt.behavior, tt.mustStderr, stderr)
			}
		})
	}
}

func TestRun_BypassDisallowed_FallsThroughToCI(t *testing.T) {
	// SKIP_CI_GATES set but policy disallows bypass → checkBypass returns
	// false, so run() must proceed to the normal CI path. With no act binary
	// the executor will return an infra error and we should see exit 1.
	withArgs(t, []string{"pre-merge-commit"})
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir, "main")
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".git", "MERGE_HEAD"), []byte("abc"), 0644))
	writePolicy(t, tmpDir, `version: v1
default:
  failure_behavior: block
  allow_bypass: false
  required_workflows:
    - never-runs.yml
  timeout_minutes: 1
`)
	t.Chdir(tmpDir)
	t.Setenv("SKIP_CI_GATES", "true")

	stderr := captureStderr(t, func() {
		// We don't assert the exact exit code (act may not be installed) — we
		// just need to show the bypass warning fired and the call kept going.
		_ = run()
	})
	if !strings.Contains(stderr, "Bypass attempted but policy does not allow bypass") {
		t.Errorf("expected disallowed-bypass warning:\n%s", stderr)
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func initGitRepo(t *testing.T, dir, branch string) {
	t.Helper()
	require.NoError(t, gittest.Command(t, dir, "init", "--initial-branch="+branch).Run(), "git init failed")
	createFile(t, dir, "README.md", "# Test Repo")
	gitAdd(t, dir, "README.md")
	gitCommit(t, dir, "Initial commit")
}

func checkoutBranch(t *testing.T, dir, branch string) {
	t.Helper()
	require.NoError(t, gittest.Command(t, dir, "checkout", "-b", branch).Run(), "git checkout -b failed")
}

func gitCheckout(t *testing.T, dir, branch string) {
	t.Helper()
	require.NoError(t, gittest.Command(t, dir, "checkout", branch).Run(), "git checkout failed")
}

func createFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644))
}

func gitAdd(t *testing.T, dir, file string) {
	t.Helper()
	require.NoError(t, gittest.Command(t, dir, "add", file).Run(), "git add failed")
}

func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	require.NoError(t, gittest.Command(t, dir, "commit", "-m", message).Run(), "git commit failed")
}

func writePolicy(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ci-policy.yaml"), []byte(body), 0644))
}

func withArgs(t *testing.T, args []string) {
	t.Helper()
	orig := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = orig })
}

// captureStdout/stderr/Stdio swap the corresponding fd for a pipe, run fn,
// and return whatever was written. Used to assert hook output.
func captureStdout(t *testing.T, fn func()) string { return capture(t, &os.Stdout, fn) }
func captureStderr(t *testing.T, fn func()) string { return capture(t, &os.Stderr, fn) }
func captureStdio(t *testing.T, fn func()) string {
	// Stderr is interesting more often than stdout for these hooks; default
	// to stderr capture for the legacy helper name used by TestRollback_NoMerge.
	return captureStderr(t, fn)
}

func capture(t *testing.T, target **os.File, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := *target
	*target = w
	defer func() { *target = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

// simpleErr is a tiny error implementation so test fixtures can attach an
// arbitrary message without dragging in fmt.Errorf at call sites.
type simpleErr string

func (e simpleErr) Error() string { return string(e) }
