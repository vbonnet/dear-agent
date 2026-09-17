package harnessexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
	"github.com/vbonnet/dear-agent/agm/internal/codexhooks"
	"github.com/vbonnet/dear-agent/agm/internal/shellquote"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/internal/fsguard"
	"github.com/vbonnet/dear-agent/pkg/override"
)

var (
	testCodexHookSourceRepo   = "/reviewed/dear-agent"
	testCodexHookSourceCommit = strings.Repeat("a", 40)
	testCodexHookDigest       = strings.Repeat("b", 64)
	testCodexHookSubject      = func() string {
		subject, err := override.CodexHookTrustSubject(
			testCodexHookSourceRepo,
			testCodexHookSourceCommit,
			testCodexHookDigest,
		)
		if err != nil {
			panic(err)
		}
		return subject
	}()
	testCapabilityMu    sync.Mutex
	testCapabilityNext  uint64
	testCapabilityStore = map[string]override.LaunchCapability{}
)

func testCodexHookProof(session, reason, actor string) override.AuthorizationProof {
	return override.AuthorizationProof{
		Kind:            override.KindCodexHookTrust,
		Reason:          reason,
		Actor:           actor,
		Session:         session,
		Subject:         testCodexHookSubject,
		AuthorizationID: "0123456789abcdef0123456789abcdef",
	}
}

