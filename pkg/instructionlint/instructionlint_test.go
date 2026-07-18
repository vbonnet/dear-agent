package instructionlint

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSegmentsClassifyExecutableFences(t *testing.T) {
	source := []byte(strings.Join([]string{
		"# W0 prose",
		"Use `git push origin main` only as a quoted bad command.",
		"```markdown",
		"W0 fixture",
		"git push origin fixture",
		"```",
		"```bash",
		"git push origin main",
		"```",
	}, "\n"))

	got := parseSegments(source)
	var compact []string
	for _, item := range got {
		compact = append(compact, string(item.Kind)+"|"+item.Text)
	}
	sort.Strings(compact)
	want := []string{
		"prose|# W0 prose",
		"prose|Use ",
		"prose| only as a quoted bad command.",
		"shell|git push origin main",
		"shell|git push origin main",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(compact, want) {
		t.Fatalf("segments = %#v, want %#v", compact, want)
	}
}

func TestRuleViolationsTeachCanonicalReplacements(t *testing.T) {
	segments := []Segment{
		{Kind: SegmentProse, Line: 1, Text: "Create W0-charter.md before D1."},
		{Kind: SegmentShell, Line: 2, Text: "bd ready"},
		{Kind: SegmentShell, Line: 3, Text: "git push -u origin branch"},
		{Kind: SegmentShell, Line: 4, Text: "gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 5, Text: "safe-pr create --emergency --reason urgent"},
		{Kind: SegmentShell, Line: 6, Text: "agm session health --json"},
		{Kind: SegmentShell, Line: 7, Text: "agm status --output json"},
		{Kind: SegmentShell, Line: 8, Text: "agm search positional-query"},
		{Kind: SegmentShell, Line: 9, Text: "agm new --workspace oss"},
		{Kind: SegmentShell, Line: 10, Text: "agm session send worker hello"},
		{Kind: SegmentShell, Line: 11, Text: "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on ready"},
	}

	var got []Violation
	for _, segment := range segments {
		got = append(got, evaluateSegment("AGENTS.md", segment)...)
	}
	if len(got) != 10 {
		t.Fatalf("violations = %v, want 10", got)
	}
	for _, item := range got {
		if item.Rule == "" || item.Replacement == "" {
			t.Fatalf("violation lacks actionable rule/replacement: %+v", item)
		}
	}
}

func TestApplyExclusionsIsExactAndStaleDetecting(t *testing.T) {
	findings := []Violation{
		{Path: "AGENTS.md", Line: 3, Rule: "bare-beads", Excerpt: "bd ready", Replacement: "canonical bd"},
		{Path: "AGENTS.md", Line: 4, Rule: "bare-beads", Excerpt: "bd ready", Replacement: "canonical bd"},
	}
	exclusions := []Exclusion{
		{Path: "AGENTS.md", Rule: "bare-beads", Excerpt: "bd ready", Count: 1, Owner: "#930", Reason: "root router"},
		{Path: "AGENTS.md", Rule: "raw-git-push", Excerpt: "git push", Count: 1, Owner: "#930", Reason: "root router"},
	}

	got := applyExclusions(findings, exclusions)
	if len(got) != 2 {
		t.Fatalf("violations = %v, want one excess and one stale exclusion", got)
	}
	if got[0].Rule != "bare-beads" || got[1].Rule != "stale-exclusion" {
		t.Fatalf("unexpected violations: %v", got)
	}
}

func TestCheckRepositoryUsesTrackedGovernedInventory(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
  exclusions:
    - path: AGENTS.md
      rule: bare-beads
      excerpt: bd ready
      count: 1
      owner: "#930"
      reason: root router owns the exact legacy line
`)
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n\n```bash\nbd ready\n```\n")
	writeTestFile(t, repo, "untracked.md", "Create W0-charter.md.\n")
	runGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")

	result, violations, err := CheckRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 || result.Files != 1 || result.Exclusions != 1 {
		t.Fatalf("result=%+v violations=%v", result, violations)
	}

	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n\nCreate W0-charter.md.\n\n```bash\nbd ready\n```\n")
	_, violations, err = CheckRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Rule != "wayfinder-v1" {
		t.Fatalf("violations = %v, want unexcluded Wayfinder V1 token", violations)
	}
}

func TestCheckRepositoryRejectsOverlappingSurfaces(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
    - match: "*.md"
      owner: catch-all
`)
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n")
	runGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")
	if _, _, err := CheckRepository(repo); err == nil || !strings.Contains(err.Error(), "multiple instruction surfaces") {
		t.Fatalf("overlapping surfaces error = %v", err)
	}
}

func TestRepositoryInstructionPolicyIsConformant(t *testing.T) {
	root := repositoryRoot(t)
	result, violations, err := CheckRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("repository instruction-policy violations: %v", violations)
	}
	if result.Files < 10 {
		t.Fatalf("governed files = %d, want a nontrivial active inventory", result.Files)
	}
}

func writeTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Skipf("not in a Git worktree: %v", err)
	}
	return strings.TrimSpace(string(output))
}
