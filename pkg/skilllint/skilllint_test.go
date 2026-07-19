package skilllint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func validSkill(name string) string {
	return fmt.Sprintf(`---
name: %s
description: Use when an agent needs a deterministic test workflow.
---

# %s

## Workflow

1. Inspect the input.
2. Apply the change.

## Verify

Confirm the expected result exists.
`, name, name)
}

const validCommand = `---
model: haiku
effort: low
description: Run a deterministic command
allowed-tools: Bash(example *)
---

# Run command
`

func TestCheckFileCommandSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		frontmatter string
		want        []string
	}{
		{name: "compliant", frontmatter: validCommand},
		{
			name: "missing required fields",
			frontmatter: `---
description: incomplete
---
`,
			want: []string{"missing `model:`", "missing `effort:`", "missing `allowed-tools:`"},
		},
		{
			name: "invalid tiers",
			frontmatter: `---
model: banana
effort: extreme
description: invalid tiers
allowed-tools: Read
---
`,
			want: []string{"model=\"banana\"", "effort=\"extreme\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeFile(t, t.TempDir(), "commands/example.md", tt.frontmatter)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			assertReasons(t, violations, tt.want)
		})
	}
}

func TestCheckFileSkillSchema(t *testing.T) {
	t.Parallel()

	longBody := strings.Repeat("Supporting detail.\n", 101)
	veryLongBody := strings.Repeat("Supporting detail.\n", 501)
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "compliant", content: validSkill("example-skill")},
		{
			name:    "missing name",
			content: strings.Replace(validSkill("example-skill"), "name: example-skill\n", "", 1),
			want:    []string{"missing `name:`"},
		},
		{
			name:    "invalid name",
			content: strings.Replace(validSkill("example-skill"), "name: example-skill", "name: Example Skill", 1),
			want:    []string{"not kebab-case"},
		},
		{
			name:    "weak trigger",
			content: strings.Replace(validSkill("example-skill"), "Use when an agent needs", "Helps an agent with", 1),
			want:    []string{"expected `Use when` or `Trigger when`"},
		},
		{
			name:    "missing workflow",
			content: strings.Replace(validSkill("example-skill"), "## Workflow\n\n1. Inspect the input.\n2. Apply the change.\n\n", "", 1),
			want:    []string{"missing procedural workflow"},
		},
		{
			name:    "missing verification",
			content: strings.Replace(validSkill("example-skill"), "## Verify", "## Notes", 1),
			want:    []string{"missing verification or completion heading"},
		},
		{
			name:    "unknown extension",
			content: strings.Replace(validSkill("example-skill"), "description:", "arguments: none\ndescription:", 1),
			want:    []string{"unsupported skill frontmatter field `arguments`"},
		},
		{
			name:    "incomplete provider tier pair",
			content: strings.Replace(validSkill("example-skill"), "description:", "model: haiku\ndescription:", 1),
			want:    []string{"`model:` and `effort:` must be declared together"},
		},
		{
			name:    "provider extension without fallback",
			content: strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: [Bash]\ndescription:", 1),
			want:    []string{"provider execution extension requires a non-provider fallback"},
		},
		{
			name: "provider extension with non-actionable unavailable text",
			content: strings.Replace(
				strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: [Bash]\ndescription:", 1),
				"Confirm the expected result exists.",
				"Stop when credentials are unavailable.",
				1,
			),
			want: []string{"provider execution extension requires a non-provider fallback"},
		},
		{
			name: "provider extension with fallback",
			content: strings.Replace(
				strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: [Bash]\ndescription:", 1),
				"Confirm the expected result exists.",
				"When skill activation is unavailable, use the same CLI steps. Confirm the expected result exists.",
				1,
			),
		},
		{
			name:    "long skill without disclosure",
			content: validSkill("example-skill") + longBody,
			want:    []string{"over 100 lines without a References, Documentation, or Resources section"},
		},
		{
			name:    "very long skill crosses review threshold",
			content: validSkill("example-skill") + "\n## References\n\nUse references/example.md when needed.\n" + veryLongBody,
			want:    []string{"exceeds the 500-line review threshold"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeFile(t, t.TempDir(), "skills/example-skill/SKILL.md", tt.content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			assertReasons(t, violations, tt.want)
		})
	}
}

