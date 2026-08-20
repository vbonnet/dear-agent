package specguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestResolveRepositoryRootUsesGuardedAdmission(t *testing.T) {
	fixture := newGuardRepository(t)
	nested := filepath.Join(fixture.root, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveRepositoryRoot(context.Background(), nested)
	if err != nil {
		t.Fatalf("ResolveRepositoryRoot() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ResolveRepositoryRoot() = %q, want %q", got, want)
	}
}

func TestUnsupportedPlatformsFailAdmissionBeforeGitLaunch(t *testing.T) {
	t.Parallel()
	for _, goos := range []string{"windows", "freebsd"} {
		failure := descendantTerminationAdmission(goos)
		if failure == nil || failure.code != "unsupported-platform" || !strings.Contains(failure.message, "before Git") {
			t.Fatalf("%s admission failure = %#v", goos, failure)
		}
	}
	if descendantTerminationAdmission("darwin") != nil || descendantTerminationAdmission("linux") != nil {
		t.Fatal("supported process-group implementations were rejected")
	}
}

func TestLiveGitProcessGroupTerminationKillsDescendants(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("guard fails admission on platforms without verified process-group termination")
	}
	directory := t.TempDir()
	marker := filepath.Join(directory, "descendant-ran")
	ready := filepath.Join(directory, "ready")
	quotedMarker := strings.ReplaceAll(marker, "'", "'\"'\"'")
	quotedReady := strings.ReplaceAll(ready, "'", "'\"'\"'")
	executable := writeFakeGit(t, fmt.Sprintf("#!/bin/sh\n(sleep 0.2; : > '%s') >/dev/null 2>&1 &\n: > '%s'\nexec sleep 30\n", quotedMarker, quotedReady))
	command := exec.Command(executable)
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(ready)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) || time.Now().After(deadline) {
			killErr := killProcessGroup(command.Process)
			waitErr := command.Wait()
			t.Fatalf("Git process group did not report descendant readiness: stat=%v kill=%v wait=%v", err, killErr, waitErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := killProcessGroup(command.Process); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("terminated Git process unexpectedly reported success")
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil || !os.IsNotExist(err) {
		t.Fatalf("descendant survived live process-group cancellation: %v", err)
	}
}

func TestGitRunOutputOverflowTerminatesLiveDescendants(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("guard fails admission on platforms without verified process-group termination")
	}
	marker := filepath.Join(t.TempDir(), "overflow-descendant-ran")
	quotedMarker := strings.ReplaceAll(marker, "'", "'\"'\"'")
	executable := writeFakeGit(t, fmt.Sprintf("#!/bin/sh\n(sleep 0.5; : > '%s') &\nprintf '%%0256d' 0\nexec sleep 30\n", quotedMarker))
	limits := defaultLimits()
	limits.gitTime = 3 * time.Second
	git := gitClient{executable: executable, limits: limits}
	started := time.Now()
	_, failure := git.run(context.Background(), "", nil, 128, "version")
	if failure == nil || failure.code != "git-output-limit" {
		t.Fatalf("overflow failure = %#v", failure)
	}
	if elapsed := time.Since(started); elapsed > limits.gitTime+time.Second {
		t.Fatalf("overflow process group exceeded the %s Git bound plus cleanup grace: %s", limits.gitTime, elapsed)
	}
	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil || !os.IsNotExist(err) {
		t.Fatalf("overflow descendant survived process-group termination: %v", err)
	}
}

func TestGitRunSuccessfulChildCleansSilentDescendant(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("guard fails admission on platforms without verified process-group termination")
	}
	marker := filepath.Join(t.TempDir(), "silent-descendant-ran")
	quotedMarker := strings.ReplaceAll(marker, "'", "'\"'\"'")
	executable := writeFakeGit(t, fmt.Sprintf("#!/bin/sh\n(sleep 0.5; : > '%s') >/dev/null 2>&1 &\nprintf 'ok\\n'\n", quotedMarker))
	git := gitClient{executable: executable, limits: defaultLimits()}
	output, failure := git.run(context.Background(), "", nil, 4096, "version")
	if failure != nil {
		t.Fatalf("successful Git child failed: %#v", failure)
	}
	if string(output) != "ok\n" {
		t.Fatalf("successful Git output = %q, want ok", output)
	}
	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil || !os.IsNotExist(err) {
		t.Fatalf("silent Git descendant survived successful command cleanup: %v", err)
	}
}

