package instructionlint

import (
	"context"
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
		"```",
		"if gh pr merge 789; then",
		"  echo merged",
		"fi",
		"```",
		"```bash",
		"git push origin main",
		"```",
		"```",
		"env TOKEN=x gh pr merge 123",
		"```",
		"```",
		`gh \`,
		"  pr merge 456",
		"```",
		"",
		"    bd ready",
		"    explanatory fixture text",
	}, "\n"))

	got := parseSegments(source)
	var compact []string
	for _, item := range got {
		compact = append(compact, string(item.Kind)+"|"+item.Text)
	}
	sort.Strings(compact)
	want := []string{
		"inline|git push origin main",
		"prose|# W0 prose",
		"prose|Use `git push origin main` only as a quoted bad command.",
		"shell|git push origin main",
		"shell|env TOKEN=x gh pr merge 123",
		"shell|gh pr merge 456",
		"shell|if gh pr merge 789; then",
		"shell|bd ready",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(compact, want) {
		t.Fatalf("segments = %#v, want %#v", compact, want)
	}
}

func TestWrappedShellCommandsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"```sh",
		`gh \`,
		"  pr merge 123",
		`git \`,
		"  push origin branch",
		`safe-pr create \`,
		"  --emergency --reason urgent",
		"```",
	}, "\n"))

	var got []string
	for _, segment := range parseSegments(source) {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			got = append(got, violation.Rule)
		}
	}
	sort.Strings(got)
	want := []string{"raw-gh-merge", "raw-git-push", "safe-pr-emergency"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
}

func TestScriptGuidanceAndCommandSubstitutionsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"#!/usr/bin/env bash",
		"# raw gh pr create is discussed here, not instructed",
		"AGM_HELP='Use `agm new worker`.'",
		"guidance='Use `safe-pr create --emergency --reason urgent`.'",
		"merged=$(gh pr merge 42)",
		"ready=$(bd ready)",
	}, "\n"))

	var got []string
	for _, segment := range parseScriptSegments(source) {
		for _, violation := range evaluateSegment("hook", segment) {
			got = append(got, violation.Rule)
		}
	}
	sort.Strings(got)
	want := []string{"agm-root-new", "bare-beads", "raw-gh-merge", "safe-pr-emergency"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v", got, want)
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
		{Kind: SegmentShell, Line: 12, Text: "bd --db=/tmp/wrong ready"},
		{Kind: SegmentShell, Line: 13, Text: "bd --db=~/beads/context-engine/.beads ready"},
		{Kind: SegmentShell, Line: 14, Text: "env TOKEN=x gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 15, Text: "echo ok && gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 16, Text: "env WORKSPACE=oss bd ready"},
		{Kind: SegmentShell, Line: 17, Text: "echo ok && bd ready"},
		{Kind: SegmentShell, Line: 18, Text: "timeout 5s bd ready"},
		{Kind: SegmentShell, Line: 19, Text: "WORKSPACE=oss bd ready"},
		{Kind: SegmentShell, Line: 20, Text: "env WORKSPACE=oss bd --db ~/beads/context-engine/.beads ready"},
		{Kind: SegmentShell, Line: 21, Text: "bd -db ~/beads/context-engine/.beads ready"},
		{Kind: SegmentShell, Line: 22, Text: "sudo gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 23, Text: "command bd ready"},
		{Kind: SegmentShell, Line: 24, Text: "nohup git push origin branch"},
		{Kind: SegmentShell, Line: 25, Text: "timeout --signal TERM 5s bd ready"},
		{Kind: SegmentShell, Line: 26, Text: "env WORKSPACE=oss agm status --output json"},
		{Kind: SegmentShell, Line: 27, Text: "cd repo && agm session send worker hello"},
		{Kind: SegmentShell, Line: 28, Text: "if gh pr merge 123; then"},
		{Kind: SegmentShell, Line: 29, Text: "gh pr create --title test"},
		{Kind: SegmentShell, Line: 30, Text: "gh pr close 123"},
		{Kind: SegmentShell, Line: 31, Text: "gh pr reopen 123"},
		{Kind: SegmentShell, Line: 32, Text: "merged=$(gh pr merge 123)"},
		{Kind: SegmentShell, Line: 33, Text: "ready=$(bd ready)"},
	}

	var got []Violation
	for _, segment := range segments {
		got = append(got, evaluateSegment("AGENTS.md", segment)...)
	}
	if len(got) != 30 {
		t.Fatalf("violations = %v, want 30", got)
	}
	for _, item := range got {
		if item.Rule == "" || item.Replacement == "" {
			t.Fatalf("violation lacks actionable rule/replacement: %+v", item)
		}
	}
}

