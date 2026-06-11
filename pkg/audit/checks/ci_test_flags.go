package checks

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// CITestFlagsCheck detects a divergence between how CI invokes go test
// and what the test code assumes about that invocation.
//
// Concrete failure mode addressed: test files that guard CI-skip logic
// with testing.Short() (via t.Skip / t.SkipNow inside an
// "if testing.Short()" block) never actually skip in CI when the CI
// workflow runs go test without -short. This is a silent dead-end: the
// skip code compiles and passes review but has zero effect at runtime.
//
// The check works in two passes:
//  1. Scan .github/workflows/*.yml for "go test" lines. If every go test
//     invocation lacks "-short", the repo CI is running in long mode.
//  2. Scan *_test.go files for "testing.Short()". Files that call it when
//     CI never passes -short are carrying dead skip logic.
//
// A finding is emitted only when both conditions are true (CI omits
// -short AND test files call testing.Short()). This avoids false
// positives in repos that do pass -short locally while CI uses long
// mode intentionally.
//
// Severity is P3 — the tests run and pass; the only harm is that the
// skip logic is permanently a no-op and developers may believe the tests
// are skipping when they are not.
//
// Config knobs:
//   - workflow_dir: string — path to CI workflow files relative to
//     WorkingDir. Default: ".github/workflows".
//   - roots: []string — roots to scan for _test.go files. Default:
//     ["pkg", "internal", "tools", "cmd", "agm"].
type CITestFlagsCheck struct{}

// Meta returns the check metadata for CITestFlagsCheck.
func (CITestFlagsCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "ci.test_flags",
		Description:     "testing.Short() guards in test files must match CI go test flags",
		Cadence:         audit.CadenceWeekly,
		SeverityCeiling: audit.SeverityP3,
	}
}

// Run implements the two-pass analysis described in the type comment.
func (CITestFlagsCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	workflowDir := stringConfig(env.Config, "workflow_dir", ".github/workflows")
	roots := stringSliceConfig(env.Config, "roots", defaultCITestRoots)

	out := audit.Result{Status: audit.StatusOK}

	// Pass 1: does CI ever pass -short to go test?
	wfDir := filepath.Join(env.WorkingDir, workflowDir)
	ciPassesShort, err := ciWorkflowPassesShort(wfDir)
	if err != nil {
		// Missing workflow directory (not a GitHub-hosted repo) — skip silently.
		if os.IsNotExist(err) {
			return out, nil
		}
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/ci.test_flags: read workflow dir: %w", err)
	}
	if ciPassesShort {
		// CI is already passing -short; no skew possible.
		return out, nil
	}

	// Pass 2: which test files call testing.Short()?
	var affectedFiles []string
	for _, root := range roots {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		rootAbs := filepath.Join(env.WorkingDir, root)
		files, err := findTestFilesWithShort(rootAbs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			out.Status = audit.StatusError
			return out, fmt.Errorf("audit/ci.test_flags: scan %s: %w", rootAbs, err)
		}
		for _, f := range files {
			rel, err := filepath.Rel(env.WorkingDir, f)
			if err != nil {
				rel = f
			}
			affectedFiles = append(affectedFiles, filepath.ToSlash(rel))
		}
	}

	if len(affectedFiles) == 0 {
		return out, nil
	}

	sort.Strings(affectedFiles)

	detail := fmt.Sprintf(
		"CI runs 'go test' without -short, but %d test file(s) guard skip logic with "+
			"testing.Short(). Those if-testing.Short() branches never fire in CI — "+
			"the tests run unconditionally regardless of what the skip comment says.\n\n"+
			"Fix options:\n"+
			"  1. Add -short to the CI 'go test' command if short-mode is intentional.\n"+
			"  2. Replace testing.Short() guards with an environment-variable gate "+
			"   (e.g. os.Getenv(\"CI_SKIP_X\") != \"\") so the intent is explicit.\n"+
			"  3. Remove the dead guard if the test should always run.\n\n"+
			"Affected files (%d):\n  %s",
		len(affectedFiles), len(affectedFiles),
		strings.Join(affectedFiles, "\n  "),
	)

	out.Findings = []audit.Finding{
		{
			Fingerprint: audit.Fingerprint("ci.test_flags", "short_skew"),
			Severity:    audit.SeverityP3,
			Title: fmt.Sprintf(
				"CI runs go test without -short but %d file(s) use testing.Short() guards",
				len(affectedFiles),
			),
			Detail: detail,
			Path:   workflowDir,
			Suggested: audit.Remediation{
				Strategy: audit.StrategyNoop,
			},
			Evidence: map[string]any{
				"affected_files":  affectedFiles,
				"ci_passes_short": false,
				"affected_count":  len(affectedFiles),
			},
		},
	}
	return out, nil
}

var defaultCITestRoots = []string{"pkg", "internal", "tools", "cmd", "agm"}

// ciWorkflowPassesShort scans every *.yml / *.yaml file in dir and
// returns true if any line that invokes "go test" also contains "-short".
func ciWorkflowPassesShort(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		found, err := fileContainsGoTestShort(filepath.Join(dir, name))
		if err != nil {
			continue // non-fatal: unreadable workflow is treated as not passing -short
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// fileContainsGoTestShort returns true if the file has a "go test" line
// that also contains "-short" (on the same logical line or continuation).
// We do a simple line-by-line grep: any line containing both "go test"
// and "-short" is a hit. This covers the common single-line invocation
// ("run: go test -short -race ./...") without requiring a YAML parser.
func fileContainsGoTestShort(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "go test") && strings.Contains(line, "-short") {
			return true, nil
		}
	}
	return false, sc.Err()
}

// findTestFilesWithShort returns every *_test.go file under root (recursive)
// that contains "testing.Short()".
func findTestFilesWithShort(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // WalkDir callback: skip unreadable entries without aborting the walk
		}
		if d.IsDir() {
			name := d.Name()
			// Skip vendor / testdata / dotdirs.
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		found, err := fileContainsTestingShort(path)
		if err != nil || !found {
			return nil //nolint:nilerr // non-fatal: unreadable test file is skipped
		}
		out = append(out, path)
		return nil
	})
	return out, err
}

// fileContainsTestingShort returns true if the file contains "testing.Short()".
func fileContainsTestingShort(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if strings.Contains(sc.Text(), "testing.Short()") {
			return true, nil
		}
	}
	return false, sc.Err()
}

// stringConfig reads a string from env.Config[key], falling back to
// defaultVal when the key is absent or the value is not a string.
func stringConfig(cfg map[string]any, key, defaultVal string) string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return defaultVal
	}
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return defaultVal
}

func init() {
	audit.Default.MustRegister(CITestFlagsCheck{})
}