func TestReserveExecutorLaunchOverridesReauthenticatesClaimsAtLiveBrake(t *testing.T) {
	brakeClaim := override.AuthorizationProof{
		Kind:            override.KindAdmissionBrake,
		Reason:          "operator verified host recovery before this launch",
		Actor:           "dispatcher-test",
		Session:         "worker-one",
		AuthorizationID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	onlyBrake := circuitbreaker.CheckResult{
		Gates: []circuitbreaker.GateResult{{
			Gate: "admission_brake", RequiresOverride: true,
		}},
	}
	checks := 0
	check := func() circuitbreaker.CheckResult {
		checks++
		return onlyBrake
	}
	fresh := &override.Reservation{}
	var reserved override.Request
	reservations, err := reserveExecutorLaunchOverrides(
		"worker-one",
		[]override.AuthorizationProof{brakeClaim},
		check,
		func(request override.Request) (*override.Reservation, error) {
			reserved = request
			return fresh, nil
		},
	)
	if err != nil {
		t.Fatalf("reserve executor overrides: %v", err)
	}
	if checks != 2 {
		t.Fatalf("live admission checks = %d, want 2", checks)
	}
	if len(reservations) != 1 || reservations[0] != fresh {
		t.Fatalf("authenticated reservations = %v, want fresh reservation", reservations)
	}
	if reserved.Kind != brakeClaim.Kind ||
		reserved.Reason != brakeClaim.Reason ||
		reserved.Actor != brakeClaim.Actor ||
		reserved.Session != brakeClaim.Session {
		t.Fatalf("fresh reservation request = %+v, want exact launch claim", reserved)
	}
}

func TestReserveExecutorLaunchOverridesDropsBrakeClaimWhenBrakeClears(t *testing.T) {
	onlyBrake := circuitbreaker.CheckResult{
		Gates: []circuitbreaker.GateResult{{
			Gate: "admission_brake", RequiresOverride: true,
		}},
	}
	results := []circuitbreaker.CheckResult{onlyBrake, {Allowed: true}}
	reservations, err := reserveExecutorLaunchOverrides(
		"worker-one",
		[]override.AuthorizationProof{{
			Kind:            override.KindAdmissionBrake,
			Reason:          "operator verified host recovery before this launch",
			Actor:           "dispatcher-test",
			Session:         "worker-one",
			AuthorizationID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		func() circuitbreaker.CheckResult {
			result := results[0]
			results = results[1:]
			return result
		},
		func(override.Request) (*override.Reservation, error) {
			return &override.Reservation{}, nil
		},
	)
	if err != nil {
		t.Fatalf("reserve executor overrides: %v", err)
	}
	if len(reservations) != 0 {
		t.Fatalf("cleared brake retained %d authorization reservations", len(reservations))
	}
}

func TestReserveExecutorLaunchOverridesRejectsOtherLiveGate(t *testing.T) {
	reserveCalls := 0
	_, err := reserveExecutorLaunchOverrides(
		"worker-one",
		[]override.AuthorizationProof{{
			Kind:            override.KindAdmissionBrake,
			Reason:          "operator verified host recovery before this launch",
			Actor:           "dispatcher-test",
			Session:         "worker-one",
			AuthorizationID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
		func() circuitbreaker.CheckResult {
			return circuitbreaker.CheckResult{
				Gates: []circuitbreaker.GateResult{{Gate: "disk"}},
			}
		},
		func(override.Request) (*override.Reservation, error) {
			reserveCalls++
			return &override.Reservation{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "live circuit breakers") {
		t.Fatalf("other-gate error = %v", err)
	}
	if reserveCalls != 0 {
		t.Fatalf("other-gate refusal made %d override reservations", reserveCalls)
	}
}

func TestReserveExecutorLaunchOverridesLeavesOrdinaryLaunchAdmissionToParent(t *testing.T) {
	checkCalls := 0
	reserveCalls := 0
	reservations, err := reserveExecutorLaunchOverrides(
		"worker-one",
		nil,
		func() circuitbreaker.CheckResult {
			checkCalls++
			return circuitbreaker.CheckResult{
				Gates: []circuitbreaker.GateResult{{Gate: "spawn_stagger"}},
			}
		},
		func(override.Request) (*override.Reservation, error) {
			reserveCalls++
			return &override.Reservation{}, nil
		},
	)
	if err != nil {
		t.Fatalf("ordinary launch override reservation error = %v", err)
	}
	if len(reservations) != 0 {
		t.Fatalf("ordinary launch retained %d override reservations", len(reservations))
	}
	if checkCalls != 0 || reserveCalls != 0 {
		t.Fatalf("ordinary launch made %d admission checks and %d reservations", checkCalls, reserveCalls)
	}
}

func TestReserveExecutorLaunchOverridesRejectsOmittedBrakeClaim(t *testing.T) {
	reserveCalls := 0
	_, err := reserveExecutorLaunchOverrides(
		"worker-one",
		[]override.AuthorizationProof{testCodexHookProof("worker-one", "reviewed hooks", "dispatcher-test")},
		func() circuitbreaker.CheckResult {
			return circuitbreaker.CheckResult{
				Gates: []circuitbreaker.GateResult{{
					Gate: "admission_brake", RequiresOverride: true,
				}},
			}
		},
		func(override.Request) (*override.Reservation, error) {
			reserveCalls++
			return &override.Reservation{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "requires an admission-brake authorization claim") {
		t.Fatalf("omitted-brake error = %v", err)
	}
	if reserveCalls != 0 {
		t.Fatalf("omitted brake claim made %d override reservations", reserveCalls)
	}
}

func TestReserveExecutorLaunchOverridesRejectsBrakeEngagedAfterReservation(t *testing.T) {
	results := []circuitbreaker.CheckResult{
		{Allowed: true},
		{Gates: []circuitbreaker.GateResult{{
			Gate: "admission_brake", RequiresOverride: true,
		}}},
	}
	reserveCalls := 0
	_, err := reserveExecutorLaunchOverrides(
		"worker-one",
		[]override.AuthorizationProof{testCodexHookProof("worker-one", "reviewed hooks", "dispatcher-test")},
		func() circuitbreaker.CheckResult {
			result := results[0]
			results = results[1:]
			return result
		},
		func(override.Request) (*override.Reservation, error) {
			reserveCalls++
			return &override.Reservation{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "requires an admission-brake authorization claim") {
		t.Fatalf("late-brake error = %v", err)
	}
	if reserveCalls != 1 {
		t.Fatalf("late brake made %d override reservations, want hook claim reauthorization before final check", reserveCalls)
	}
}

func TestMain(m *testing.M) {
	original := scheduleHandoffExpiry
	originalAdmission := currentLaunchAdmission
	originalDiskAdmission := currentSandboxDiskAdmission
	originalIssueCapability := issueLaunchCapability
	originalLoadCapability := loadLaunchCapability
	originalConsumeCapability := consumeLaunchCapability
	scheduleHandoffExpiry = func(string, string, time.Time, bool) (io.Closer, error) { return nil, nil }
	currentLaunchAdmission = func(diskAdmissionPolicy) circuitbreaker.CheckResult {
		return circuitbreaker.CheckResult{Allowed: true}
	}
	currentSandboxDiskAdmission = func(diskAdmissionPolicy) error { return nil }
	issueLaunchCapability = func(
		claim override.LaunchCapabilityClaim,
	) (override.LaunchCapability, error) {
		testCapabilityMu.Lock()
		defer testCapabilityMu.Unlock()
		testCapabilityNext++
		capability := override.LaunchCapability{
			Version:               override.LaunchCapabilityVersion,
			ID:                    fmt.Sprintf("%032x", testCapabilityNext),
			LaunchCapabilityClaim: claim,
			IssuedUTC:             time.Now().UTC(),
		}
		testCapabilityStore[capability.ID] = capability
		return capability, nil
	}
	loadLaunchCapability = func(id string) (override.LaunchCapability, error) {
		testCapabilityMu.Lock()
		defer testCapabilityMu.Unlock()
		capability, ok := testCapabilityStore[id]
		if !ok {
			return override.LaunchCapability{}, os.ErrNotExist
		}
		return capability, nil
	}
	consumeLaunchCapability = func(capability override.LaunchCapability) error {
		testCapabilityMu.Lock()
		defer testCapabilityMu.Unlock()
		stored, ok := testCapabilityStore[capability.ID]
		if !ok {
			return os.ErrNotExist
		}
		storedJSON, err := override.EncodeLaunchCapability(stored)
		if err != nil {
			return err
		}
		consumeJSON, err := override.EncodeLaunchCapability(capability)
		if err != nil {
			return err
		}
		if !slices.Equal(storedJSON, consumeJSON) {
			return errors.New("launch capability consume mismatch")
		}
		delete(testCapabilityStore, capability.ID)
		return nil
	}
	code := m.Run()
	scheduleHandoffExpiry = original
	currentLaunchAdmission = originalAdmission
	currentSandboxDiskAdmission = originalDiskAdmission
	issueLaunchCapability = originalIssueCapability
	loadLaunchCapability = originalLoadCapability
	consumeLaunchCapability = originalConsumeCapability
	os.Exit(code)
}

func TestValidatePrivateFilesystemAuthorityRequiresImmediateChild(t *testing.T) {
	physicalBase, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	diskRoot := filepath.Join(physicalBase, "sandboxes")
	rootParent := filepath.Dir(diskRoot)
	filesystemRoot := filepath.VolumeName(diskRoot) + string(filepath.Separator)

	tests := []struct {
		name             string
		protocol         string
		diskRoot         string
		sandboxWorkspace string
		wantErr          bool
	}{
		{
			name:     "empty legacy authority",
			protocol: CodexProtocol,
		},
		{
			name:     "disk root without sandbox workspace",
			protocol: CodexProtocol,
			diskRoot: diskRoot,
		},
		{
			name:             "missing disk root",
			protocol:         CodexProtocol,
			sandboxWorkspace: filepath.Join(diskRoot, "session-a"),
			wantErr:          true,
		},
		{
			name:             "filesystem root",
			protocol:         CodexProtocol,
			diskRoot:         filesystemRoot,
			sandboxWorkspace: filepath.Join(filesystemRoot, "session-a"),
			wantErr:          true,
		},
		{
			name:             "workspace equals disk root",
			protocol:         CodexProtocol,
			diskRoot:         diskRoot,
			sandboxWorkspace: diskRoot,
			wantErr:          true,
		},
		{
			name:             "outside disk root",
			protocol:         CodexProtocol,
			diskRoot:         diskRoot,
			sandboxWorkspace: filepath.Join(rootParent, "outside", "session-a"),
			wantErr:          true,
		},
		{
			name:             "sibling of disk root",
			protocol:         CodexProtocol,
			diskRoot:         diskRoot,
			sandboxWorkspace: filepath.Join(rootParent, "session-a"),
			wantErr:          true,
		},
		{
			name:             "disk root prefix lookalike",
			protocol:         CodexProtocol,
			diskRoot:         diskRoot,
			sandboxWorkspace: filepath.Join(diskRoot+"-other", "session-a"),
			wantErr:          true,
		},
		{
			name:             "nested child",
			protocol:         CodexProtocol,
			diskRoot:         diskRoot,
			sandboxWorkspace: filepath.Join(diskRoot, "session-a", "merged"),
			wantErr:          true,
		},
		{
			name:             "valid immediate child",
			protocol:         ClaudeProtocol,
			diskRoot:         diskRoot,
			sandboxWorkspace: filepath.Join(diskRoot, "session-a"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePrivateFilesystemAuthority(tt.protocol, tt.diskRoot, tt.sandboxWorkspace)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePrivateFilesystemAuthority(%q, %q, %q) error = %v, wantErr %v",
					tt.protocol, tt.diskRoot, tt.sandboxWorkspace, err, tt.wantErr)
			}
		})
	}
}

func TestPrivateFilesystemAuthorityUsesHandoffWithoutCommandLeakage(t *testing.T) {
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	diskRoot := filepath.Join(t.TempDir(), "disk-authority-canary")
	sandboxWorkspace := filepath.Join(diskRoot, "session-authority-canary")
	workDir := filepath.Join(sandboxWorkspace, "merged", "repo")
	staleWorkspaceAssignment := "FSGUARD_SANDBOX_WORKSPACE=/stale/pane/workspace"

	tests := []struct {
		name     string
		protocol string
		prepare  func() (PreparedCommand, error)
	}{
		{
			name:     "Codex",
			protocol: CodexProtocol,
			prepare: func() (PreparedCommand, error) {
				return PrepareCodexCommand(CodexLaunch{
					SessionName: "authority-codex",
					DiskRoot:    diskRoot, SandboxWorkspace: sandboxWorkspace,
					WorkDir: workDir, Sandbox: "workspace-write",
				}, []string{staleWorkspaceAssignment, "PATH=/usr/bin", "HOME=/tmp/home"})
			},
		},
		{
			name:     "Claude",
			protocol: ClaudeProtocol,
			prepare: func() (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "authority-claude",
					DiskRoot:    diskRoot, SandboxWorkspace: sandboxWorkspace,
					WorkDir: workDir, DisableOAuth: true,
				}, []string{staleWorkspaceAssignment})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AGM_STATE_DIR", t.TempDir())
			prepared, err := tt.prepare()
			if err != nil {
				t.Fatalf("prepare %s command: %v", tt.name, err)
			}
			t.Cleanup(func() { _ = prepared.Cancel() })

			for _, leaked := range []string{
				"--disk-root", "--sandbox-workspace", "FSGUARD_SANDBOX_WORKSPACE=",
				"/stale/pane/workspace",
			} {
				if strings.Contains(prepared.Command, leaked) {
					t.Fatalf("prepared %s command exposed private filesystem authority %q: %s",
						tt.name, leaked, prepared.Command)
				}
			}
			if !strings.Contains(prepared.Command, workDir) {
				t.Fatalf("prepared %s command omitted its ordinary workdir %q: %s", tt.name, workDir, prepared.Command)
			}

			handoff, err := consumeHandoff(prepared.path, tt.protocol, "")
			if err != nil {
				t.Fatalf("consume %s handoff: %v", tt.name, err)
			}
			if handoff.DiskRoot != diskRoot || handoff.SandboxWorkspace != sandboxWorkspace {
				t.Fatalf("%s filesystem authority round trip = (%q, %q), want (%q, %q)",
					tt.name, handoff.DiskRoot, handoff.SandboxWorkspace, diskRoot, sandboxWorkspace)
			}
			for _, entry := range handoff.Environment {
				if strings.HasPrefix(entry, "FSGUARD_SANDBOX_WORKSPACE=") {
					t.Fatalf("%s handoff encoded filesystem authority as environment entry %q", tt.name, entry)
				}
			}
		})
	}
}

func TestOrdinaryCodexHandoffRejectsChangedLaunchArguments(t *testing.T) {
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	workDir := t.TempDir()
	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "bound-codex", Model: "gpt-test",
		WorkDir: workDir, Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("PrepareCodexCommand() error = %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })

	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "bound-codex",
		"--model", "gpt-test",
		"--workdir", t.TempDir(),
		"--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "does not authorize the requested Codex launch") {
		t.Fatalf("Run() error = %v, want exact ordinary-launch binding refusal", err)
	}
}

func TestPrivateExecutorsRejectRetargetedSandboxWorkDir(t *testing.T) {
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	for _, protocol := range []string{CodexProtocol, ClaudeProtocol} {
		t.Run(protocol, func(t *testing.T) {
			t.Setenv("AGM_STATE_DIR", t.TempDir())
			physicalRoot, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			diskRoot := filepath.Join(physicalRoot, "sandboxes")
			workspace := filepath.Join(diskRoot, "session-a")
			upper := filepath.Join(workspace, "upper")
			workDir := filepath.Join(workspace, "merged", "repo")
			if err := os.MkdirAll(filepath.Join(upper, "repo"), 0o700); err != nil {
				t.Fatal(err)
			}
			merged := filepath.Join(workspace, "merged")
			if err := os.Symlink(upper, merged); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			var prepared PreparedCommand
			switch protocol {
			case CodexProtocol:
				prepared, err = PrepareCodexCommand(CodexLaunch{
					SessionName: "retarget-codex", Model: "gpt-test",
					DiskRoot: diskRoot, SandboxWorkspace: workspace,
					WorkDir: workDir, Sandbox: "workspace-write",
				}, nil)
			case ClaudeProtocol:
				prepared, err = PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "retarget-claude", DisableOAuth: true,
					DiskRoot: diskRoot, SandboxWorkspace: workspace,
					WorkDir: workDir,
				}, nil)
			}
			if err != nil {
				t.Fatalf("prepare %s: %v", protocol, err)
			}
			t.Cleanup(func() { _ = prepared.Cancel() })

			external := filepath.Join(physicalRoot, "external")
			if err := os.MkdirAll(filepath.Join(external, "repo"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(merged); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, merged); err != nil {
				t.Fatal(err)
			}

			args := []string{"--handoff", prepared.path}
			if protocol == CodexProtocol {
				args = append(args,
					"--session", "retarget-codex", "--model", "gpt-test",
					"--workdir", workDir, "--sandbox", "workspace-write",
				)
			} else {
				args = append(args,
					"--session", "retarget-claude", "--workdir", workDir, "--disable-oauth",
				)
			}
			err = Run(protocol, args)
			if err == nil || !strings.Contains(err.Error(), "outside workspace") {
				t.Fatalf("Run(%s) error = %v, want retargeted-workdir refusal", protocol, err)
			}
		})
	}
}

func TestResolvePrivateSandboxWorkDirRejectsRetargetedWorkspaceAuthority(t *testing.T) {
	physicalRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(physicalRoot, "sandboxes", "session-a")
	if err := os.MkdirAll(filepath.Join(workspace, "repo"), 0o700); err != nil {
		t.Fatal(err)
	}
	displaced := workspace + "-displaced"
	if err := os.Rename(workspace, displaced); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(physicalRoot, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "repo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, workspace); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err = resolvePrivateSandboxWorkDir(filepath.Join(workspace, "repo"), workspace)
	if err == nil || !strings.Contains(err.Error(), "exact physical authority") {
		t.Fatalf("resolvePrivateSandboxWorkDir() error = %v, want retargeted workspace refusal", err)
	}
}

func TestDefaultPrivateDiskAdmissionDoesNotSkipEmptyRoot(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "missing-home"))
	if err := checkSandboxDiskAdmission(snapshotDiskAdmissionPolicy("")); err == nil {
		t.Fatal("currentSandboxDiskAdmission(\"\") error = nil, want default-volume probe failure")
	}
}

func TestCarriedDiskThresholdOverridesStaleExecutorEnvironment(t *testing.T) {
	t.Setenv("AGM_MIN_FREE_DISK_GB", "0")
	policy := diskAdmissionPolicy{root: "/planned/sandboxes", minFreeGB: 73}
	if got := circuitBreakerConfigForDiskPolicy(policy).MinFreeDiskGB; got != 73 {
		t.Fatalf("proof-bearing admission threshold = %v, want carried threshold 73", got)
	}
	if err := validateDiskAdmissionPolicy(policy); err != nil {
		t.Fatalf("validate carried disk policy: %v", err)
	}
}

func TestHandoffDiskThresholdRequiresExplicitFinitePresence(t *testing.T) {
	zero := 0.0
	valid := launchHandoff{DiskRoot: "", MinFreeDiskGB: &zero}
	if err := validateHandoffSandboxWorkspace(valid, CodexProtocol); err != nil {
		t.Fatalf("explicit zero disk threshold rejected: %v", err)
	}
	if err := validateHandoffSandboxWorkspace(launchHandoff{}, CodexProtocol); err == nil ||
		!strings.Contains(err.Error(), "omits its disk admission threshold") {
		t.Fatalf("missing disk threshold error = %v", err)
	}
	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1)} {
		err := validateHandoffSandboxWorkspace(
			launchHandoff{MinFreeDiskGB: &invalid}, CodexProtocol,
		)
		if err == nil || !strings.Contains(err.Error(), "finite and non-negative") {
			t.Fatalf("invalid disk threshold %v error = %v", invalid, err)
		}
	}
}

func TestPrivateExecutorsRecheckSandboxDiskWithoutOverrideProofs(t *testing.T) {
	originalExecutablePath := executablePath
	originalSandboxDiskAdmission := currentSandboxDiskAdmission
	originalLookPathInEnvironment := lookPathInEnvironment
	originalCommitLaunchOverrideProofs := commitLaunchOverrideProofs
	originalReplaceProcess := replaceProcess
	originalChangeDirectory := changeDirectory
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		currentSandboxDiskAdmission = originalSandboxDiskAdmission
		lookPathInEnvironment = originalLookPathInEnvironment
		commitLaunchOverrideProofs = originalCommitLaunchOverrideProofs
		replaceProcess = originalReplaceProcess
		changeDirectory = originalChangeDirectory
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	physicalBase, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	diskRoot := filepath.Join(physicalBase, "sandboxes")
	sandboxWorkspace := filepath.Join(diskRoot, "session-a")
	workDir := filepath.Join(sandboxWorkspace, "merged", "repo")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	diskDenied := errors.New("sandbox volume is below the disk floor")

	tests := []struct {
		name     string
		protocol string
		prepare  func() (PreparedCommand, error)
		args     func(string) []string
	}{
		{
			name:     "Codex",
			protocol: CodexProtocol,
			prepare: func() (PreparedCommand, error) {
				return PrepareCodexCommand(CodexLaunch{
					SessionName: "disk-recheck-codex", Model: "gpt-test",
					DiskRoot: diskRoot, SandboxWorkspace: sandboxWorkspace,
					WorkDir: workDir, Sandbox: "workspace-write",
				}, nil)
			},
			args: func(handoffPath string) []string {
				return []string{
					"--handoff", handoffPath,
					"--session", "disk-recheck-codex",
					"--model", "gpt-test",
					"--workdir", workDir,
					"--sandbox", "workspace-write",
				}
			},
		},
		{
			name:     "Claude",
			protocol: ClaudeProtocol,
			prepare: func() (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "disk-recheck-claude",
					DiskRoot:    diskRoot, SandboxWorkspace: sandboxWorkspace,
					WorkDir: workDir, DisableOAuth: true,
				}, nil)
			},
			args: func(handoffPath string) []string {
				return []string{
					"--handoff", handoffPath,
					"--session", "disk-recheck-claude",
					"--workdir", workDir,
					"--disable-oauth",
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AGM_STATE_DIR", t.TempDir())
			t.Setenv("AGM_MIN_FREE_DISK_GB", "73")
			prepared, err := tt.prepare()
			if err != nil {
				t.Fatalf("prepare %s command: %v", tt.name, err)
			}
			t.Cleanup(func() { _ = prepared.Cancel() })

			payload, err := os.ReadFile(prepared.path)
			if err != nil {
				t.Fatalf("read staged %s handoff: %v", tt.name, err)
			}
			var staged launchHandoff
			if err := json.Unmarshal(payload, &staged); err != nil {
				t.Fatalf("decode staged %s handoff: %v", tt.name, err)
			}
			if len(staged.OverrideProofs) != 0 || staged.LaunchCapabilityID != "" {
				t.Fatalf("staged %s handoff unexpectedly has override authority: proofs=%d capability=%q",
					tt.name, len(staged.OverrideProofs), staged.LaunchCapabilityID)
			}
			if staged.MinFreeDiskGB == nil || *staged.MinFreeDiskGB != 73 {
				t.Fatalf("staged %s disk threshold = %v, want explicit caller snapshot 73", tt.name, staged.MinFreeDiskGB)
			}
			// Simulate a stale long-lived pane that would otherwise weaken the
			// launch-boundary floor after the caller has queued the command.
			t.Setenv("AGM_MIN_FREE_DISK_GB", "0")

			var checkedPolicies []diskAdmissionPolicy
			currentSandboxDiskAdmission = func(got diskAdmissionPolicy) error {
				checkedPolicies = append(checkedPolicies, got)
				return diskDenied
			}
			lookupCalls := 0
			lookPathInEnvironment = func(name string, _ []string) (string, error) {
				lookupCalls++
				return "/fixed/" + name, nil
			}
			changeDirectory = func(string) error { return nil }
			commitLaunchOverrideProofs = func(string, diskAdmissionPolicy, ...override.AuthorizationProof) error {
				t.Fatal("disk-denied private launch reached override commitment")
				return nil
			}
			replaceProcess = func(string, []string, []string) error {
				t.Fatal("disk-denied private launch reached process replacement")
				return nil
			}

			err = Run(tt.protocol, tt.args(prepared.path))
			if !errors.Is(err, diskDenied) {
				t.Fatalf("run disk-denied %s command error = %v, want %v", tt.name, err, diskDenied)
			}
			if len(checkedPolicies) != 1 || checkedPolicies[0].root != diskRoot || checkedPolicies[0].minFreeGB != 73 {
				t.Fatalf("%s disk rechecks = %+v, want root %q with caller threshold 73", tt.name, checkedPolicies, diskRoot)
			}
			if lookupCalls != 1 {
				t.Fatalf("%s executable lookups = %d, want preparation to finish before final disk recheck", tt.name, lookupCalls)
			}
		})
	}
}

func TestSandboxDiskAdmissionFailsClosed(t *testing.T) {
	t.Run("low disk", func(t *testing.T) {
		t.Setenv("AGM_MIN_FREE_DISK_GB", "1e300")
		diskRoot, resolveErr := filepath.EvalSymlinks(t.TempDir())
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		err := checkSandboxDiskAdmission(snapshotDiskAdmissionPolicy(diskRoot))
		if err == nil || !strings.Contains(err.Error(), "free disk too low") {
			t.Fatalf("low-disk admission error = %v, want free-disk refusal", err)
		}
	})

	t.Run("disk path reader error", func(t *testing.T) {
		rootParent := t.TempDir()
		diskRoot := filepath.Join(rootParent, "sandboxes")
		if err := os.Symlink(t.TempDir(), diskRoot); err != nil {
			t.Fatalf("create sandbox-root symlink: %v", err)
		}

		err := checkSandboxDiskAdmission(snapshotDiskAdmissionPolicy(diskRoot))
		if err == nil ||
			!strings.Contains(err.Error(), "prepare sandbox-volume disk check") ||
			!strings.Contains(err.Error(), "symlink") {
			t.Fatalf("reader-error admission = %v, want fail-closed symlink refusal", err)
		}
	})
}

func TestPrivateExecutorsReplaceOrClearSandboxWorkspace(t *testing.T) {
	originalExecutablePath := executablePath
	originalSandboxDiskAdmission := currentSandboxDiskAdmission
	originalLookPathInEnvironment := lookPathInEnvironment
	originalCommitLaunchOverrideProofs := commitLaunchOverrideProofs
	originalReplaceProcess := replaceProcess
	originalChangeDirectory := changeDirectory
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		currentSandboxDiskAdmission = originalSandboxDiskAdmission
		lookPathInEnvironment = originalLookPathInEnvironment
		commitLaunchOverrideProofs = originalCommitLaunchOverrideProofs
		replaceProcess = originalReplaceProcess
		changeDirectory = originalChangeDirectory
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	physicalBase, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	diskRoot := filepath.Join(physicalBase, "sandboxes")
	workspace := filepath.Join(diskRoot, "session-a")
	staleWorkspace := filepath.Join(diskRoot, "stale-session")
	workDir := filepath.Join(workspace, "merged", "repo")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	physicalWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		protocol  string
		workspace string
		prepare   func(string) (PreparedCommand, error)
		args      func(string) []string
	}{
		{
			name: "Codex replaces stale workspace", protocol: CodexProtocol, workspace: workspace,
			prepare: func(selected string) (PreparedCommand, error) {
				return PrepareCodexCommand(CodexLaunch{
					SessionName: "workspace-codex", Model: "gpt-test",
					DiskRoot: diskRoot, SandboxWorkspace: selected,
					WorkDir: workDir, Sandbox: "workspace-write",
				}, []string{fsguard.EnvSandboxWorkspace + "=" + staleWorkspace})
			},
			args: func(handoffPath string) []string {
				return []string{
					"--handoff", handoffPath,
					"--session", "workspace-codex",
					"--model", "gpt-test",
					"--workdir", workDir,
					"--sandbox", "workspace-write",
				}
			},
		},
		{
			name: "Codex clears stale workspace", protocol: CodexProtocol,
			prepare: func(selected string) (PreparedCommand, error) {
				return PrepareCodexCommand(CodexLaunch{
					SessionName: "workspace-codex", Model: "gpt-test",
					DiskRoot: diskRoot, SandboxWorkspace: selected,
					WorkDir: workDir, Sandbox: "workspace-write",
				}, []string{fsguard.EnvSandboxWorkspace + "=" + staleWorkspace})
			},
			args: func(handoffPath string) []string {
				return []string{
					"--handoff", handoffPath,
					"--session", "workspace-codex",
					"--model", "gpt-test",
					"--workdir", workDir,
					"--sandbox", "workspace-write",
				}
			},
		},
		{
			name: "Claude replaces stale workspace", protocol: ClaudeProtocol, workspace: workspace,
			prepare: func(selected string) (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "workspace-claude",
					DiskRoot:    diskRoot, SandboxWorkspace: selected,
					WorkDir: workDir, DisableOAuth: true,
				}, []string{fsguard.EnvSandboxWorkspace + "=" + staleWorkspace})
			},
			args: func(handoffPath string) []string {
				return []string{
					"--handoff", handoffPath,
					"--session", "workspace-claude",
					"--workdir", workDir,
					"--disable-oauth",
				}
			},
		},
		{
			name: "Claude clears stale workspace", protocol: ClaudeProtocol,
			prepare: func(selected string) (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "workspace-claude",
					DiskRoot:    diskRoot, SandboxWorkspace: selected,
					WorkDir: workDir, DisableOAuth: true,
				}, []string{fsguard.EnvSandboxWorkspace + "=" + staleWorkspace})
			},
			args: func(handoffPath string) []string {
				return []string{
					"--handoff", handoffPath,
					"--session", "workspace-claude",
					"--workdir", workDir,
					"--disable-oauth",
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AGM_STATE_DIR", t.TempDir())
			t.Setenv(fsguard.EnvSandboxWorkspace, staleWorkspace)

			var events []string
			currentSandboxDiskAdmission = func(got diskAdmissionPolicy) error {
				if got.root != diskRoot {
					t.Fatalf("disk admission root = %q, want %q", got.root, diskRoot)
				}
				events = append(events, "disk")
				return nil
			}
			lookPathInEnvironment = func(name string, _ []string) (string, error) {
				events = append(events, "lookup")
				return "/fixed/" + name, nil
			}
			commitLaunchOverrideProofs = func(_ string, got diskAdmissionPolicy, proofs ...override.AuthorizationProof) error {
				if got.root != diskRoot || len(proofs) != 0 {
					t.Fatalf("override commit = root %q proofs %v, want root %q and no proofs", got.root, proofs, diskRoot)
				}
				events = append(events, "commit")
				return nil
			}
			changeDirectory = func(gotWorkDir string) error {
				wantWorkDir := workDir
				if tt.workspace != "" {
					wantWorkDir = physicalWorkDir
				}
				if gotWorkDir != wantWorkDir {
					t.Fatalf("Claude workdir = %q, want %q", gotWorkDir, wantWorkDir)
				}
				return nil
			}
			var childEnvironment []string
			replaceProcess = func(_ string, _ []string, environment []string) error {
				events = append(events, "exec")
				childEnvironment = append([]string(nil), environment...)
				return nil
			}

			prepared, err := tt.prepare(tt.workspace)
			if err != nil {
				t.Fatalf("prepare %s command: %v", tt.name, err)
			}
			t.Cleanup(func() { _ = prepared.Cancel() })

			err = Run(tt.protocol, tt.args(prepared.path))
			if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
				t.Fatalf("run %s error = %v, want unexpected-return guard", tt.name, err)
			}
			childValues := environmentMap(childEnvironment)
			gotWorkspace, workspacePresent := childValues[fsguard.EnvSandboxWorkspace]
			if tt.workspace == "" && workspacePresent {
				t.Fatalf("child retained omitted sandbox workspace entry %q", gotWorkspace)
			}
			if tt.workspace != "" && (!workspacePresent || gotWorkspace != tt.workspace) {
				t.Fatalf("child sandbox workspace = %q (present %v), want %q", gotWorkspace, workspacePresent, tt.workspace)
			}
			if want := []string{"lookup", "disk", "commit", "exec"}; !slices.Equal(events, want) {
				t.Fatalf("executor events = %q, want %q", events, want)
			}
		})
	}
}