func TestGitProcessGroupLifecycleRejectsLateCancellation(t *testing.T) {
	lifecycle := newGitProcessGroupLifecycle(&exec.Cmd{})
	if err := lifecycle.complete(false, true); err != nil {
		t.Fatalf("complete sealed Git lifecycle: %v", err)
	}
	if observed, err := lifecycle.cancelObserved(); observed || !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("sealed lifecycle cancellation = (%v, %v), want (false, os.ErrProcessDone)", observed, err)
	}
	var cancellations sync.WaitGroup
	for range 64 {
		cancellations.Go(func() {
			for range 100 {
				if err := lifecycle.cancel(); !errors.Is(err, os.ErrProcessDone) {
					t.Errorf("late Git lifecycle cancel error = %v, want os.ErrProcessDone", err)
				}
			}
		})
	}
	cancellations.Wait()
}

func TestGitProcessGroupLifecycleRecordsSuccessfulTerminationSignal(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("guard fails admission on platforms without verified process-group termination")
	}
	command := exec.Command("/bin/sh", "-c", "exec sleep 30")
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start Git child: %v", err)
	}
	reaped := false
	defer func() {
		if !reaped {
			_ = killProcessGroup(command.Process)
			_ = command.Wait()
		}
	}()

	lifecycle := newGitProcessGroupLifecycle(command)
	observed, signalErr := lifecycle.cancelObserved()
	if !observed || signalErr != nil || !lifecycle.terminationSignaled {
		t.Fatalf("early termination signal = (%v, %v, recorded=%v), want successful recorded signal", observed, signalErr, lifecycle.terminationSignaled)
	}
	observedExitErr := waitForGitCommandExitWithoutReaping(command.Process.Pid)
	if cleanupErr := lifecycle.complete(observedExitErr == nil, errors.Is(observedExitErr, syscall.ECHILD)); cleanupErr != nil {
		t.Fatalf("complete signaled Git lifecycle: %v", cleanupErr)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("terminated Git child unexpectedly reported success")
	}
	reaped = true
}

func TestGitRunPreCanceledContextDoesNotStart(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "git-started")
	quotedMarker := strings.ReplaceAll(marker, "'", "'\"'\"'")
	executable := writeFakeGit(t, fmt.Sprintf("#!/bin/sh\n: > '%s'\n", quotedMarker))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, failure := (gitClient{executable: executable, limits: defaultLimits()}).run(ctx, "", nil, 4096, "version")
	if failure == nil || failure.code != "git-time-limit" {
		t.Fatalf("pre-canceled Git failure = %#v", failure)
	}
	if _, err := os.Stat(marker); err == nil || !os.IsNotExist(err) {
		t.Fatalf("pre-canceled Git command launched: %v", err)
	}
}

func TestGitCancellationEPERMNormalizesOnlyAfterSafeFinalCleanup(t *testing.T) {
	if err := normalizeGitCancellationSignalError(syscall.EPERM, nil, nil); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("safe final cleanup normalized error = %v, want os.ErrProcessDone", err)
	}
	observedErr := errors.New("waitid failed")
	if err := normalizeGitCancellationSignalError(syscall.EPERM, observedErr, nil); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("unobserved exit normalized error = %v, want EPERM", err)
	}
	cleanupErr := errors.New("cleanup failed")
	if err := normalizeGitCancellationSignalError(syscall.EPERM, nil, cleanupErr); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("failed cleanup normalized error = %v, want EPERM", err)
	}
}

func TestGitContextCancellationClassificationTracksLifecycleOrdering(t *testing.T) {
	tests := []struct {
		name            string
		execution       gitCommandExecution
		wantFailureCode string
	}{
		{
			name: "context wins before process-done signal",
			execution: gitCommandExecution{
				cancellationSignalErr:       os.ErrProcessDone,
				contextCancellationObserved: true,
			},
			wantFailureCode: "git-time-limit",
		},
		{
			name: "context wins before normalized Darwin EPERM",
			execution: gitCommandExecution{
				cancellationSignalErr:       syscall.EPERM,
				contextCancellationObserved: true,
			},
			wantFailureCode: "git-time-limit",
		},
		{
			name: "cleanup seals before late context wake",
			execution: gitCommandExecution{
				cancellationSignalErr: os.ErrProcessDone,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, failure := classifyGitCommandExecution([]byte("ok"), nil, false, test.execution, time.Second)
			if test.wantFailureCode == "" {
				if failure != nil || string(output) != "ok" {
					t.Fatalf("classification = (%q, %#v), want success", output, failure)
				}
				return
			}
			if failure == nil || failure.code != test.wantFailureCode {
				t.Fatalf("classification failure = %#v, want %q", failure, test.wantFailureCode)
			}
		})
	}
}

