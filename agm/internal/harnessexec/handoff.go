package harnessexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	"github.com/vbonnet/dear-agent/pkg/override"
)

const (
	handoffVersion  = 1
	handoffMaxAge   = 10 * time.Minute
	handoffMaxSize  = 64 << 10
	expiryHelperEnv = "AGM_PRIVATE_HANDOFF_EXPIRY_HELPER"
)

var (
	executablePath        = os.Executable
	scheduleHandoffExpiry = startHandoffExpiry
	reapExpiryProcess     = func(cmd *exec.Cmd) {
		go func() { _ = cmd.Wait() }()
	}
)

// PreparedCommand is a token-free pane command plus cleanup for a handoff that
// was never delivered. Once the command executes, the private executor removes
// the handoff before replacing itself with the harness.
type PreparedCommand struct {
	Command       string
	path          string
	lease         io.Closer
	bindOverrides func(...*override.Reservation) error
}

// Cancel removes an undelivered one-shot handoff and releases any producer
// liveness lease. It is safe after consumption.
func (p PreparedCommand) Cancel() error {
	var removeErr error
	if p.path != "" {
		removeErr = os.Remove(p.path)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
	}
	var leaseErr error
	if p.lease != nil {
		leaseErr = p.lease.Close()
	}
	return errors.Join(removeErr, leaseErr)
}

// BindOverrideReservations seals the final launch-admission reservations into
// a staged Codex handoff. The private executor revalidates and commits them as
// one transaction only after every other fallible launch check succeeds.
func (p PreparedCommand) BindOverrideReservations(reservations ...*override.Reservation) error {
	if p.bindOverrides == nil {
		if len(reservations) == 0 {
			return nil
		}
		return errors.New("prepared harness command cannot defer override commitment")
	}
	return p.bindOverrides(reservations...)
}

// ResolveSubmission transfers ownership of a staged private launch across
// tmux's irreversible Enter boundary. An uncertain acknowledgement may mean
// the executor is already queued, so only a confirmed submission failure may
// cancel the one-shot handoff.
func ResolveSubmission(submissionErr error, cancelUndelivered func() error) (bool, error) {
	if submissionErr == nil {
		return false, nil
	}
	if tmux.PromptSubmissionMayHaveOccurred(submissionErr) {
		return true, nil
	}
	if cancelUndelivered == nil {
		return false, submissionErr
	}
	return false, errors.Join(submissionErr, cancelUndelivered())
}

type launchHandoff struct {
	Version                   int                           `json:"version"`
	Protocol                  string                        `json:"protocol"`
	CreatedAt                 string                        `json:"created_at"`
	DeferredUntilProducerExit bool                          `json:"deferred_until_producer_exit,omitempty"`
	Environment               []string                      `json:"environment"`
	OverrideProofs            []override.AuthorizationProof `json:"override_proofs,omitempty"`
	CodexHookRoot             string                        `json:"codex_hook_root,omitempty"`
	CodexLaunch               *codexLaunchBinding           `json:"codex_launch,omitempty"`
}

type codexLaunchBinding struct {
	SessionName           string                      `json:"session_name"`
	Model                 string                      `json:"model"`
	WorkDir               string                      `json:"workdir"`
	Sandbox               string                      `json:"sandbox"`
	Approval              string                      `json:"approval,omitempty"`
	AddDirs               []string                    `json:"add_dirs,omitempty"`
	ResumeID              string                      `json:"resume_id,omitempty"`
	Remote                bool                        `json:"remote,omitempty"`
	HookRoot              string                      `json:"hook_root"`
	HookTrustReason       string                      `json:"hook_trust_reason"`
	HookTrustActor        string                      `json:"hook_trust_actor"`
	HookTrustSourceRepo   string                      `json:"hook_trust_source_repo"`
	HookTrustSourceCommit string                      `json:"hook_trust_source_commit"`
	HookTrustDigest       string                      `json:"hook_trust_digest"`
	HookTrustProof        override.AuthorizationProof `json:"hook_trust_proof"`
}

