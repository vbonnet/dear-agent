package adrlint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckRepositoryClean(t *testing.T) {
	t.Parallel()

	repo := newADRRepo(t)
	writeADRFile(t, repo, ".dear-agent.yml", policyFixture())
	writeADRFile(t, repo, "docs/adr/001-example.md", recordFixture("001", "Example decision", "Accepted"))
	writeADRFile(t, repo, "docs/adr/README.md", indexFixture("001", "001-example.md", "Example decision", "Accepted"))
	writeADRFile(t, repo, "pkg/hash/ADR.md", "# Hash decisions\n\nStatus: Accepted\n\n## Context\n\nStable hashes.\n")
	writeADRFile(t, repo, "fixtures/testdata/ADR-999-fixture.md", "fixture\n")
	writeADRFile(t, repo, "docs/releases/2026-notes.md", "# 2026 release notes\n")
	writeADRFile(t, repo, "untracked/ADR-777-ignore.md", "not tracked\n")
	gitADR(t, repo, "add", ".dear-agent.yml", "docs", "pkg", "fixtures")
	gitADR(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if report.Records != 2 {
		t.Fatalf("Records = %d, want 2", report.Records)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("Violations = %#v", report.Violations)
	}
}

func TestCheckRepositoryReportsContractDrift(t *testing.T) {
	t.Parallel()

	repo := newADRRepo(t)
	writeADRFile(t, repo, ".dear-agent.yml", policyFixture())
	writeADRFile(t, repo, "docs/adr/ADR-001-first.md", `# ADR-002: Wrong identity

Status: Accepted

[broken](missing.md)
`)
	writeADRFile(t, repo, "docs/adr/ADR-001-second.md", recordFixture("001", "Second", "Proposed"))
	writeADRFile(t, repo, "docs/adr/ADR-003-no-status.md", "# ADR-003: No status\n")
	writeADRFile(t, repo, "docs/adr/ADR-004-old.md", recordFixture("004", "Old", "Superseded"))
	writeADRFile(t, repo, "docs/adr/README.md", `# Index

| ADR | Decision | Status |
| --- | --- | --- |
| [001](ADR-001-first.md) | Different title | Proposed |
| [999](ADR-999-ghost.md) | Ghost | Accepted |
`)
	writeADRFile(t, repo, "pkg/hash/ADR.md", "# Hash decisions\n\nStatus: Accepted\n")
	writeADRFile(t, repo, "other/ADR-007-undeclared.md", recordFixture("007", "Ungoverned", "Accepted"))
	writeADRFile(t, repo, "other/008-bare-undeclared.md", recordFixture("008", "Bare ungoverned", "Accepted"))
	gitADR(t, repo, "add", ".")
	gitADR(t, repo, "commit", "-m", "broken fixture")

	report, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	wantReasons := []string{
		"heading ID",
		"duplicate ADR identity 1",
		"one normalized Status",
		"relative link",
		"Superseded record must link",
		"index identity/title/status differs",
		"missing index entry",
		"index points to non-record",
		"ungoverned ADR path",
	}
	for _, want := range wantReasons {
		if !hasReason(report.Violations, want) {
			t.Errorf("missing violation containing %q; got %#v", want, report.Violations)
		}
	}
	if !report.Blocking() {
		t.Fatal("broken report should block")
	}
}

func TestADRSuccessorLinkMustResolveToAnotherLocalRecord(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "adr")
	writeADRFile(t, root, "docs/adr/ADR-001-old.md", recordFixture("001", "Old", "Superseded"))
	writeADRFile(t, root, "docs/adr/ADR-002-new.md", recordFixture("002", "New", "Accepted"))
	writeADRFile(t, root, "docs/adr/ADR-003-retired.md", recordFixture("003", "Retired", "Deprecated"))

	tests := map[string]struct {
		body string
		want bool
	}{
		"local live successor":   {body: "Status: Superseded by [new](ADR-002-new.md)", want: true},
		"titled live successor":  {body: `Status: Superseded by [new](ADR-002-new.md "successor")`, want: true},
		"self link":              {body: "Status: Superseded by [self](ADR-001-old.md)"},
		"external shape":         {body: "Status: Superseded by [remote](https://example.com/ADR-999-missing.md)"},
		"missing local":          {body: "Status: Superseded by [missing](ADR-999-missing.md)"},
		"ungoverned local":       {body: "Status: Superseded by [fixture](../../fixtures/testdata/ADR-999-fixture.md)"},
		"retired successor":      {body: "Status: Superseded by [retired](ADR-003-retired.md)"},
		"unrelated context link": {body: "[new](ADR-002-new.md)"},
	}
	governed := map[string]bool{
		"docs/adr/ADR-001-old.md":     true,
		"docs/adr/ADR-002-new.md":     true,
		"docs/adr/ADR-003-retired.md": true,
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := hasADRSuccessorLink(root, filepath.ToSlash(filepath.Join("docs", "adr", "ADR-001-old.md")), []byte(tt.body), governed)
			if got != tt.want {
				t.Fatalf("hasADRSuccessorLink from %s = %v, want %v", dir, got, tt.want)
			}
		})
	}
}

