package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping in short mode (requires go build)")
	}
	out := filepath.Join(t.TempDir(), "skill-lint")
	if b, err := exec.Command("go", "build", "-o", out, ".").CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, b)
	}
	return out
}

func writeSkill(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeSkill: %v", err)
	}
	return path
}

const validSkill = `---
model: sonnet
effort: medium
---
# My Skill
Does something useful.
`

const invalidSkill = `# Missing frontmatter
Does not have model/effort fields.
`

func TestBuild(t *testing.T) {
	buildBinary(t)
}

func TestLint_NoArgs_PrintsUsage(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin)
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "Usage") {
		t.Errorf("expected usage output, got:\n%s", out)
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", cmd.ProcessState.ExitCode())
	}
}

func TestLint_ValidFile_ExitsZero(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	// commands/*.md are the files skill-lint checks via -file.
	path := writeSkill(t, dir, "commands/test.md", validSkill)

	cmd := exec.Command(bin, "-file", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("valid skill failed: %v\n%s", err, out)
	}
}

func TestLint_InvalidFile_ExitsOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeSkill(t, dir, "commands/bad.md", invalidSkill)

	cmd := exec.Command(bin, "-file", path)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit for invalid skill")
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", cmd.ProcessState.ExitCode())
	}
}

func TestLint_ValidDir_ExitsZero(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	// Place skills in commands/ so they are picked up by the dir scan.
	writeSkill(t, dir, "commands/a.md", validSkill)
	writeSkill(t, dir, "commands/b.md", validSkill)

	cmd := exec.Command(bin, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("valid dir failed: %v\n%s", err, out)
	}
}

func TestLint_DirWithViolation_ExitsOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	writeSkill(t, dir, "commands/good.md", validSkill)
	writeSkill(t, dir, "commands/bad.md", invalidSkill)

	cmd := exec.Command(bin, dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected non-zero exit for dir with violations")
	}
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", cmd.ProcessState.ExitCode())
	}
}