// PrepareCodexCommand snapshots only Codex's documented allowlist from the
// caller and writes it to an owner-only, one-shot handoff. This keeps caller
// authentication authoritative even when a long-lived tmux server has stale
// environment state.
func PrepareCodexCommand(launch CodexLaunch, parent []string) (PreparedCommand, error) {
	if err := validateCodexPastedValues(launch); err != nil {
		return PreparedCommand{}, err
	}
	executable, err := resolvePrivateExecutable()
	if err != nil {
		return PreparedCommand{}, fmt.Errorf("resolve AGM private executor: %w", err)
	}
	if err := validateText("private executable", executable); err != nil {
		return PreparedCommand{}, fmt.Errorf("validate Codex pane command: %w", err)
	}
	snapshot := removeEnvironment(CodexEnvironment(parent, launch.SessionName), paneRuntimeEnvironment)
	if err := validateTrustedHandoffIsolation(launch); err != nil {
		return PreparedCommand{}, err
	}
	var binding *codexLaunchBinding
	if launch.BypassHookTrust {
		bound := bindCodexLaunch(launch)
		binding = &bound
	}
	handoffPath, err := stageHandoff(
		CodexProtocol, snapshot, launch.DeferUntilProducerExit, launch.HookRoot, binding,
	)
	if err != nil {
		return PreparedCommand{}, err
	}
	if err := validateText("private handoff", handoffPath); err != nil {
		return PreparedCommand{}, cleanupInvalidHandoff(handoffPath, fmt.Errorf("validate Codex pane command: %w", err))
	}
	lease, err := scheduleHandoffExpiry(
		executable, handoffPath, time.Now().Add(handoffMaxAge), launch.DeferUntilProducerExit,
	)
	if err != nil {
		return PreparedCommand{}, cleanupFailedHandoff(handoffPath, err)
	}
	launch.Executable = executable
	launch.HandoffPath = handoffPath
	return PreparedCommand{
		Command: BuildCodexCommand(launch),
		path:    handoffPath,
		lease:   lease,
		bindOverrides: func(reservations ...*override.Reservation) error {
			return bindHandoffOverrideReservations(handoffPath, launch.BypassHookTrust, reservations...)
		},
	}, nil
}

// PrepareClaudeCommand snapshots the caller's selected authentication and
// telemetry configuration into an owner-only, one-shot handoff.
func PrepareClaudeCommand(launch ClaudeLaunch, parent []string) (PreparedCommand, error) {
	if err := validateClaudePastedValues(launch); err != nil {
		return PreparedCommand{}, err
	}
	executable, err := resolvePrivateExecutable()
	if err != nil {
		return PreparedCommand{}, fmt.Errorf("resolve AGM private executor: %w", err)
	}
	if err := validateText("private executable", executable); err != nil {
		return PreparedCommand{}, fmt.Errorf("validate Claude pane command: %w", err)
	}
	values := environmentMap(parent)
	forward := make([]string, 0, 4)
	if !launch.DisableOAuth {
		if token := resolveClaudeOAuth(); token != "" {
			forward = append(forward, auth.OAuthEnvVar+"="+token)
		} else if key := values["ANTHROPIC_API_KEY"]; key != "" {
			forward = append(forward, "ANTHROPIC_API_KEY="+key)
		}
	} else if key := values["ANTHROPIC_API_KEY"]; key != "" {
		forward = append(forward, "ANTHROPIC_API_KEY="+key)
	}
	if launch.ForwardTelemetry {
		for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS"} {
			if value := values[name]; value != "" {
				forward = append(forward, name+"="+value)
			}
		}
	}
	handoffPath, err := stageHandoff(ClaudeProtocol, forward, launch.DeferUntilProducerExit, "")
	if err != nil {
		return PreparedCommand{}, err
	}
	if err := validateText("private handoff", handoffPath); err != nil {
		return PreparedCommand{}, cleanupInvalidHandoff(handoffPath, fmt.Errorf("validate Claude pane command: %w", err))
	}
	lease, err := scheduleHandoffExpiry(
		executable, handoffPath, time.Now().Add(handoffMaxAge), launch.DeferUntilProducerExit,
	)
	if err != nil {
		return PreparedCommand{}, cleanupFailedHandoff(handoffPath, err)
	}
	launch.Executable = executable
	launch.HandoffPath = handoffPath
	return PreparedCommand{Command: BuildClaudeCommand(launch), path: handoffPath, lease: lease}, nil
}

