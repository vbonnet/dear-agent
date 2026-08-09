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

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestEvaluateRejectsMalformedRequests(t *testing.T) {
	t.Parallel()
	tests := []Request{
		{},
		{Repository: ".", Mode: "other"},
		{Repository: ".", Mode: ModeStaged, Base: "main"},
		{Repository: ".", Mode: ModeCommitted},
		{Repository: ".", Mode: ModeCommitted, Base: "--upload-pack=bad"},
		{Repository: ".", Mode: ModeCommitted, Base: "main..other"},
	}
	for _, request := range tests {
		t.Run(fmt.Sprintf("%s-%s", request.Mode, request.Base), func(t *testing.T) {
			result := Evaluate(context.Background(), request)
			assertDecisionAndCode(t, result, DecisionBlock, "invalid-input")
		})
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
	executable := writeFakeGit(t, fmt.Sprintf("#!/bin/sh\n(sleep 0.5; : > '%s') &\nwhile :; do printf '0123456789abcdef'; done\n", quotedMarker))
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

func TestNoGovernedChangesAreAllowed(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("README.md", "ordinary documentation change\n")
	fixture.git("add", "--", "README.md")

	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if result.Decision != DecisionAllow || len(result.Changed) != 0 || len(result.Findings) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestChangedSpecRequiresStrictEARSNeutralOwnerAndReciprocalBDD(t *testing.T) {
	t.Parallel()
	t.Run("harness registration owners", func(t *testing.T) {
		for _, specPath := range []string{
			".codex/SPEC.md",
			"claude-code/SPEC.md",
			"codex-cli/SPEC.md",
			"opencode-cli/SPEC.md",
			"pi-cli/SPEC.md",
			"agy-cli/SPEC.md",
			"antigravity/SPEC.md",
			"wayfinder/.claude-plugin/new/SPEC.md",
			"product/.gemini/SPEC.md",
			"agm/harnesses/pi/SPEC.md",
			"agm/harness/future-harness/SPEC.md",
			"agm/harnesses/future-cli/SPEC.md",
			"agm/internal/.codex/SPEC.md",
			"agm/cmd/.claude/SPEC.md",
			"agm/internal/harnesses/future-cli/SPEC.md",
			"agm/internal/plugins/example/SPEC.md",
			"agm/cmd/future-plugin/SPEC.md",
			"agm/agm-plugin/commands/SPEC.md",
		} {
			t.Run(strings.ReplaceAll(specPath, "/", "_"), func(t *testing.T) {
				fixture := newGuardRepository(t)
				fixture.write(specPath, validSpec("agm/test/bdd/features/harness.feature"))
				fixture.write("agm/test/bdd/features/harness.feature", validFeature(specPath))
				fixture.git("add", "--", specPath, "agm/test/bdd/features/harness.feature")
				result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
				assertDecisionAndCode(t, result, DecisionBlock, "non-neutral-spec-owner")
			})
		}
	})

	t.Run("logical internal owners", func(t *testing.T) {
		for _, specPath := range []string{
			".dear-agent/SPEC.md",
			"agm/internal/agent/SPEC.md",
			"internal/codexarchive/SPEC.md",
		} {
			t.Run(strings.ReplaceAll(specPath, "/", "_"), func(t *testing.T) {
				fixture := newGuardRepository(t)
				fixture.write(specPath, validSpec("agm/test/bdd/features/logical.feature"))
				fixture.write("agm/test/bdd/features/logical.feature", validFeature(specPath))
				fixture.git("add", "--", specPath, "agm/test/bdd/features/logical.feature")
				result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
				if result.Decision != DecisionReminder || len(result.Findings) != 0 {
					t.Fatalf("result = %#v, want reminder without deterministic findings", result)
				}
			})
		}
	})

	t.Run("missing reciprocal feature link", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/other/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "nonreciprocal-bdd-link")
	})

	t.Run("two primary owners", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("pkg/other/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\n# SPEC: pkg/other/SPEC.md\nFeature: Example\n  Scenario: Works\n    Given an observable condition\n")
		fixture.git("add", "--", "pkg/example/SPEC.md", "pkg/other/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "bdd-primary-owner-count")
	})

	t.Run("Given and When only are not runnable BDD", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: No assertion\n    Given an observable condition\n    When the contract is evaluated\n")
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "scenario-without-assertion")
	})

	t.Run("Scenario Outline requires nonempty Examples", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Missing examples\n    Given an observable <condition>\n    Then the outcome is preserved\n")
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "outline-without-examples")
	})
}

