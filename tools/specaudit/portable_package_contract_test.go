//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/specpackage"
)

func TestPortableSpecGovernancePackageRunsFromUnrelatedWorkingDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sourceRoot := portablePackageRepositoryRoot(t)
	artifact := buildPortableSpecaudit(t, ctx, sourceRoot)
	stagingParent := t.TempDir()

	sourceBefore := portablePackageSourceSnapshot(t, sourceRoot)
	staged, err := specpackage.Stage(ctx, sourceRoot, artifact, stagingParent)
	if err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	validated, err := specpackage.Validate(ctx, staged.Root)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if validated.ManifestSHA256 != staged.Receipt.ManifestSHA256 {
		t.Fatalf("Validate() digest = %q, Stage() digest = %q", validated.ManifestSHA256, staged.Receipt.ManifestSHA256)
	}

	sandbox := gittest.New(t)
	target := sandbox.NewRepo(t)
	writePortableTargetFile(t, target, "component/SPEC.md", `# Portable target specification

## EARS Requirements

**PORTABLE-01** When the target is audited, the system shall report the pinned requirement.

## BDD Traceability

- Feature: `+"`features/portable.feature`"+`
`)
	writePortableTargetFile(t, target, "features/portable.feature", `# SPEC: component/SPEC.md
Feature: Portable target
  Scenario: Pinned behavior
    Then the pinned behavior is reported
`)
	sandbox.Run(t, target, "add", "component/SPEC.md", "features/portable.feature")
	sandbox.Run(t, target, "commit", "-m", "add portable contract")
	revision := strings.TrimSpace(sandbox.Run(t, target, "rev-parse", "HEAD"))
	statusBefore := sandbox.Run(t, target, "status", "--porcelain")

	unrelatedCWD := t.TempDir()
	if pathWithin(unrelatedCWD, sourceRoot) || pathWithin(unrelatedCWD, staged.Root) || pathWithin(unrelatedCWD, target) {
		t.Fatalf("unrelated CWD %q overlaps source, package, or target", unrelatedCWD)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("resolve Git: %v", err)
	}
	gitPath, err = filepath.EvalSymlinks(gitPath)
	if err != nil {
		t.Fatalf("canonicalize Git: %v", err)
	}
	runtimeTools := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(runtimeTools, "git")); err != nil {
		t.Fatalf("create isolated Git toolchain entry: %v", err)
	}
	command := exec.CommandContext(ctx,
		filepath.Join(staged.Root, "bin", "specaudit"),
		"inventory",
		"-repo", target,
		"-repository", "example/portable-target",
		"-revision", revision,
	)
	command.Dir = unrelatedCWD
	command.Env = portableRuntimeEnvironment(sandbox.Env(), runtimeTools)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("staged specaudit inventory failed: %v\n%s", err, stderr.String())
	}
	var inventory report
	if err := json.Unmarshal(output, &inventory); err != nil {
		t.Fatalf("decode staged inventory: %v\n%s", err, output)
	}
	if inventory.Snapshot.Repository != "example/portable-target" || inventory.Snapshot.Revision != revision {
		t.Fatalf("inventory snapshot = %#v, want exact repository and revision", inventory.Snapshot)
	}
	if inventory.Methodology.RuntimeStatus != runtimeStatusUnverified {
		t.Fatalf("inventory runtime status = %q, want %q", inventory.Methodology.RuntimeStatus, runtimeStatusUnverified)
	}
	foundSpec := false
	for _, file := range inventory.Inventory {
		if file.Path == "component/SPEC.md" {
			foundSpec = true
			if len(file.Requirements) != 1 || file.Requirements[0].ID != "PORTABLE-01" {
				t.Fatalf("portable SPEC requirements = %#v, want PORTABLE-01", file.Requirements)
			}
		}
	}
	if !foundSpec {
		t.Fatalf("inventory omitted component/SPEC.md: %#v", inventory.Inventory)
	}
	if statusAfter := sandbox.Run(t, target, "status", "--porcelain"); statusAfter != statusBefore {
		t.Fatalf("target status changed: before %q after %q", statusBefore, statusAfter)
	}
	if head := strings.TrimSpace(sandbox.Run(t, target, "rev-parse", "HEAD")); head != revision {
		t.Fatalf("target HEAD changed to %q, want %q", head, revision)
	}
	if sourceAfter := portablePackageSourceSnapshot(t, sourceRoot); !slices.Equal(sourceAfter, sourceBefore) {
		t.Fatalf("canonical package source changed during staging")
	}
}

