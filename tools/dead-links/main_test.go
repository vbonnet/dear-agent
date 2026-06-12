package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsExternalLink(t *testing.T) {
	cases := []struct {
		target string
		want   bool
	}{
		{"https://example.com/foo", true},
		{"http://example.com/foo", true},
		{"mailto:user@example.com", true},
		{"ftp://files.example.com", true},
		{"//cdn.example.com/foo", true},
		{"./path/to/file.md", false},
		{"../docs/ADR.md", false},
		{"SPEC.md", false},
		{"#section-anchor", false},
		{"", false},
	}
	for _, tc := range cases {
		got := isExternalLink(tc.target)
		if got != tc.want {
			t.Errorf("isExternalLink(%q) = %v, want %v", tc.target, got, tc.want)
		}
	}
}

func TestCheckFile_Clean(t *testing.T) {
	dir := t.TempDir()
	// Create a target file so the link is valid.
	target := filepath.Join(dir, "other.md")
	if err := os.WriteFile(target, []byte("# Other"), 0o644); err != nil {
		t.Fatal(err)
	}
	mdFile := filepath.Join(dir, "source.md")
	content := "[Other](./other.md)\n[External](https://example.com)\n[Anchor](#section)\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := checkFile(mdFile, dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheckFile_BrokenLink(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "source.md")
	content := "[Missing](./does-not-exist.md)\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := checkFile(mdFile, dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d: %v", len(findings), findings)
	}
}

func TestCheckFile_ExternalLinksSkipped(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "source.md")
	content := "[GitHub](https://github.com/foo/bar)\n[Docs](http://docs.example.com)\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := checkFile(mdFile, dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (external links skipped), got %d: %v", len(findings), findings)
	}
}

func TestCheckFile_PureAnchorSkipped(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "source.md")
	content := "[Section](#some-section)\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := checkFile(mdFile, dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (anchors skipped), got %d: %v", len(findings), findings)
	}
}

func TestCheckFile_LinkWithAnchor(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "other.md")
	if err := os.WriteFile(target, []byte("# Other"), 0o644); err != nil {
		t.Fatal(err)
	}
	mdFile := filepath.Join(dir, "source.md")
	// Link to a real file with an anchor fragment — should pass (file exists).
	content := "[Other Section](./other.md#section)\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := checkFile(mdFile, dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (file exists, anchor is not checked), got %d: %v", len(findings), findings)
	}
}

func TestCheckFile_RootRelativeLink(t *testing.T) {
	root := t.TempDir()
	// A repo-root-relative target /docs/README.md must resolve against root,
	// not against the linking file's own directory.
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "README.md"), []byte("# R"), 0o644); err != nil {
		t.Fatal(err)
	}
	mdDir := filepath.Join(root, "sub", "deep")
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mdFile := filepath.Join(mdDir, "source.md")
	content := "[Root link](/docs/README.md)\n"
	if err := os.WriteFile(mdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := checkFile(mdFile, root, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (root-relative link resolves to existing file), got %d: %v", len(findings), findings)
	}
}

func TestCheckFile_LongLine(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "source.md")
	// A single line longer than bufio.Scanner's 64KB default token limit,
	// ending in a broken link. A line-scanner would silently skip it.
	var b []byte
	b = append(b, []byte("padding ")...)
	for len(b) < 70*1024 {
		b = append(b, 'x')
	}
	b = append(b, []byte(" [Missing](./does-not-exist.md)\n")...)
	if err := os.WriteFile(mdFile, b, 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := checkFile(mdFile, dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding on a >64KB line, got %d: %v", len(findings), findings)
	}
}

func TestFindMarkdown_DotRootNotSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	// Invoke from inside the temp dir with root ".", reproducing the
	// default-invocation bug where the root's base name "." triggered
	// SkipDir and scanned zero files.
	t.Chdir(dir)
	found, err := findMarkdown(".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("findMarkdown(\".\") found %d files, want 1; got: %v", len(found), found)
	}
}

func TestFindMarkdown(t *testing.T) {
	dir := t.TempDir()
	// Create some files.
	files := []struct {
		path    string
		include bool
	}{
		{"README.md", true},
		{"docs/SPEC.md", true},
		{"code.go", false},
		{".hidden/secret.md", false}, // hidden dir excluded
		{"vendor/dep/README.md", false},
	}
	for _, f := range files {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := findMarkdown(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 0
	for _, f := range files {
		if f.include {
			want++
		}
	}
	if len(found) != want {
		t.Errorf("findMarkdown found %d files, want %d; got: %v", len(found), want, found)
	}
}