func TestValidateSkillLengthDoesNotCountTerminalNewline(t *testing.T) {
	data := []byte(strings.Repeat("line\n", 100))
	if violations := validateSkillLength("SKILL.md", "", data); len(violations) != 0 {
		t.Fatalf("100-line skill with terminal newline produced violations: %v", violations)
	}
}

func TestCheckFileReportsFrontmatterDefectsAsViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "missing", content: "# No frontmatter\n", want: "no YAML frontmatter"},
		{name: "unterminated", content: "---\nname: broken\n", want: "not terminated"},
		{name: "invalid yaml", content: "---\nname: [\n---\n", want: "frontmatter yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeFile(t, t.TempDir(), "skills/example/SKILL.md", tt.content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			assertReasons(t, violations, []string{tt.want})
		})
	}
}

func TestCheckDirScansEverySurfaceShape(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "commands/good.md", validCommand)
	writeFile(t, dir, "commands/bad.md", "---\ndescription: incomplete\n---\n")
	writeFile(t, dir, ".agents/skills/hidden/SKILL.md", validSkill("hidden"))
	writeFile(t, dir, "root-skill/SKILL.md", strings.Replace(validSkill("root-skill"), "## Verify", "## Notes", 1))
	writeFile(t, dir, "commands/SPEC.md", "# Contract\n")
	writeFile(t, dir, "commands/README.md", "# Documentation\n")

	violations, err := CheckDir(dir)
	if err != nil {
		t.Fatalf("CheckDir: %v", err)
	}
	assertReasons(t, violations, []string{
		"missing `model:`",
		"missing `effort:`",
		"missing `allowed-tools:`",
		"missing verification or completion heading",
	})
}

func TestCheckRepositoryUsesTrackedInventoryAndDetectsDuplicates(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	duplicate := validSkill("duplicate-skill")
	writeFile(t, repo, ".agents/skills/first/SKILL.md", duplicate)
	writeFile(t, repo, "unusual/second/SKILL.md", duplicate)
	writeFile(t, repo, "plugin/commands/bad.md", "---\ndescription: incomplete\n---\n")
	writeFile(t, repo, "untracked/SKILL.md", "# intentionally invalid and untracked\n")
	runGit(t, repo, "add", ".agents/skills/first/SKILL.md", "unusual/second/SKILL.md", "plugin/commands/bad.md")

	violations, err := CheckRepository(repo)
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	assertReasons(t, violations, []string{
		"missing `model:`",
		"missing `effort:`",
		"missing `allowed-tools:`",
		"byte-identical to .agents/skills/first/SKILL.md",
	})
	for _, violation := range violations {
		if strings.Contains(violation.Path, "untracked") {
			t.Fatalf("untracked file was linted: %v", violation)
		}
		if filepath.IsAbs(violation.Path) {
			t.Fatalf("repository violation path should be relative: %v", violation)
		}
	}
}

func TestCheckRepositoryRequiresGitRepository(t *testing.T) {
	t.Parallel()

	if _, err := CheckRepository(t.TempDir()); err == nil {
		t.Fatal("expected operational error outside a Git repository")
	}
}

func TestRepositorySkillSurfacesAreCompliant(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot find repo root: %v", err)
		return
	}
	violations, err := CheckRepository(root)
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d repository skill or command violation(s):", len(violations))
		for _, violation := range violations {
			t.Errorf("  %s", violation)
		}
	}
}

func assertReasons(t *testing.T, violations []Violation, wants []string) {
	t.Helper()
	if len(violations) != len(wants) {
		t.Fatalf("got %d violations, want %d:\n%v", len(violations), len(wants), violations)
	}
	for _, want := range wants {
		found := false
		for _, violation := range violations {
			if strings.Contains(violation.Reason, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no violation contains %q: %v", want, violations)
		}
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