func TestStagedSnapshotBlocksDirtyGovernedWorktreeWithoutParsingMutableBody(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
	fixture.write("pkg/example/SPEC.md", "mutable worktree bytes are deliberately not a valid SPEC\n")

	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	assertDecisionAndCode(t, result, DecisionBlock, "dirty-governed-worktree")
	if hasCode(result, "invalid-ears") || hasCode(result, "missing-ears-requirement") {
		t.Fatalf("mutable worktree body was semantically parsed: %#v", result.Findings)
	}
	if !strings.Contains(result.Findings[0].Message, "stage the intended contract state or resolve") {
		t.Fatalf("dirty finding lacks remediation: %#v", result.Findings[0])
	}
}

func TestStagedSnapshotOverridesRepositoryMetadataTrustConfiguration(t *testing.T) {
	fixture := newGuardRepository(t)
	const specPath = "pkg/example/SPEC.md"
	const featurePath = "agm/test/bdd/features/example.feature"
	original := validSpec(featurePath)
	mutated := strings.Replace(original, "provider-neutral", "attacker-neutral", 1)
	if mutated == original || len(mutated) != len(original) {
		t.Fatal("metadata-spoof fixture must change content without changing size")
	}
	fixture.write(specPath, original)
	fixture.write(featurePath, validFeature(specPath))

	// Keep the worktree entry older than the index so Git's racy-clean
	// mitigation cannot independently force a content comparison.
	stableTime := time.Unix(946684800, 0)
	fullPath := filepath.Join(fixture.root, filepath.FromSlash(specPath))
	if err := os.Chtimes(fullPath, stableTime, stableTime); err != nil {
		t.Fatal(err)
	}
	fixture.git("add", "--", specPath, featurePath)
	fixture.git("commit", "-m", "add governed baseline")

	fixture.git("config", "core.trustctime", "false")
	fixture.git("config", "core.checkStat", "minimal")
	// Cross a whole-second boundary so the changed ctime is distinct even on
	// filesystems whose stat cache has only second-level ctime precision.
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)) + 50*time.Millisecond)
	fixture.write(specPath, mutated)
	if err := os.Chtimes(fullPath, stableTime, stableTime); err != nil {
		t.Fatal(err)
	}

	if raw := fixture.git("diff-files", "--name-only", "--", specPath); raw != "" {
		t.Fatalf("repository-configured raw Git detected metadata-spoofed change: %q", raw)
	}

	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	assertDecisionAndCode(t, result, DecisionBlock, "dirty-governed-worktree")
}

func TestStagedSnapshotUsesImmutableStagedContentWhenWorktreeIsClean(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")

	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if result.Decision != DecisionReminder {
		t.Fatalf("decision = %q, findings = %#v", result.Decision, result.Findings)
	}
	if result.Findings == nil {
		t.Fatal("successful result must encode findings as an empty array, not null")
	}
	if result.Source != "Git index object IDs compared with pinned HEAD after bounded dirty-worktree path/status and index-flag admission" {
		t.Fatalf("source = %q", result.Source)
	}
	if len(result.SnapshotID) != 64 {
		t.Fatalf("snapshot identity = %q, want SHA-256 hex", result.SnapshotID)
	}
	repeated := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if repeated.SnapshotID != result.SnapshotID {
		t.Fatalf("stable index identities differ: %q != %q", repeated.SnapshotID, result.SnapshotID)
	}
	fixture.write("README.md", "new staged snapshot\n")
	fixture.git("add", "--", "README.md")
	changed := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if changed.SnapshotID == result.SnapshotID {
		t.Fatalf("snapshot identity did not change after staged index mutation: %q", changed.SnapshotID)
	}
}