func TestCheckRepositoryGovernsAgentReadableYAML(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: .dear-agent.yml
      owner: repository-policy
pr-policy:
  emergency: safe-pr create --emergency --reason bypass
`)
	runGit(t, repo, "add", ".dear-agent.yml")

	result, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 || len(violations) != 1 || violations[0].Rule != "safe-pr-emergency" {
		t.Fatalf("result=%+v violations=%v, want governed YAML emergency violation", result, violations)
	}
}

func TestMarkdownFrontmatterCommandsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte("---\nallowed-tools: Bash(gh pr merge:*), Bash(agm status *)\n---\n\n# Command\n"))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("command.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"agm-root-status", "raw-gh-merge"}) {
		t.Fatalf("frontmatter command rules = %v", rules)
	}
}

func TestMarkdownFrontmatterSequenceCommandsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte("---\nallowed-tools:\n  - Bash(gh pr merge:*)\n  - Bash(agm status *)\n---\n\n# Command\n"))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("command.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"agm-root-status", "raw-gh-merge"}) {
		t.Fatalf("sequence frontmatter command rules = %v", rules)
	}
}

func TestAGMPersistentFlagsDoNotHideLegacyRootCommands(t *testing.T) {
	for _, input := range []string{"agm -o json status", "agm -C repo new worker"} {
		var rules []string
		for _, violation := range evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: input}) {
			rules = append(rules, violation.Rule)
		}
		if len(rules) != 1 {
			t.Fatalf("%q rules = %v, want one legacy AGM finding", input, rules)
		}
	}
}

func TestCheckRepositoryRejectsImportOutsideGovernedInventory(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
`)
	writeTestFile(t, repo, "AGENTS.md", "@import hidden.md\n")
	writeTestFile(t, repo, "hidden.md", "# Hidden instructions\n")
	runGit(t, repo, "add", ".")

	_, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Rule != "ungoverned-import" {
		t.Fatalf("violations = %v, want ungoverned import", violations)
	}
}

func TestCheckRepositoryRejectsUntrackedImportMatchingBroadSurface(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
    - match: skills/**/SKILL.md
      owner: skills
`)
	writeTestFile(t, repo, "AGENTS.md", "@import skills/missing/SKILL.md\n")
	writeTestFile(t, repo, "skills/existing/SKILL.md", "# Existing\n")
	runGit(t, repo, "add", ".")

	_, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Rule != "ungoverned-import" {
		t.Fatalf("violations = %v, want untracked import finding", violations)
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
	rules := []string{got[0].Rule, got[1].Rule}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"bare-beads", "stale-exclusion"}) {
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

	result, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 || result.Files != 1 || result.Exclusions != 1 {
		t.Fatalf("result=%+v violations=%v", result, violations)
	}

	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n\nCreate W0-charter.md.\n\n```bash\nbd ready\n```\n")
	_, violations, err = CheckRepository(context.Background(), repo)
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
	if _, _, err := CheckRepository(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "multiple instruction surfaces") {
		t.Fatalf("overlapping surfaces error = %v", err)
	}
}

func TestCheckRepositoryRejectsSurfaceWithNoTrackedMatches(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
    - match: CODEX.md
      owner: codex
`)
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n")
	runGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")
	if _, _, err := CheckRepository(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "CODEX.md") {
		t.Fatalf("zero-match surface error = %v", err)
	}
}

func TestRepositoryInstructionPolicyIsConformant(t *testing.T) {
	root := repositoryRoot(t)
	result, violations, err := CheckRepository(context.Background(), root)
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
