//go:build darwin || linux

package specpackage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const testManifestDigestDomain = "dear-agent/spec-governance-package-manifest/v1\x00"

var canonicalTestSourceFiles = mustReadCanonicalTestSourceFiles()

type testFixture struct {
	base          string
	sourceRoot    string
	artifactPath  string
	stagingParent string
	staged        StagedPackage
}

type testPayload struct {
	sourcePath string
	role       string
	mode       fs.FileMode
	content    []byte
}

func TestStageAndValidateProduceDeterministicRootIndependentPackage(t *testing.T) {
	fixture := newFixtureInputs(t)

	unrelatedWorkingDirectory := t.TempDir()
	t.Chdir(unrelatedWorkingDirectory)

	first, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if err != nil {
		t.Fatalf("stage first package from unrelated working directory: %v", err)
	}
	second, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if err != nil {
		t.Fatalf("stage second package from unrelated working directory: %v", err)
	}
	if first.Root == second.Root {
		t.Fatalf("Stage reused private root %q", first.Root)
	}
	for _, root := range []string{first.Root, second.Root} {
		if !filepath.IsAbs(root) {
			t.Errorf("staged root %q is not absolute", root)
		}
		if filepath.Dir(root) != fixture.stagingParent {
			t.Errorf("staged root parent = %q, want %q", filepath.Dir(root), fixture.stagingParent)
		}
	}
	if !reflect.DeepEqual(first.Receipt, second.Receipt) {
		t.Fatalf("root-dependent receipts:\nfirst:  %#v\nsecond: %#v", first.Receipt, second.Receipt)
	}

	validatedFirst, err := Validate(context.Background(), first.Root)
	if err != nil {
		t.Fatalf("validate first staged package: %v", err)
	}
	validatedSecond, err := Validate(context.Background(), second.Root)
	if err != nil {
		t.Fatalf("validate second staged package: %v", err)
	}
	if !reflect.DeepEqual(first.Receipt, validatedFirst) || !reflect.DeepEqual(validatedFirst, validatedSecond) {
		t.Fatalf("Stage and Validate receipts differ:\nstaged: %#v\nfirst:  %#v\nsecond: %#v", first.Receipt, validatedFirst, validatedSecond)
	}

	assertExactPackage(t, first.Root, first.Receipt)
	assertExactPackage(t, second.Root, second.Receipt)
	assertPackageBytesEqual(t, first.Root, second.Root)
}

func TestValidateRejectsTreeContentModeAndManifestTampering(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, *testFixture)
		wantError string
	}{
		{
			name: "missing payload",
			mutate: func(t *testing.T, fixture *testFixture) {
				mustRemove(t, filepath.Join(fixture.staged.Root, "skills", "write-spec", "references", "ears-and-bdd.md"))
			},
			wantError: "file count",
		},
		{
			name: "extra payload",
			mutate: func(t *testing.T, fixture *testFixture) {
				mustWriteFile(t, filepath.Join(fixture.staged.Root, "unexpected.txt"), []byte("not declared\n"), 0o444)
			},
			wantError: "file count",
		},
		{
			name: "content changed",
			mutate: func(t *testing.T, fixture *testFixture) {
				path := filepath.Join(fixture.staged.Root, "skills", "write-spec", "references", "ears-and-bdd.md")
				content := mustReadFile(t, path)
				replaceReadOnlyFile(t, path, append(content, []byte("\nchanged\n")...), 0o444)
			},
			wantError: "approved content digest",
		},
		{
			name: "payload mode changed",
			mutate: func(t *testing.T, fixture *testFixture) {
				mustChmod(t, filepath.Join(fixture.staged.Root, "skills", "audit-specs", "references", "audit-verdicts.md"), 0o644)
			},
			wantError: "mode is 0644, want 0444",
		},
		{
			name: "executable mode changed",
			mutate: func(t *testing.T, fixture *testFixture) {
				mustChmod(t, filepath.Join(fixture.staged.Root, "bin", "specaudit"), 0o755)
			},
			wantError: "mode is 0755, want 0555",
		},
		{
			name: "directory mode changed",
			mutate: func(t *testing.T, fixture *testFixture) {
				mustChmod(t, filepath.Join(fixture.staged.Root, "skills", "audit-specs", "references"), 0o755)
			},
			wantError: "mode is 0755, want 0700",
		},
		{
			name: "manifest content changed",
			mutate: func(t *testing.T, fixture *testFixture) {
				path := filepath.Join(fixture.staged.Root, manifestPath)
				content := mustReadFile(t, path)
				replaceReadOnlyFile(t, path, append(content, '\n'), 0o444)
			},
			wantError: "exact canonical package manifest",
		},
		{
			name: "manifest is equivalent but noncanonical JSON",
			mutate: func(t *testing.T, fixture *testFixture) {
				path := filepath.Join(fixture.staged.Root, manifestPath)
				var compact bytes.Buffer
				if err := json.Compact(&compact, mustReadFile(t, path)); err != nil {
					t.Fatalf("compact manifest: %v", err)
				}
				replaceReadOnlyFile(t, path, append(compact.Bytes(), '\n'), 0o444)
			},
			wantError: "exact canonical package manifest",
		},
		{
			name: "manifest schema changed",
			mutate: func(t *testing.T, fixture *testFixture) {
				path := filepath.Join(fixture.staged.Root, manifestPath)
				content := bytes.Replace(
					mustReadFile(t, path),
					[]byte(`"schema_version": "spec-governance-package/v1"`),
					[]byte(`"schema_version": "spec-governance-package/v2"`),
					1,
				)
				replaceReadOnlyFile(t, path, content, 0o444)
			},
			wantError: "exact canonical package manifest",
		},
		{
			name: "manifest mode changed",
			mutate: func(t *testing.T, fixture *testFixture) {
				mustChmod(t, filepath.Join(fixture.staged.Root, manifestPath), 0o644)
			},
			wantError: "mode is 0644, want 0444",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStagedFixture(t)
			test.mutate(t, fixture)
			_, err := Validate(context.Background(), fixture.staged.Root)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate error = %v, want error containing %q", err, test.wantError)
			}
		})
	}
}