func TestStagedSnapshotBlocksGovernedDirtyAddModifyAndDelete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(guardRepository)
	}{
		{
			name: "nonignored untracked addition",
			mutate: func(fixture guardRepository) {
				fixture.write("untracked/SPEC.md", "untracked mutable contract\n")
			},
		},
		{
			name: "tracked modification",
			mutate: func(fixture guardRepository) {
				fixture.write("pkg/example/SPEC.md", "modified mutable contract\n")
			},
		},
		{
			name: "tracked deletion",
			mutate: func(fixture guardRepository) {
				if err := os.Remove(filepath.Join(fixture.root, "agm", "test", "bdd", "features", "example.feature")); err != nil {
					fixture.t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGuardRepository(t)
			fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
			fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
			fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
			fixture.git("commit", "-m", "governed baseline")
			test.mutate(fixture)

			result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
			assertDecisionAndCode(t, result, DecisionBlock, "dirty-governed-worktree")
		})
	}
}

func TestStagedSnapshotRejectsGovernedIndexFlags(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		for _, governedPath := range []string{
			"pkg/example/SPEC.md",
			"internal/adapter/SPEC.owner",
			"agm/test/bdd/features/example.feature",
		} {
			for _, dirty := range []bool{false, true} {
				name := strings.TrimPrefix(flag, "--") + "/" + strings.ReplaceAll(governedPath, "/", "-")
				if dirty {
					name += "/dirty"
				} else {
					name += "/clean"
				}
				t.Run(name, func(t *testing.T) {
					fixture := newGuardRepository(t)
					featurePath := "agm/test/bdd/features/example.feature"
					fixture.write("pkg/example/SPEC.md", validSpec(featurePath))
					fixture.write(featurePath, validFeature("pkg/example/SPEC.md"))
					fixture.write("internal/adapter/adapter.go", "package adapter\n")
					fixture.write("internal/adapter/SPEC.owner", "pkg/example/SPEC.md\n")
					fixture.git("add", "--", "pkg/example/SPEC.md", featurePath, "internal/adapter/adapter.go", "internal/adapter/SPEC.owner")
					fixture.git("commit", "-m", "governed baseline")
					fixture.git("update-index", flag, "--", governedPath)
					if dirty {
						fixture.write(governedPath, "mutable contract hidden by an index flag\n")
					}

					result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
					assertDecisionAndCode(t, result, DecisionBlock, "index-flagged-governed-path")
				})
			}
		}
	}
}

func TestStagedSnapshotAllowsStableIndexFlagsOnUngovernedPaths(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			fixture := newGuardRepository(t)
			fixture.git("update-index", flag, "--", "README.md")
			result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
			if result.Decision != DecisionAllow || len(result.Findings) != 0 {
				t.Fatalf("result = %#v, want clean ungoverned index flag admission", result)
			}
		})
	}
}

func TestStagedSnapshotDetectsDirtyWorktreeRace(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	mutated := false
	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterDirtyWorktreeRead: func() {
			mutated = true
			fixture.write("raced/SPEC.md", "appeared after dirty-worktree admission\n")
		},
	})
	if !mutated {
		t.Fatal("dirty-worktree mutation hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "dirty-worktree-race")
}

func TestStagedSnapshotDetectsIndexRace(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")

	mutated := false
	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterIndexRead: func() {
			mutated = true
			fixture.write("README.md", "changed during evaluation\n")
			fixture.git("add", "--", "README.md")
		},
	})
	if !mutated {
		t.Fatal("index mutation hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "index-race")
}

func TestStagedSnapshotRechecksIndexAfterFinalDirtyWorktreeAdmission(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")

	mutated := false
	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterFinalDirtyRead: func() {
			mutated = true
			fixture.write("pkg/example/SPEC.md", strings.Replace(validSpec("agm/test/bdd/features/example.feature"), "provider-neutral outcome", "rechecked provider-neutral outcome", 1))
			fixture.git("add", "--", "pkg/example/SPEC.md")
		},
	})
	if !mutated {
		t.Fatal("final dirty-worktree mutation hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "index-race")
}

func TestStagedSnapshotRechecksIndexFlagsAfterFinalDirtyWorktreeAdmission(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	featurePath := "agm/test/bdd/features/example.feature"
	fixture.write("pkg/example/SPEC.md", validSpec(featurePath))
	fixture.write(featurePath, validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", featurePath)

	mutated := false
	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterFinalDirtyRead: func() {
			mutated = true
			fixture.git("update-index", "--skip-worktree", "--", "pkg/example/SPEC.md")
		},
	})
	if !mutated {
		t.Fatal("final index-flag mutation hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "index-race")
}

func TestCommittedSnapshotValidatesPinnedBaseToHead(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("pkg/example/SPEC.md", invalidSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Base contract\n  Scenario: Base has no assertion\n    Given a condition\n")
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
	fixture.git("commit", "-m", "add invalid base contract")
	base := fixture.git("rev-parse", "HEAD")
	fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
	fixture.git("commit", "-m", "repair governed contract")

	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeCommitted, Base: base})
	if result.Decision != DecisionReminder {
		t.Fatalf("decision = %q, findings = %#v", result.Decision, result.Findings)
	}
	if result.Base != base || result.Head == "" || result.Source != "Git commit-tree object IDs from pinned base..HEAD" {
		t.Fatalf("unexpected snapshot identity: %#v", result)
	}
	if len(result.SnapshotID) != 64 {
		t.Fatalf("committed snapshot identity = %q, want SHA-256 hex", result.SnapshotID)
	}
	repeated := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeCommitted, Base: base})
	if repeated.SnapshotID != result.SnapshotID {
		t.Fatalf("stable commit-pair identities differ: %q != %q", repeated.SnapshotID, result.SnapshotID)
	}
}