func validateCodexPastedValues(launch CodexLaunch) error {
	for _, field := range []struct{ name, value string }{
		{"session", launch.SessionName},
		{"model", launch.Model},
		{"workdir", launch.WorkDir},
		{"sandbox", launch.Sandbox},
		{"approval", launch.Approval},
		{"resume-id", launch.ResumeID},
		{"hook-root", launch.HookRoot},
	} {
		if err := validateOptionalText(field.name, field.value); err != nil {
			return fmt.Errorf("validate Codex pane command: %w", err)
		}
	}
	if err := validateTextList("add-dir", launch.AddDirs); err != nil {
		return fmt.Errorf("validate Codex pane command: %w", err)
	}
	switch {
	case launch.BypassHookTrust:
		if !filepath.IsAbs(launch.HookRoot) || filepath.Clean(launch.HookRoot) != launch.HookRoot {
			return errors.New("validate Codex pane command: hook root must be a clean absolute path")
		}
		if _, err := override.ValidateReason(launch.HookTrustReason); err != nil {
			return fmt.Errorf("validate Codex pane command: invalid hook-trust reason: %w", err)
		}
		if launch.HookTrustActor == "" {
			return errors.New("validate Codex pane command: hook-trust actor is required")
		}
		subject, err := override.CodexHookTrustSubject(
			launch.HookTrustSourceRepo,
			launch.HookTrustSourceCommit,
			launch.HookTrustDigest,
		)
		if err != nil || subject != launch.HookTrustSubject {
			return errors.New("validate Codex pane command: exact hook-trust source identity is required")
		}
		proof := launch.HookTrustProof
		if proof.Kind != override.KindCodexHookTrust ||
			proof.Reason != launch.HookTrustReason ||
			proof.Actor != launch.HookTrustActor ||
			proof.Session != launch.SessionName ||
			proof.Subject == "" ||
			proof.Subject != launch.HookTrustSubject ||
			proof.AuthorizationID == "" {
			return errors.New("validate Codex pane command: exact hook-trust authorization proof is required")
		}
	case launch.HookRoot != "":
		return errors.New("validate Codex pane command: hook root requires hook-trust bypass")
	case launch.HookTrustReason != "" || launch.HookTrustActor != "":
		return errors.New("validate Codex pane command: hook-trust metadata requires hook-trust bypass")
	}
	return nil
}

func validateTrustedHandoffIsolation(launch CodexLaunch) error {
	if !launch.BypassHookTrust {
		return nil
	}
	root, err := trustedHandoffRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create trusted Codex handoff root: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect trusted Codex handoff root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("trusted Codex handoff root must not be a symlink")
	}
	root, err = resolveProspectivePath(root)
	if err != nil {
		return fmt.Errorf("resolve trusted Codex handoff root: %w", err)
	}
	for _, writable := range append([]string{launch.WorkDir}, launch.AddDirs...) {
		if writable == "" {
			continue
		}
		canonicalWritable, resolveErr := resolveProspectivePath(writable)
		if resolveErr != nil {
			return fmt.Errorf("resolve agent-writable root %q: %w", writable, resolveErr)
		}
		if pathsOverlap(root, canonicalWritable) {
			return fmt.Errorf("trusted Codex handoff root %q overlaps agent-writable root %q", root, writable)
		}
	}
	// #nosec G302 -- this is an owner-only directory and needs execute bits.
	if err := os.Chmod(root, 0o700); err != nil {
		return fmt.Errorf("secure trusted Codex handoff root: %w", err)
	}
	return validateHandoffDirectorySecurity(root)
}

func bindCodexLaunch(launch CodexLaunch) codexLaunchBinding {
	return codexLaunchBinding{
		SessionName:           launch.SessionName,
		Model:                 launch.Model,
		WorkDir:               launch.WorkDir,
		Sandbox:               launch.Sandbox,
		Approval:              launch.Approval,
		AddDirs:               append([]string(nil), launch.AddDirs...),
		ResumeID:              launch.ResumeID,
		Remote:                launch.Remote,
		HookRoot:              launch.HookRoot,
		HookTrustReason:       launch.HookTrustReason,
		HookTrustActor:        launch.HookTrustActor,
		HookTrustSourceRepo:   launch.HookTrustSourceRepo,
		HookTrustSourceCommit: launch.HookTrustSourceCommit,
		HookTrustDigest:       launch.HookTrustDigest,
		HookTrustProof:        launch.HookTrustProof,
	}
}

func bindHandoffOverrideReservations(
	path string,
	trusted bool,
	reservations ...*override.Reservation,
) error {
	proofs := make([]override.AuthorizationProof, 0, len(reservations))
	for _, reservation := range reservations {
		if reservation == nil {
			return errors.New("bind private launch overrides: nil reservation")
		}
		proofs = append(proofs, reservation.Proof())
	}
	return bindHandoffOverrideProofs(path, trusted, proofs)
}

func bindHandoffOverrideProofs(
	path string,
	trusted bool,
	proofs []override.AuthorizationProof,
) error {
	file, openedInfo, err := openHandoffForOverrideBinding(path, trusted)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	handoff, err := decodeHandoff(file)
	if err != nil {
		return err
	}
	handoff.OverrideProofs = proofs
	if err := validateHandoff(handoff, CodexProtocol, time.Now(), openedInfo.ModTime()); err != nil {
		return err
	}
	encoded, err := encodeBoundHandoff(handoff)
	if err != nil {
		return err
	}
	return writeBoundHandoff(file, path, openedInfo, encoded)
}

func openHandoffForOverrideBinding(
	path string,
	trusted bool,
) (*os.File, os.FileInfo, error) {
	if err := validatePrivateHandoffLocation(path, trusted); err != nil {
		return nil, nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect private launch handoff before binding overrides: %w", err)
	}
	if !isOwnerOnlyHandoffFile(info) {
		return nil, nil, errors.New("private launch handoff is not an owner-only regular file")
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open private launch handoff to bind overrides: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("inspect opened private launch handoff before binding overrides: %w", err)
	}
	if !isOwnerOnlyHandoffFile(openedInfo) || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, nil, errors.New("private launch handoff changed while binding overrides")
	}
	return file, openedInfo, nil
}