func TestValidateRejectsPackageTreeAboveEntryBound(t *testing.T) {
	fixture := newStagedFixture(t)
	for index := range maxPackageTreeEntries {
		mustWriteFile(
			t,
			filepath.Join(fixture.staged.Root, fmt.Sprintf("unexpected-%02d", index)),
			[]byte("bounded\n"),
			0o444,
		)
	}

	_, err := Validate(context.Background(), fixture.staged.Root)
	wantError := fmt.Sprintf("package tree exceeds the %d-entry bound", maxPackageTreeEntries)
	if err == nil || !strings.Contains(err.Error(), wantError) {
		t.Fatalf("Validate error = %v, want bounded-tree rejection", err)
	}
}

func TestValidateNoPathAliasesRejectsCaseAndUnicodeAliases(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
	}{
		{name: "case", paths: []string{"skills/audit-specs", "skills/AUDIT-SPECS"}},
		{name: "Unicode normalization", paths: []string{"skills/caf\u00e9", "skills/cafe\u0301"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateNoPathAliases(test.paths)
			if err == nil || !strings.Contains(err.Error(), "case or Unicode aliases") {
				t.Fatalf("validateNoPathAliases error = %v, want alias rejection", err)
			}
		})
	}
}

func TestStageRejectsWrongSkillFrontmatterAndRetainsPrivateRoot(t *testing.T) {
	fixture := newFixtureInputs(t)
	path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "SKILL.md")
	content := strings.Replace(string(mustReadFile(t, path)), "name: audit-specs", "name: write-spec", 1)
	mustWriteFile(t, path, []byte(content), 0o600)

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if err == nil || !strings.Contains(err.Error(), `has name "write-spec", want "audit-specs"`) {
		t.Fatalf("Stage = %#v, error = %v; want wrong-name rejection", staged, err)
	}
	assertRetainedFailedRoot(t, fixture.stagingParent, err)
}

func TestStageRejectsExtraCanonicalSourceTreeEntry(t *testing.T) {
	fixture := newFixtureInputs(t)
	extra := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "undeclared.md")
	mustWriteFile(t, extra, []byte("# Undeclared\n"), 0o600)

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if err == nil || !strings.Contains(err.Error(), "source skill file count") {
		t.Fatalf("Stage = %#v, error = %v; want exact-source-tree rejection", staged, err)
	}
	assertDirectoryEmpty(t, fixture.stagingParent)
}

