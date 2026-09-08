package skilllint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
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
	cmd := gittest.Command(t, dir, args...)
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

func TestCheckContentMatchesCheckFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		content string
	}{
		{
			name:    "skill",
			path:    "skills/example/SKILL.md",
			content: strings.Replace(validSkill("example-skill"), "## Verify", "## Notes", 1),
		},
		{
			name:    "command",
			path:    "commands/example.md",
			content: "---\ndescription: incomplete\n---\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, t.TempDir(), tt.path, tt.content)
			fromFile, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			fromContent := CheckContent(path, []byte(tt.content))
			if !reflect.DeepEqual(fromContent, fromFile) {
				t.Fatalf("CheckContent = %v, CheckFile = %v", fromContent, fromFile)
			}
		})
	}
}

func TestCheckContentRejectsUnknownSurfaceWithoutReadingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "notes", "example.md")
	fromContent := CheckContent(path, []byte(validSkill("example-skill")))
	assertReasons(t, fromContent, []string{"not a recognized SKILL.md or commands/*.md surface"})

	fromFile, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile tried to read an unknown surface: %v", err)
	}
	if !reflect.DeepEqual(fromContent, fromFile) {
		t.Fatalf("CheckContent = %v, CheckFile = %v", fromContent, fromFile)
	}
}

func TestCheckFileCommandSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		frontmatter string
		want        []string
	}{
		{name: "compliant", frontmatter: validCommand},
		{
			name:        "verification criteria accepted",
			frontmatter: strings.Replace(validCommand, "description:", "verification_criteria:\n  - command exits 0\ndescription:", 1),
		},
		{
			name:        "verification criteria must be a list",
			frontmatter: strings.Replace(validCommand, "description:", "verification_criteria: command exits 0\ndescription:", 1),
			want:        []string{"must be a nonempty list of strings"},
		},
		{
			name:        "verification criteria entries must be strings",
			frontmatter: strings.Replace(validCommand, "description:", "verification_criteria:\n  - false\ndescription:", 1),
			want:        []string{"verification_criteria[0] must be a nonempty string"},
		},
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
		{
			name: "allowed tools must be strings",
			frontmatter: `---
model: haiku
effort: low
description: invalid tools
allowed-tools: false
---
`,
			want: []string{"invalid value"},
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
			name:    "content hash accepted",
			content: strings.Replace(validSkill("example-skill"), "description:", "content-hash: 0123456789abcdef\ndescription:", 1),
		},
		{
			name:    "verification criteria accepted",
			content: strings.Replace(validSkill("example-skill"), "description:", "verification_criteria:\n  - output file exists\ndescription:", 1),
		},
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
			name:    "harness-neutral description",
			content: strings.Replace(validSkill("example-skill"), "Use when an agent needs", "Helps an agent with", 1),
		},
		{
			name:    "missing workflow",
			content: strings.Replace(validSkill("example-skill"), "## Workflow\n\n1. Inspect the input.\n2. Apply the change.\n\n", "", 1),
			want:    []string{"missing procedural workflow"},
		},
		{
			name:    "missing verification",
			content: strings.Replace(validSkill("example-skill"), "## Verify", "## Notes", 1),
			want:    []string{"missing nonempty verification or completion section"},
		},
		{
			name: "fenced examples do not satisfy structure",
			content: `---
name: example-skill
description: Use when an agent needs an example.
---

# Example

~~~markdown
## Workflow
1. Example step.
2. Example step.
## Verify
~~~
`,
			want: []string{"missing procedural workflow", "missing nonempty verification or completion section"},
		},
		{
			name: "empty headings do not satisfy structure",
			content: `---
name: example-skill
description: Deterministic example skill.
---

# Example

## Use Cases

## End
`,
			want: []string{"missing procedural workflow", "missing nonempty verification or completion section"},
		},
		{
			name: "comment-only headings do not satisfy structure",
			content: `---
name: example-skill
description: Deterministic example skill.
---

# Example

## Workflow

<!-- TODO: write the workflow -->

## Verify

<!-- TODO: write completion checks -->
`,
			want: []string{"missing procedural workflow", "missing nonempty verification or completion section"},
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
			name:    "empty provider tier pair",
			content: strings.Replace(validSkill("example-skill"), "description:", "model: \"\"\neffort: \"\"\ndescription:", 1),
			want:    []string{"model=\"\" not allowed", "effort=\"\" not allowed"},
		},
		{
			name:    "null provider tier remains declared",
			content: strings.Replace(validSkill("example-skill"), "description:", "model: null\ndescription:", 1),
			want:    []string{"`model:` and `effort:` must be declared together", "model=\"\" not allowed"},
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
			name: "provider extension with provider-only fallback",
			content: strings.Replace(
				strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: [Bash]\ndescription:", 1),
				"Confirm the expected result exists.",
				"When Bash is unavailable, use Bash on another host.",
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
			name: "provider path and neutral fallback share a paragraph",
			content: strings.Replace(
				strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: [Bash]\ndescription:", 1),
				"Confirm the expected result exists.",
				"Use Bash to run checks. If Bash is unavailable, run the command-line program directly.",
				1,
			),
		},
		{
			name:    "provider loader field without fallback",
			content: strings.Replace(validSkill("example-skill"), "description:", "disable-model-invocation: true\ndescription:", 1),
			want:    []string{"provider execution extension requires a non-provider fallback"},
		},
		{
			name: "empty provider tool list remains invalid with fallback",
			content: strings.Replace(
				strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: []\ndescription:", 1),
				"Confirm the expected result exists.",
				"When the provider is unavailable, use the same CLI steps. Confirm the expected result exists.",
				1,
			),
			want: []string{"invalid `allowed-tools:` value"},
		},
		{
			name: "boolean provider tool declaration remains invalid with fallback",
			content: strings.Replace(
				strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: false\ndescription:", 1),
				"Confirm the expected result exists.",
				"When the provider is unavailable, use the same CLI steps. Confirm the expected result exists.",
				1,
			),
			want: []string{"invalid `allowed-tools:` value"},
		},
		{
			name: "provider extension with dotted fallback command",
			content: strings.Replace(
				strings.Replace(validSkill("example-skill"), "description:", "allowed-tools: [Bash]\ndescription:", 1),
				"Confirm the expected result exists.",
				"When the provider is unavailable, run `./bin/tool` directly from a terminal.",
				1,
			),
		},
		{
			name:    "long skill without disclosure",
			content: validSkill("example-skill") + longBody,
			want:    []string{"over 100 lines without a References, Documentation, or Resources section"},
		},
		{
			name: "long skill with fenced-only references",
			content: validSkill("example-skill") +
				"\n```markdown\n## References\n```\n" + longBody,
			want: []string{"over 100 lines without a References, Documentation, or Resources section"},
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
		"missing nonempty verification or completion section",
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

	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	assertReasons(t, violations, []string{
		"missing `model:`",
		"missing `effort:`",
		"missing `allowed-tools:`",
		"content-equivalent to .agents/skills/first/SKILL.md",
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

func TestCheckRepositoryDetectsNormalizedSkillDuplicates(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	first := strings.Replace(
		validSkill("duplicate-skill"),
		"description:",
		"content-hash: aaaaaaaaaaaaaaaa\ndescription:",
		1,
	)
	second := strings.Replace(validSkill("duplicate-skill"),
		"name: duplicate-skill\ndescription: Use when an agent needs a deterministic test workflow.",
		"description: Use when an agent needs a deterministic test workflow.\nname: duplicate-skill",
		1,
	)
	second = strings.Replace(second, "description:", "content-hash: bbbbbbbbbbbbbbbb\ndescription:", 1)
	second = strings.Replace(second, "# duplicate-skill", "<!-- discovery note -->\n# duplicate-skill", 1)
	writeFile(t, repo, "first/SKILL.md", first)
	writeFile(t, repo, "second/SKILL.md", second)
	runGit(t, repo, "add", ".")

	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{"content-equivalent to first/SKILL.md"})
}

func TestCheckRepositoryRejectsDivergentRegularFilesWithSameSkillName(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	first := validSkill("shared-skill")
	second := strings.Replace(first, "2. Apply the change.", "2. Report the result.", 1)
	writeFile(t, repo, "first/SKILL.md", first)
	writeFile(t, repo, "second/SKILL.md", second)
	runGit(t, repo, "add", ".")

	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{`skill name "shared-skill" already has regular-file owner first/SKILL.md`})
}

func TestCheckRepositoryRejectsFrontmatterOnlyDivergenceForSameSkillName(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	first := validSkill("shared-skill")
	second := strings.Replace(
		first,
		"description:",
		"model: haiku\neffort: low\ndescription:",
		1,
	)
	writeFile(t, repo, "first/SKILL.md", first)
	writeFile(t, repo, "second/SKILL.md", second)
	runGit(t, repo, "add", ".")

	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{`skill name "shared-skill" already has regular-file owner first/SKILL.md`})
}

func TestCheckRepositoryDoesNotTreatSymlinkAliasAsDuplicate(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeFile(t, repo, "canonical/SKILL.md", validSkill("canonical"))
	aliasDir := filepath.Join(repo, ".agents", "skills", "canonical")
	if err := os.MkdirAll(aliasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "..", "canonical", "SKILL.md"), filepath.Join(aliasDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "canonical/SKILL.md", ".agents/skills/canonical/SKILL.md")
	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("symlink alias produced violations: %v", violations)
	}
}