func TestMarkdownTargetsIncludeReferenceDefinitions(t *testing.T) {
	targets := markdownTargets([]byte("[inline](ADR-002-live.md \"successor\")\n[decision][successor]\n\n[successor]: <missing.md> \"title\"\n"))
	if !slices.Contains(targets, "ADR-002-live.md") {
		t.Fatalf("markdownTargets() = %v, want titled inline-link target", targets)
	}
	if !slices.Contains(targets, "missing.md") {
		t.Fatalf("markdownTargets() = %v, want reference-definition target", targets)
	}
}

func TestLoadPolicyRejectsInvalidDeclarations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "missing scopes", body: "adr-governance:\n  scopes: []\n"},
		{name: "duplicate scope", body: `adr-governance:
  scopes:
    - path: docs/adr
      index: README.md
    - path: docs/adr
      index: OTHER.md
`},
		{name: "reasonless exclusion", body: `adr-governance:
  scopes:
    - path: docs/adr
      index: README.md
  exclusions:
    - match: "**/testdata/**"
`},
		{name: "negative scope budget", body: `adr-governance:
  max-lines: 300
  scopes:
    - path: docs/adr
      index: README.md
      max-lines: -1
`},
		{name: "negative aggregate budget", body: `adr-governance:
  max-lines: 300
  scopes:
    - path: docs/adr
      index: README.md
  aggregates:
    - path: pkg/hash/ADR.md
      max-lines: -1
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), ".dear-agent.yml")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadPolicy(path); err == nil {
				t.Fatal("expected policy error")
			}
		})
	}
}

func TestGlobPathMatch(t *testing.T) {
	t.Parallel()
	if !globPathMatch("**/testdata/**", "pkg/x/testdata/ADR-001-fixture.md") {
		t.Fatal("recursive glob should match")
	}
	if globPathMatch("**/testdata/**", "pkg/x/ADR-001-live.md") {
		t.Fatal("recursive glob should not overmatch")
	}
}

func policyFixture() string {
	return `adr-governance:
  max-lines: 300
  scopes:
    - path: docs/adr
      index: README.md
  aggregates:
    - path: pkg/hash/ADR.md
  exclusions:
    - match: "**/testdata/**"
      reason: generated fixtures
`
}

func TestCheckRepositoryRejectsMalformedADRInputs(t *testing.T) {
	t.Parallel()

	repo := newADRRepo(t)
	writeADRFile(t, repo, ".dear-agent.yml", policyFixture())
	writeADRFile(t, repo, "docs/adr/ADR-001-example.md", recordFixture("001", "Example decision", "Accepted")+"\nStatus: Draft\n")
	writeADRFile(t, repo, "docs/adr/ADR-37-malformed.md", "# ADR-37: malformed\n\nStatus: Accepted\n")
	writeADRFile(t, repo, "docs/adr/001.md", "# ADR-001: malformed bare name\n\nStatus: Accepted\n")
	writeADRFile(t, repo, "docs/adr/ADR042-sneaky.md", "# ADR-042: malformed missing separator\n\nStatus: Accepted\n")
	writeADRFile(t, repo, "docs/adr/042sneaky.md", "# ADR-042: malformed bare separator\n\nStatus: Accepted\n")
	writeADRFile(t, repo, "docs/adr/ADR 043-spaced.md", "# ADR-043: malformed spaced separator\n\nStatus: Accepted\n")
	writeADRFile(t, repo, "docs/adr/README.md", indexFixture("001", "ADR-001-example.md", "Example decision", "Accepted")+"  | [ADR-001](ADR-001-example.md#context) | Duplicate invalid status | Draft |\n")
	writeADRFile(t, repo, "pkg/hash/ADR.md", "# Hash decisions\n\nStatus: Accepted\n")
	gitADR(t, repo, "add", ".")
	gitADR(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"malformed ADR filename", "malformed ADR index row"} {
		if !hasReason(report.Violations, want) {
			t.Errorf("missing %q violation: %#v", want, report.Violations)
		}
	}
}

func TestADRLikeFilenameBoundaries(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"ADR042-sneaky.md":        true,
		"042sneaky.md":            true,
		"ADR 001-new-decision.md": true,
		"0001_sneaky.md":          true,
		"2026-notes.md":           false,
		"v2-release.md":           false,
	}
	for name, want := range tests {
		if got := adrLikeFilename(name); got != want {
			t.Errorf("adrLikeFilename(%q) = %t, want %t", name, got, want)
		}
	}
}

func TestCheckRepositoryNormalizesNumericIdentityWidth(t *testing.T) {
	t.Parallel()

	repo := newADRRepo(t)
	writeADRFile(t, repo, ".dear-agent.yml", policyFixture())
	writeADRFile(t, repo, "docs/adr/ADR-016-first.md", recordFixture("016", "First", "Accepted"))
	writeADRFile(t, repo, "docs/adr/ADR-0016-second.md", recordFixture("0016", "Second", "Accepted"))
	writeADRFile(t, repo, "docs/adr/README.md", indexFixture("016", "ADR-016-first.md", "First", "Accepted")+indexFixture("0016", "ADR-0016-second.md", "Second", "Accepted"))
	writeADRFile(t, repo, "pkg/hash/ADR.md", "# Hash decisions\n\nStatus: Accepted\n")
	gitADR(t, repo, "add", ".")
	gitADR(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !hasReason(report.Violations, "duplicate ADR identity 16") {
		t.Fatalf("missing normalized identity collision: %#v", report.Violations)
	}
}

func TestCheckRepositoryHonorsPerRecordBudget(t *testing.T) {
	repo := newADRRepo(t)
	policy := strings.Replace(policyFixture(), "index: README.md", "index: README.md\n      max-lines: 5", 1)
	writeADRFile(t, repo, ".dear-agent.yml", policy)
	writeADRFile(t, repo, "docs/adr/ADR-001-example.md", recordFixture("001", "Example decision", "Accepted"))
	writeADRFile(t, repo, "docs/adr/README.md", indexFixture("001", "ADR-001-example.md", "Example decision", "Accepted"))
	writeADRFile(t, repo, "pkg/hash/ADR.md", "# Hash decisions\n\nStatus: Accepted\n")
	gitADR(t, repo, "add", ".")
	gitADR(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !hasReason(report.Violations, "5-line ADR review budget") {
		t.Fatalf("missing per-scope line-budget violation: %#v", report.Violations)
	}
}

func TestCheckRepositoryEnforcesADRLineBudget(t *testing.T) {
	repo := newADRRepo(t)
	writeADRFile(t, repo, ".dear-agent.yml", policyFixture())
	writeADRFile(t, repo, "docs/adr/ADR-001-example.md", recordFixture("001", "Example decision", "Accepted")+strings.Repeat("detail\n", 300))
	writeADRFile(t, repo, "docs/adr/README.md", indexFixture("001", "ADR-001-example.md", "Example decision", "Accepted"))
	writeADRFile(t, repo, "pkg/hash/ADR.md", "# Hash decisions\n\nStatus: Accepted\n")
	gitADR(t, repo, "add", ".")
	gitADR(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !hasReason(report.Violations, "ADR review budget") {
		t.Fatalf("missing line-budget violation: %#v", report.Violations)
	}
}

func TestCheckRepositoryRequiresTrackedIndex(t *testing.T) {
	t.Parallel()

	repo := newADRRepo(t)
	writeADRFile(t, repo, ".dear-agent.yml", policyFixture())
	writeADRFile(t, repo, "docs/adr/ADR-001-example.md", recordFixture("001", "Example decision", "Accepted"))
	writeADRFile(t, repo, "docs/adr/README.md", indexFixture("001", "ADR-001-example.md", "Example decision", "Accepted"))
	writeADRFile(t, repo, "pkg/hash/ADR.md", "# Hash decisions\n\nStatus: Accepted\n")
	gitADR(t, repo, "add", ".dear-agent.yml", "docs/adr/ADR-001-example.md", "pkg/hash/ADR.md")
	gitADR(t, repo, "commit", "-m", "fixture")

	report, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !hasReason(report.Violations, "declared ADR index is not tracked") {
		t.Fatalf("missing untracked-index violation: %#v", report.Violations)
	}
}

func recordFixture(id, title, status string) string {
	return "# ADR-" + id + ": " + title + "\n\nStatus: " + status + "\n\n## Context\n\nContext.\n\n## Decision\n\nDecision.\n\n## Consequences\n\nConsequences.\n"
}

func indexFixture(id, name, title, status string) string {
	return "# Index\n\n| ADR | Decision | Status |\n| --- | --- | --- |\n| [" + id + "](" + name + ") | " + title + " | " + status + " |\n"
}

func newADRRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitADR(t, dir, "init", "-b", "main")
	gitADR(t, dir, "config", "user.email", "test@example.com")
	gitADR(t, dir, "config", "user.name", "Test")
	return dir
}

func writeADRFile(t *testing.T, root, relative, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitADR(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func hasReason(violations []Violation, fragment string) bool {
	for _, violation := range violations {
		if strings.Contains(violation.Reason, fragment) {
			return true
		}
	}
	return false
}