func TestStageRejectsUnapprovedPortableShellCommandsAndRetainsPrivateRoot(t *testing.T) {
	for _, command := range []string{
		"go run ./tools/specaudit",
		"go   run ../tools/specaudit",
		"make lint-specs",
		"cmd/ears-lint",
		"internal/speccoverage",
		`"<distribution-root>/bin/specaudit" inventory -repo . -repository owner/name -revision deadbeef; go run ../tools/specaudit`,
	} {
		t.Run(command, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
			content := append(mustReadFile(t, path), []byte("\n```sh\n"+command+"\n```\n")...)
			mustWriteFile(t, path, content, 0o600)

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if err == nil || !strings.Contains(err.Error(), "not an approved packaged command template") {
				t.Fatalf("Stage = %#v, error = %v; want positive shell-template rejection", staged, err)
			}
			assertRetainedFailedRoot(t, fixture.stagingParent, err)
		})
	}
}

func TestStageRejectsNonportableSpecauditReferences(t *testing.T) {
	for _, test := range []struct {
		name      string
		addition  string
		wantError string
	}{
		{name: "inline checkout command", addition: "`go run ../tools/specaudit`\n", wantError: "nonportable inline specaudit reference"},
		{name: "inline absolute checkout path", addition: "`/tmp/dear-agent/tools/specaudit`\n", wantError: "nonportable inline specaudit reference"},
		{name: "inline whitespace variant", addition: "`go   run specaudit`\n", wantError: "nonportable inline specaudit reference"},
		{name: "prose checkout path", addition: "Use /tmp/dear-agent/tools/specaudit.\n", wantError: "outside a declared non-executable context"},
		{name: "prose PATH and CWD command", addition: "> Run specaudit inventory -repo .\n", wantError: "outside a declared non-executable context"},
		{name: "prose make command", addition: "Run make lint-specs.\n", wantError: "approved content digest"},
		{name: "prose unrelated checkout command", addition: "Please run go run ../tools/other-audit.\n", wantError: "approved content digest"},
		{name: "prose alternate command verb", addition: "Execute make lint-specs.\n", wantError: "approved content digest"},
		{name: "prose entity evasion", addition: "Run spec&#97;udit inventory -repo .\n", wantError: "outside a declared non-executable context"},
		{name: "prose emphasis evasion", addition: "Run spec**audit** inventory -repo .\n", wantError: "outside a declared non-executable context"},
		{name: "prefixed installed child", addition: "`/tmp/<distribution-root>/bin/specaudit`\n", wantError: "nonportable inline specaudit reference"},
		{name: "wrapper alias", addition: "`<distribution-root>/bin/specaudit-wrapper`\n", wantError: "nonportable inline specaudit reference"},
		{name: "case alias", addition: "`/tmp/dear-agent/tools/SpecAudit`\n", wantError: "nonportable inline specaudit reference"},
		{name: "Unicode-adjacent alias", addition: "Use Ⱥspecaudit.\n", wantError: "outside a declared non-executable context"},
		{name: "indented command", addition: "    go run ../tools/specaudit\n", wantError: "ambiguous indented code block"},
		{name: "untyped fence", addition: "```\ngo run ../tools/specaudit\n```\n", wantError: "ambiguous untyped code block"},
		{name: "YAML command", addition: "```yaml\ncommand: go run ../tools/specaudit\n```\n", wantError: "unsupported code block language"},
		{name: "JSON command", addition: "```json\n{\"collector\":\"go run specaudit\"}\n```\n", wantError: "unapproved specaudit reference"},
		{name: "JSON escaped command", addition: "```json\n{\"command\":\"go run ./tools/spec\\u0061udit\"}\n```\n", wantError: "unapproved specaudit reference"},
		{name: "text command", addition: "```text\nspecaudit inventory\n```\n", wantError: "not an approved packaged data template"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
			content := append(mustReadFile(t, path), []byte("\n"+test.addition)...)
			mustWriteFile(t, path, content, 0o600)

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Stage = %#v, error = %v; want error containing %q", staged, err, test.wantError)
			}
			assertRetainedFailedRoot(t, fixture.stagingParent, err)
		})
	}
}