func encodeBoundHandoff(handoff launchHandoff) ([]byte, error) {
	encoded, err := json.Marshal(handoff)
	if err != nil {
		return nil, fmt.Errorf("encode private launch handoff overrides: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > handoffMaxSize {
		return nil, errors.New("private launch handoff exceeds the size limit")
	}
	return encoded, nil
}

func writeBoundHandoff(
	file *os.File,
	path string,
	openedInfo os.FileInfo,
	encoded []byte,
) error {
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("truncate private launch handoff before binding overrides: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind private launch handoff before binding overrides: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("write private launch handoff overrides: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync private launch handoff overrides: %w", err)
	}
	currentInfo, err := os.Lstat(path)
	if err != nil || !os.SameFile(openedInfo, currentInfo) {
		return errors.New("private launch handoff was removed or replaced while binding overrides")
	}
	return nil
}

func (binding codexLaunchBinding) matches(other codexLaunchBinding) bool {
	// The hook-trust reason and actor never appear in the pasted command. The
	// executor accepts them only from this validated owner-only binding after
	// every command-visible field matches.
	return binding.SessionName == other.SessionName &&
		binding.Model == other.Model &&
		binding.WorkDir == other.WorkDir &&
		binding.Sandbox == other.Sandbox &&
		binding.Approval == other.Approval &&
		slices.Equal(binding.AddDirs, other.AddDirs) &&
		binding.ResumeID == other.ResumeID &&
		binding.Remote == other.Remote &&
		binding.HookRoot == other.HookRoot
}

func validateClaudePastedValues(launch ClaudeLaunch) error {
	for _, field := range []struct{ name, value string }{
		{"session", launch.SessionName},
		{"session-id", launch.SessionID},
		{"resume-id", launch.ResumeID},
		{"workdir", launch.WorkDir},
		{"model", launch.Model},
		{"permission", launch.Permission},
	} {
		if err := validateOptionalText(field.name, field.value); err != nil {
			return fmt.Errorf("validate Claude pane command: %w", err)
		}
	}
	if err := validateTextList("add-dir", launch.AddDirs); err != nil {
		return fmt.Errorf("validate Claude pane command: %w", err)
	}
	return nil
}

func cleanupFailedHandoff(path string, scheduleErr error) error {
	removeErr := os.Remove(path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(fmt.Errorf("schedule private launch handoff expiration: %w", scheduleErr), removeErr)
}

func cleanupInvalidHandoff(path string, validationErr error) error {
	removeErr := os.Remove(path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(validationErr, removeErr)
}

func startHandoffExpiry(executable, path string, expiresAt time.Time, deferred bool) (io.Closer, error) {
	var leaseReader, leaseWriter *os.File
	if deferred {
		var err error
		leaseReader, leaseWriter, err = os.Pipe()
		if err != nil {
			return nil, fmt.Errorf("create producer liveness lease: %w", err)
		}
	}
	closeLease := func() {
		if leaseReader != nil {
			_ = leaseReader.Close()
		}
		if leaseWriter != nil {
			_ = leaseWriter.Close()
		}
	}
	args := []string{
		ExpiryProtocol,
		"--handoff", path,
		"--expires-at", expiresAt.UTC().Format(time.RFC3339Nano),
	}
	if deferred {
		// ExtraFiles begins at descriptor 3 on the supported Unix platforms.
		args = append(args, "--producer-lease-fd", "3")
	}
	cmd := exec.Command( // #nosec G204,G702 -- executable is the resolved current/co-installed AGM binary.
		executable,
		args...,
	)
	// The expiry helper needs neither caller credentials nor terminal ownership.
	// Its isolated process group lets it outlive an AGM caller that exits
	// immediately after queuing a tmux command.
	cmd.Env = []string{expiryHelperEnv + "=1"}
	if stateDir := os.Getenv("AGM_STATE_DIR"); stateDir != "" {
		cmd.Env = append(cmd.Env, "AGM_STATE_DIR="+stateDir)
	}
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	if deferred {
		cmd.ExtraFiles = []*os.File{leaseReader}
	}
	if err := cmd.Start(); err != nil {
		closeLease()
		return nil, fmt.Errorf("start expiration helper: %w", err)
	}
	if leaseReader != nil {
		_ = leaseReader.Close()
	}
	// Wait asynchronously: short-lived CLI callers may exit first, while
	// long-lived MCP callers must reap every completed helper.
	reapExpiryProcess(cmd)
	if !deferred {
		return nil, nil
	}
	return newProducerLease(leaseWriter), nil
}

// producerLease deliberately keeps the pipe writer reachable until explicit
// cancellation or process exit. For current-pane launchers, process exit is
// the exact event that permits the queued shell command to run.
type producerLease struct {
	writer *os.File
	done   chan struct{}
	once   sync.Once
	err    error
}

func newProducerLease(writer *os.File) *producerLease {
	lease := &producerLease{writer: writer, done: make(chan struct{})}
	go func(held *os.File, done <-chan struct{}) {
		<-done
		// Reference held after the receive so garbage collection cannot close
		// the file while the producing AGM process remains alive.
		_ = held.Name()
	}(writer, lease.done)
	return lease
}

func (l *producerLease) Close() error {
	l.once.Do(func() {
		l.err = l.writer.Close()
		close(l.done)
	})
	return l.err
}

func expireHandoff(path string, expiresAt time.Time, now func() time.Time, wait func(time.Duration)) error {
	original, err := expiryTarget(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for {
		current, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return fmt.Errorf("inspect expiring private launch handoff: %w", statErr)
		}
		if !os.SameFile(original, current) {
			return errors.New("private launch handoff changed before expiration")
		}
		remaining := expiresAt.Sub(now())
		if remaining <= 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("expire private launch handoff: %w", err)
			}
			return nil
		}
		if remaining > 250*time.Millisecond {
			remaining = 250 * time.Millisecond
		}
		wait(remaining)
	}
}

func expireDeferredHandoff(path string, lease io.Reader, lifetime, heartbeat time.Duration) error {
	original, err := expiryTarget(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	producerExited := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(io.Discard, lease)
		producerExited <- struct{}{}
	}()
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-producerExited:
			now := time.Now()
			if err := refreshExpiryTarget(path, original, now); errors.Is(err, os.ErrNotExist) {
				return nil
			} else if err != nil {
				return err
			}
			return expireHandoff(path, now.Add(lifetime), time.Now, time.Sleep)
		case now := <-ticker.C:
			if err := refreshExpiryTarget(path, original, now); errors.Is(err, os.ErrNotExist) {
				return nil
			} else if err != nil {
				return err
			}
		}
	}
}