func TestResolveSubmissionPreservesUncertainAndCancelsConfirmedFailure(t *testing.T) {
	for _, tc := range []struct {
		name          string
		submissionErr error
		wantUncertain bool
		wantErr       bool
		wantCancelled bool
	}{
		{
			name:          "uncertain acknowledgement preserves handoff",
			submissionErr: tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement")),
			wantUncertain: true,
		},
		{
			name:          "confirmed failure cancels handoff",
			submissionErr: errors.New("send rejected"),
			wantErr:       true,
			wantCancelled: true,
		},
		{name: "confirmed success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cancelled := false
			uncertain, err := ResolveSubmission(tc.submissionErr, func() error {
				cancelled = true
				return nil
			})
			if uncertain != tc.wantUncertain {
				t.Fatalf("uncertain = %v, want %v", uncertain, tc.wantUncertain)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if cancelled != tc.wantCancelled {
				t.Fatalf("cancelled = %v, want %v", cancelled, tc.wantCancelled)
			}
		})
	}
}

func TestPreparedHarnessCommandCommitsBeforeExec(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalScheduleExpiry := scheduleHandoffExpiry
	originalCommitProofs := commitLaunchOverrideProofs
	originalRecordSpawn := recordLaunchSpawn
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduleExpiry
		commitLaunchOverrideProofs = originalCommitProofs
		recordLaunchSpawn = originalRecordSpawn
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	scheduleHandoffExpiry = func(string, string, time.Time, bool) (io.Closer, error) {
		return nil, nil
	}

	const command = "cd '/tmp/agy work' && agy --model fixture"
	prepared, err := PrepareHarnessCommand("agy-worker", command, false, false)
	if err != nil {
		t.Fatalf("PrepareHarnessCommand() error = %v", err)
	}
	if !strings.HasPrefix(prepared.Command, "'/opt/agm/bin/agm' "+HarnessProtocol) ||
		!strings.HasSuffix(prepared.Command, " && exit") {
		t.Fatalf("prepared harness command = %q", prepared.Command)
	}
	executable, arguments, err := prepared.DirectInvocation()
	if err != nil {
		t.Fatalf("direct harness invocation: %v", err)
	}
	if executable != "/opt/agm/bin/agm" || !slices.Equal(arguments, []string{
		HarnessProtocol,
		"--handoff", prepared.path,
		"--session", "agy-worker",
	}) {
		t.Fatalf("direct harness invocation = %q %q", executable, arguments)
	}
	if err := prepared.BindOverrideReservations(true); err != nil {
		t.Fatalf("bind harness launch effects: %v", err)
	}

	var events []string
	commitLaunchOverrideProofs = func(sessionName string, diskPolicy diskAdmissionPolicy, proofs ...override.AuthorizationProof) error {
		if sessionName != "agy-worker" || diskPolicy.root != "" || len(proofs) != 0 {
			t.Fatalf("commit launch = (%q, %+v, %v)", sessionName, diskPolicy, proofs)
		}
		events = append(events, "commit")
		return nil
	}
	recordLaunchSpawn = func() error {
		events = append(events, "record-spawn")
		return nil
	}
	replaceProcess = func(path string, argv, _ []string) error {
		if path != "/bin/sh" || !slices.Equal(argv, []string{"sh", "-c", command}) {
			t.Fatalf("replace process = (%q, %v)", path, argv)
		}
		events = append(events, "exec")
		return errors.New("fixture exec returned")
	}
	err = Run(arguments[0], arguments[1:])
	if err == nil || !strings.Contains(err.Error(), "fixture exec returned") {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := strings.Join(events, ","), "commit,record-spawn,exec"; got != want {
		t.Fatalf("executor events = %q, want %q", got, want)
	}
}

func TestGenericHandoffRejectsSelfGeneratedOverrideProof(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalCommitProofs := commitLaunchOverrideProofs
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		commitLaunchOverrideProofs = originalCommitProofs
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	prepared, err := PrepareHarnessCommand(
		"forged-generic",
		"printf harmless",
		true,
		false,
	)
	if err != nil {
		t.Fatalf("prepare generic handoff: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })

	data, err := os.ReadFile(prepared.path)
	if err != nil {
		t.Fatalf("read generic handoff: %v", err)
	}
	var handoff launchHandoff
	if err := json.Unmarshal(data, &handoff); err != nil {
		t.Fatalf("decode generic handoff: %v", err)
	}
	handoff.OverrideProofs = []override.AuthorizationProof{{
		Kind:            override.KindAdmissionBrake,
		Reason:          "operator reviewed host recovery before this launch",
		Actor:           "forged-actor",
		Session:         "forged-generic",
		AuthorizationID: "ffffffffffffffffffffffffffffffff",
	}}
	handoff.LaunchCapabilityID = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	encoded, err := encodeBoundHandoff(handoff)
	if err != nil {
		t.Fatalf("encode forged generic handoff: %v", err)
	}
	if err := os.WriteFile(prepared.path, encoded, 0o600); err != nil {
		t.Fatalf("write forged generic handoff: %v", err)
	}
	commitLaunchOverrideProofs = func(string, diskAdmissionPolicy, ...override.AuthorizationProof) error {
		t.Fatal("self-generated proof reached override reauthorization")
		return nil
	}
	replaceProcess = func(string, []string, []string) error {
		t.Fatal("self-generated proof reached harness execution")
		return nil
	}
	err = Run(HarnessProtocol, []string{
		"--handoff", prepared.path,
		"--session", "forged-generic",
	})
	if err == nil || !strings.Contains(err.Error(), "root-attested private launch capability") {
		t.Fatalf("forged generic handoff error = %v", err)
	}
}

func TestGenericHandoffCapabilityRejectsPostIssueMutation(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalCommitProofs := commitLaunchOverrideProofs
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		commitLaunchOverrideProofs = originalCommitProofs
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	prepared, err := PrepareHarnessCommand(
		"mutated-generic",
		"agy --model reviewed",
		true,
		false,
	)
	if err != nil {
		t.Fatalf("prepare generic handoff: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	proof := override.AuthorizationProof{
		Kind:            override.KindAdmissionBrake,
		Reason:          "operator reviewed host recovery before this launch",
		Actor:           "dispatcher-test",
		Session:         "mutated-generic",
		AuthorizationID: "dddddddddddddddddddddddddddddddd",
	}
	if err := bindHandoffOverrideProofs(
		prepared.path,
		HarnessProtocol,
		false,
		true,
		[]override.AuthorizationProof{proof},
	); err != nil {
		t.Fatalf("bind root-attested generic handoff: %v", err)
	}
	data, err := os.ReadFile(prepared.path)
	if err != nil {
		t.Fatalf("read bound generic handoff: %v", err)
	}
	var handoff launchHandoff
	if err := json.Unmarshal(data, &handoff); err != nil {
		t.Fatalf("decode bound generic handoff: %v", err)
	}
	handoff.HarnessCommand = "agy --model substituted"
	encoded, err := encodeBoundHandoff(handoff)
	if err != nil {
		t.Fatalf("encode mutated generic handoff: %v", err)
	}
	if err := os.WriteFile(prepared.path, encoded, 0o600); err != nil {
		t.Fatalf("write mutated generic handoff: %v", err)
	}
	commitLaunchOverrideProofs = func(string, diskAdmissionPolicy, ...override.AuthorizationProof) error {
		t.Fatal("mutated handoff reached override reauthorization")
		return nil
	}
	replaceProcess = func(string, []string, []string) error {
		t.Fatal("mutated handoff reached harness execution")
		return nil
	}
	err = Run(HarnessProtocol, []string{
		"--handoff", prepared.path,
		"--session", "mutated-generic",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match the private handoff") {
		t.Fatalf("mutated generic handoff error = %v", err)
	}
}

func TestGenericHandoffCapabilityIsOneShot(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	t.Cleanup(func() {
		executablePath = originalExecutablePath
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	prepared, err := PrepareHarnessCommand(
		"replayed-generic",
		"printf harmless",
		true,
		false,
	)
	if err != nil {
		t.Fatalf("prepare generic handoff: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	proof := override.AuthorizationProof{
		Kind:            override.KindAdmissionBrake,
		Reason:          "operator reviewed host recovery before this launch",
		Actor:           "dispatcher-test",
		Session:         "replayed-generic",
		AuthorizationID: "abababababababababababababababab",
	}
	if err := bindHandoffOverrideProofs(
		prepared.path,
		HarnessProtocol,
		false,
		false,
		[]override.AuthorizationProof{proof},
	); err != nil {
		t.Fatalf("bind root-attested generic handoff: %v", err)
	}
	encoded, err := os.ReadFile(prepared.path)
	if err != nil {
		t.Fatalf("read bound generic handoff: %v", err)
	}
	if _, err := consumeHandoff(prepared.path, HarnessProtocol, ""); err != nil {
		t.Fatalf("consume first generic handoff: %v", err)
	}
	if err := os.WriteFile(prepared.path, encoded, 0o600); err != nil {
		t.Fatalf("restore copied generic handoff: %v", err)
	}
	if _, err := consumeHandoff(prepared.path, HarnessProtocol, ""); err == nil ||
		!strings.Contains(err.Error(), "root-attested private launch capability") {
		t.Fatalf("replayed generic handoff error = %v", err)
	}
}

func TestPreparedHarnessCommandRefusesExecWhenCommitFails(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalScheduleExpiry := scheduleHandoffExpiry
	originalCommitProofs := commitLaunchOverrideProofs
	originalRecordSpawn := recordLaunchSpawn
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduleExpiry
		commitLaunchOverrideProofs = originalCommitProofs
		recordLaunchSpawn = originalRecordSpawn
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	scheduleHandoffExpiry = func(string, string, time.Time, bool) (io.Closer, error) {
		return nil, nil
	}
	prepared, err := PrepareHarnessCommand("agy-denied", "agy --model fixture", true, false)
	if err != nil {
		t.Fatalf("PrepareHarnessCommand() error = %v", err)
	}
	if err := prepared.BindOverrideReservations(true); err != nil {
		t.Fatalf("bind harness launch effects: %v", err)
	}

	refusal := errors.New("override ledger unavailable")
	commitLaunchOverrideProofs = func(string, diskAdmissionPolicy, ...override.AuthorizationProof) error {
		return refusal
	}
	recordLaunchSpawn = func() error {
		t.Fatal("spawn recorded after override commit failed")
		return nil
	}
	replaceProcess = func(string, []string, []string) error {
		t.Fatal("harness executed after override commit failed")
		return nil
	}
	err = Run(HarnessProtocol, []string{
		"--handoff", prepared.path,
		"--session", "agy-denied",
	})
	if !errors.Is(err, refusal) {
		t.Fatalf("Run() error = %v, want %v", err, refusal)
	}
}

func TestPreparedClaudeCommandCarriesCallerOnlyOAuthAndTelemetry(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Setenv("ANTHROPIC_API_KEY", "stale-pane-api-key")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "stale-pane-endpoint")

	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	resolveClaudeOAuth = func() string { return "caller-only-oauth" }

	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		SessionName: "handoff-claude", Model: "claude-test", AddDirs: []string{"/tmp/work"},
		ForwardTelemetry: true,
	}, []string{
		"ANTHROPIC_API_KEY=caller-api-key",
		"OTEL_EXPORTER_OTLP_ENDPOINT=caller-endpoint",
		"OTEL_EXPORTER_OTLP_HEADERS=authorization=caller-telemetry",
	})
	if err != nil {
		t.Fatalf("prepare Claude command: %v", err)
	}
	for _, secret := range []string{"caller-only-oauth", "caller-api-key", "caller-endpoint", "caller-telemetry"} {
		if strings.Contains(prepared.Command, secret) {
			t.Fatalf("prepared command exposed %q: %s", secret, prepared.Command)
		}
	}
	if !strings.HasPrefix(prepared.Command, "'/opt/agm/bin/agm' "+ClaudeProtocol) {
		t.Fatalf("prepared command did not pin current AGM executable: %s", prepared.Command)
	}
	assertPrivateHandoffMode(t, prepared.path)

	var childEnvironment []string
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/claude", nil }
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	resolveClaudeOAuth = func() string { return "" }
	err = Run(ClaudeProtocol, []string{
		"--handoff", prepared.path,
		"--session", "handoff-claude", "--model", "claude-test", "--add-dir", "/tmp/work",
		"--forward-telemetry",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Claude command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for name, want := range map[string]string{
		"CLAUDE_CODE_OAUTH_TOKEN":     "caller-only-oauth",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "caller-endpoint",
		"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=caller-telemetry",
	} {
		if got := values[name]; got != want {
			t.Errorf("Claude child %s = %q, want %q", name, got, want)
		}
	}
	if _, ok := values["ANTHROPIC_API_KEY"]; ok {
		t.Fatal("Claude child retained stale Anthropic API key beside caller OAuth")
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("consumed Claude handoff still exists: %v", err)
	}
}

func TestPreparedClaudeDirectInvocationDoesNotCommitWhenBinaryDisappears(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalCommitLaunchOverrideProofs := commitLaunchOverrideProofs
	originalRecordLaunchSpawn := recordLaunchSpawn
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		commitLaunchOverrideProofs = originalCommitLaunchOverrideProofs
		recordLaunchSpawn = originalRecordLaunchSpawn
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	resolveClaudeOAuth = func() string { return "fresh-supervisor-oauth" }

	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		Binary:      "/opt/claude/bin/claude",
		SessionName: "supervisor-s1",
		Model:       "claude-test",
		AutoMode:    true,
		Permission:  "auto",
		ExtraArgs:   []string{"--verbose"},
		Persistent:  true,
	}, nil)
	if err != nil {
		t.Fatalf("prepare direct Claude command: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	supervisorProof := override.AuthorizationProof{
		Kind:            override.KindSupervisorOAuthCheck,
		Reason:          "development supervisor has no stored OAuth credentials",
		Actor:           "operator-test",
		Session:         "supervisor-s1",
		AuthorizationID: "0123456789abcdef0123456789abcdef",
	}
	if err := bindHandoffOverrideProofs(
		prepared.path,
		ClaudeProtocol,
		false,
		true,
		[]override.AuthorizationProof{supervisorProof},
	); err != nil {
		t.Fatalf("bind direct Claude launch effects: %v", err)
	}
	executable, arguments, err := prepared.DirectInvocation()
	if err != nil {
		t.Fatalf("direct Claude invocation: %v", err)
	}
	if executable != "/opt/agm/bin/agm" ||
		len(arguments) == 0 ||
		arguments[0] != ClaudeProtocol ||
		!slices.Contains(arguments, "/opt/claude/bin/claude") ||
		!slices.Contains(arguments, "--verbose") {
		t.Fatalf("direct Claude invocation = %q %q", executable, arguments)
	}

	lookPathInEnvironment = func(name string, _ []string) (string, error) {
		if name != "/opt/claude/bin/claude" {
			t.Fatalf("resolved Claude binary = %q", name)
		}
		return "", errors.New("claude disappeared")
	}
	commits := 0
	recordedSpawns := 0
	commitLaunchOverrideProofs = func(string, diskAdmissionPolicy, ...override.AuthorizationProof) error {
		commits++
		return nil
	}
	recordLaunchSpawn = func() error {
		recordedSpawns++
		return nil
	}
	err = Run(arguments[0], arguments[1:])
	if err == nil || !strings.Contains(err.Error(), "resolve claude executable") {
		t.Fatalf("missing direct Claude binary error = %v", err)
	}
	if commits != 0 || recordedSpawns != 0 {
		t.Fatalf("missing Claude binary committed %d transactions and recorded %d spawns", commits, recordedSpawns)
	}
}

func TestPreparedClaudeDirectInvocationRejectsRequestSubstitution(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalCommitLaunchOverrideProofs := commitLaunchOverrideProofs
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		commitLaunchOverrideProofs = originalCommitLaunchOverrideProofs
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		Binary:      "/opt/claude/bin/claude",
		SessionName: "supervisor-bound",
		Model:       "claude-test",
		ExtraArgs:   []string{"--verbose"},
		Persistent:  true,
	}, nil)
	if err != nil {
		t.Fatalf("prepare direct Claude command: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if err := prepared.BindOverrideReservations(true); err != nil {
		t.Fatalf("bind direct Claude launch effects: %v", err)
	}
	_, arguments, err := prepared.DirectInvocation()
	if err != nil {
		t.Fatalf("direct Claude invocation: %v", err)
	}
	for index := range arguments {
		if arguments[index] == "--binary" && index+1 < len(arguments) {
			arguments[index+1] = "/opt/unreviewed/bin/claude"
			break
		}
	}
	lookPathInEnvironment = func(string, []string) (string, error) {
		t.Fatal("substituted Claude request reached executable resolution")
		return "", nil
	}
	commitLaunchOverrideProofs = func(string, diskAdmissionPolicy, ...override.AuthorizationProof) error {
		t.Fatal("substituted Claude request committed launch effects")
		return nil
	}
	err = Run(arguments[0], arguments[1:])
	if err == nil || !strings.Contains(err.Error(), "does not authorize the requested launch") {
		t.Fatalf("substituted direct Claude request error = %v", err)
	}
}

func TestPreparedCodexCommandCarriesCallerAllowlistAndPreservesPaneRuntime(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "stale-pane-openai")
	t.Setenv("CODEX_ACCESS_TOKEN", "stale-pane-codex")
	t.Setenv("TMUX", "live-pane-tmux")
	t.Setenv("TMUX_PANE", "%9")
	t.Setenv("TERM", "tmux-256color")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM_PROGRAM", "tmux")
	t.Setenv("TERM_PROGRAM_VERSION", "3.6a")

	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "handoff-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{
		"PATH=/caller/bin", "HOME=/caller/home", "PWD=/stale/caller/work",
		"TMUX=stale-caller-tmux", "TMUX_PANE=%1",
		"TERM=dumb", "COLORTERM=stale-caller-color",
		"TERM_PROGRAM=CodexDesktop", "TERM_PROGRAM_VERSION=0.0",
		"OPENAI_API_KEY=caller-openai", "CODEX_ACCESS_TOKEN=caller-codex",
		CodexWorkerWriteRootsEnv + `=["/caller/worktree","/caller/satellite.git"]`,
		"ANTHROPIC_API_KEY=rejected-anthropic",
	})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	for _, secret := range []string{"caller-openai", "caller-codex", "rejected-anthropic"} {
		if strings.Contains(prepared.Command, secret) {
			t.Fatalf("prepared command exposed %q: %s", secret, prepared.Command)
		}
	}
	assertPrivateHandoffMode(t, prepared.path)

	var childEnvironment []string
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/codex", nil }
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "handoff-codex", "--model", "gpt-test", "--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Codex command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for name, want := range map[string]string{
		"OPENAI_API_KEY":         "caller-openai",
		"CODEX_ACCESS_TOKEN":     "caller-codex",
		"TMUX":                   "live-pane-tmux",
		"TMUX_PANE":              "%9",
		"TERM":                   "tmux-256color",
		"COLORTERM":              "truecolor",
		"TERM_PROGRAM":           "tmux",
		"TERM_PROGRAM_VERSION":   "3.6a",
		"PWD":                    "/tmp/work",
		CodexWorkerWriteRootsEnv: `["/caller/worktree","/caller/satellite.git"]`,
	} {
		if got := values[name]; got != want {
			t.Errorf("Codex child %s = %q, want %q", name, got, want)
		}
	}
	if _, ok := values["ANTHROPIC_API_KEY"]; ok {
		t.Fatal("Codex child inherited rejected Anthropic credential")
	}
	if got := values["CODEX_ACCESS_TOKEN"]; got != "caller-codex" {
		t.Fatalf("Codex child CODEX_ACCESS_TOKEN = %q, want caller snapshot", got)
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("consumed Codex handoff still exists: %v", err)
	}
}

func TestCodexHookBypassRequiresTrustedBoundHandoff(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalCodexHookOverrides := codexHookOverrides
	originalVerifyCodexHookTrustAttestation := verifyCodexHookTrustAttestation
	originalCommitLaunchOverrideProofs := commitLaunchOverrideProofs
	originalRecordLaunchSpawn := recordLaunchSpawn
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
		codexHookOverrides = originalCodexHookOverrides
		verifyCodexHookTrustAttestation = originalVerifyCodexHookTrustAttestation
		commitLaunchOverrideProofs = originalCommitLaunchOverrideProofs
		recordLaunchSpawn = originalRecordLaunchSpawn
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	hookConfigurationPrepared := false
	executableResolved := false
	lookPathInEnvironment = func(string, []string) (string, error) {
		executableResolved = true
		return "/fixed/codex", nil
	}
	codexHookOverrides = func(root, digest, workDir string) ([]string, error) {
		hookConfigurationPrepared = true
		if digest != testCodexHookDigest {
			t.Fatalf("hook digest = %q, want %q", digest, testCodexHookDigest)
		}
		return []string{
			`projects={"` + workDir + `"={trust_level="untrusted"}}`,
			`hooks={"PreToolUse":[]}`,
		}, nil
	}

	const hookRoot = "/trusted/hooks/0123456789abcdef"
	const hookTrustReason = "sandbox path rotates per spawn so hooks cannot be pre-trusted"
	const hookTrustActor = "dispatcher-test"
	brakeProof := override.AuthorizationProof{
		Kind:            override.KindAdmissionBrake,
		Reason:          "operator verified host recovery before this launch",
		Actor:           "dispatcher-test",
		Session:         "bypass-codex",
		AuthorizationID: "fedcba9876543210fedcba9876543210",
	}
	hookProof := testCodexHookProof("bypass-codex", hookTrustReason, hookTrustActor)
	verifyCodexHookTrustAttestation = func(attestation codexhooks.Attestation, workDir string) error {
		if attestation.SourceRepo != testCodexHookSourceRepo ||
			attestation.SourceCommit != testCodexHookSourceCommit ||
			attestation.Digest != testCodexHookDigest ||
			attestation.HookRoot != hookRoot ||
			workDir != "/tmp/work" {
			t.Fatalf("hook attestation = %#v, workdir %q", attestation, workDir)
		}
		return nil
	}
	authorizedUses := 0
	recordedSpawns := 0
	commitLaunchOverrideProofs = func(sessionName string, _ diskAdmissionPolicy, proofs ...override.AuthorizationProof) error {
		if !hookConfigurationPrepared || !executableResolved {
			t.Fatal("hook-trust use recorded before launch preparation completed")
		}
		if sessionName != "bypass-codex" {
			t.Fatalf("override session = %q, want bypass-codex", sessionName)
		}
		want := []override.AuthorizationProof{brakeProof, hookProof}
		if !slices.Equal(proofs, want) {
			t.Fatalf("committed override proofs = %+v, want complete transaction %+v", proofs, want)
		}
		authorizedUses++
		return nil
	}
	recordLaunchSpawn = func() error {
		if authorizedUses != 1 {
			t.Fatalf("spawn recorded after %d authorization calls, want 1", authorizedUses)
		}
		recordedSpawns++
		return nil
	}
	bindProof := func(prepared PreparedCommand, proofs ...override.AuthorizationProof) {
		t.Helper()
		if err := bindHandoffOverrideProofs(
			prepared.path, CodexProtocol, true, true, proofs,
		); err != nil {
			t.Fatalf("bind override proof into trusted Codex handoff: %v", err)
		}
	}
	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName:           "bypass-codex",
		Model:                 "gpt-test",
		WorkDir:               "/tmp/work",
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("bypass-codex", hookTrustReason, hookTrustActor),
	}, nil)
	if err != nil {
		t.Fatalf("prepare trusted Codex bypass: %v", err)
	}
	trustedRoot, err := trustedHandoffRoot()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(prepared.path) != trustedRoot {
		t.Fatalf("bypass handoff directory = %q, want %q", filepath.Dir(prepared.path), trustedRoot)
	}
	misboundBrakeProof := brakeProof
	misboundBrakeProof.Session = "different-worker"
	if err := bindHandoffOverrideProofs(
		prepared.path,
		CodexProtocol,
		true,
		true,
		[]override.AuthorizationProof{misboundBrakeProof, hookProof},
	); err == nil || !strings.Contains(err.Error(), "unrelated admission-brake proof") {
		t.Fatalf("misbound admission-brake proof error = %v", err)
	}
	bindProof(prepared, brakeProof, hookProof)

	var childArguments, childEnvironment []string
	replaceProcess = func(_ string, argv, env []string) error {
		if authorizedUses != 1 {
			t.Fatalf("Codex replacement began after %d authorization calls, want 1", authorizedUses)
		}
		if recordedSpawns != 1 {
			t.Fatalf("Codex replacement began after %d spawn records, want 1", recordedSpawns)
		}
		childArguments = append([]string(nil), argv...)
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "bypass-codex",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Codex bypass: %v", err)
	}
	if got := environmentMap(childEnvironment)["AGM_CODEX_HOOK_ROOT"]; got != hookRoot {
		t.Fatalf("Codex hook root environment = %q, want %q", got, hookRoot)
	}
	if !slices.Contains(childArguments, `hooks={"PreToolUse":[]}`) {
		t.Fatalf("Codex argv does not carry immutable hook configuration: %q", childArguments)
	}

	bound, err := PrepareCodexCommand(CodexLaunch{
		SessionName:           "bound-bypass",
		Model:                 "gpt-test",
		WorkDir:               "/tmp/work",
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("bound-bypass", hookTrustReason, hookTrustActor),
	}, nil)
	if err != nil {
		t.Fatalf("prepare bound Codex bypass: %v", err)
	}
	bindProof(bound, testCodexHookProof("bound-bypass", hookTrustReason, hookTrustActor))
	err = Run(CodexProtocol, []string{
		"--handoff", bound.path,
		"--session", "bound-bypass",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", "/trusted/hooks/different",
	})
	if err == nil || !strings.Contains(err.Error(), "does not authorize") {
		t.Fatalf("mismatched Codex hook root error = %v", err)
	}

	bound, err = PrepareCodexCommand(CodexLaunch{
		SessionName:           "bound-bypass",
		Model:                 "gpt-test",
		WorkDir:               "/tmp/work",
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("bound-bypass", hookTrustReason, hookTrustActor),
	}, nil)
	if err != nil {
		t.Fatalf("prepare launch-bound Codex bypass: %v", err)
	}
	bindProof(bound, testCodexHookProof("bound-bypass", hookTrustReason, hookTrustActor))
	err = Run(CodexProtocol, []string{
		"--handoff", bound.path,
		"--session", "bound-bypass",
		"--model", "gpt-test",
		"--workdir", "/tmp",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "does not authorize the requested Codex launch") {
		t.Fatalf("mismatched Codex launch error = %v", err)
	}

	bound, err = PrepareCodexCommand(CodexLaunch{
		SessionName:           "bound-cold-remote-resume",
		Model:                 "gpt-test",
		WorkDir:               "/tmp/work",
		Sandbox:               "workspace-write",
		ResumeID:              "thread-123",
		Remote:                true,
		RemoteResume:          true,
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("bound-cold-remote-resume", hookTrustReason, hookTrustActor),
	}, nil)
	if err != nil {
		t.Fatalf("prepare cold remote resume Codex bypass: %v", err)
	}
	bindProof(bound, testCodexHookProof("bound-cold-remote-resume", hookTrustReason, hookTrustActor))
	err = Run(CodexProtocol, []string{
		"--handoff", bound.path,
		"--session", "bound-cold-remote-resume",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--resume-id", "thread-123",
		"--remote",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "does not authorize the requested Codex launch") {
		t.Fatalf("cold remote resume marker mutation error = %v", err)
	}

	err = Run(CodexProtocol, []string{
		"--session", "forged-bypass",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "requires a prepared private handoff") {
		t.Fatalf("direct Codex bypass error = %v, want prepared-handoff refusal", err)
	}

	ordinary, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "ordinary-handoff",
		Model:       "gpt-test",
		WorkDir:     "/tmp/work",
		Sandbox:     "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare ordinary Codex handoff: %v", err)
	}
	err = Run(CodexProtocol, []string{
		"--handoff", ordinary.path,
		"--session", "ordinary-handoff",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "outside the expected staging directory") {
		t.Fatalf("ordinary handoff bypass error = %v", err)
	}
	if err := ordinary.Cancel(); err != nil {
		t.Fatalf("cancel rejected ordinary handoff: %v", err)
	}

	configFailure, err := PrepareCodexCommand(CodexLaunch{
		SessionName:           "config-failure",
		Model:                 "gpt-test",
		WorkDir:               "/tmp/work",
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("config-failure", hookTrustReason, hookTrustActor),
	}, nil)
	if err != nil {
		t.Fatalf("prepare config-failure Codex bypass: %v", err)
	}
	bindProof(configFailure, testCodexHookProof("config-failure", hookTrustReason, hookTrustActor))
	codexHookOverrides = func(string, string, string) ([]string, error) {
		return nil, errors.New("configuration failed")
	}
	err = Run(CodexProtocol, []string{
		"--handoff", configFailure.path,
		"--session", "config-failure",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "prepare immutable Codex hook configuration") {
		t.Fatalf("hook configuration failure = %v", err)
	}

	resolveFailure, err := PrepareCodexCommand(CodexLaunch{
		SessionName:           "resolve-failure",
		Model:                 "gpt-test",
		WorkDir:               "/tmp/work",
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("resolve-failure", hookTrustReason, hookTrustActor),
	}, nil)
	if err != nil {
		t.Fatalf("prepare resolve-failure Codex bypass: %v", err)
	}
	bindProof(resolveFailure, testCodexHookProof("resolve-failure", hookTrustReason, hookTrustActor))
	codexHookOverrides = func(string, string, string) ([]string, error) { return nil, nil }
	lookPathInEnvironment = func(string, []string) (string, error) {
		return "", errors.New("codex missing")
	}
	err = Run(CodexProtocol, []string{
		"--handoff", resolveFailure.path,
		"--session", "resolve-failure",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "resolve codex executable") {
		t.Fatalf("Codex resolution failure = %v", err)
	}

	if _, err := PrepareCodexCommand(CodexLaunch{
		SessionName:           "overlapping-bypass",
		Model:                 "gpt-test",
		WorkDir:               filepath.Dir(trustedRoot),
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("overlapping-bypass", hookTrustReason, hookTrustActor),
	}, nil); err == nil || !strings.Contains(err.Error(), "overlaps agent-writable root") {
		t.Fatalf("overlapping trusted handoff error = %v", err)
	}

	symlinkedRoot := filepath.Join(t.TempDir(), "trusted-via-symlink")
	if err := os.Symlink(trustedRoot, symlinkedRoot); err != nil {
		t.Fatalf("symlink trusted handoff root: %v", err)
	}
	if _, err := PrepareCodexCommand(CodexLaunch{
		SessionName:           "symlink-overlapping-bypass",
		Model:                 "gpt-test",
		WorkDir:               symlinkedRoot,
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("symlink-overlapping-bypass", hookTrustReason, hookTrustActor),
	}, nil); err == nil || !strings.Contains(err.Error(), "overlaps agent-writable root") {
		t.Fatalf("symlink-overlapping trusted handoff error = %v", err)
	}

	attestationFailure, err := PrepareCodexCommand(CodexLaunch{
		SessionName:           "attestation-failure",
		Model:                 "gpt-test",
		WorkDir:               "/tmp/work",
		Sandbox:               "workspace-write",
		BypassHookTrust:       true,
		HookRoot:              hookRoot,
		HookTrustReason:       hookTrustReason,
		HookTrustActor:        hookTrustActor,
		HookTrustSubject:      testCodexHookSubject,
		HookTrustSourceRepo:   testCodexHookSourceRepo,
		HookTrustSourceCommit: testCodexHookSourceCommit,
		HookTrustDigest:       testCodexHookDigest,
		HookTrustProof:        testCodexHookProof("attestation-failure", hookTrustReason, hookTrustActor),
	}, nil)
	if err != nil {
		t.Fatalf("prepare attestation-failure Codex bypass: %v", err)
	}
	bindProof(attestationFailure, testCodexHookProof("attestation-failure", hookTrustReason, hookTrustActor))
	verifyCodexHookTrustAttestation = func(codexhooks.Attestation, string) error {
		return errors.New("persisted Git attestation changed")
	}
	err = Run(CodexProtocol, []string{
		"--handoff", attestationFailure.path,
		"--session", "attestation-failure",
		"--model", "gpt-test",
		"--workdir", "/tmp/work",
		"--sandbox", "workspace-write",
		"--bypass-hook-trust",
		"--hook-root", hookRoot,
	})
	if err == nil || !strings.Contains(err.Error(), "revalidate approved Codex hook source") {
		t.Fatalf("changed attestation error = %v", err)
	}
	if authorizedUses != 1 {
		t.Fatalf("failed or ordinary launches recorded %d hook-trust uses, want only the real launch", authorizedUses)
	}
}

func TestPreparedCodexCommandResolvesExecutableFromCallerPATH(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	staleBin := t.TempDir()
	callerBin := t.TempDir()
	for _, path := range []string{
		filepath.Join(staleBin, "codex"),
		filepath.Join(callerBin, "codex"),
	} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
			t.Fatalf("write fake Codex executable: %v", err)
		}
	}
	t.Setenv("PATH", staleBin)

	originalExecutablePath := executablePath
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "caller-path", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{"PATH=" + callerBin, "HOME=/caller/home"})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	var gotPath string
	var gotEnvironment []string
	replaceProcess = func(path string, _ []string, environment []string) error {
		gotPath = path
		gotEnvironment = append([]string(nil), environment...)
		return nil
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "caller-path", "--model", "gpt-test",
		"--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Codex command: %v", err)
	}
	if want := filepath.Join(callerBin, "codex"); gotPath != want {
		t.Fatalf("Codex executable = %q, want caller PATH executable %q (stale pane PATH was %q)", gotPath, want, staleBin)
	}
	if got := environmentMap(gotEnvironment)["PATH"]; got != callerBin {
		t.Fatalf("Codex child PATH = %q, want caller PATH %q", got, callerBin)
	}
}