func TestCommittedSnapshotRejectsNonAncestorBase(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.git("checkout", "-b", "side")
	fixture.write("side.txt", "side\n")
	fixture.git("add", "--", "side.txt")
	fixture.git("commit", "-m", "side")
	side := fixture.git("rev-parse", "HEAD")
	fixture.git("checkout", "main")
	fixture.write("main.txt", "main\n")
	fixture.git("add", "--", "main.txt")
	fixture.git("commit", "-m", "main")

	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeCommitted, Base: side})
	assertDecisionAndCode(t, result, DecisionBlock, "base-not-ancestor")
}

func TestRepositoryIdentityDetectsReplacementDuringAdmission(t *testing.T) {
	parent := t.TempDir()
	fixture := newGuardRepositoryAt(t, filepath.Join(parent, "repo"))
	replacement := newGuardRepositoryAt(t, filepath.Join(parent, "replacement"))
	displaced := filepath.Join(parent, "displaced")
	mutated := false

	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterAdmissionGitCommand: func() {
			if mutated {
				return
			}
			mutated = true
			if err := os.Rename(fixture.root, displaced); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement.root, fixture.root); err != nil {
				t.Fatal(err)
			}
		},
	})
	if !mutated {
		t.Fatal("admission replacement hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "repository-identity-changed")
}

func TestRepositoryIdentityDetectsSamePathReplacementAfterAdmission(t *testing.T) {
	parent := t.TempDir()
	fixture := newGuardRepositoryAt(t, filepath.Join(parent, "repo"))
	replacement := newGuardRepositoryAt(t, filepath.Join(parent, "replacement"))
	displaced := filepath.Join(parent, "displaced")
	mutated := false

	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterRepositoryAdmission: func() {
			mutated = true
			if err := os.Rename(fixture.root, displaced); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement.root, fixture.root); err != nil {
				t.Fatal(err)
			}
		},
	})
	if !mutated {
		t.Fatal("repository replacement hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "repository-identity-changed")
}

func TestCommittedRepositoryIdentityFailureIsNotRewritten(t *testing.T) {
	parent := t.TempDir()
	fixture := newGuardRepositoryAt(t, filepath.Join(parent, "repo"))
	replacement := newGuardRepositoryAt(t, filepath.Join(parent, "replacement"))
	displaced := filepath.Join(parent, "displaced")

	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeCommitted, Base: "HEAD"}, defaultLimits(), guardDependencies{
		afterRepositoryAdmission: func() {
			if err := os.Rename(fixture.root, displaced); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement.root, fixture.root); err != nil {
				t.Fatal(err)
			}
		},
	})
	assertDecisionAndCode(t, result, DecisionBlock, "repository-identity-changed")
}