func TestHarnessRegistrationOwnerBoundary(t *testing.T) {
	t.Parallel()
	for _, specPath := range []string{
		".agents/SPEC.md",
		".claude/SPEC.md",
		".codex/SPEC.md",
		".gemini/SPEC.md",
		".opencode/SPEC.md",
		".pi/SPEC.md",
		"claude-code/SPEC.md",
		"claudecode/SPEC.md",
		"codex-cli/SPEC.md",
		"codex_cli/SPEC.md",
		"opencode-cli/SPEC.md",
		"opencodecli/SPEC.md",
		"pi-cli/SPEC.md",
		"picli/SPEC.md",
		"agy-cli/SPEC.md",
		"antigravity/SPEC.md",
		"product/.claude-code/SPEC.md",
		"wayfinder/.claude-plugin/new/SPEC.md",
		"product/.codex-plugin/SPEC.md",
		"Internal/.codex/SPEC.md",
		"agm/internal/.codex/SPEC.md",
		"agm/cmd/.claude/SPEC.md",
		"agm/internal/plugins/example/SPEC.md",
		"agm/cmd/future-plugin/SPEC.md",
		"agm/harness/codex/SPEC.md",
		"agm/harness/codex-cli/SPEC.md",
		"agm/harness/future-harness/SPEC.md",
		"agm/harnesses/pi/SPEC.md",
		"agm/harnesses/future-cli/SPEC.md",
		"agm/internal/harnesses/future-cli/SPEC.md",
		"agm/youtube-plugin/SPEC.md",
	} {
		if !isHarnessRegistrationOwner(specPath) {
			t.Errorf("registration owner %q was accepted", specPath)
		}
	}
	for _, specPath := range []string{
		".dear-agent/SPEC.md",
		"agm/internal/agent/SPEC.md",
		"internal/codexarchive/SPEC.md",
		"agm/internal/codex-cli/SPEC.md",
		"agm/cmd/claude/SPEC.md",
		"agm/cmd/opencode-cli/SPEC.md",
		"future-cli/SPEC.md",
		"pkg/plugin/SPEC.md",
	} {
		if isHarnessRegistrationOwner(specPath) {
			t.Errorf("logical owner %q was rejected", specPath)
		}
	}
}

