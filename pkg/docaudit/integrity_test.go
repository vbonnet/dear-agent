package docaudit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestClassifyMarker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		age  int
		want FindingKind
	}{
		{name: "current", body: "# Doc\n<!-- Last audited at: 2026-07-18 -->\n", age: 90},
		{name: "marker first", body: "<!-- Last audited at: 2026-07-18 -->\n# Doc\n", age: 90},
		{name: "missing", body: "# Doc\n", age: 90, want: MissingMarker},
		{name: "body example is not marker", body: "# Doc\n\nExplains the convention.\n\n<!-- Last audited at: 2026-07-18 -->\n", age: 90, want: MissingMarker},
		{name: "placeholder", body: "<!-- Last audited at: NEEDS-AUDIT -->\n", age: 90, want: NeedsAudit},
		{name: "malformed", body: "<!-- Last audited at: 2026-07-18 ce-123 -->\n", age: 90, want: MalformedMarker},
		{name: "duplicate", body: "<!-- Last audited at: 2026-07-18 -->\n<!-- Last audited at: 2026-07-18 -->\n", age: 90, want: DuplicateMarker},
		{name: "invalid date", body: "<!-- Last audited at: 2026-02-31 -->\n", age: 90, want: InvalidDate},
		{name: "future", body: "<!-- Last audited at: 2026-07-19 -->\n", age: 90, want: FutureDate},
		{name: "boundary current", body: "<!-- Last audited at: 2026-04-19 -->\n", age: 90},
		{name: "stale", body: "<!-- Last audited at: 2026-04-18 -->\n", age: 90, want: StaleDate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyMarker([]byte(tt.body), tt.age, testAsOf); got != tt.want {
				t.Fatalf("classifyMarker() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckRepositoryRejectsGovernedSymlink(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	writeTestFile(t, repo, ".dear-agent.yml", testPolicy(".living-doc-audit-baseline.txt"))
	writeTestFile(t, repo, ".living-doc-audit-baseline.txt", "")
	outDir := t.TempDir()
	writeTestFile(t, outDir, "SPEC.md", "# External\n\n<!-- Last audited at: 2026-07-18 -->\n")
	symlinkPath := filepath.Join(repo, "pkg", "external", "SPEC.md")
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outDir, "SPEC.md"), symlinkPath); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".")
	_, err := CheckRepository(context.Background(), repo, Options{AsOf: testAsOf})
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("error = %v, want governed-symlink rejection", err)
	}
}

func TestCheckRepositoryRejectsOptionLikeBaselineRef(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	writeTestFile(t, repo, ".dear-agent.yml", testPolicy(".living-doc-audit-baseline.txt"))
	writeTestFile(t, repo, ".living-doc-audit-baseline.txt", "")
	writeTestFile(t, repo, "pkg/good/SPEC.md", "# Good\n\n<!-- Last audited at: 2026-07-18 -->\n")
	gitTest(t, repo, "add", ".")
	_, err := CheckRepository(context.Background(), repo, Options{AsOf: testAsOf, BaselineRef: "--help"})
	if err == nil || !strings.Contains(err.Error(), "invalid baseline ref") {
		t.Fatalf("error = %v, want invalid baseline ref", err)
	}
}

func TestFreshnessBaseRefResolverScenarios(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "scripts", "resolve-doc-audit-base-ref.sh")
	for _, entrypoint := range []string{"Makefile", filepath.Join(".github", "workflows", "doc-freshness.yml")} {
		data, readErr := os.ReadFile(filepath.Join(root, entrypoint))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(data), "scripts/resolve-doc-audit-base-ref.sh") {
			t.Errorf("%s does not invoke the tested base-ref resolver", entrypoint)
		}
	}
	repo := newTestRepo(t)
	writeTestFile(t, repo, "README.md", "base\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTestOutput(t, repo, "rev-parse", "HEAD"))
	writeTestFile(t, repo, "README.md", "current\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "current")
	current := strings.TrimSpace(gitTestOutput(t, repo, "rev-parse", "HEAD"))
	tests := []struct {
		name     string
		explicit string
		env      []string
		want     string
	}{
		{name: "pull request", env: []string{"GITHUB_EVENT_NAME=pull_request", "BASE_SHA=" + base}, want: base},
		{name: "ordinary push", env: []string{"GITHUB_EVENT_NAME=push", "BEFORE_SHA=" + base, "CURRENT_SHA=" + current}, want: base},
		{name: "new branch push", env: []string{"GITHUB_EVENT_NAME=push", "BEFORE_SHA=" + strings.Repeat("0", 40), "CURRENT_SHA=" + current, "DEFAULT_BRANCH=main"}, want: base},
		{name: "schedule skips mutation comparison", env: []string{"GITHUB_EVENT_NAME=schedule"}},
		{name: "explicit local target", explicit: base, want: base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{script}
			if tt.explicit != "" {
				args = append(args, tt.explicit)
			}
			cmd := exec.Command("bash", args...)
			cmd.Dir = repo
			cmd.Env = append(withoutGitHubEventEnv(os.Environ()), "REPO_ROOT="+repo)
			cmd.Env = append(cmd.Env, tt.env...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("resolver: %v\n%s", err, output)
			}
			if got := strings.TrimSpace(string(output)); got != tt.want {
				t.Fatalf("resolved ref = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTrackedMarkdownHasNoNumberedPrincipleReferences(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files, err := trackedFiles(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?i)\bprinciple\s+#?[0-9]+\b`)
	for _, name := range files {
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if match := pattern.Find(data); match != nil {
			t.Errorf("%s retains numbered principle reference %q", name, match)
		}
	}
}

func withoutGitHubEventEnv(environ []string) []string {
	blocked := []string{"GITHUB_EVENT_NAME=", "BASE_SHA=", "BEFORE_SHA=", "CURRENT_SHA=", "DEFAULT_BRANCH="}
	clean := make([]string, 0, len(environ))
	for _, entry := range environ {
		keep := true
		for _, prefix := range blocked {
			if strings.HasPrefix(entry, prefix) {
				keep = false
				break
			}
		}
		if keep {
			clean = append(clean, entry)
		}
	}
	return clean
}

func equalStrings(got, want []string) bool {
	return strings.Join(got, "\x00") == strings.Join(want, "\x00")
}