func refreshExpiryTarget(path string, original os.FileInfo, now time.Time) error {
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(original, current) {
		return errors.New("private launch handoff changed while producer lease was live")
	}
	if err := os.Chtimes(path, now, now); err != nil {
		return fmt.Errorf("refresh deferred private launch handoff: %w", err)
	}
	current, err = os.Lstat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(original, current) {
		return errors.New("private launch handoff changed while producer lease was refreshed")
	}
	return nil
}

func expiryTarget(path string) (os.FileInfo, error) {
	if ordinaryErr := validatePrivateHandoffLocation(path, false); ordinaryErr != nil {
		if trustedErr := validatePrivateHandoffLocation(path, true); trustedErr != nil {
			return nil, errors.Join(ordinaryErr, trustedErr)
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !isOwnerOnlyHandoffFile(info) {
		return nil, errors.New("private launch handoff expiration target is invalid")
	}
	return info, nil
}

func resolvePrivateExecutable() (string, error) {
	current, err := executablePath()
	if err != nil {
		return "", err
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return "", err
	}
	name := filepath.Base(current)
	suffix := ""
	switch name {
	case "agm-mcp-server":
	case "agm-mcp-server-" + runtime.GOOS + "-" + runtime.GOARCH:
		suffix = "-" + runtime.GOOS + "-" + runtime.GOARCH
	default:
		return current, nil
	}
	// Companion binaries such as agm-mcp-server must execute the co-installed
	// AGM binary, because they do not intercept the private protocol themselves.
	sibling := filepath.Join(filepath.Dir(current), "agm"+suffix)
	if isExecutableFile(sibling) {
		return sibling, nil
	}
	return "", fmt.Errorf("find co-installed AGM private executor beside %s", current)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

func privateExecutable(configured string) string {
	if configured != "" {
		return configured
	}
	return "agm"
}

func stageHandoff(
	protocol string,
	environment []string,
	deferred bool,
	codexHookRoot string,
	codexLaunch ...*codexLaunchBinding,
) (string, error) {
	if err := validateHandoffEnvironment(protocol, environment); err != nil {
		return "", err
	}
	binding, err := stagedCodexBinding(codexHookRoot, codexLaunch)
	if err != nil {
		return "", err
	}
	root, err := resolvedHandoffRoot(codexHookRoot != "")
	if err != nil {
		return "", fmt.Errorf("resolve private launch handoff directory: %w", err)
	}
	return writeHandoff(root, protocol, environment, deferred, codexHookRoot, binding)
}

func stagedCodexBinding(
	codexHookRoot string,
	codexLaunch []*codexLaunchBinding,
) (*codexLaunchBinding, error) {
	if len(codexLaunch) > 1 {
		return nil, errors.New("private launch handoff received multiple Codex launch bindings")
	}
	var binding *codexLaunchBinding
	if len(codexLaunch) == 1 {
		binding = codexLaunch[0]
	}
	if (codexHookRoot == "") != (binding == nil) {
		return nil, errors.New("private Codex hook capability requires an exact launch binding")
	}
	return binding, nil
}

func writeHandoff(
	root, protocol string,
	environment []string,
	deferred bool,
	codexHookRoot string,
	binding *codexLaunchBinding,
) (string, error) {
	if err := validateText("private handoff directory", root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", fmt.Errorf("create private launch handoff directory: %w", err)
	}
	// #nosec G302 -- the owner-only directory requires traversal permission.
	if err := os.Chmod(root, 0700); err != nil {
		return "", fmt.Errorf("secure private launch handoff directory: %w", err)
	}
	removeStaleHandoffs(root, time.Now())

	file, err := os.CreateTemp(root, "launch-*.json")
	if err != nil {
		return "", fmt.Errorf("create private launch handoff: %w", err)
	}
	path := file.Name()
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return "", fmt.Errorf("secure private launch handoff: %w", err)
	}
	payload := launchHandoff{
		Version: handoffVersion, Protocol: protocol,
		CreatedAt:                 time.Now().UTC().Format(time.RFC3339Nano),
		DeferredUntilProducerExit: deferred,
		Environment:               append([]string(nil), environment...),
		CodexHookRoot:             codexHookRoot,
		CodexLaunch:               binding,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("write private launch handoff: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > handoffMaxSize {
		return "", errors.New("private launch handoff exceeds the size limit")
	}
	if _, err := file.Write(encoded); err != nil {
		return "", fmt.Errorf("write private launch handoff: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync private launch handoff: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close private launch handoff: %w", err)
	}
	closed = true
	return path, nil
}

func consumeHandoff(
	path, protocol, expectedCodexHookRoot string,
	expectedCodexLaunch ...*codexLaunchBinding,
) (launchHandoff, error) {
	if len(expectedCodexLaunch) > 1 {
		return launchHandoff{}, errors.New("private launch handoff received multiple expected Codex launch bindings")
	}
	var expectedBinding *codexLaunchBinding
	if len(expectedCodexLaunch) == 1 {
		expectedBinding = expectedCodexLaunch[0]
	}
	if (expectedCodexHookRoot == "") != (expectedBinding == nil) {
		return launchHandoff{}, errors.New("private Codex hook capability requires an exact launch binding")
	}
	file, err := openPrivateHandoff(path, expectedCodexHookRoot != "")
	if err != nil {
		return launchHandoff{}, err
	}
	defer func() { _ = file.Close() }()
	// Unlink immediately after securely opening the exact owner-only file. The
	// open descriptor remains readable, while every decode or validation
	// rejection is still one-shot and cannot leave credentials on disk.
	if err := os.Remove(path); err != nil {
		return launchHandoff{}, fmt.Errorf("consume private launch handoff: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return launchHandoff{}, fmt.Errorf("inspect opened private launch handoff: %w", err)
	}
	handoff, err := decodeHandoff(file)
	if err != nil {
		return launchHandoff{}, err
	}
	if err := validateHandoff(handoff, protocol, time.Now(), info.ModTime()); err != nil {
		return launchHandoff{}, err
	}
	if handoff.CodexHookRoot != expectedCodexHookRoot {
		return launchHandoff{}, errors.New("private launch handoff does not authorize the requested Codex hook root")
	}
	if expectedBinding != nil &&
		(handoff.CodexLaunch == nil || !handoff.CodexLaunch.matches(*expectedBinding)) {
		return launchHandoff{}, errors.New("private launch handoff does not authorize the requested Codex launch")
	}
	return handoff, nil
}

func openPrivateHandoff(path string, trusted bool) (*os.File, error) {
	if err := validatePrivateHandoffLocation(path, trusted); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect private launch handoff: %w", err)
	}
	if !isOwnerOnlyHandoffFile(info) {
		return nil, errors.New("private launch handoff is not an owner-only regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open private launch handoff: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened private launch handoff: %w", err)
	}
	if !isOwnerOnlyHandoffFile(openedInfo) || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, errors.New("private launch handoff changed while it was being opened")
	}
	return file, nil
}

func validatePrivateHandoffLocation(path string, trusted bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("private launch handoff requires a canonical absolute path")
	}
	name := filepath.Base(path)
	if !strings.HasPrefix(name, "launch-") || !strings.HasSuffix(name, ".json") {
		return errors.New("private launch handoff name is outside the staging namespace")
	}
	directory := filepath.Dir(path)
	if filepath.Base(directory) != "private-launch" {
		return errors.New("private launch handoff is outside a private-launch staging directory")
	}
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return fmt.Errorf("resolve private launch handoff directory: %w", err)
	}
	if filepath.Base(canonicalDirectory) != "private-launch" {
		return errors.New("private launch handoff directory resolves outside the staging namespace")
	}
	expectedDirectory, err := resolvedHandoffRoot(trusted)
	if err != nil {
		return fmt.Errorf("resolve expected private launch handoff directory: %w", err)
	}
	expectedDirectory, err = filepath.EvalSymlinks(expectedDirectory)
	if err != nil {
		return fmt.Errorf("resolve expected private launch handoff identity: %w", err)
	}
	if canonicalDirectory != expectedDirectory {
		return errors.New("private launch handoff is outside the expected staging directory")
	}
	return validateHandoffDirectorySecurity(canonicalDirectory)
}