func TestBareTopLevelHarnessAuthorityIsPinnedClosed(t *testing.T) {
	t.Parallel()
	if !isHarnessRegistrationOwner("codex-cli/SPEC.md") {
		t.Fatal("pinned canonical harness owner was accepted")
	}
	if isHarnessRegistrationOwner("future-cli/SPEC.md") {
		t.Fatal("unknown bare top-level owner was classified without a pinned-authority update")
	}
	if !isHarnessRegistrationOwner("harnesses/future-cli/SPEC.md") {
		t.Fatal("structural harness collection incorrectly depended on the pinned authority")
	}
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

func TestGovernedDeletionValidatesSurvivingGraphAndAllowsReviewedRetirement(t *testing.T) {
	t.Parallel()
	newFixture := func(t *testing.T) guardRepository {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		fixture.git("commit", "-m", "add contract")
		return fixture
	}

	t.Run("deleted SPEC leaves a dangling feature edge", func(t *testing.T) {
		fixture := newFixture(t)
		if err := os.Remove(filepath.Join(fixture.root, "pkg/example/SPEC.md")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", "pkg/example/SPEC.md")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-related-spec")
	})

	t.Run("deleted feature leaves a dangling SPEC edge", func(t *testing.T) {
		fixture := newFixture(t)
		if err := os.Remove(filepath.Join(fixture.root, "agm/test/bdd/features/example.feature")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-bdd-feature")
	})

	t.Run("deleted SPEC leaves a dangling owner edge", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		for _, relative := range []string{"pkg/example/SPEC.md", "agm/test/bdd/features/example.feature"} {
			if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(relative))); err != nil {
				t.Fatal(err)
			}
		}
		fixture.git("add", "-u", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-spec-owner-target")
	})

	t.Run("deleted owner leaves a live implementation unowned", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.Remove(filepath.Join(fixture.root, ".opencode/plugins/SPEC.owner")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", ".opencode/plugins/SPEC.owner")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-implementation-spec-owner")
	})

	t.Run("owner and implementation retirement reaches semantic review", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		for _, relative := range []string{".opencode/plugins/SPEC.owner", ".opencode/plugins/adapter.mjs"} {
			if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(relative))); err != nil {
				t.Fatal(err)
			}
		}
		fixture.git("add", "-u", "--", ".opencode/plugins/SPEC.owner", ".opencode/plugins/adapter.mjs")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || strings.Join(result.Changed, "\n") != ".opencode/plugins/SPEC.owner" {
			t.Fatalf("result = %#v, want reviewed implementation retirement reminder", result)
		}
	})

	t.Run("owner deletion cannot hide an unowned implementation relocation", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.MkdirAll(filepath.Join(fixture.root, "internal", "relocated"), 0o700); err != nil {
			t.Fatal(err)
		}
		fixture.git("mv", ".opencode/plugins/adapter.mjs", "internal/relocated/adapter.mjs")
		if err := os.Remove(filepath.Join(fixture.root, ".opencode/plugins/SPEC.owner")); err != nil {
			t.Fatal(err)
		}
		fixture.git("add", "-u", "--", ".opencode/plugins/SPEC.owner")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "missing-implementation-spec-owner")
	})

	t.Run("implementation relocation with its owner reaches semantic review", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
		fixture.write(".opencode/plugins/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", ".opencode/plugins/adapter.mjs", ".opencode/plugins/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.MkdirAll(filepath.Join(fixture.root, "internal", "relocated"), 0o700); err != nil {
			t.Fatal(err)
		}
		fixture.git("mv", ".opencode/plugins/adapter.mjs", "internal/relocated/adapter.mjs")
		fixture.git("mv", ".opencode/plugins/SPEC.owner", "internal/relocated/SPEC.owner")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || len(result.Changed) != 2 {
			t.Fatalf("result = %#v, want valid owned implementation relocation reminder", result)
		}
	})

	t.Run("owner is replaced by permitted local SPEC ownership", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.write("internal/adapter/adapter.go", "package adapter\n")
		fixture.write("internal/adapter/SPEC.owner", "pkg/example/SPEC.md\n")
		fixture.git("add", "--", "internal/adapter/adapter.go", "internal/adapter/SPEC.owner")
		fixture.git("commit", "-m", "add implementation owner edge")
		if err := os.Remove(filepath.Join(fixture.root, "internal/adapter/SPEC.owner")); err != nil {
			t.Fatal(err)
		}
		fixture.write("internal/adapter/SPEC.md", validSpec("agm/test/bdd/features/adapter.feature"))
		fixture.write("agm/test/bdd/features/adapter.feature", validFeature("internal/adapter/SPEC.md"))
		fixture.git("add", "-A", "--", "internal/adapter/SPEC.owner", "internal/adapter/SPEC.md", "agm/test/bdd/features/adapter.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 {
			t.Fatalf("result = %#v, want valid replacement ownership reminder", result)
		}
	})

	t.Run("complete retirement reaches semantic review", func(t *testing.T) {
		fixture := newFixture(t)
		for _, relative := range []string{"pkg/example/SPEC.md", "agm/test/bdd/features/example.feature"} {
			if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(relative))); err != nil {
				t.Fatal(err)
			}
		}
		fixture.git("add", "-u", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || len(result.Changed) != 2 ||
			!strings.Contains(result.Reminder, "retirement or stable-ID migration") {
			t.Fatalf("result = %#v, want reviewed retirement reminder", result)
		}
	})

	t.Run("same-change relocation validates replacement graph", func(t *testing.T) {
		fixture := newFixture(t)
		if err := os.MkdirAll(filepath.Join(fixture.root, "pkg", "relocated"), 0o700); err != nil {
			t.Fatal(err)
		}
		fixture.git("mv", "pkg/example/SPEC.md", "pkg/relocated/SPEC.md")
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/relocated/SPEC.md"))
		fixture.git("add", "--", "agm/test/bdd/features/example.feature")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder || len(result.Findings) != 0 || len(result.Changed) != 3 {
			t.Fatalf("result = %#v, want valid relocation reminder", result)
		}
	})
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