func TestStageRejectsExecutableInlineAndBlockquotedCommands(t *testing.T) {
	for _, test := range []struct {
		name      string
		addition  string
		wantError string
	}{
		{name: "inline make command", addition: "`make lint-specs`\n", wantError: "approved content digest"},
		{name: "split inline make command", addition: "Run `make` `lint-specs`.\n", wantError: "approved content digest"},
		{name: "split inline specaudit command", addition: "Run `specaudit` `inventory` `-repo` `.`.\n", wantError: "approved content digest"},
		{name: "HTTP inline shell separator", addition: "Run `https://example.test/specaudit;make`.\n", wantError: "nonportable inline specaudit reference"},
		{name: "inline shell separator", addition: "`make;lint-specs`\n", wantError: "approved content digest"},
		{name: "inline command path", addition: "`cmd/ears-lint`\n", wantError: "approved content digest"},
		{name: "inline internal path", addition: "`internal/speccoverage`\n", wantError: "approved content digest"},
		{name: "typed text command", addition: "```text\nmake lint-specs\n```\n", wantError: "approved content digest"},
		{name: "mislabelled language command", addition: "```gherkin\nmake lint-specs\n```\n", wantError: "unsupported code block language"},
		{name: "blockquoted shell fence", addition: "> ```sh\n> make lint-specs\n> ```\n", wantError: "ambiguous blockquoted code block"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
			content := append(mustReadFile(t, path), []byte("\n"+test.addition)...)
			mustWriteFile(t, path, content, 0o600)

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Stage = %#v, error = %v; want error containing %q", staged, err, test.wantError)
			}
			assertRetainedFailedRoot(t, fixture.stagingParent, err)
		})
	}
}

func TestStageBoundsSpecauditReferences(t *testing.T) {
	fixture := newFixtureInputs(t)
	path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
	content := append(mustReadFile(t, path), []byte("\n"+strings.Repeat("`specaudit` ", maxMarkdownSpecauditReferences+1)+"\n")...)
	mustWriteFile(t, path, content, 0o600)

	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	wantError := fmt.Sprintf("exceeds the %d-specaudit-reference bound", maxMarkdownSpecauditReferences)
	if err == nil || !strings.Contains(err.Error(), wantError) {
		t.Fatalf("Stage = %#v, error = %v; want bounded-reference rejection %q", staged, err, wantError)
	}
	assertRetainedFailedRoot(t, fixture.stagingParent, err)
}

func TestStageRejectsMutatedPortableCommandTemplates(t *testing.T) {
	for _, test := range []struct {
		name string
		old  string
		new  string
		want string
	}{
		{name: "double backslash then injected command", old: "     -repo \"<repository-path>\" \\\n", new: "     -repo \"<repository-path>\" \\\\\n   go run ../tools/specaudit\n", want: "not an approved packaged command template"},
		{name: "missing flag", old: "     -repository \"<owner/name>\" \\\n", new: "", want: "not an approved packaged command template"},
		{name: "unknown flag", old: "     -repo \"<repository-path>\" \\\n", new: "     -unknown \"<repository-path>\" \\\n", want: "not an approved packaged command template"},
		{name: "reordered flags", old: "     -repo \"<repository-path>\" \\\n     -repository \"<owner/name>\" \\\n", new: "     -repository \"<owner/name>\" \\\n     -repo \"<repository-path>\" \\\n", want: "not an approved packaged command template"},
		{name: "parameter expansion", old: "\"<distribution-root>/bin/specaudit\" inventory \\\n", new: "\"${UNSET}<distribution-root>/bin/specaudit\" inventory \\\n", want: "dynamic word"},
		{name: "command substitution", old: "\"<distribution-root>/bin/specaudit\" inventory \\\n", new: "\"$(printf specaudit)\" inventory \\\n", want: "dynamic word"},
		{name: "append redirection", old: "> inventory.json\n", new: ">> inventory.json\n", want: "unapproved redirection"},
		{name: "shell info metadata", old: "```sh\n", new: "```sh copy\n", want: "noncanonical code block info"},
		{name: "shell info case alias", old: "```sh\n", new: "```SH\n", want: "noncanonical code block info"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "SKILL.md")
			content := string(mustReadFile(t, path))
			mutated := strings.Replace(content, test.old, test.new, 1)
			if mutated == content {
				t.Fatalf("fixture did not contain mutation target %q", test.old)
			}
			mustWriteFile(t, path, []byte(mutated), 0o600)

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Stage = %#v, error = %v; want exact command-template rejection containing %q", staged, err, test.want)
			}
			assertRetainedFailedRoot(t, fixture.stagingParent, err)
		})
	}
}

