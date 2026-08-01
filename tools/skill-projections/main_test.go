//go:build darwin || linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const canonicalSkill = `---
name: %s
description: Use when a test needs %s.
---

# Canonical
`

func TestCheckDelegatesAcceptsExactProjection(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	writeTestDelegates(t, repo)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 canonical skill discovery delegates") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestWriteDelegatesCreatesExactBytesAndMode(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	root := openTestRepositoryRoot(t, repo)
	expected, err := expectedDelegates(root)
	if err != nil {
		t.Fatalf("expectedDelegates: %v", err)
	}
	if err := writeDelegates(root); err != nil {
		t.Fatalf("writeDelegates: %v", err)
	}
	for _, item := range projections {
		path := filepath.Join(repo, filepath.FromSlash(item.delegate))
		actual, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", item.delegate, err)
		}
		if string(actual) != expected[item.delegate] {
			t.Fatalf("%s bytes differ\ngot:\n%s\nwant:\n%s", item.delegate, actual, expected[item.delegate])
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", item.delegate, err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("%s mode = %v, want regular file", item.delegate, info.Mode())
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("%s mode = %04o, want 0644", item.delegate, info.Mode().Perm())
		}
	}
}

func TestCheckDelegatesRejectsDrift(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	writeTestDelegates(t, repo)
	path := filepath.Join(repo, ".agents", "skills", "write-spec", "SKILL.md")
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale delegate: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "generated delegate is stale") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCheckModeNeverWritesMissingDelegate(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	path := filepath.Join(repo, ".agents", "skills", "write-spec", "SKILL.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("check mode wrote %s: stat error=%v", path, err)
	}
}

func TestCheckDelegatesRejectsUnexpectedMarker(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	writeTestDelegates(t, repo)
	path := filepath.Join(repo, "other", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(generatedMarker), 0o644); err != nil {
		t.Fatalf("write unexpected delegate: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unexpected generated delegate marker") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWriteDelegatesNeverOverwritesOrDeletesExistingEntry(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	first := filepath.Join(repo, filepath.FromSlash(projections[0].delegate))
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const sentinel = "maintainer-owned bytes\n"
	if err := os.WriteFile(first, []byte(sentinel), 0o640); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	root := openTestRepositoryRoot(t, repo)
	if err := writeDelegates(root); err == nil || !strings.Contains(err.Error(), "refusing to overwrite or delete an existing entry") {
		t.Fatalf("writeDelegates error = %v", err)
	}
	actual, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if string(actual) != sentinel {
		t.Fatalf("existing entry changed: got %q, want %q", actual, sentinel)
	}
	second := filepath.Join(repo, filepath.FromSlash(projections[1].delegate))
	if _, err := os.Lstat(second); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second delegate was created before preflight completed: %v", err)
	}
}

func TestWriteDelegatesRefusesExistingFinalSymlink(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	target := filepath.Join(repo, filepath.FromSlash(projections[0].delegate))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	root := openTestRepositoryRoot(t, repo)
	if err := writeDelegates(root); err == nil || !strings.Contains(err.Error(), "refusing to overwrite or delete an existing entry") {
		t.Fatalf("writeDelegates error = %v", err)
	}
	actual, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside: %v", err)
	}
	if string(actual) != "outside\n" {
		t.Fatalf("outside bytes changed: %q", actual)
	}
}

func TestWriteDelegatesRefusesSymlinkedParent(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".agents", "skills")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	root := openTestRepositoryRoot(t, repo)
	if err := writeDelegates(root); err == nil || !strings.Contains(err.Error(), "refusing to traverse symlinked parent") {
		t.Fatalf("writeDelegates error = %v", err)
	}
}

func TestRunWriteRefusesCallerSelectedArbitraryRoot(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", repo, "-write"}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "write mode refuses caller-selected repository root") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	for _, item := range projections {
		path := filepath.Join(repo, filepath.FromSlash(item.delegate))
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("arbitrary-root write created %s: %v", item.delegate, err)
		}
	}
}

func TestRunWriteRejectsInheritedGitRepositoryOverride(t *testing.T) {
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "attacker-git-dir"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-repo", ".", "-write"}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "refuses inherited Git environment override GIT_DIR") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAuthenticateWriteRootAcceptsOnlyTheCurrentLinkedWorktree(t *testing.T) {
	primary, linked := createLinkedWorktree(t)

	got, err := authenticateWriteRootFrom(linked, linked)
	if err != nil {
		t.Fatalf("authenticate linked worktree: %v", err)
	}
	closeRepositoryRootAtCleanup(t, got.repository)
	want, err := canonicalExistingPath(linked)
	if err != nil {
		t.Fatalf("canonical linked worktree: %v", err)
	}
	if got.rootPath != want {
		t.Fatalf("authenticate linked worktree = %q, want %q", got.rootPath, want)
	}
	if got.repository.identity() == "" {
		t.Fatal("authenticated root omitted its retained identity")
	}
	if _, err := authenticateWriteRootFrom(primary, primary); err == nil || !strings.Contains(err.Error(), "primary checkout") {
		t.Fatalf("authenticate primary worktree error = %v", err)
	}

	fakeDirectory := t.TempDir()
	fakeGit := filepath.Join(fakeDirectory, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nexit 91\n"), 0o755); err != nil {
		t.Fatalf("write fake PATH git: %v", err)
	}
	t.Setenv("PATH", fakeDirectory)
	second, err := authenticateWriteRootFrom(linked, linked)
	if err != nil {
		t.Fatalf("authenticate linked worktree with fake PATH: %v", err)
	}
	closeRepositoryRootAtCleanup(t, second.repository)
	if second.gitExecutable == fakeGit {
		t.Fatalf("authentication trusted caller PATH executable %s", fakeGit)
	}
}

func TestAuthenticateRetainsRequestedRootBeforeDiscoveryAndRejectsAncestorReplacement(t *testing.T) {
	base, err := canonicalExistingPath(t.TempDir())
	if err != nil {
		t.Fatalf("canonical fixture root: %v", err)
	}
	ancestor := filepath.Join(base, "caller")
	requested := filepath.Join(ancestor, "worktree")
	heldAncestor := ancestor + "-held"
	commonDirectory := filepath.Join(base, "common")
	gitDirectory := filepath.Join(commonDirectory, "worktrees", "projection")
	if err := os.MkdirAll(requested, 0o755); err != nil {
		t.Fatalf("mkdir requested root: %v", err)
	}
	if err := os.MkdirAll(gitDirectory, 0o755); err != nil {
		t.Fatalf("mkdir Git directory: %v", err)
	}
	gitEntry := filepath.Join(requested, ".git")
	if err := os.WriteFile(gitEntry, []byte("gitdir: "+gitDirectory+"\n"), 0o644); err != nil {
		t.Fatalf("write original .git pointer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "gitdir"), []byte(gitEntry+"\n"), 0o644); err != nil {
		t.Fatalf("write reciprocal backlink: %v", err)
	}
	originalRoot, err := openRepositoryRoot(requested)
	if err != nil {
		t.Fatalf("open original root identity: %v", err)
	}
	originalIdentity := originalRoot.identity()
	if err := originalRoot.close(); err != nil {
		t.Fatalf("close original root identity: %v", err)
	}

	discoveryCalled := false
	discover := func(_, _ string, _ []string) (string, string, string, error) {
		discoveryCalled = true
		if err := os.Rename(ancestor, heldAncestor); err != nil {
			return "", "", "", fmt.Errorf("replace caller-root ancestor: %w", err)
		}
		if err := os.MkdirAll(requested, 0o755); err != nil {
			return "", "", "", fmt.Errorf("create same-path replacement: %w", err)
		}
		if err := os.WriteFile(filepath.Join(requested, ".git"), []byte("gitdir: "+gitDirectory+"\n"), 0o644); err != nil {
			return "", "", "", fmt.Errorf("write replacement .git pointer: %w", err)
		}
		worktreePath, err := canonicalExistingPath(requested)
		if err != nil {
			return "", "", "", err
		}
		gitPath, err := canonicalExistingPath(gitDirectory)
		if err != nil {
			return "", "", "", err
		}
		commonPath, err := canonicalExistingPath(commonDirectory)
		if err != nil {
			return "", "", "", err
		}
		return worktreePath, gitPath, commonPath, nil
	}
	authentication, err := authenticateWriteRootFromWithDiscovery(requested, requested, discover)
	if authentication.repository != nil {
		_ = authentication.repository.close()
	}
	if !discoveryCalled {
		t.Fatal("Git discovery seam was not exercised")
	}
	if err == nil || !strings.Contains(err.Error(), "Git-reported worktree identity") || !strings.Contains(err.Error(), "identities do not match") {
		t.Fatalf("ancestor-replacement authentication error = %v", err)
	}
	heldRoot := openTestRepositoryRoot(t, filepath.Join(heldAncestor, "worktree"))
	if heldRoot.identity() != originalIdentity {
		t.Fatalf("retained original identity moved to %s, want %s", heldRoot.identity(), originalIdentity)
	}
	replacementRoot := openTestRepositoryRoot(t, requested)
	if replacementRoot.identity() == originalIdentity {
		t.Fatalf("same-path replacement unexpectedly reused original identity %s", originalIdentity)
	}
}

func TestGitAuthenticationEnvironmentRejectsRepositoryOverrides(t *testing.T) {
	gitExecutable, err := resolveTrustedGitExecutable()
	if err != nil {
		t.Fatalf("resolveTrustedGitExecutable: %v", err)
	}
	overrides := []string{
		"GIT_DIR=/attacker",
		"GIT_WORK_TREE=/attacker",
		"GIT_COMMON_DIR=/attacker",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.worktree",
		"GIT_CONFIG_VALUE_0=/attacker",
	}
	for _, override := range overrides {
		t.Run(strings.SplitN(override, "=", 2)[0], func(t *testing.T) {
			if _, err := gitAuthenticationEnvironment([]string{"PATH=/attacker", override}, gitExecutable); err == nil || !strings.Contains(err.Error(), "refuses inherited Git environment override") {
				t.Fatalf("gitAuthenticationEnvironment(%q) error = %v", override, err)
			}
		})
	}
}

func TestGitAuthenticationEnvironmentIsAllowlistedAndConfigIsDisabled(t *testing.T) {
	gitExecutable, err := resolveTrustedGitExecutable()
	if err != nil {
		t.Fatalf("resolveTrustedGitExecutable: %v", err)
	}
	environment, err := gitAuthenticationEnvironment([]string{
		"PATH=/attacker",
		"LANG=C",
		"LC_CTYPE=UTF-8",
		"HOME=/attacker-home",
		"SECRET=value",
		"GIT_PAGER=attacker-pager",
		"GIT_TERMINAL_PROMPT=1",
	}, gitExecutable)
	if err != nil {
		t.Fatalf("gitAuthenticationEnvironment: %v", err)
	}
	joined := strings.Join(environment, "\n")
	for _, expected := range []string{
		"PATH=" + filepath.Dir(gitExecutable) + string(os.PathListSeparator) + "/bin",
		"LANG=C",
		"LC_CTYPE=UTF-8",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("sanitized environment %q omits %q", environment, expected)
		}
	}
	for _, forbidden := range []string{"PATH=/attacker", "HOME=", "SECRET=", "GIT_PAGER=", "GIT_TERMINAL_PROMPT=1"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("sanitized environment %q retains %q", environment, forbidden)
		}
	}
}

func TestVerifyLinkedWorktreePointersRequiresReciprocalRegularFiles(t *testing.T) {
	worktreeBase, err := canonicalExistingPath(t.TempDir())
	if err != nil {
		t.Fatalf("canonical worktree fixture root: %v", err)
	}
	gitBase, err := canonicalExistingPath(t.TempDir())
	if err != nil {
		t.Fatalf("canonical Git fixture root: %v", err)
	}
	worktree := filepath.Join(worktreeBase, "worktree")
	gitDirectory := filepath.Join(gitBase, "common", "worktrees", "projection")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	if err := os.MkdirAll(gitDirectory, 0o755); err != nil {
		t.Fatalf("mkdir git directory: %v", err)
	}
	gitEntry := filepath.Join(worktree, ".git")
	if err := os.WriteFile(gitEntry, []byte("gitdir: "+gitDirectory+"\n"), 0o644); err != nil {
		t.Fatalf("write .git pointer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "gitdir"), []byte(gitEntry+"\n"), 0o644); err != nil {
		t.Fatalf("write gitdir backlink: %v", err)
	}
	root := openTestRepositoryRoot(t, worktree)
	if err := verifyLinkedWorktreePointers(root, worktree, gitDirectory); err != nil {
		t.Fatalf("verifyLinkedWorktreePointers: %v", err)
	}

	otherWorktree := filepath.Join(t.TempDir(), ".git")
	if err := os.WriteFile(otherWorktree, []byte("other\n"), 0o644); err != nil {
		t.Fatalf("write other worktree pointer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "gitdir"), []byte(otherWorktree+"\n"), 0o644); err != nil {
		t.Fatalf("misdirect gitdir backlink: %v", err)
	}
	if err := verifyLinkedWorktreePointers(root, worktree, gitDirectory); err == nil || !strings.Contains(err.Error(), "retained worktree path") {
		t.Fatalf("misdirected backlink error = %v", err)
	}
}

func TestAuthenticatedRootDescriptorSurvivesRenameAndReplacement(t *testing.T) {
	_, linked := createLinkedWorktree(t)
	writeCanonicalSkills(t, linked)
	authentication, err := authenticateWriteRootFrom(linked, linked)
	if err != nil {
		t.Fatalf("authenticate linked worktree: %v", err)
	}
	closeRepositoryRootAtCleanup(t, authentication.repository)
	expected, err := expectedDelegates(authentication.repository)
	if err != nil {
		t.Fatalf("expected delegates: %v", err)
	}
	identity := authentication.repository.identity()
	held := linked + "-held"
	created := make([]string, 0, len(projections))
	creator := func(root *repositoryRoot, relative string, content []byte) (bool, error) {
		if root != authentication.repository || root.identity() != identity {
			return false, fmt.Errorf("creator received a different retained root identity")
		}
		wasCreated, createErr := root.createExclusive(relative, content)
		if createErr != nil {
			return wasCreated, createErr
		}
		created = append(created, relative)
		if len(created) != 1 {
			return wasCreated, nil
		}
		if err := os.Rename(linked, held); err != nil {
			return wasCreated, fmt.Errorf("rename authenticated root: %w", err)
		}
		if err := os.Mkdir(linked, 0o755); err != nil {
			return wasCreated, fmt.Errorf("create replacement root: %w", err)
		}
		writeCanonicalSkills(t, linked)
		unexpected := filepath.Join(linked, "replacement", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(unexpected), 0o755); err != nil {
			return wasCreated, err
		}
		if err := os.WriteFile(unexpected, []byte(generatedMarker), 0o644); err != nil {
			return wasCreated, err
		}
		return wasCreated, nil
	}
	if err := writeDelegatesWithCreator(authentication.repository, creator); err != nil {
		t.Fatalf("write delegates through retained root: %v", err)
	}
	if err := checkDelegates(authentication.repository); err != nil {
		t.Fatalf("post-write check through retained root: %v", err)
	}
	if authentication.repository.identity() != identity {
		t.Fatalf("retained identity changed: got %s, want %s", authentication.repository.identity(), identity)
	}
	heldRoot := openTestRepositoryRoot(t, held)
	if heldRoot.identity() != identity {
		t.Fatalf("renamed root identity = %s, want retained %s", heldRoot.identity(), identity)
	}
	replacementRoot := openTestRepositoryRoot(t, linked)
	if replacementRoot.identity() == identity {
		t.Fatalf("replacement root unexpectedly reused retained identity %s", identity)
	}
	if len(created) != len(projections) {
		t.Fatalf("created = %v, want both fixed delegates", created)
	}
	for _, item := range projections {
		heldPath := filepath.Join(held, filepath.FromSlash(item.delegate))
		actual, err := os.ReadFile(heldPath)
		if err != nil {
			t.Fatalf("read retained delegate %s: %v", item.delegate, err)
		}
		if string(actual) != expected[item.delegate] {
			t.Fatalf("retained delegate %s bytes differ", item.delegate)
		}
		replacementPath := filepath.Join(linked, filepath.FromSlash(item.delegate))
		if _, err := os.Lstat(replacementPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("replacement identity received delegate %s: %v", item.delegate, err)
		}
	}
}

func TestWriteDelegatesPreservesPartialFileForManualRemoval(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	root := openTestRepositoryRoot(t, repo)
	creator := func(root *repositoryRoot, relative string, _ []byte) (bool, error) {
		created, err := root.createExclusive(relative, []byte("partial"))
		if err != nil {
			return created, err
		}
		return true, errors.New("injected write failure")
	}
	err := writeDelegatesWithCreator(root, creator)
	if err == nil || !strings.Contains(err.Error(), "never rolls back or deletes possible partial files") || !strings.Contains(err.Error(), "remove these paths manually") {
		t.Fatalf("writeDelegatesWithCreator error = %v", err)
	}
	path := filepath.Join(repo, filepath.FromSlash(projections[0].delegate))
	actual, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read preserved partial file: %v", readErr)
	}
	if string(actual) != "partial" {
		t.Fatalf("partial bytes = %q", actual)
	}
}

func TestCanonicalAndRenderedInputsAreBounded(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	path := filepath.Join(repo, filepath.FromSlash(projections[0].canonical))
	oversized := bytes.Repeat([]byte("x"), maxCanonicalSkillBytes+1)
	if err := os.WriteFile(path, oversized, 0o644); err != nil {
		t.Fatalf("write oversized canonical skill: %v", err)
	}
	root := openTestRepositoryRoot(t, repo)
	if _, err := expectedDelegates(root); err == nil || !strings.Contains(err.Error(), "file exceeds") {
		t.Fatalf("expectedDelegates error = %v", err)
	}
}

func TestRunUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
}

func openTestRepositoryRoot(t *testing.T, repo string) *repositoryRoot {
	t.Helper()
	root, err := openRepositoryRoot(repo)
	if err != nil {
		t.Fatalf("open repository root: %v", err)
	}
	closeRepositoryRootAtCleanup(t, root)
	return root
}

func closeRepositoryRootAtCleanup(t *testing.T, root *repositoryRoot) {
	t.Helper()
	t.Cleanup(func() {
		if err := root.close(); err != nil {
			t.Errorf("close repository root: %v", err)
		}
	})
}

func writeTestDelegates(t *testing.T, repo string) {
	t.Helper()
	root := openTestRepositoryRoot(t, repo)
	if err := writeDelegates(root); err != nil {
		t.Fatalf("writeDelegates: %v", err)
	}
}

func writeCanonicalSkills(t *testing.T, repo string) {
	t.Helper()
	for _, item := range projections {
		path := filepath.Join(repo, filepath.FromSlash(item.canonical))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		name := filepath.Base(filepath.Dir(path))
		if err := os.WriteFile(path, fmt.Appendf(nil, canonicalSkill, name, name), 0o644); err != nil {
			t.Fatalf("write canonical: %v", err)
		}
	}
}

func createLinkedWorktree(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	primary := filepath.Join(base, "primary")
	linked := filepath.Join(base, "linked")
	if err := os.Mkdir(primary, 0o755); err != nil {
		t.Fatalf("mkdir primary: %v", err)
	}
	runTestGit(t, "-C", primary, "init", "-q")
	runTestGit(t, "-C", primary, "-c", "user.name=Projection Test", "-c", "user.email=projection@example.invalid", "commit", "-q", "--allow-empty", "-m", "initial")
	runTestGit(t, "-C", primary, "worktree", "add", "-q", "-b", "projection-test", linked)
	return primary, linked
}

func runTestGit(t *testing.T, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