func TestParseSpecOwnerAcceptsNeutralCanonicalTargets(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"SPEC.md", ".dear-agent/SPEC.md", "internal/hookparity/SPEC.md"} {
		got, err := ParseSpecOwner([]byte(target+"\n"), ".opencode/plugins/SPEC.md")
		if err != nil || got != target {
			t.Errorf("ParseSpecOwner(%q) = (%q, %v)", target, got, err)
		}
	}
}

func TestParseSpecOwnerRejectsMalformedAndHarnessOwnedTargets(t *testing.T) {
	t.Parallel()
	invalid := [][]byte{
		nil,
		[]byte("../internal/shared/SPEC.md\n"),
		[]byte("internal/../shared/SPEC.md\n"),
		[]byte("/internal/shared/SPEC.md\n"),
		[]byte("internal\\shared\\SPEC.md\n"),
		[]byte("https://example.com/SPEC.md\n"),
		[]byte("internal/shared/SPEC.md#fragment\n"),
		[]byte("internal/*/SPEC.md\n"),
		[]byte("internal/shared/SPEC.md\r\n"),
		[]byte("internal/shared/SPEC.md\ninternal/other/SPEC.md\n"),
		[]byte("codex-cli/SPEC.md\n"),
		[]byte(".opencode/SPEC.md\n"),
		[]byte("internal/plugins/SPEC.md\n"),
		[]byte(".opencode/plugins/SPEC.md\n"),
		{0xff, '\n'},
		append([]byte("internal/shared/"), append([]byte{0x01}, []byte("SPEC.md\n")...)...),
		[]byte(strings.Repeat("a", MaxSpecOwnerBytes+1)),
	}
	for index, owner := range invalid {
		if target, err := ParseSpecOwner(owner, ".opencode/plugins/SPEC.md"); err == nil {
			t.Errorf("invalid owner %d resolved to %q", index, target)
		}
	}
}

func TestSpecOwnerChangesGovernCanonicalSharedContract(t *testing.T) {
	fixture := newGuardRepository(t)
	const (
		specPath    = "internal/hookparity/SPEC.md"
		featurePath = "agm/test/bdd/features/hook_parity.feature"
		ownerPath   = ".opencode/plugins/SPEC.owner"
	)
	fixture.write(specPath, validSpec(featurePath))
	fixture.write(featurePath, validFeature(specPath))
	fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
	fixture.git("add", "--", specPath, featurePath, ".opencode/plugins/adapter.mjs")
	fixture.git("commit", "-m", "add shared contract")
	base := fixture.git("rev-parse", "HEAD")

	fixture.write(ownerPath, specPath+"\n")
	fixture.git("add", "--", ownerPath)
	staged := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if staged.Decision != DecisionReminder || strings.Join(staged.Changed, "\n") != ownerPath {
		t.Fatalf("staged owner result = %#v", staged)
	}

	fixture.git("commit", "-m", "link adapter to shared contract")
	committed := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeCommitted, Base: base})
	if committed.Decision != DecisionReminder || strings.Join(committed.Changed, "\n") != ownerPath {
		t.Fatalf("committed owner result = %#v", committed)
	}
}

func TestAddingLocalSpecBesideExistingOwnerFailsClosed(t *testing.T) {
	fixture := newGuardRepository(t)
	const (
		sharedSpec    = "internal/hookparity/SPEC.md"
		sharedFeature = "agm/test/bdd/features/hook_parity.feature"
		ownerPath     = ".opencode/plugins/SPEC.owner"
		localSpec     = ".opencode/plugins/SPEC.md"
		localFeature  = "agm/test/bdd/features/local.feature"
	)
	fixture.write(sharedSpec, validSpec(sharedFeature))
	fixture.write(sharedFeature, validFeature(sharedSpec))
	fixture.write(ownerPath, sharedSpec+"\n")
	fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
	fixture.git("add", "--", sharedSpec, sharedFeature, ownerPath, ".opencode/plugins/adapter.mjs")
	fixture.git("commit", "-m", "add shared ownership edge")

	fixture.write(localSpec, validSpec(localFeature))
	fixture.write(localFeature, validFeature(localSpec))
	fixture.git("add", "--", localSpec, localFeature)
	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	assertDecisionAndCode(t, result, DecisionBlock, "ambiguous-spec-owner")
}

