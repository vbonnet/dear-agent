package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func runTool(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb strings.Builder
	code = run(context.Background(), args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestRunRejectsBadFlags(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		args       []string
	}{
		{"negative limit", "-limit must be positive", []string{"-limit", "0"}},
		{"unknown format", "unknown -format", []string{"-format", "yaml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, code := runTool(t, tc.args...)
			if code != exitUsage {
				t.Errorf("exit = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tc.want)
			}
		})
	}
}

func TestRunReportsUnreadableRepository(t *testing.T) {
	_, stderr, code := runTool(t, "-repo", t.TempDir())
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "pr-size-audit:") {
		t.Errorf("stderr should name the tool: %q", stderr)
	}
}

// buildHistory makes a repo with one small merge and one oversized merge, so
// the audit has a real offender to find.
func buildHistory(t *testing.T) string {
	t.Helper()
	sb := gittest.Default(t)
	dir := sb.NewRepo(t)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sb.Run(t, dir, "commit", "--allow-empty", "-m", "initial commit")

	write("small.go", "package a\n\nvar Small = 1\n")
	sb.Run(t, dir, "add", "-A")
	sb.Run(t, dir, "commit", "-m", "feat: a small change (#11)")

	var big strings.Builder
	big.WriteString("package a\n")
	for i := range 900 {
		big.WriteString("\nvar Big" + strconv.Itoa(i) + " = " + strconv.Itoa(i) + "\n")
	}
	write("big.go", big.String())
	sb.Run(t, dir, "add", "-A")
	sb.Run(t, dir, "commit", "-m", "feat: a very large change (#12)")
	return dir
}

func TestRunFindsTheOversizedMergeAndSpareTheSmallOne(t *testing.T) {
	dir := buildHistory(t)
	stdout, _, code := runTool(t, "-repo", dir, "-base", "HEAD", "-limit", "10")
	if code != exitOK {
		t.Fatalf("exit = %d, want 0 — the audit reports, it never fails", code)
	}
	if !strings.Contains(stdout, "#12") {
		t.Errorf("the oversized merge must be listed as an offender:\n%s", stdout)
	}
	if strings.Contains(stdout, "| #11 |") {
		t.Errorf("the small merge must NOT be an offender:\n%s", stdout)
	}
	if !strings.Contains(stdout, "<!-- pr-size-audit -->") {
		t.Errorf("markdown output needs its marker:\n%s", stdout)
	}
}

func TestRunTextFormatSummarises(t *testing.T) {
	dir := buildHistory(t)
	stdout, _, code := runTool(t, "-repo", dir, "-base", "HEAD", "-limit", "10", "-format", "text")
	if code != exitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "over_budget=1") {
		t.Errorf("expected exactly one offender:\n%s", stdout)
	}
}

// A budget nobody can exceed must produce a clean report, not an empty one.
func TestRunReportsCleanWindow(t *testing.T) {
	dir := buildHistory(t)
	stdout, _, _ := runTool(t, "-repo", dir, "-base", "HEAD", "-limit", "10",
		"-max-lines", "100000", "-max-files", "1000")
	if !strings.Contains(stdout, "No offenders in this window.") {
		t.Errorf("want an explicit clean statement:\n%s", stdout)
	}
}

func TestOverBudgetUsesEitherDimension(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    merge
		want bool
	}{
		{"within both", merge{Lines: 100, Files: 3}, false},
		{"lines only", merge{Lines: 900, Files: 3}, true},
		{"files only", merge{Lines: 100, Files: 40}, true},
		{"exactly at budget", merge{Lines: 400, Files: 15}, false},
		{"one over on lines", merge{Lines: 401, Files: 15}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.m.OverBudget(defaultMaxLines, defaultMaxFiles); got != tc.want {
				t.Errorf("OverBudget = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPRNumberExtraction(t *testing.T) {
	for subject, want := range map[string]string{
		"fix(disk): alarm on a stale sandbox reaper (#1160)": "1160",
		"Add external PR reviewer poller (#1185)":            "1185",
		"a commit with no pr reference":                      "",
		"mentions (#123) mid-subject but not at the end":     "",
	} {
		got := ""
		if g := prNumber.FindStringSubmatch(subject); g != nil {
			got = g[1]
		}
		if got != want {
			t.Errorf("subject %q -> %q, want %q", subject, got, want)
		}
	}
}

func TestPercentile(t *testing.T) {
	v := []int{5, 1, 4, 2, 3}
	if got := percentile(v, 0.5); got != 3 {
		t.Errorf("median = %d, want 3", got)
	}
	if got := percentile(v, 1); got != 5 {
		t.Errorf("max = %d, want 5", got)
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	// percentile must not reorder the caller's slice.
	if v[0] != 5 {
		t.Errorf("percentile mutated its input: %v", v)
	}
}

// A subject with a pipe would otherwise split the Markdown table column.
func TestSanitizeEscapesTableBreakers(t *testing.T) {
	if got := sanitize("feat: a|b change"); got != `feat: a\|b change` {
		t.Errorf("sanitize = %q", got)
	}
}