func TestPreparedClaudeCommandClearsCallerAbsentPaneState(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "stale-pane-oauth")
	t.Setenv("ANTHROPIC_API_KEY", "stale-pane-api")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "stale-pane-endpoint")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "stale-pane-headers")

	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	originalResolveClaudeOAuth := resolveClaudeOAuth
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
		resolveClaudeOAuth = originalResolveClaudeOAuth
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	resolveClaudeOAuth = func() string { return "" }
	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		SessionName: "clear-stale", DisableOAuth: true,
	}, nil)
	if err != nil {
		t.Fatalf("prepare Claude command: %v", err)
	}
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/claude", nil }
	var childEnvironment []string
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	err = Run(ClaudeProtocol, []string{
		"--handoff", prepared.path, "--session", "clear-stale", "--disable-oauth",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Claude command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for _, name := range []string{
		"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS",
	} {
		if value, ok := values[name]; ok {
			t.Errorf("Claude child retained caller-absent %s=%q from stale pane", name, value)
		}
	}
}

func TestPreparedCodexCommandClearsCallerAbsentPaneCredentials(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "stale-pane-openai")
	t.Setenv("CODEX_ACCESS_TOKEN", "stale-pane-codex")
	t.Setenv(CodexWorkerWriteRootsEnv, `["/stale/pane"]`)

	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	originalReplaceProcess := replaceProcess
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
		replaceProcess = originalReplaceProcess
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "clear-stale", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	lookPathInEnvironment = func(string, []string) (string, error) { return "/fixed/codex", nil }
	var childEnvironment []string
	replaceProcess = func(_ string, _ []string, env []string) error {
		childEnvironment = append([]string(nil), env...)
		return nil
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "clear-stale", "--model", "gpt-test", "--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "returned unexpectedly") {
		t.Fatalf("run prepared Codex command: %v", err)
	}
	values := environmentMap(childEnvironment)
	for _, name := range []string{"OPENAI_API_KEY", "CODEX_ACCESS_TOKEN", CodexWorkerWriteRootsEnv} {
		if value, ok := values[name]; ok {
			t.Errorf("Codex child retained caller-absent %s=%q from stale pane", name, value)
		}
	}
}