func TestSourceLessOrNonregularSpecOwnerFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, guardRepository, string)
	}{
		{name: "no source"},
		{
			name: "symlink source",
			setup: func(t *testing.T, fixture guardRepository, _ string) {
				fixture.write("adapter-target", "export default {};\n")
				if err := os.MkdirAll(filepath.Join(fixture.root, ".opencode", "plugins"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join("..", "..", "adapter-target"), filepath.Join(fixture.root, ".opencode", "plugins", "adapter.mjs")); err != nil {
					t.Fatal(err)
				}
				fixture.git("add", "--", "adapter-target", ".opencode/plugins/adapter.mjs")
			},
		},
		{
			name: "gitlink source",
			setup: func(_ *testing.T, fixture guardRepository, head string) {
				fixture.git("update-index", "--add", "--cacheinfo", "160000,"+head+",.opencode/plugins/adapter.go")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGuardRepository(t)
			const (
				specPath    = "internal/hookparity/SPEC.md"
				featurePath = "agm/test/bdd/features/hook_parity.feature"
				ownerPath   = ".opencode/plugins/SPEC.owner"
			)
			fixture.write(specPath, validSpec(featurePath))
			fixture.write(featurePath, validFeature(specPath))
			fixture.git("add", "--", specPath, featurePath)
			fixture.git("commit", "-m", "add shared contract")
			head := fixture.git("rev-parse", "HEAD")
			if test.setup != nil {
				test.setup(t, fixture, head)
			}
			fixture.write(ownerPath, specPath+"\n")
			fixture.git("add", "--", ownerPath)
			result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
			assertDecisionAndCode(t, result, DecisionBlock, "orphan-spec-owner")
		})
	}
}

func TestSpecOwnerChangesFailClosedOnInvalidOwnershipEdges(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		owner string
		setup func(guardRepository)
		code  string
	}{
		{name: "malformed", owner: "../shared/SPEC.md\n", code: "invalid-spec-owner"},
		{name: "missing target", owner: "internal/missing/SPEC.md\n", code: "missing-spec-owner-target"},
		{
			name: "ambiguous local owner", owner: "internal/shared/SPEC.md\n", code: "ambiguous-spec-owner",
			setup: func(fixture guardRepository) {
				fixture.write(".opencode/plugins/SPEC.md", validSpec("agm/test/bdd/features/local.feature"))
				fixture.write("agm/test/bdd/features/local.feature", validFeature(".opencode/plugins/SPEC.md"))
				fixture.write("internal/shared/SPEC.md", validSpec("agm/test/bdd/features/shared.feature"))
				fixture.write("agm/test/bdd/features/shared.feature", validFeature("internal/shared/SPEC.md"))
				fixture.git("add", "--", ".opencode/plugins/SPEC.md", "agm/test/bdd/features/local.feature", "internal/shared/SPEC.md", "agm/test/bdd/features/shared.feature")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGuardRepository(t)
			fixture.write(".opencode/plugins/adapter.mjs", "export default {};\n")
			fixture.git("add", "--", ".opencode/plugins/adapter.mjs")
			if test.setup != nil {
				test.setup(fixture)
			}
			fixture.write(".opencode/plugins/SPEC.owner", test.owner)
			fixture.git("add", "--", ".opencode/plugins/SPEC.owner")
			result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
			assertDecisionAndCode(t, result, DecisionBlock, test.code)
		})
	}
}

func TestLogicalCorpusCountsDuplicateObjectPaths(t *testing.T) {
	t.Parallel()
	body := []byte(strings.Repeat("x", 40))
	oid := strings.Repeat("a", 40)
	entries := []treeEntry{
		{path: "one/SPEC.md", mode: "100644", oid: oid},
		{path: "two/SPEC.md", mode: "100644", oid: oid},
	}
	if _, failure := attachBodies(entries, map[string][]byte{oid: body}, 60); failure == nil || failure.code != "corpus-size-limit" {
		t.Fatalf("logical corpus failure = %#v", failure)
	}
}