func TestPortableSpecGovernancePackageRejectsSourceOverlapBeforeAllocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	repositoryRoot := portablePackageRepositoryRoot(t)
	artifact := buildPortableSpecaudit(t, ctx, repositoryRoot)
	sourceRoot := copyPortablePackageSources(t, repositoryRoot)
	stagingParent := filepath.Join(sourceRoot, "nested-staging-parent")
	if err := os.Mkdir(stagingParent, 0o700); err != nil {
		t.Fatalf("create overlapping staging parent: %v", err)
	}
	sourceBefore := portablePackageSourceSnapshot(t, sourceRoot)

	staged, err := specpackage.Stage(ctx, sourceRoot, artifact, stagingParent)
	if err == nil || !strings.Contains(err.Error(), "staging parent must not be the source root or a source descendant") {
		t.Fatalf("Stage() = %#v, error = %v; want source-overlap rejection", staged, err)
	}
	if staged.Root != "" || staged.Receipt.SchemaVersion != "" ||
		staged.Receipt.ManifestSHA256 != "" || len(staged.Receipt.Files) != 0 {
		t.Fatalf("Stage() returned partial package %#v after source-overlap rejection", staged)
	}
	entries, err := os.ReadDir(stagingParent)
	if err != nil {
		t.Fatalf("read rejected staging parent: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("overlapping staging parent contains allocated entries: %v", entries)
	}
	if sourceAfter := portablePackageSourceSnapshot(t, sourceRoot); !slices.Equal(sourceAfter, sourceBefore) {
		t.Fatal("canonical package source changed during rejected staging")
	}
}

func TestPortableSpecGovernancePackageRejectsUnapprovedExecutableReferencesAndRetainsPrivateRoot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	repositoryRoot := portablePackageRepositoryRoot(t)
	canonicalBefore := portablePackageSourceSnapshot(t, repositoryRoot)
	tests := []struct {
		name      string
		addition  string
		wantError string
	}{
		{name: "combined inline command", addition: "Run `make lint-specs`.", wantError: "approved content digest"},
		{name: "split inline command", addition: "Run `make` `lint-specs`.", wantError: "approved content digest"},
		{name: "executable HTTP code span", addition: "Run `https://example.test/specaudit;make`.", wantError: "nonportable inline specaudit reference"},
		{name: "plain prose command", addition: "Use make lint-specs.", wantError: "approved content digest"},
		{name: "decoded JSON command", addition: "```json\n{\"command\":\"go run ./tools/spec\\u0061udit\"}\n```", wantError: "unapproved specaudit reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sourceRoot := copyPortablePackageSources(t, repositoryRoot)
			artifact := filepath.Join(t.TempDir(), "specaudit")
			if err := os.WriteFile(artifact, []byte("portable-specaudit-fixture"), 0o700); err != nil {
				t.Fatalf("write executable fixture: %v", err)
			}
			mutatedPath := filepath.Join(sourceRoot, "spec-governance", "skills", "audit-specs", "references", "audit-verdicts.md")
			content, err := os.ReadFile(mutatedPath)
			if err != nil {
				t.Fatalf("read copied package source: %v", err)
			}
			content = append(content, []byte("\n"+test.addition+"\n")...)
			if err := os.WriteFile(mutatedPath, content, 0o600); err != nil {
				t.Fatalf("append unapproved executable reference: %v", err)
			}
			stagingParent := t.TempDir()

			staged, stageErr := specpackage.Stage(ctx, sourceRoot, artifact, stagingParent)
			if stageErr == nil || !strings.Contains(stageErr.Error(), test.wantError) {
				t.Fatalf("Stage() = %#v, error = %v; want rejection containing %q", staged, stageErr, test.wantError)
			}
			if staged.Root != "" || staged.Receipt.SchemaVersion != "" ||
				staged.Receipt.ManifestSHA256 != "" || len(staged.Receipt.Files) != 0 {
				t.Fatalf("Stage() returned partial package %#v after portable-command rejection", staged)
			}
			var retained *specpackage.RetainedStagingRootError
			if !errors.As(stageErr, &retained) {
				t.Fatalf("Stage() error = %v; want retained-root receipt", stageErr)
			}
			if !retained.IdentityVerified || filepath.Dir(retained.Root) != stagingParent {
				t.Fatalf("retained root = %#v, want verified child of %q", retained, stagingParent)
			}
			if info, err := os.Lstat(retained.Root); err != nil || !info.IsDir() {
				t.Fatalf("retained failed root missing: info = %#v, error = %v", info, err)
			}
		})
	}
	if canonicalAfter := portablePackageSourceSnapshot(t, repositoryRoot); !slices.Equal(canonicalAfter, canonicalBefore) {
		t.Fatal("canonical package source changed during rejected staging")
	}
}

func portablePackageRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve portable package test working directory: %v", err)
	}
	for {
		module, readErr := os.ReadFile(filepath.Join(root, "go.mod"))
		if readErr == nil && bytes.Contains(module, []byte("module github.com/vbonnet/dear-agent\n")) {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("find dear-agent repository root from portable package test working directory")
		}
		root = parent
	}
}

func copyPortablePackageSources(t *testing.T, repositoryRoot string) string {
	t.Helper()
	sourceRoot := filepath.Join(t.TempDir(), "source")
	for _, relative := range payloadLayoutForTest {
		content, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read canonical package source %s: %v", relative, err)
		}
		absolute := filepath.Join(sourceRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatalf("create copied package source directory: %v", err)
		}
		if err := os.WriteFile(absolute, content, 0o600); err != nil {
			t.Fatalf("copy canonical package source %s: %v", relative, err)
		}
	}
	return sourceRoot
}

func buildPortableSpecaudit(t *testing.T, ctx context.Context, sourceRoot string) string {
	t.Helper()
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("resolve Go: %v", err)
	}
	buildRoot := t.TempDir()
	artifact := filepath.Join(buildRoot, "specaudit")
	command := exec.CommandContext(ctx, goPath,
		"build", "-trimpath", "-buildvcs=false", "-o", artifact, "./tools/specaudit",
	)
	command.Dir = sourceRoot
	command.Env = portableBuildEnvironment(os.Environ(), t.TempDir(), t.TempDir())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build portable specaudit: %v\n%s", err, output)
	}
	return artifact
}

func portableBuildEnvironment(base []string, cacheRoot, temporaryRoot string) []string {
	return replacePortableEnvironment(base, map[string]string{
		"CGO_ENABLED": "0",
		"GOCACHE":     cacheRoot,
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOTOOLCHAIN": "local",
		"GOTMPDIR":    temporaryRoot,
		"GOWORK":      "off",
	})
}

func portableRuntimeEnvironment(base []string, toolDirectory string) []string {
	updates := map[string]string{
		"PATH":        toolDirectory,
		"CGO_ENABLED": "",
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	}
	return replacePortableEnvironment(base, updates)
}

func replacePortableEnvironment(base []string, updates map[string]string) []string {
	result := make([]string, 0, len(base)+len(updates))
	for _, item := range base {
		name, _, found := strings.Cut(item, "=")
		if found {
			if _, replaced := updates[name]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		result = append(result, key+"="+updates[key])
	}
	return result
}

func portablePackageSourceSnapshot(t *testing.T, root string) []string {
	t.Helper()
	result := make([]string, 0, len(payloadLayoutForTest))
	for _, relative := range payloadLayoutForTest {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read package source %s: %v", relative, err)
		}
		result = append(result, fmt.Sprintf("%s\x00%x", relative, sha256.Sum256(content)))
	}
	return result
}

var payloadLayoutForTest = []string{
	"spec-governance/skills/audit-specs/SKILL.md",
	"spec-governance/skills/audit-specs/references/audit-verdicts.md",
	"spec-governance/skills/audit-specs/references/report-schema.md",
	"spec-governance/skills/write-spec/SKILL.md",
	"spec-governance/skills/write-spec/references/contract-model.md",
	"spec-governance/skills/write-spec/references/ears-and-bdd.md",
}

func writePortableTargetFile(t *testing.T, root, relative, content string) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
		t.Fatalf("write target fixture: %v", err)
	}
}

func pathWithin(candidate, root string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