func TestPrepareCommandsRejectTerminalControlsBeforeStagingHandoff(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "must-not-exist")
	t.Setenv("AGM_STATE_DIR", stateDir)

	tests := []struct {
		name    string
		prepare func() (PreparedCommand, error)
	}{
		{
			name: "Codex newline in workdir",
			prepare: func() (PreparedCommand, error) {
				return PrepareCodexCommand(CodexLaunch{
					SessionName: "codex", Model: "gpt-test", WorkDir: "/tmp/safe\ninjected",
					Sandbox: "workspace-write",
				}, nil)
			},
		},
		{
			name: "Claude bracketed paste escape in add-dir",
			prepare: func() (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "claude", WorkDir: "/tmp/work",
					AddDirs: []string{"/tmp/safe\x1b[201~\n"},
				}, nil)
			},
		},
		{
			name: "Claude invalid UTF-8 resume ID",
			prepare: func() (PreparedCommand, error) {
				return PrepareClaudeCommand(ClaudeLaunch{
					SessionName: "claude", ResumeID: string([]byte{'i', 'd', 0xff}),
				}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared, err := tt.prepare()
			if err == nil {
				_ = prepared.Cancel()
				t.Fatal("Prepare command succeeded, want terminal-control rejection")
			}
			if prepared.Command != "" {
				t.Fatalf("prepared pane command %q before validation", prepared.Command)
			}
			if !strings.Contains(err.Error(), "invalid harness launch request") {
				t.Fatalf("Prepare error = %v, want launch validation", err)
			}
		})
	}

	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("private handoff state created before validation: %v", err)
	}
}