func TestMarkdownFenceRequiresMatchingRunLengthAndTermination(t *testing.T) {
	t.Parallel()
	body := []byte("**EXAMPLE-01** The example contract shall remain valid.\n````text\n```\nAfter a trigger, the hidden example shall be ignored.\n````\n## BDD Traceability\n- Feature: `agm/test/bdd/features/example.feature`\n")
	document := parseSpecDocument("pkg/example/SPEC.md", body)
	for _, issue := range document.issues {
		if issue.code == "invalid-ears" || issue.code == "unterminated-markdown-fence" {
			t.Fatalf("unexpected issue: %#v", issue)
		}
	}
	unterminated := parseSpecDocument("pkg/example/SPEC.md", []byte("**EXAMPLE-01** The example contract shall remain valid.\n```text\n"))
	found := false
	for _, issue := range unterminated.issues {
		found = found || issue.code == "unterminated-markdown-fence"
	}
	if !found {
		t.Fatalf("unterminated issues = %#v", unterminated.issues)
	}
}

func TestSpecTraceabilityRejectsNonExecutableFeaturePaths(t *testing.T) {
	t.Parallel()
	for _, featurePath := range []string{
		"docs/contracts/example.feature",
		"features/example.feature",
		"agm/test/bdd/features-archive/example.feature",
		"AGM/test/bdd/features/example.feature",
		"agm/test/bdd/features/nested/example.feature",
		"agm/test/bdd/features/example feature.feature",
	} {
		document := parseSpecDocument("pkg/example/SPEC.md", []byte(validSpec(featurePath)))
		if !hasParseIssueCode(document.issues, "non-executable-bdd-reference") || len(document.features) != 0 {
			t.Fatalf("feature %q produced features=%#v issues=%#v", featurePath, document.features, document.issues)
		}
	}
	traversal := parseSpecDocument("pkg/example/SPEC.md", []byte(validSpec("agm/test/bdd/features/../example.feature")))
	if !hasParseIssueCode(traversal.issues, "malformed-bdd-reference") || len(traversal.features) != 0 {
		t.Fatalf("traversal feature produced features=%#v issues=%#v", traversal.features, traversal.issues)
	}
	document := parseSpecDocument("pkg/example/SPEC.md", []byte(validSpec("agm/test/bdd/features/example.feature")))
	if hasParseIssueCode(document.issues, "non-executable-bdd-reference") || !slices.Equal(document.features, []string{"agm/test/bdd/features/example.feature"}) {
		t.Fatalf("executable feature produced features=%#v issues=%#v", document.features, document.issues)
	}
}

func TestNonExecutableFeatureEvidenceFailsInStagedAndCommittedModes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		featurePath string
		code        string
		writePath   bool
	}{
		{name: "documentation example", featurePath: "docs/contracts/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "near-prefix archive", featurePath: "agm/test/bdd/features-archive/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "case variant", featurePath: "AGM/test/bdd/features/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "nested feature", featurePath: "agm/test/bdd/features/nested/example.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "unparseable basename", featurePath: "agm/test/bdd/features/example feature.feature", code: "non-executable-bdd-reference", writePath: true},
		{name: "traversal", featurePath: "agm/test/bdd/features/../example.feature", code: "malformed-bdd-reference"},
	}
	for _, test := range tests {
		for _, mode := range []Mode{ModeStaged, ModeCommitted} {
			t.Run(test.name+"/"+string(mode), func(t *testing.T) {
				fixture := newGuardRepository(t)
				base := fixture.git("rev-parse", "HEAD")
				fixture.write("pkg/example/SPEC.md", validSpec(test.featurePath))
				paths := []string{"pkg/example/SPEC.md"}
				if test.writePath {
					fixture.write(test.featurePath, validFeature("pkg/example/SPEC.md"))
					paths = append(paths, test.featurePath)
				}
				gitArgs := append([]string{"add", "--"}, paths...)
				fixture.git(gitArgs...)
				request := Request{Repository: fixture.root, Mode: mode}
				if mode == ModeCommitted {
					fixture.git("commit", "-m", "add non-executable contract evidence")
					request.Base = base
				}
				result := Evaluate(context.Background(), request)
				assertDecisionAndCode(t, result, DecisionBlock, test.code)
			})
		}
	}
}