func TestStageAllowsDeclaredNonExecutableSpecauditData(t *testing.T) {
	fixture := newFixtureInputs(t)
	path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
	addition := "\nThe logical command is `specaudit`. See https://example.test/tools/specaudit, [specaudit](https://example.test/tools/specaudit), and <https://example.test/tools/specaudit>.\n\n" +
		"The installed child is `<distribution-root>/bin/specaudit`.\n\n" +
		"Canonical traceability labels include `BDD Tests`, `BDD Traceability`, `BDD feature`, `Command BDD`, `Cross-surface BDD`, `Cross-surface contracts`, `Package Test Traceability`, `Related feature`, `Status BDD`, `Strict-spec linkage`, `Strictness BDD`, and `Test Traceability`.\n\n" +
		"A full-line claim is `` - `path.feature` ``, and data outcomes include `needs product decision`, `no normative behavior changed`, `runtime_status: UNVERIFIED`, and `spec updated`.\n\n" +
		"```json\n{\"collector\": \"specaudit inventory\"}\n```\n"
	content := append(mustReadFile(t, path), []byte(addition)...)
	mustWriteFile(t, path, content, 0o600)

	if err := validatePortableMarkdownCommands("skills/audit-specs/references/audit-verdicts.md", content); err != nil {
		t.Fatalf("validatePortableMarkdownCommands() = %v; want declared non-executable data accepted", err)
	}
}