func TestPrepareCodexRejectsGeneratedTerminalControlsBeforeBuildingCommand(t *testing.T) {
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	launch := CodexLaunch{
		SessionName: "codex", Model: "gpt-test", WorkDir: "/tmp/work",
		Sandbox: "workspace-write",
	}

	t.Run("resolved executable", func(t *testing.T) {
		stateDir := filepath.Join(t.TempDir(), "must-not-exist")
		t.Setenv("AGM_STATE_DIR", stateDir)
		executablePath = func() (string, error) {
			return "/opt/agm/bin/agm\x1b[201~\x15/quit", nil
		}

		prepared, err := PrepareCodexCommand(launch, nil)
		if err == nil {
			_ = prepared.Cancel()
			t.Fatal("Prepare command accepted a generated executable with terminal controls")
		}
		if prepared.Command != "" || !strings.Contains(err.Error(), "private executable contains control characters") {
			t.Fatalf("Prepare result = %#v, %v; want generated executable validation", prepared, err)
		}
		if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
			t.Fatalf("handoff state created before executable validation: %v", statErr)
		}
	})

	t.Run("resolved handoff root", func(t *testing.T) {
		stateDir := filepath.Join(t.TempDir(), "state\x1b[201~\x15/quit")
		t.Setenv("AGM_STATE_DIR", stateDir)
		executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

		prepared, err := PrepareCodexCommand(launch, nil)
		if err == nil {
			_ = prepared.Cancel()
			t.Fatal("Prepare command accepted a generated handoff path with terminal controls")
		}
		if prepared.Command != "" || !strings.Contains(err.Error(), "private handoff directory contains control characters") {
			t.Fatalf("Prepare result = %#v, %v; want generated handoff validation", prepared, err)
		}
		if _, statErr := os.Stat(stateDir); !os.IsNotExist(statErr) {
			t.Fatalf("unsafe handoff root created before path validation: %v", statErr)
		}
	})
}