func TestFeatureRunnableScenarioContract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		body         string
		wantRunnable int
		wantCode     string
	}{
		{
			name:         "concrete Then with assertion continuations",
			body:         "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Runnable\n    Given a condition\n    When an action occurs\n    Then an outcome is visible\n    And another outcome is visible\n    But no false outcome is visible\n",
			wantRunnable: 1,
		},
		{
			name:     "Given and When only",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Not runnable\n    Given a condition\n    When an action occurs\n",
			wantCode: "scenario-without-assertion",
		},
		{
			name:     "conjunction without Then",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Not runnable\n    Given a condition\n    And another condition\n    But no assertion\n",
			wantCode: "scenario-without-assertion",
		},
		{
			name:     "outline without Examples",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Not runnable\n    Given <condition>\n    Then an outcome is visible\n",
			wantCode: "outline-without-examples",
		},
		{
			name:     "outline with header only",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Not runnable\n    Given <condition>\n    Then an outcome is visible\n    Examples:\n      | condition |\n",
			wantCode: "outline-without-examples",
		},
		{
			name:         "outline with data row",
			body:         "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Outline: Runnable\n    Given <condition>\n    Then an outcome is visible\n    Examples:\n      | condition |\n      | ready     |\n",
			wantRunnable: 1,
		},
		{
			name:     "Scenario Template is outside the contract",
			body:     "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario Template: Not admitted\n    Given <condition>\n    Then an outcome is visible\n    Examples:\n      | condition |\n      | ready     |\n",
			wantCode: "unsupported-scenario-kind",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := parseFeatureDocument([]byte(test.body))
			if document.executableScenarios != test.wantRunnable {
				t.Fatalf("executable scenarios = %d, want %d; issues = %#v", document.executableScenarios, test.wantRunnable, document.issues)
			}
			if test.wantCode != "" && !hasParseIssueCode(document.issues, test.wantCode) {
				t.Fatalf("issues = %#v, want %q", document.issues, test.wantCode)
			}
		})
	}
}

func TestAmbientGitIndexAndReplacementObjectsCannotChangeEvidence(t *testing.T) {
	t.Run("ambient index", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		other := newGuardRepository(t)
		t.Setenv("GIT_INDEX_FILE", filepath.Join(other.root, ".git", "index"))
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		if result.Decision != DecisionReminder {
			t.Fatalf("decision = %q, findings = %#v", result.Decision, result.Findings)
		}
	})

	t.Run("replacement object", func(t *testing.T) {
		fixture := newGuardRepository(t)
		fixture.write("pkg/example/SPEC.md", invalidSpec("agm/test/bdd/features/example.feature"))
		fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
		fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
		staged := strings.Fields(fixture.git("ls-files", "--stage", "--", "pkg/example/SPEC.md"))
		if len(staged) < 2 {
			t.Fatalf("unexpected ls-files output: %q", staged)
		}
		validPath := filepath.Join(fixture.root, "valid-replacement")
		if err := os.WriteFile(validPath, []byte(validSpec("agm/test/bdd/features/example.feature")), 0o600); err != nil {
			t.Fatal(err)
		}
		validOID := fixture.gitInput(validSpec("agm/test/bdd/features/example.feature"), "hash-object", "-w", "--stdin")
		fixture.git("replace", staged[1], validOID)
		t.Setenv("GIT_NO_REPLACE_OBJECTS", "0")
		result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
		assertDecisionAndCode(t, result, DecisionBlock, "invalid-ears")
	})
}