func TestRepositoryIdentityDetectsGitDirectoryReplacementAfterAdmission(t *testing.T) {
	fixture := newGuardRepository(t)
	gitDirectory := filepath.Join(fixture.root, ".git")
	displaced := filepath.Join(fixture.root, ".git-displaced")
	mutated := false

	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterRepositoryAdmission: func() {
			mutated = true
			if err := os.Rename(gitDirectory, displaced); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(gitDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
		},
	})
	if !mutated {
		t.Fatal("Git directory replacement hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "repository-identity-changed")
}

func TestRepositoryIdentityDetectsLinkedWorktreeSelectorRetarget(t *testing.T) {
	parent := t.TempDir()
	primary := newGuardRepositoryAt(t, filepath.Join(parent, "primary"))
	linkedRoot := filepath.Join(parent, "linked")
	primary.git("worktree", "add", "-b", "linked", linkedRoot)
	selectorPath := filepath.Join(linkedRoot, ".git")
	before, err := os.Lstat(selectorPath)
	if err != nil || !before.Mode().IsRegular() {
		t.Fatalf("linked-worktree selector = %#v, %v", before, err)
	}
	mutated := false

	result := evaluate(context.Background(), Request{Repository: linkedRoot, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterRepositoryAdmission: func() {
			mutated = true
			if err := os.WriteFile(selectorPath, []byte("gitdir: /definitely-not-the-admitted-git-directory\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			after, statErr := os.Lstat(selectorPath)
			if statErr != nil || !os.SameFile(before, after) {
				t.Fatalf("selector inode changed instead of being retargeted in place: %v", statErr)
			}
		},
	})
	if !mutated {
		t.Fatal("linked-worktree retarget hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "repository-identity-changed")
}

func TestRepositoryIdentityDetectsLinkedWorktreeCommonDirectoryRetarget(t *testing.T) {
	parent := t.TempDir()
	primary := newGuardRepositoryAt(t, filepath.Join(parent, "primary"))
	linkedRoot := filepath.Join(parent, "linked")
	primary.git("worktree", "add", "-b", "linked", linkedRoot)
	gitFile, err := os.ReadFile(filepath.Join(linkedRoot, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	gitDirectory := strings.TrimSpace(strings.TrimPrefix(string(gitFile), "gitdir:"))
	commonSelectorPath := filepath.Join(gitDirectory, "commondir")
	before, err := os.Lstat(commonSelectorPath)
	if err != nil || !before.Mode().IsRegular() {
		t.Fatalf("linked-worktree common-directory selector = %#v, %v", before, err)
	}

	result := evaluate(context.Background(), Request{Repository: linkedRoot, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterRepositoryAdmission: func() {
			if err := os.WriteFile(commonSelectorPath, []byte("/definitely-not-the-admitted-common-directory\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			after, statErr := os.Lstat(commonSelectorPath)
			if statErr != nil || !os.SameFile(before, after) {
				t.Fatalf("common-directory selector inode changed instead of being retargeted in place: %v", statErr)
			}
		},
	})
	assertDecisionAndCode(t, result, DecisionBlock, "repository-identity-changed")
}

func TestRepositoryIdentityDetectsAncestorReplacementDuringGitCommand(t *testing.T) {
	parent := t.TempDir()
	ancestor := filepath.Join(parent, "ancestor")
	fixture := newGuardRepositoryAt(t, filepath.Join(ancestor, "repo"))
	displaced := filepath.Join(parent, "ancestor-displaced")
	mutated := false

	result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, defaultLimits(), guardDependencies{
		afterGitCommand: func() {
			if mutated {
				return
			}
			mutated = true
			if err := os.Rename(ancestor, displaced); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(ancestor, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(displaced, "repo"), fixture.root); err != nil {
				t.Fatal(err)
			}
		},
	})
	if !mutated {
		t.Fatal("ancestor replacement hook did not run")
	}
	assertDecisionAndCode(t, result, DecisionBlock, "repository-identity-changed")
}

func TestBoundsFailClosed(t *testing.T) {
	t.Parallel()
	t.Run("file size", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		limits := defaultLimits()
		limits.maxFileBytes = 32
		result := evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged}, limits, guardDependencies{})
		assertDecisionAndCode(t, result, DecisionBlock, "file-size-limit")
	})

	t.Run("wall time", func(t *testing.T) {
		if descendantTerminationAdmission(runtime.GOOS) != nil {
			t.Skip("guard rejects this platform before executing the POSIX test helper")
		}
		executable := writeFakeGit(t, "#!/bin/sh\nexec sleep 5\n")
		limits := defaultLimits()
		limits.gitTime = 25 * time.Millisecond
		limits.wallTime = 100 * time.Millisecond
		started := time.Now()
		result := evaluate(context.Background(), Request{Repository: t.TempDir(), Mode: ModeStaged}, limits, guardDependencies{gitExecutable: executable})
		assertDecisionAndCode(t, result, DecisionBlock, "git-time-limit")
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("time bound took %s", elapsed)
		}
	})

	t.Run("Git output", func(t *testing.T) {
		if descendantTerminationAdmission(runtime.GOOS) != nil {
			t.Skip("guard rejects this platform before executing the POSIX test helper")
		}
		// The finite helper may exit before os/exec drains its pipe. The runner
		// must still classify the bound while waitid pins the leader until cleanup.
		executable := writeFakeGit(t, "#!/bin/sh\nprintf '%0256d' 0\n")
		limits := defaultLimits()
		limits.maxGitOutput = 128
		limits.gitTime = 5 * time.Second
		result := evaluate(context.Background(), Request{Repository: t.TempDir(), Mode: ModeStaged}, limits, guardDependencies{gitExecutable: executable})
		assertDecisionAndCode(t, result, DecisionBlock, "git-output-limit")
		if !strings.Contains(result.Findings[0].Message, "output exceeded") {
			t.Fatalf("message = %q", result.Findings[0].Message)
		}
	})
}

func TestPathEscapeAndNonregularModesAreRejected(t *testing.T) {
	t.Parallel()
	oid := strings.Repeat("a", 40)
	if _, failure := parseIndexEntries([]byte("100644 "+oid+" 0\t../SPEC.md\x00"), defaultLimits()); failure == nil || failure.code != "path-escape" {
		t.Fatalf("path escape failure = %#v", failure)
	}
	entries, failure := parseIndexEntries([]byte("120000 "+oid+" 0\tpkg/SPEC.md\x00"), defaultLimits())
	if failure != nil {
		t.Fatal(failure.message)
	}
	if _, failure = selectGovernedEntries(entries, defaultLimits()); failure == nil || failure.code != "nonregular-git-mode" {
		t.Fatalf("mode failure = %#v", failure)
	}
}

func TestDirtyPathListParsingIsNULSafeAndBounded(t *testing.T) {
	t.Parallel()
	limits := defaultLimits()
	paths, failure := parsePathList([]byte("dir with spaces/SPEC.md\x00features/caf\xc3\xa9.feature\x00"), limits)
	if failure != nil || len(paths) != 2 || paths[0] != "dir with spaces/SPEC.md" || paths[1] != "features/café.feature" {
		t.Fatalf("paths = %#v, failure = %#v", paths, failure)
	}

	tests := []struct {
		name string
		data []byte
		code string
	}{
		{name: "unterminated", data: []byte("pkg/SPEC.md"), code: "malformed-git-output"},
		{name: "control byte", data: []byte("pkg/line\nbreak/SPEC.md\x00"), code: "invalid-git-path"},
		{name: "invalid UTF-8", data: []byte{'p', 'k', 'g', '/', 0xff, '/', 'S', 'P', 'E', 'C', '.', 'm', 'd', 0}, code: "invalid-git-path"},
		{name: "duplicate", data: []byte("pkg/SPEC.md\x00pkg/SPEC.md\x00"), code: "malformed-git-output"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, failure := parsePathList(test.data, limits)
			if failure == nil || failure.code != test.code {
				t.Fatalf("failure = %#v, want %q", failure, test.code)
			}
		})
	}

	limited := limits
	limited.maxEntries = 1
	if _, failure := parsePathList([]byte("one/SPEC.md\x00two/SPEC.md\x00"), limited); failure == nil || failure.code != "git-entry-limit" {
		t.Fatalf("entry limit failure = %#v", failure)
	}
}

func TestIndexFlagParsingIsNULSafeAndGoverned(t *testing.T) {
	t.Parallel()
	output := []byte("H README.md\x00h pkg/example/SPEC.md\x00S agm/test/bdd/features/example.feature\x00s docs/other/SPEC.md\x00")
	flagged, failure := parseIndexFlaggedPaths(output, defaultLimits())
	if failure != nil {
		t.Fatal(failure.message)
	}
	want := []string{"agm/test/bdd/features/example.feature", "docs/other/SPEC.md", "pkg/example/SPEC.md"}
	if !slices.Equal(flagged, want) {
		t.Fatalf("flagged paths = %#v, want %#v", flagged, want)
	}
	for _, malformed := range [][]byte{
		[]byte("H missing-terminator"),
		[]byte("! unsupported/SPEC.md\x00"),
		[]byte("H duplicate/SPEC.md\x00H duplicate/SPEC.md\x00"),
	} {
		if _, failure := parseIndexFlaggedPaths(malformed, defaultLimits()); failure == nil {
			t.Fatalf("malformed index-flag output %q was accepted", malformed)
		}
	}
}