func TestPreparedCommandCancelRemovesUndeliveredHandoff(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "cancel-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{"OPENAI_API_KEY=cancel-canary"})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	if err := prepared.Cancel(); err != nil {
		t.Fatalf("cancel handoff: %v", err)
	}
	if err := prepared.Cancel(); err != nil {
		t.Fatalf("cancel handoff twice: %v", err)
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("cancelled handoff still exists: %v", err)
	}
}

func TestPreparedCommandSchedulesIndependentExpiration(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalScheduler := scheduleHandoffExpiry
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduler
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	var gotExecutable, gotPath string
	var gotDeadline time.Time
	var gotDeferred bool
	scheduleHandoffExpiry = func(executable, path string, deadline time.Time, deferred bool) (io.Closer, error) {
		gotExecutable, gotPath, gotDeadline = executable, path, deadline
		gotDeferred = deferred
		return nil, nil
	}

	startedAt := time.Now()
	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "expiring-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, []string{"OPENAI_API_KEY=expiry-canary"})
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if gotExecutable != "/opt/agm/bin/agm" || gotPath != prepared.path {
		t.Fatalf("expiration scheduled for executable=%q path=%q, want current AGM and %q", gotExecutable, gotPath, prepared.path)
	}
	if gotDeadline.Before(startedAt.Add(handoffMaxAge)) || gotDeadline.After(time.Now().Add(handoffMaxAge)) {
		t.Fatalf("expiration deadline %s is not one bounded lifetime from preparation", gotDeadline)
	}
	if gotDeferred {
		t.Fatal("ordinary detached launch unexpectedly requested a producer liveness lease")
	}
}

func TestPreparedDeferredCommandSchedulesProducerLease(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalScheduler := scheduleHandoffExpiry
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduler
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	lease := &recordingCloser{}
	var gotDeferred bool
	scheduleHandoffExpiry = func(_ string, _ string, _ time.Time, deferred bool) (io.Closer, error) {
		gotDeferred = deferred
		return lease, nil
	}

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "deferred-codex", Model: "gpt-test", WorkDir: "/tmp/work",
		Sandbox: "workspace-write", DeferUntilProducerExit: true,
	}, []string{"OPENAI_API_KEY=deferred-canary"})
	if err != nil {
		t.Fatalf("prepare deferred Codex command: %v", err)
	}
	if !gotDeferred {
		t.Fatal("deferred launch did not request a producer liveness lease")
	}
	payload, err := os.ReadFile(prepared.path)
	if err != nil {
		t.Fatalf("read deferred handoff: %v", err)
	}
	var handoff launchHandoff
	if err := json.Unmarshal(payload, &handoff); err != nil {
		t.Fatalf("decode deferred handoff: %v", err)
	}
	if !handoff.DeferredUntilProducerExit {
		t.Fatal("deferred handoff omitted its producer-liveness marker")
	}
	if err := prepared.Cancel(); err != nil {
		t.Fatalf("cancel deferred command: %v", err)
	}
	if !lease.closed {
		t.Fatal("cancelling a deferred command did not release its producer lease")
	}
}