func TestCheckRepositoryRejectsUntrackedSkillSymlinkTarget(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeFile(t, repo, "untracked/SKILL.md", validSkill("canonical"))
	aliasDir := filepath.Join(repo, "plugin", "skills", "canonical")
	if err := os.MkdirAll(aliasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "..", "untracked", "SKILL.md"), filepath.Join(aliasDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "plugin/skills/canonical/SKILL.md")

	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{`skill symlink for name "canonical" does not resolve to a tracked regular-file canonical SKILL.md owner`})
}

func TestCheckRepositoryRejectsTrackedNonSkillSymlinkTarget(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeFile(t, repo, "canonical/SKILL.md", validSkill("canonical"))
	writeFile(t, repo, "sources/canonical.md", validSkill("canonical"))
	aliasDir := filepath.Join(repo, "plugin", "skills", "canonical")
	if err := os.MkdirAll(aliasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "..", "sources", "canonical.md"), filepath.Join(aliasDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "canonical/SKILL.md", "sources/canonical.md", "plugin/skills/canonical/SKILL.md")

	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{`skill symlink for name "canonical" resolves to a noncanonical target; expected canonical/SKILL.md`})
}

func TestCheckRepositoryRejectsSymlinkOutsideRoot(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	outside := writeFile(t, t.TempDir(), "SKILL.md", validSkill("outside"))
	aliasDir := filepath.Join(repo, "plugin", "skills", "outside")
	if err := os.MkdirAll(aliasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(aliasDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "plugin/skills/outside/SKILL.md")

	violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	assertReasons(t, violations, []string{"resolves outside the validation root"})
}

func TestCheckRepositoryRequiresGitRepository(t *testing.T) {
	t.Parallel()

	if _, err := CheckRepository(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected operational error outside a Git repository")
	}
}

func TestRepositorySkillSurfacesAreCompliant(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot find repo root: %v", err)
		return
	}
	violations, err := CheckRepository(context.Background(), root)
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

func TestCheckRepositoryHonorsCallerCancellation(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckRepository(ctx, repo); err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("CheckRepository with canceled context = %v, want context canceled", err)
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