func TestMissingPromisorBlobDoesNotLazyFetch(t *testing.T) {
	t.Parallel()
	fixture := newGuardRepository(t)
	fixture.write("pkg/example/SPEC.md", validSpec("agm/test/bdd/features/example.feature"))
	fixture.write("agm/test/bdd/features/example.feature", validFeature("pkg/example/SPEC.md"))
	fixture.git("add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
	fields := strings.Fields(fixture.git("ls-files", "--stage", "--", "pkg/example/SPEC.md"))
	if len(fields) < 2 {
		t.Fatalf("unexpected ls-files output: %q", fields)
	}
	oid := fields[1]
	fixture.git("config", "extensions.partialClone", "origin")
	fixture.git("config", "remote.origin.promisor", "true")
	fixture.git("config", "remote.origin.url", "file:///definitely-not-a-repository")
	objectPath := filepath.Join(fixture.root, ".git", "objects", oid[:2], oid[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatalf("remove test-owned loose object: %v", err)
	}

	started := time.Now()
	result := Evaluate(context.Background(), Request{Repository: fixture.root, Mode: ModeStaged})
	if result.Decision != DecisionBlock || (!hasCode(result, "git-object-read") && !hasCode(result, "missing-git-object")) {
		t.Fatalf("result = %#v", result)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("missing promisor object took %s; lazy fetch may have been attempted", elapsed)
	}
}

func TestCleanGitEnvironmentDropsAmbientControlVariables(t *testing.T) {
	t.Setenv("GIT_INDEX_FILE", "/tmp/hostile-index")
	t.Setenv("GIT_OBJECT_DIRECTORY", "/tmp/hostile-objects")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.fsmonitor'='hostile'")
	t.Setenv("GIT_NO_LAZY_FETCH", "0")
	environment := cleanGitEnvironment("/usr/bin/git")
	joined := "\n" + strings.Join(environment, "\n") + "\n"
	for _, forbidden := range []string{"GIT_INDEX_FILE=", "GIT_OBJECT_DIRECTORY=", "GIT_CONFIG_PARAMETERS="} {
		if strings.Contains(joined, "\n"+forbidden) {
			t.Fatalf("ambient %s leaked into %#v", forbidden, environment)
		}
	}
	for _, required := range []string{"GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_OPTIONAL_LOCKS=0"} {
		if !strings.Contains(joined, "\n"+required+"\n") {
			t.Fatalf("required %s missing from %#v", required, environment)
		}
	}
}

type guardRepository struct {
	t       *testing.T
	root    string
	sandbox *gittest.Sandbox
}

func newGuardRepository(t *testing.T) guardRepository {
	t.Helper()
	return newGuardRepositoryAt(t, t.TempDir())
}

func newGuardRepositoryAt(t *testing.T, root string) guardRepository {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	sandbox := gittest.New(t)
	sandbox.Run(t, root, "init")
	fixture := guardRepository{t: t, root: root, sandbox: sandbox}
	fixture.write("README.md", "base\n")
	fixture.git("add", "--", "README.md")
	fixture.git("commit", "-m", "base")
	return fixture
}

func (fixture guardRepository) write(relative, body string) {
	fixture.t.Helper()
	filePath := filepath.Join(fixture.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		fixture.t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(body), 0o600); err != nil {
		fixture.t.Fatal(err)
	}
}

func (fixture guardRepository) git(args ...string) string {
	fixture.t.Helper()
	return strings.TrimSpace(fixture.sandbox.Run(fixture.t, fixture.root, args...))
}

func (fixture guardRepository) gitInput(input string, args ...string) string {
	fixture.t.Helper()
	command := fixture.sandbox.Command(fixture.root, args...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		fixture.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func validSpec(featurePath string) string {
	return "# Example contract\n\n## Requirements\n\n**EXAMPLE-01** The example contract shall preserve one provider-neutral outcome.\n\n## BDD Traceability\n\n- BDD: `" + featurePath + "`\n"
}

func invalidSpec(featurePath string) string {
	return "# Example contract\n\n## Requirements\n\n**EXAMPLE-01** After an event, the example contract shall preserve an ambiguous outcome.\n\n## BDD Traceability\n\n- BDD: `" + featurePath + "`\n"
}

func validFeature(specPath string) string {
	return "# SPEC: " + specPath + "\nFeature: Example contract\n  Scenario: Preserve the outcome\n    Given an observable condition\n    When the contract is evaluated\n    Then the outcome is preserved\n"
}

func hasCode(result Result, code string) bool {
	for _, finding := range result.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func hasParseIssueCode(issues []parseIssue, code string) bool {
	for _, issue := range issues {
		if issue.code == code {
			return true
		}
	}
	return false
}

func assertDecisionAndCode(t *testing.T, result Result, decision Decision, code string) {
	t.Helper()
	if result.Decision != decision || !hasCode(result, code) {
		t.Fatalf("decision = %q, wanted %q with %q; findings = %#v", result.Decision, decision, code, result.Findings)
	}
	if result.Scope != GuardScope || !strings.Contains(result.EvidenceClaim, "no provider") {
		t.Fatalf("scope/evidence claim = %q / %q", result.Scope, result.EvidenceClaim)
	}
}

func writeFakeGit(t *testing.T, body string) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(filePath, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return filePath
}
