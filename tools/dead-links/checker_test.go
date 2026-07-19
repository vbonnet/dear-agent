package main

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

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseDocumentUsesMarkdownAST(t *testing.T) {
	markdown := newLinkChecker(".", false).markdown
	source := []byte(strings.Join([]string{
		"# Repeat",
		"# Repeat",
		"# [Linked](target.md) `Code` **Bold**",
		"# Foo",
		"# Foo-1",
		"# Foo",
		`<a id="explicit"></a>`,
		"```md",
		"[not live](missing.md)",
		"```",
		"",
		"[reference][target]",
		"![image](asset.png)",
		"",
		"[target]: target.md#repeat",
	}, "\n"))
	doc := parseDocument(markdown, source)
	for _, anchor := range []string{"repeat", "repeat-1", "linked-code-bold", "foo", "foo-1", "foo-2", "explicit"} {
		if !doc.anchors[anchor] {
			t.Errorf("missing anchor %q: %v", anchor, doc.anchors)
		}
	}
	var targets []string
	for _, link := range doc.links {
		targets = append(targets, link.target)
	}
	sort.Strings(targets)
	if want := []string{"asset.png", "target.md", "target.md#repeat"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %v, want %v", targets, want)
	}
}

func TestRepositoryRootAndInventoryFromSubdirectory(t *testing.T) {
	repo := t.TempDir()
	if output, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	mustWrite(t, filepath.Join(repo, "README.md"), "# Root\n")
	mustWrite(t, filepath.Join(repo, "docs", "nested", "README.md"), "# Nested\n")
	if output, err := exec.Command("git", "-C", repo, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	root, err := repositoryRoot(context.Background(), filepath.Join(repo, "docs", "nested"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := findMarkdown(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("findMarkdown from nested root found %d files, want 2: %v", len(files), files)
	}
}

func TestCheckFileValidatesMissingAnchors(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	mustWrite(t, target, "# Existing Heading\n\n<a name=\"legacy\"></a>\n")
	source := filepath.Join(root, "source.md")
	mustWrite(t, source, strings.Join([]string{
		"# Local Heading",
		"[local ok](#local-heading)",
		"[local missing](#not-here)",
		"[cross ok](target.md#existing-heading)",
		"[explicit ok](target.md#legacy)",
		"[cross missing](target.md#not-here)",
	}, "\n"))
	findings, err := checkFile(source, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %v, want two missing anchors", findings)
	}
}

func TestCheckFileSkipsSchemedDestinations(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.md")
	mustWrite(t, source, strings.Join([]string{
		"[phone](tel:+15551212)",
		"[data](data:text/plain,hello)",
		"[editor](vscode://file/tmp/example)",
		"[network](//cdn.example.test/a.png)",
	}, "\n"))
	findings, err := checkFile(source, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("schemed destinations produced findings: %v", findings)
	}
}

func TestCheckFileRejectsExistingPathOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	mustWrite(t, filepath.Join(parent, "outside.md"), "# Outside\n")
	source := filepath.Join(root, "source.md")
	mustWrite(t, source, "[escape](../outside.md)\n")
	findings, err := checkFile(source, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("outside-root link findings = %v, want one", findings)
	}
}

func TestLinkCheckerCachesTargetDocuments(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "target.md"), "# Target\n")
	mustWrite(t, filepath.Join(root, "one.md"), "[one](target.md#target)\n")
	mustWrite(t, filepath.Join(root, "two.md"), "[two](target.md#target)\n")
	checker := newLinkChecker(root, false)
	for _, source := range []string{"one.md", "two.md"} {
		findings, err := checker.checkFile(filepath.Join(root, source))
		if err != nil || len(findings) != 0 {
			t.Fatalf("check %s = (%v, %v)", source, findings, err)
		}
	}
	if len(checker.documents) != 3 {
		t.Fatalf("document cache size = %d, want three unique documents", len(checker.documents))
	}
}

func TestGithubSlug(t *testing.T) {
	cases := map[string]string{
		"Hello, World!":     "hello-world",
		"API_name v2":       "api_name-v2",
		"Déjà Vu":           "déjà-vu",
		"---":               "---",
		"symbols only (!?)": "symbols-only-",
	}
	for input, want := range cases {
		if got := githubSlug(input); got != want {
			t.Errorf("githubSlug(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLoadAndApplyBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.txt")
	mustWrite(t, path, "# known debt\na.md\tmissing.md\nb.md\t#missing\n")
	baseline, err := loadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	findings := []finding{
		{file: "a.md", line: 2, target: "missing.md"},
		{file: "c.md", line: 4, target: "new.md"},
	}
	outstanding, stale, matched := applyBaseline(findings, baseline)
	if len(outstanding) != 1 || outstanding[0].file != "c.md" {
		t.Fatalf("outstanding = %v", outstanding)
	}
	if want := []string{"b.md\t#missing"}; !reflect.DeepEqual(stale, want) {
		t.Fatalf("stale = %v, want %v", stale, want)
	}
	if matched != 1 {
		t.Fatalf("matched = %d, want 1", matched)
	}
}

func TestLoadBaselineRejectsMalformedAndDuplicateEntries(t *testing.T) {
	for name, content := range map[string]string{
		"malformed": "missing-tab\n",
		"duplicate": "a.md\tx.md\na.md\tx.md\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "baseline.txt")
			mustWrite(t, path, content)
			if _, err := loadBaseline(path); err == nil {
				t.Fatal("invalid baseline accepted")
			}
		})
	}
}

func TestRepositoryBaselineHasNoNewOrStaleDebt(t *testing.T) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("not in a Git worktree: %v", err)
	}
	root := strings.TrimSpace(string(out))
	files, err := findMarkdown(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	checker := newLinkChecker(root, false)
	var findings []finding
	for _, file := range files {
		items, err := checker.checkFile(file)
		if err != nil {
			t.Fatal(err)
		}
		findings = append(findings, items...)
	}
	baseline, err := loadBaseline(filepath.Join(root, ".dead-links-baseline.txt"))
	if err != nil {
		t.Fatal(err)
	}
	newFindings, stale, _ := applyBaseline(findings, baseline)
	if len(newFindings) != 0 || len(stale) != 0 {
		t.Fatalf("link baseline mismatch: new=%v stale=%v", newFindings, stale)
	}
}