func validateHandoffDirectorySecurity(directory string) error {
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("inspect private launch handoff directory: %w", err)
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode().Perm()&0077 != 0 || !ownedByCurrentUser(directoryInfo) {
		return errors.New("private launch handoff directory is not owner-only and current-user-owned")
	}
	return nil
}

func isOwnerOnlyHandoffFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0077 == 0 && ownedByCurrentUser(info)
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

func decodeHandoff(file *os.File) (launchHandoff, error) {
	payload, err := io.ReadAll(io.LimitReader(file, handoffMaxSize+1))
	if err != nil {
		return launchHandoff{}, fmt.Errorf("read private launch handoff: %w", err)
	}
	if len(payload) > handoffMaxSize {
		return launchHandoff{}, errors.New("private launch handoff exceeds the size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var handoff launchHandoff
	if err := decoder.Decode(&handoff); err != nil {
		return launchHandoff{}, fmt.Errorf("decode private launch handoff: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return launchHandoff{}, errors.New("private launch handoff contains trailing content")
	}
	return handoff, nil
}

func validateHandoff(handoff launchHandoff, protocol string, now, modifiedAt time.Time) error {
	createdAt, err := time.Parse(time.RFC3339Nano, handoff.CreatedAt)
	validAt := createdAt
	if handoff.DeferredUntilProducerExit {
		validAt = modifiedAt
	}
	age := now.Sub(validAt)
	if err != nil || age < 0 || age > handoffMaxAge {
		return errors.New("private launch handoff is expired or has an invalid timestamp")
	}
	if handoff.Version != handoffVersion || handoff.Protocol != protocol {
		return errors.New("private launch handoff does not match the requested protocol")
	}
	if err := validateHandoffOverrideProofs(handoff); err != nil {
		return err
	}
	if handoff.CodexHookRoot != "" {
		if protocol != CodexProtocol ||
			!filepath.IsAbs(handoff.CodexHookRoot) ||
			filepath.Clean(handoff.CodexHookRoot) != handoff.CodexHookRoot ||
			handoff.CodexLaunch == nil ||
			handoff.CodexLaunch.HookRoot != handoff.CodexHookRoot {
			return errors.New("private launch handoff contains an invalid Codex hook capability")
		}
		if normalized, reasonErr := override.ValidateReason(handoff.CodexLaunch.HookTrustReason); reasonErr != nil || normalized != handoff.CodexLaunch.HookTrustReason {
			return errors.New("private launch handoff contains an invalid Codex hook-trust reason")
		}
		if handoff.CodexLaunch.HookTrustActor == "" {
			return errors.New("private launch handoff contains an invalid Codex hook-trust actor")
		}
		subject, subjectErr := override.CodexHookTrustSubject(
			handoff.CodexLaunch.HookTrustSourceRepo,
			handoff.CodexLaunch.HookTrustSourceCommit,
			handoff.CodexLaunch.HookTrustDigest,
		)
		proof := handoff.CodexLaunch.HookTrustProof
		if subjectErr != nil ||
			proof.Kind != override.KindCodexHookTrust ||
			proof.Reason != handoff.CodexLaunch.HookTrustReason ||
			proof.Actor != handoff.CodexLaunch.HookTrustActor ||
			proof.Session != handoff.CodexLaunch.SessionName ||
			proof.Subject == "" ||
			proof.Subject != subject ||
			proof.AuthorizationID == "" {
			return errors.New("private launch handoff contains an invalid Codex hook-trust authorization proof")
		}
	} else if handoff.CodexLaunch != nil {
		return errors.New("private launch handoff contains a Codex launch binding without a hook capability")
	}
	if err := validateHandoffEnvironment(protocol, handoff.Environment); err != nil {
		return err
	}
	return nil
}

func validateHandoffOverrideProofs(handoff launchHandoff) error {
	seen := make(map[override.Kind]struct{}, len(handoff.OverrideProofs))
	hookProofFound := false
	for _, proof := range handoff.OverrideProofs {
		isHookProof, err := validateHandoffOverrideProof(handoff, proof)
		if err != nil {
			return err
		}
		hookProofFound = hookProofFound || isHookProof
		if _, duplicate := seen[proof.Kind]; duplicate {
			return fmt.Errorf("private launch handoff repeats override kind %q", proof.Kind)
		}
		seen[proof.Kind] = struct{}{}
		if proof.Reason == "" || proof.Actor == "" || proof.AuthorizationID == "" {
			return errors.New("private launch handoff contains incomplete override proof")
		}
	}
	if handoff.CodexLaunch != nil && !hookProofFound {
		return errors.New("private launch handoff omits the Codex hook-trust proof")
	}
	return nil
}

func validateHandoffOverrideProof(
	handoff launchHandoff,
	proof override.AuthorizationProof,
) (bool, error) {
	switch proof.Kind {
	case override.KindAdmissionBrake:
		if proof.Subject != "" {
			return false, errors.New("private launch handoff contains an admission-brake subject")
		}
		if proof.Session == "" ||
			(handoff.CodexLaunch != nil && proof.Session != handoff.CodexLaunch.SessionName) {
			return false, errors.New("private launch handoff contains an unrelated admission-brake proof")
		}
		return false, nil
	case override.KindCodexHookTrust:
		if handoff.CodexLaunch == nil || proof != handoff.CodexLaunch.HookTrustProof {
			return false, errors.New("private launch handoff contains an unrelated Codex hook-trust proof")
		}
		return true, nil
	case override.KindSupervisorOAuthCheck:
		return false, fmt.Errorf("private launch handoff contains unsupported override kind %q", proof.Kind)
	default:
		return false, fmt.Errorf("private launch handoff contains unsupported override kind %q", proof.Kind)
	}
}

func validateHandoffEnvironment(protocol string, environment []string) error {
	allowed := handoffEnvironmentAllowlist(protocol)
	seen := make(map[string]bool, len(environment))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "" || !allowed[name] || seen[name] || strings.IndexFunc(entry, unicode.IsControl) >= 0 {
			return errors.New("private launch handoff contains an invalid environment entry")
		}
		seen[name] = true
	}
	return nil
}

func handoffEnvironmentAllowlist(protocol string) map[string]bool {
	allowed := make(map[string]bool)
	switch protocol {
	case CodexProtocol:
		for _, name := range codexAllowedEnvironment {
			allowed[name] = true
		}
		allowed["AGM_SESSION_NAME"] = true
	case ClaudeProtocol:
		for _, name := range []string{
			auth.OAuthEnvVar, "ANTHROPIC_API_KEY",
			"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS",
		} {
			allowed[name] = true
		}
	}
	return allowed
}

func handoffRoot() string {
	stateDir := os.Getenv("AGM_STATE_DIR")
	if stateDir == "" {
		stateDir = fmt.Sprintf("/tmp/agm-%d", os.Getuid())
	}
	return filepath.Join(stateDir, "private-launch")
}

func trustedHandoffRoot() (string, error) {
	account, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve current user for trusted private handoff: %w", err)
	}
	if account.HomeDir == "" {
		return "", errors.New("resolve current user for trusted private handoff: home directory is empty")
	}
	return filepath.Join(account.HomeDir, ".local", "share", "dear-agent", "private-launch"), nil
}

func resolvedHandoffRoot(trusted bool) (string, error) {
	root := handoffRoot()
	if trusted {
		var err error
		root, err = trustedHandoffRoot()
		if err != nil {
			return "", err
		}
	}
	return filepath.Abs(root)
}

func pathsOverlap(left, right string) bool {
	left, leftErr := filepath.Abs(left)
	right, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return pathWithin(left, right) || pathWithin(right, left)
}

func resolveProspectivePath(path string) (string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		if _, lstatErr := os.Lstat(current); lstatErr == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", resolveErr
			}
			for _, component := range slices.Backward(missing) {
				resolved = filepath.Join(resolved, component)
			}
			return filepath.Clean(resolved), nil
		} else if !errors.Is(lstatErr, os.ErrNotExist) {
			return "", lstatErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func removeStaleHandoffs(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "launch-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr == nil && now.Sub(info.ModTime()) > handoffMaxAge {
			_ = os.Remove(filepath.Join(root, entry.Name()))
		}
	}
}

func overlayEnvironment(base, overrides []string) []string {
	values := environmentMap(base)
	for _, entry := range overrides {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" {
			values[name] = value
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+values[name])
	}
	return result
}

func removeEnvironment(base []string, remove map[string]bool) []string {
	result := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if ok && !remove[name] {
			result = append(result, entry)
		}
	}
	return result
}

func selectedEnvironment(base []string, names map[string]bool) []string {
	result := make([]string, 0, len(names))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if ok && names[name] {
			result = append(result, entry)
		}
	}
	return result
}