func TestStageRejectsEveryUnapprovedProseMutation(t *testing.T) {
	for _, prose := range []string{
		"Run make lint-specs.",
		"Use make lint-specs.",
		"Call make lint-specs.",
		"This is a benign but unapproved sentence.",
	} {
		t.Run(prose, func(t *testing.T) {
			fixture := newFixtureInputs(t)
			path := filepath.Join(fixture.sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
			content := append(mustReadFile(t, path), []byte("\n"+prose+"\n")...)
			mustWriteFile(t, path, content, 0o600)

			staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
			if err == nil || !strings.Contains(err.Error(), "approved content digest") {
				t.Fatalf("Stage = %#v, error = %v; want closed approved-content rejection", staged, err)
			}
			assertRetainedFailedRoot(t, fixture.stagingParent, err)
		})
	}
}

func TestValidateSkillNameRequiresExactFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError string
	}{
		{name: "missing", content: "# Audit\n", wantError: "missing YAML frontmatter"},
		{name: "unterminated", content: "---\nname: audit-specs\n", wantError: "unterminated YAML frontmatter"},
		{name: "malformed", content: "---\nname: [\n---\n", wantError: "parse skill"},
		{name: "wrong name", content: "---\nname: write-spec\n---\n", wantError: `has name "write-spec", want "audit-specs"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSkillName("skills/audit-specs/SKILL.md", []byte(test.content), "audit-specs")
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateSkillName error = %v, want error containing %q", err, test.wantError)
			}
		})
	}
	if err := validateSkillName(
		"skills/audit-specs/SKILL.md",
		[]byte("---\nname: audit-specs\ndescription: portable audit\n---\n# Audit\n"),
		"audit-specs",
	); err != nil {
		t.Fatalf("validate valid frontmatter: %v", err)
	}
}

func TestValidateMarkdownLinksClosesReferencesAndIgnoresCode(t *testing.T) {
	const source = "skills/write-spec/SKILL.md"
	valid := []struct {
		name    string
		content string
	}{
		{name: "inline reference", content: "[contract](references/contract-model.md)\n"},
		{name: "reference definition", content: "[contract][model]\n\n[model]: references/contract-model.md\n"},
		{name: "image reference", content: "![contract model](references/contract-model.md)\n"},
		{name: "same-document fragment", content: "[section](#section)\n"},
		{name: "web and mail links", content: "[web](https://example.com) [mail](mailto:a@example.com)\n"},
		{name: "web and mail autolinks", content: "<https://example.com> <a@example.com>\n"},
		{
			name: "link syntax in code",
			content: "`[inline](../../escape.md)`\n\n" +
				"```markdown\n[block](../../escape.md)\n![image](missing.md)\n```\n",
		},
	}
	for _, test := range valid {
		t.Run("accepts "+test.name, func(t *testing.T) {
			if err := validateMarkdownLinks(source, []byte(test.content)); err != nil {
				t.Fatalf("validateMarkdownLinks rejected %s: %v", test.name, err)
			}
		})
	}

	invalid := []struct {
		name      string
		content   string
		wantError string
	}{
		{name: "escaping link", content: "[escape](../../outside.md)\n", wantError: "escaping local path"},
		{name: "missing link", content: "[missing](references/missing.md)\n", wantError: "unresolved package reference"},
		{name: "escaping image", content: "![escape](../../outside.png)\n", wantError: "escaping local path"},
		{name: "missing image", content: "![missing](references/missing.png)\n", wantError: "unresolved package reference"},
		{
			name:      "missing reference-style link",
			content:   "[missing][target]\n\n[target]: references/missing.md\n",
			wantError: "unresolved package reference",
		},
		{name: "absolute local link", content: "[absolute](/etc/passwd)\n", wantError: "noncanonical local path"},
		{name: "file URL", content: "[file](file:///etc/passwd)\n", wantError: "forbidden link scheme"},
		{name: "file URL autolink", content: "<file:///etc/passwd>\n", wantError: "forbidden link scheme"},
		{name: "raw HTML link", content: "<a href=\"file:///etc/passwd\">file</a>\n", wantError: "unsupported raw HTML"},
		{name: "query-bearing local link", content: "[query](references/contract-model.md?raw=1)\n", wantError: "query-bearing local link"},
	}
	for _, test := range invalid {
		t.Run("rejects "+test.name, func(t *testing.T) {
			err := validateMarkdownLinks(source, []byte(test.content))
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateMarkdownLinks error = %v, want error containing %q", err, test.wantError)
			}
		})
	}

	t.Run("rejects autolinks above the link bound", func(t *testing.T) {
		content := strings.Repeat("<https://example.com>\n", maxMarkdownLinks+1)
		err := validateMarkdownLinks(source, []byte(content))
		if err == nil || !strings.Contains(err.Error(), "exceeds the 4096-link bound") {
			t.Fatalf("validateMarkdownLinks error = %v, want autolink-bound rejection", err)
		}
	})
}

func TestContextCancellationIsFailClosed(t *testing.T) {
	t.Run("nil Stage context", func(t *testing.T) {
		fixture := newFixtureInputs(t)
		var nilContext context.Context
		staged, err := Stage(nilContext, fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
		if err == nil || !strings.Contains(err.Error(), "non-nil context") {
			t.Fatalf("Stage = %#v, error = %v; want nil-context rejection", staged, err)
		}
		assertDirectoryEmpty(t, fixture.stagingParent)
	})

	t.Run("cancelled Validate context", func(t *testing.T) {
		fixture := newStagedFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Validate(ctx, fixture.staged.Root)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Validate error = %v, want context.Canceled", err)
		}
	})

	t.Run("cancellation after artifact open retains allocated root", func(t *testing.T) {
		fixture := newFixtureInputs(t)
		ctx, cancel := context.WithCancel(context.Background())
		oldHook := afterRegularFileOpen
		afterRegularFileOpen = func(relative string) {
			if relative == filepath.Base(fixture.artifactPath) {
				cancel()
			}
		}
		defer func() { afterRegularFileOpen = oldHook }()

		staged, err := Stage(ctx, fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Stage = %#v, error = %v; want context.Canceled", staged, err)
		}
		assertRetainedFailedRoot(t, fixture.stagingParent, err)
	})
}

func newStagedFixture(t *testing.T) *testFixture {
	t.Helper()
	fixture := newFixtureInputs(t)
	staged, err := Stage(context.Background(), fixture.sourceRoot, fixture.artifactPath, fixture.stagingParent)
	if err != nil {
		t.Fatalf("stage valid fixture: %v", err)
	}
	fixture.staged = staged
	return fixture
}

func newFixtureInputs(t *testing.T) *testFixture {
	t.Helper()
	base := t.TempDir()
	sourceRoot := filepath.Join(base, "source")
	stagingParent := filepath.Join(base, "staging")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatalf("create source root: %v", err)
	}
	if err := os.MkdirAll(stagingParent, 0o700); err != nil {
		t.Fatalf("create staging parent: %v", err)
	}

	for path, content := range testSourceFiles() {
		mustWriteFile(t, filepath.Join(sourceRoot, filepath.FromSlash(path)), content, 0o600)
	}
	artifactPath := filepath.Join(base, "specaudit-artifact")
	mustWriteFile(t, artifactPath, testArtifactBytes(), 0o755)
	return &testFixture{
		base:          base,
		sourceRoot:    sourceRoot,
		artifactPath:  artifactPath,
		stagingParent: stagingParent,
	}
}

func testSourceFiles() map[string][]byte {
	files := make(map[string][]byte, len(canonicalTestSourceFiles))
	for sourcePath, content := range canonicalTestSourceFiles {
		files[sourcePath] = bytes.Clone(content)
	}
	return files
}

func mustReadCanonicalTestSourceFiles() map[string][]byte {
	repositoryRoot := mustFindTestRepositoryRoot()
	files := make(map[string][]byte, len(payloadLayout)-1)
	for _, entry := range payloadLayout {
		if entry.sourcePath == "" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(entry.sourcePath)))
		if err != nil {
			panic(fmt.Sprintf("read canonical test source %q: %v", entry.sourcePath, err))
		}
		files[entry.sourcePath] = content
	}
	return files
}

func mustFindTestRepositoryRoot() string {
	directory, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("resolve specpackage test working directory: %v", err))
	}
	for {
		module, readErr := os.ReadFile(filepath.Join(directory, "go.mod"))
		if readErr == nil && bytes.Contains(module, []byte("module github.com/vbonnet/dear-agent\n")) {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			panic("find dear-agent repository root from specpackage test working directory")
		}
		directory = parent
	}
}

func testArtifactBytes() []byte {
	return []byte("#!/bin/sh\nprintf 'portable specaudit\\n'\n")
}

func expectedTestPayloads() map[string]testPayload {
	source := testSourceFiles()
	return map[string]testPayload{
		"bin/specaudit": {
			role:    "executable",
			mode:    0o555,
			content: testArtifactBytes(),
		},
		"skills/audit-specs/SKILL.md": {
			sourcePath: "spec-governance/skills/audit-specs/SKILL.md",
			role:       "skill",
			mode:       0o444,
			content:    source["spec-governance/skills/audit-specs/SKILL.md"],
		},
		"skills/audit-specs/references/audit-verdicts.md": {
			sourcePath: "spec-governance/skills/audit-specs/references/audit-verdicts.md",
			role:       "reference",
			mode:       0o444,
			content:    source["spec-governance/skills/audit-specs/references/audit-verdicts.md"],
		},
		"skills/audit-specs/references/report-schema.md": {
			sourcePath: "spec-governance/skills/audit-specs/references/report-schema.md",
			role:       "reference",
			mode:       0o444,
			content:    source["spec-governance/skills/audit-specs/references/report-schema.md"],
		},
		"skills/write-spec/SKILL.md": {
			sourcePath: "spec-governance/skills/write-spec/SKILL.md",
			role:       "skill",
			mode:       0o444,
			content:    source["spec-governance/skills/write-spec/SKILL.md"],
		},
		"skills/write-spec/references/contract-model.md": {
			sourcePath: "spec-governance/skills/write-spec/references/contract-model.md",
			role:       "reference",
			mode:       0o444,
			content:    source["spec-governance/skills/write-spec/references/contract-model.md"],
		},
		"skills/write-spec/references/ears-and-bdd.md": {
			sourcePath: "spec-governance/skills/write-spec/references/ears-and-bdd.md",
			role:       "reference",
			mode:       0o444,
			content:    source["spec-governance/skills/write-spec/references/ears-and-bdd.md"],
		},
	}
}

func assertExactPackage(t *testing.T, root string, receipt Receipt) {
	t.Helper()
	payloads := expectedTestPayloads()
	expectedModes := map[string]fs.FileMode{
		".":                             0o700,
		"bin":                           0o700,
		"skills":                        0o700,
		"skills/audit-specs":            0o700,
		"skills/audit-specs/references": 0o700,
		"skills/write-spec":             0o700,
		"skills/write-spec/references":  0o700,
		"package-manifest.json":         0o444,
		"bin/specaudit":                 0o555,
		"skills/audit-specs/SKILL.md":   0o444,
		"skills/audit-specs/references/audit-verdicts.md": 0o444,
		"skills/audit-specs/references/report-schema.md":  0o444,
		"skills/write-spec/SKILL.md":                      0o444,
		"skills/write-spec/references/contract-model.md":  0o444,
		"skills/write-spec/references/ears-and-bdd.md":    0o444,
	}

	actualPaths := make([]string, 0, len(expectedModes))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		actualPaths = append(actualPaths, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		wantMode, ok := expectedModes[relative]
		if !ok {
			t.Errorf("unexpected staged path %q", relative)
			return nil
		}
		actualMode := info.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky)
		if actualMode != wantMode {
			t.Errorf("mode for %q = %04o, want %04o", relative, actualMode, wantMode)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk staged package: %v", err)
	}
	if len(actualPaths) != len(expectedModes) {
		t.Errorf("staged path count = %d, want %d: %v", len(actualPaths), len(expectedModes), actualPaths)
	}

	if receipt.SchemaVersion != SchemaVersion {
		t.Errorf("receipt schema = %q, want %q", receipt.SchemaVersion, SchemaVersion)
	}
	if len(receipt.Files) != len(payloads) {
		t.Fatalf("receipt file count = %d, want %d", len(receipt.Files), len(payloads))
	}
	for index, file := range receipt.Files {
		if index > 0 && receipt.Files[index-1].Path >= file.Path {
			t.Errorf("receipt files are not strictly sorted at index %d", index)
		}
		expected, ok := payloads[file.Path]
		if !ok {
			t.Errorf("unexpected receipt path %q", file.Path)
			continue
		}
		digest := sha256.Sum256(expected.content)
		if file.Role != expected.role || file.LogicalMode != modeString(expected.mode) || file.Size != int64(len(expected.content)) || file.SHA256 != hex.EncodeToString(digest[:]) {
			t.Errorf("receipt for %q = %#v, want role=%q mode=%q size=%d sha256=%s", file.Path, file, expected.role, modeString(expected.mode), len(expected.content), hex.EncodeToString(digest[:]))
		}
		if actual := mustReadFile(t, filepath.Join(root, filepath.FromSlash(file.Path))); !bytes.Equal(actual, expected.content) {
			t.Errorf("payload %q differs from expected bytes", file.Path)
		}
	}

	manifest := mustReadFile(t, filepath.Join(root, manifestPath))
	var decoded struct {
		SchemaVersion string        `json:"schema_version"`
		Files         []FileReceipt `json:"files"`
	}
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatalf("decode staged manifest: %v", err)
	}
	if decoded.SchemaVersion != SchemaVersion || !reflect.DeepEqual(decoded.Files, receipt.Files) {
		t.Errorf("manifest does not encode receipt payload: %#v", decoded)
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(testManifestDigestDomain))
	_, _ = digest.Write(manifest)
	if want := hex.EncodeToString(digest.Sum(nil)); receipt.ManifestSHA256 != want {
		t.Errorf("manifest digest = %q, want %q", receipt.ManifestSHA256, want)
	}
}

func assertPackageBytesEqual(t *testing.T, first, second string) {
	t.Helper()
	paths := make([]string, 0, len(expectedTestPayloads())+1)
	for path := range expectedTestPayloads() {
		paths = append(paths, path)
	}
	paths = append(paths, manifestPath)
	sort.Strings(paths)
	for _, path := range paths {
		firstBytes := mustReadFile(t, filepath.Join(first, filepath.FromSlash(path)))
		secondBytes := mustReadFile(t, filepath.Join(second, filepath.FromSlash(path)))
		if !bytes.Equal(firstBytes, secondBytes) {
			t.Errorf("root-dependent bytes for %q", path)
		}
	}
}

func modeString(mode fs.FileMode) string {
	return fmt.Sprintf("%04o", mode)
}

func mustWriteFile(t *testing.T, path string, content []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create parent for %q: %v", path, err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set mode for %q: %v", path, err)
	}
}

func replaceReadOnlyFile(t *testing.T, path string, content []byte, finalMode fs.FileMode) {
	t.Helper()
	mustChmod(t, path, 0o600)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("replace %q: %v", path, err)
	}
	mustChmod(t, path, finalMode)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return content
}

func mustChmod(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %q: %v", path, err)
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %q: %v", path, err)
	}
}

func assertDirectoryEmpty(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read directory %q: %v", path, err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("directory %q retains failed staging entries: %v", path, names)
	}
}

func assertRetainedFailedRoot(t *testing.T, stagingParent string, stageErr error) string {
	t.Helper()
	return assertRetainedFailedRootWithMode(t, stagingParent, stageErr, privateDirectoryMode)
}

func assertRetainedFailedRootWithMode(
	t *testing.T,
	stagingParent string,
	stageErr error,
	wantMode fs.FileMode,
) string {
	t.Helper()
	var retained *RetainedStagingRootError
	if !errors.As(stageErr, &retained) {
		t.Fatalf("Stage error = %v, want RetainedStagingRootError", stageErr)
	}
	if filepath.Dir(retained.Root) != stagingParent {
		t.Fatalf("retained root = %q, want parent %q", retained.Root, stagingParent)
	}
	if !retained.IdentityVerified {
		t.Fatalf("retained root %q identity is not verified", retained.Root)
	}
	info, err := os.Lstat(retained.Root)
	if err != nil || !info.IsDir() || info.Mode().Perm() != wantMode {
		t.Fatalf("retained root identity = %#v, error = %v", info, err)
	}
	entries, err := os.ReadDir(stagingParent)
	if err != nil {
		t.Fatalf("read staging parent %q: %v", stagingParent, err)
	}
	if len(entries) != 1 || filepath.Join(stagingParent, entries[0].Name()) != retained.Root {
		t.Fatalf("staging parent entries = %v, want only retained root %q", entries, retained.Root)
	}
	return retained.Root
}
