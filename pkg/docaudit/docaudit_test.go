package docaudit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testAsOf = time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)

func TestClassifyMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		age  int
		want FindingKind
	}{
		{name: "current", body: "# Doc\n<!-- Last audited at: 2026-07-18 -->\n", age: 90},
		{name: "missing", body: "# Doc\n", age: 90, want: MissingMarker},
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
			got := classifyMarker([]byte(tt.body), tt.age, testAsOf)
			if got != tt.want {
				t.Fatalf("classifyMarker() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckRepositoryTrackedInventoryAndExactBaseline(t *testing.T) {
	t.Parallel()

	repo := newTestRepo(t)
	writeTestFile(t, repo, ".dear-agent.yml", testPolicy(".living-doc-audit-baseline.txt"))
	writeTestFile(t, repo, "pkg/good/SPEC.md", "<!-- Last audited at: 2026-07-18 -->\n")
	writeTestFile(t, repo, "pkg/debt/SPEC.md", "<!-- Last audited at: NEEDS-AUDIT -->\n")
	writeTestFile(t, repo, ".living-doc-audit-baseline.txt", "needs-audit\tpkg/debt/SPEC.md\n")
	writeTestFile(t, repo, "pkg/untracked/SPEC.md", "# Missing marker\n")
	gitTest(t, repo, "add", ".dear-agent.yml", ".living-doc-audit-baseline.txt", "pkg/good/SPEC.md", "pkg/debt/SPEC.md")
	gitTest(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo, Options{AsOf: testAsOf})
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if report.Documents != 2 {
		t.Fatalf("Documents = %d, want 2", report.Documents)
	}
	if len(report.Findings) != 1 || report.Findings[0].ID() != "needs-audit\tpkg/debt/SPEC.md" {
		t.Fatalf("Findings = %#v", report.Findings)
	}
	if report.Blocking() {
		t.Fatalf("report unexpectedly blocking: %#v", report)
	}
}

func TestCheckRepositoryReportsNewAndStaleBaseline(t *testing.T) {
	t.Parallel()

	repo := newTestRepo(t)
	writeTestFile(t, repo, ".dear-agent.yml", testPolicy(".living-doc-audit-baseline.txt"))
	writeTestFile(t, repo, "pkg/new/SPEC.md", "# Missing marker\n")
	writeTestFile(t, repo, "pkg/fixed/SPEC.md", "<!-- Last audited at: 2026-07-18 -->\n")
	writeTestFile(t, repo, ".living-doc-audit-baseline.txt", "needs-audit\tpkg/fixed/SPEC.md\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo, Options{AsOf: testAsOf})
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if got := findingIDs(report.NewFindings); !equalStrings(got, []string{"missing-marker\tpkg/new/SPEC.md"}) {
		t.Fatalf("NewFindings = %v", got)
	}
	if got := baselineIDs(report.StaleBaseline); !equalStrings(got, []string{"needs-audit\tpkg/fixed/SPEC.md"}) {
		t.Fatalf("StaleBaseline = %v", got)
	}
	if !report.Blocking() {
		t.Fatal("report should block")
	}
}

func TestCheckRepositoryRejectsBaselineGrowthAgainstRef(t *testing.T) {
	t.Parallel()

	repo := newTestRepo(t)
	writeTestFile(t, repo, ".dear-agent.yml", testPolicy(".living-doc-audit-baseline.txt"))
	writeTestFile(t, repo, "pkg/old/SPEC.md", "<!-- Last audited at: NEEDS-AUDIT -->\n")
	writeTestFile(t, repo, ".living-doc-audit-baseline.txt", "needs-audit\tpkg/old/SPEC.md\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTestOutput(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, "pkg/new/SPEC.md", "# Missing marker\n")
	writeTestFile(t, repo, ".living-doc-audit-baseline.txt", "missing-marker\tpkg/new/SPEC.md\nneeds-audit\tpkg/old/SPEC.md\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "grow")

	report, err := CheckRepository(context.Background(), repo, Options{AsOf: testAsOf, BaselineRef: base})
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if got := baselineIDs(report.AddedBaseline); !equalStrings(got, []string{"missing-marker\tpkg/new/SPEC.md"}) {
		t.Fatalf("AddedBaseline = %v", got)
	}
}

func TestCheckRepositoryRejectsBaselineGrowthAfterPathRename(t *testing.T) {
	t.Parallel()

	repo := newTestRepo(t)
	writeTestFile(t, repo, ".dear-agent.yml", testPolicy("old-baseline.txt"))
	writeTestFile(t, repo, "pkg/old/SPEC.md", "<!-- Last audited at: NEEDS-AUDIT -->\n")
	writeTestFile(t, repo, "old-baseline.txt", "needs-audit\tpkg/old/SPEC.md\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTestOutput(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, ".dear-agent.yml", testPolicy("new-baseline.txt"))
	writeTestFile(t, repo, "pkg/new/SPEC.md", "# Missing marker\n")
	writeTestFile(t, repo, "new-baseline.txt", "missing-marker\tpkg/new/SPEC.md\nneeds-audit\tpkg/old/SPEC.md\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "rename and grow")

	report, err := CheckRepository(context.Background(), repo, Options{AsOf: testAsOf, BaselineRef: base})
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if got := baselineIDs(report.AddedBaseline); !equalStrings(got, []string{"missing-marker\tpkg/new/SPEC.md"}) {
		t.Fatalf("AddedBaseline = %v", got)
	}
}

func TestFreshnessEntrypointsUseMutationBaseRef(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "doc-freshness.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflowText := string(workflow)
	for _, required := range []string{"reopened, edited", "BEFORE_SHA: ${{ github.event.before }}", "ref=$BEFORE_SHA"} {
		if !strings.Contains(workflowText, required) {
			t.Errorf("doc-freshness workflow missing %q", required)
		}
	}
	if strings.Contains(workflowText, "ref=HEAD~1") {
		t.Error("push comparison still uses HEAD~1 instead of the event before SHA")
	}
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(makefile), "go run ./tools/doc-audit -repo . -baseline-ref") {
		t.Error("lint-doc-freshness does not enforce a baseline ref")
	}
}

func TestCheckRepositoryAllowsInitialBaselineBootstrap(t *testing.T) {
	t.Parallel()

	repo := newTestRepo(t)
	writeTestFile(t, repo, "README.md", "base\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTestOutput(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, ".dear-agent.yml", testPolicy(".living-doc-audit-baseline.txt"))
	writeTestFile(t, repo, "pkg/debt/SPEC.md", "<!-- Last audited at: NEEDS-AUDIT -->\n")
	writeTestFile(t, repo, ".living-doc-audit-baseline.txt", "needs-audit\tpkg/debt/SPEC.md\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "bootstrap")

	report, err := CheckRepository(context.Background(), repo, Options{AsOf: testAsOf, BaselineRef: base})
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if report.Blocking() {
		t.Fatalf("bootstrap should pass: %#v", report)
	}
}

func TestLoadBaselineRejectsMalformedDuplicateAndUnsorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: "not-an-entry\n"},
		{name: "duplicate", body: "needs-audit\ta/SPEC.md\nneeds-audit\ta/SPEC.md\n"},
		{name: "unsorted", body: "needs-audit\tz/SPEC.md\nmissing-marker\ta/SPEC.md\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "baseline.txt")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadBaselineFile(path); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadPolicyRejectsIncompleteAndOverlappingSurfaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".dear-agent.yml")
	bad := `living-docs:
  baseline: baseline.txt
  surfaces:
    - name: incomplete
      match: "**/SPEC.md"
      max-age-days: 90
`
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPolicy(path); err == nil {
		t.Fatal("expected incomplete policy error")
	}

	good := testPolicy("baseline.txt")
	if err := os.WriteFile(path, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := loadPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	policy.Surfaces = append(policy.Surfaces, policy.Surfaces[0])
	if _, err := matchingSurface("pkg/a/SPEC.md", policy.Surfaces); err == nil {
		t.Fatal("expected overlapping surface error")
	}
}

func testPolicy(baseline string) string {
	return `living-docs:
  baseline: ` + baseline + `
  surfaces:
    - name: specifications
      match: "**/SPEC.md"
      owner: CODEOWNERS
      verification-command: make lint-specs STRICT=1
      max-age-days: 90
`
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitTest(t, dir, "init", "-b", "main")
	gitTest(t, dir, "config", "user.email", "test@example.com")
	gitTest(t, dir, "config", "user.name", "Test")
	return dir
}

func writeTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitTest(t *testing.T, repo string, args ...string) {
	t.Helper()
	_ = gitTestOutput(t, repo, args...)
}

func gitTestOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func findingIDs(findings []Finding) []string {
	ids := make([]string, len(findings))
	for i := range findings {
		ids[i] = findings[i].ID()
	}
	return ids
}

func baselineIDs(entries []BaselineEntry) []string {
	ids := make([]string, len(entries))
	for i := range entries {
		ids[i] = entries[i].ID()
	}
	return ids
}

func equalStrings(got, want []string) bool {
	return strings.Join(got, "\x00") == strings.Join(want, "\x00")
}