func TestPreparedCommandRemovesHandoffWhenExpirationCannotBeScheduled(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	originalExecutablePath := executablePath
	originalScheduler := scheduleHandoffExpiry
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		scheduleHandoffExpiry = originalScheduler
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	scheduleHandoffExpiry = func(string, string, time.Time, bool) (io.Closer, error) {
		return nil, errors.New("scheduler unavailable")
	}

	_, err := PrepareClaudeCommand(ClaudeLaunch{SessionName: "expiry-failure"}, nil)
	if err == nil || !strings.Contains(err.Error(), "schedule private launch handoff expiration") {
		t.Fatalf("prepare error = %v, want expiration scheduling failure", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(stateDir, "private-launch"))
	if readErr != nil {
		t.Fatalf("read private launch directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unscheduled private handoff remained on disk: %v", entries)
	}
}

func TestDeferredHandoffRemainsLiveUntilProducerExitThenExpires(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, []string{"OPENAI_API_KEY=deferred-expiry-canary"}, true, nil, "", "")
	if err != nil {
		t.Fatalf("stage deferred handoff: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create producer lease: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	oldModTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, oldModTime, oldModTime); err != nil {
		t.Fatalf("age deferred handoff before lease refresh: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- expireDeferredHandoff(path, reader, 125*time.Millisecond, 25*time.Millisecond)
	}()

	initial, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat initial deferred handoff: %v", err)
	}
	time.Sleep(175 * time.Millisecond)
	live, err := os.Stat(path)
	if err != nil {
		t.Fatalf("deferred handoff expired while producer was live: %v", err)
	}
	if !live.ModTime().After(initial.ModTime()) {
		t.Fatalf("deferred handoff lease did not refresh mtime: initial=%s live=%s", initial.ModTime(), live.ModTime())
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("release producer lease: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("deferred handoff vanished without its bounded post-exit lifetime: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expire deferred handoff: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deferred handoff did not expire after producer exit")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("post-exit deferred handoff still exists: %v", err)
	}
}

func TestExpiryProtocolRemovesUnconsumedHandoffAtDeadline(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, []string{"OPENAI_API_KEY=expiry-canary"}, false, nil, "", "")
	if err != nil {
		t.Fatalf("stage handoff: %v", err)
	}
	err = Run(ExpiryProtocol, []string{
		"--handoff", path,
		"--expires-at", time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("run expiration protocol: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired private handoff still exists: %v", err)
	}
}

func TestDetachedExpiryHelperIsReapedAsynchronously(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, nil, false, nil, "", "")
	if err != nil {
		t.Fatalf("stage handoff: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	originalReaper := reapExpiryProcess
	t.Cleanup(func() { reapExpiryProcess = originalReaper })
	reaped := make(chan error, 1)
	reapExpiryProcess = func(cmd *exec.Cmd) {
		go func() { reaped <- cmd.Wait() }()
	}

	if _, err := startHandoffExpiry(os.Args[0], path, time.Now().Add(50*time.Millisecond), false); err != nil {
		t.Fatalf("start asynchronously reaped helper: %v", err)
	}
	select {
	case err := <-reaped:
		if err != nil {
			t.Fatalf("expiration helper exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expiration helper was not asynchronously reaped")
	}
}

func TestDetachedExpiryHelperInterceptsGoTestBinaryBeforeTestsRun(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	path, err := stageHandoff(CodexProtocol, []string{"OPENAI_API_KEY=detached-expiry-canary"}, false, nil, "", "")
	if err != nil {
		t.Fatalf("stage handoff: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	if _, err := startHandoffExpiry(os.Args[0], path, time.Now().Add(250*time.Millisecond), false); err != nil {
		t.Fatalf("start detached expiration helper from Go test binary: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, statErr := os.Stat(path)
		if os.IsNotExist(statErr) {
			return
		}
		if statErr != nil {
			t.Fatalf("inspect detached handoff: %v", statErr)
		}
		if time.Now().After(deadline) {
			t.Fatal("detached expiration helper re-entered tests or failed to remove the handoff")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestConsumeHandoffUsesDeferredLeaseFreshnessAndUnlinksRejections(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	for name, deferred := range map[string]bool{
		"ordinary": false,
		"deferred": true,
	} {
		t.Run(name, func(t *testing.T) {
			path, err := stageHandoff(AgyProtocol, nil, deferred, nil, "", "")
			if err != nil {
				t.Fatalf("stage handoff: %v", err)
			}
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read handoff: %v", err)
			}
			var handoff launchHandoff
			if err := json.Unmarshal(payload, &handoff); err != nil {
				t.Fatalf("decode handoff: %v", err)
			}
			handoff.CreatedAt = time.Now().Add(-2 * handoffMaxAge).UTC().Format(time.RFC3339Nano)
			payload, err = json.Marshal(handoff)
			if err != nil {
				t.Fatalf("encode handoff: %v", err)
			}
			if err := os.WriteFile(path, append(payload, '\n'), 0600); err != nil {
				t.Fatalf("rewrite handoff timestamp: %v", err)
			}

			_, consumeErr := consumeHandoff(path, AgyProtocol, "")
			if deferred && consumeErr != nil {
				t.Fatalf("deferred handoff rejected recent producer lease freshness: %v", consumeErr)
			}
			if !deferred && consumeErr == nil {
				t.Fatal("ordinary handoff accepted expired creation time")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("consumed or rejected one-shot handoff still exists: %v", err)
			}
		})
	}
}

func TestPreparedCommandUsesCoInstalledAGMFromCompanionBinary(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	binDir := t.TempDir()
	agmPath := filepath.Join(binDir, "agm")
	if err := os.WriteFile(agmPath, []byte("test executable"), 0700); err != nil {
		t.Fatalf("write co-installed AGM: %v", err)
	}
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return filepath.Join(binDir, "agm-mcp-server"), nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "companion-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command from companion: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !strings.HasPrefix(prepared.Command, shellquote.Quote(agmPath)+" "+CodexProtocol) {
		t.Fatalf("companion command did not pin co-installed AGM: %s", prepared.Command)
	}
}

func TestPreparedCommandUsesMatchingVersionedAGMFromReleaseCompanion(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	binDir := t.TempDir()
	suffix := "-" + runtime.GOOS + "-" + runtime.GOARCH
	agmPath := filepath.Join(binDir, "agm"+suffix)
	if err := os.WriteFile(agmPath, []byte("test executable"), 0700); err != nil {
		t.Fatalf("write versioned co-installed AGM: %v", err)
	}
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) {
		return filepath.Join(binDir, "agm-mcp-server"+suffix), nil
	}

	prepared, err := PrepareClaudeCommand(ClaudeLaunch{
		SessionName: "release-companion-claude", WorkDir: "/tmp/work",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Claude command from versioned release companion: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !strings.HasPrefix(prepared.Command, shellquote.Quote(agmPath)+" "+ClaudeProtocol) {
		t.Fatalf("versioned companion command did not pin matching AGM artifact: %s", prepared.Command)
	}
}

func TestPreparedCommandRejectsCompanionWithoutCoInstalledAGM(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	binDir := t.TempDir()
	pathDir := t.TempDir()
	pathAGM := filepath.Join(pathDir, "agm")
	if err := os.WriteFile(pathAGM, []byte("stale executable"), 0700); err != nil {
		t.Fatalf("write PATH AGM: %v", err)
	}
	t.Setenv("PATH", pathDir)
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return filepath.Join(binDir, "agm-mcp-server"), nil }

	_, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "missing-companion-agm", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err == nil {
		t.Fatal("companion without a co-installed AGM used an unverified PATH fallback")
	}
	if !strings.Contains(err.Error(), "find co-installed AGM private executor") {
		t.Fatalf("unexpected companion resolution error: %v", err)
	}
}

func TestPreparedCommandUsesRenamedCurrentAGMExecutable(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm-v2026.07", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "renamed-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command from renamed AGM: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !strings.HasPrefix(prepared.Command, "'/opt/agm/bin/agm-v2026.07' "+CodexProtocol) {
		t.Fatalf("renamed AGM command did not pin current executable: %s", prepared.Command)
	}
}

func TestPreparedCommandMakesRelativeStateDirectoryAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AGM_STATE_DIR", "relative-state")
	originalExecutablePath := executablePath
	t.Cleanup(func() { executablePath = originalExecutablePath })
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "absolute-handoff", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command with relative state directory: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Cancel() })
	if !filepath.IsAbs(prepared.path) {
		t.Fatalf("handoff path = %q, want absolute", prepared.path)
	}
	if !strings.Contains(prepared.Command, "--handoff "+shellquote.Quote(prepared.path)) {
		t.Fatalf("prepared command omitted absolute handoff path: %s", prepared.Command)
	}
}

func TestExecutorConsumesHandoffBeforeHarnessLookup(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	originalExecutablePath := executablePath
	originalLookPathInEnvironment := lookPathInEnvironment
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		lookPathInEnvironment = originalLookPathInEnvironment
	})
	executablePath = func() (string, error) { return "/opt/agm/bin/agm", nil }
	lookPathInEnvironment = func(string, []string) (string, error) { return "", os.ErrNotExist }

	prepared, err := PrepareCodexCommand(CodexLaunch{
		SessionName: "missing-codex", Model: "gpt-test", WorkDir: "/tmp/work", Sandbox: "workspace-write",
	}, nil)
	if err != nil {
		t.Fatalf("prepare Codex command: %v", err)
	}
	err = Run(CodexProtocol, []string{
		"--handoff", prepared.path,
		"--session", "missing-codex", "--model", "gpt-test", "--workdir", "/tmp/work", "--sandbox", "workspace-write",
	})
	if err == nil || !strings.Contains(err.Error(), "resolve codex executable") {
		t.Fatalf("run with missing Codex executable: %v", err)
	}
	if _, err := os.Stat(prepared.path); !os.IsNotExist(err) {
		t.Fatalf("handoff survived failed harness lookup: %v", err)
	}
}

func TestConsumeHandoffRejectsCrossHarnessAndPublicState(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())

	if _, err := stageHandoff(CodexProtocol, []string{"ANTHROPIC_API_KEY=must-not-cross"}, false, nil, "", ""); err == nil {
		t.Fatal("Codex staging accepted an Anthropic credential")
	}

	wrongProtocol, err := stageHandoff(ClaudeProtocol, nil, false, nil, "", "")
	if err != nil {
		t.Fatalf("stage wrong-protocol handoff: %v", err)
	}
	if _, err := consumeHandoff(wrongProtocol, CodexProtocol, ""); err == nil {
		t.Fatal("Codex executor accepted a Claude handoff")
	}
	if _, err := os.Stat(wrongProtocol); !os.IsNotExist(err) {
		t.Fatalf("rejected wrong-protocol handoff still exists: %v", err)
	}

	public, err := stageHandoff(CodexProtocol, nil, false, nil, "", "")
	if err != nil {
		t.Fatalf("stage public-mode handoff: %v", err)
	}
	if err := os.Chmod(public, 0644); err != nil {
		t.Fatalf("make handoff public: %v", err)
	}
	if _, err := consumeHandoff(public, CodexProtocol, ""); err == nil {
		t.Fatal("executor accepted a group/world-readable handoff")
	}
	if err := os.Remove(public); err != nil {
		t.Fatalf("remove rejected public handoff: %v", err)
	}
}

func TestConsumeHandoffPreservesFilesOutsidePrivateStagingNamespace(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	stagingDir := filepath.Join(stateDir, "private-launch")
	if err := os.Mkdir(stagingDir, 0700); err != nil {
		t.Fatalf("create staging directory: %v", err)
	}
	tests := map[string]string{
		"unrelated owner-only file":        filepath.Join(t.TempDir(), "id_ed25519"),
		"staging-shaped file outside root": filepath.Join(t.TempDir(), "launch-forged.json"),
		"invalid name inside staging root": filepath.Join(stagingDir, "config.json"),
	}
	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			const content = "owner-only content that must survive"
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatalf("write protected test file: %v", err)
			}
			if _, err := consumeHandoff(path, CodexProtocol, ""); err == nil {
				t.Fatal("executor accepted a file outside the private staging namespace")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("rejected file was removed: %v", err)
			}
			if string(got) != content {
				t.Fatalf("rejected file content changed: %q", got)
			}
		})
	}
}

func TestConsumeHandoffRejectsTrailingAndOversizedContent(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())

	for name, suffix := range map[string]string{
		"trailing JSON": "{}",
		"oversized":     strings.Repeat(" ", handoffMaxSize),
	} {
		t.Run(name, func(t *testing.T) {
			path, err := stageHandoff(CodexProtocol, nil, false, nil, "", "")
			if err != nil {
				t.Fatalf("stage handoff: %v", err)
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatalf("open handoff for corruption: %v", err)
			}
			if _, err := file.WriteString(suffix); err != nil {
				_ = file.Close()
				t.Fatalf("corrupt handoff: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close corrupted handoff: %v", err)
			}
			if _, err := consumeHandoff(path, CodexProtocol, ""); err == nil {
				t.Fatal("executor accepted a corrupted handoff")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("rejected corrupted handoff still exists: %v", err)
			}
		})
	}
}

func assertPrivateHandoffMode(t *testing.T, path string) {
	t.Helper()
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat handoff: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("handoff mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat handoff directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("handoff directory mode = %o, want 700", got)
	}
}

type recordingCloser struct {
	closed bool
}

func (c *recordingCloser) Close() error {
	c.closed = true
	return nil
}
