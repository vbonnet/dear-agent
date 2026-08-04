// Package harnessexec owns AGM's private, argument-only launch protocol for
// interactive harnesses whose credentials must not appear in tmux commands.
package harnessexec

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
	"github.com/vbonnet/dear-agent/agm/internal/codexhooks"
	"github.com/vbonnet/dear-agent/agm/internal/shellquote"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	"github.com/vbonnet/dear-agent/pkg/override"
)

const (
	// CodexProtocol is the launch protocol token intercepted by agm before Cobra,
	// configuration, telemetry, or debug logging starts. It carries only
	// non-secret Codex metadata; credentials are resolved inside the executor.
	CodexProtocol = "__exec-codex"
	// ClaudeProtocol is the launch protocol token intercepted by agm before Cobra,
	// configuration, telemetry, or debug logging starts. It carries only
	// non-secret Claude metadata; OAuth is resolved inside the executor.
	ClaudeProtocol = "__exec-claude"
	// HarnessProtocol is the private launch boundary for non-Codex, non-Claude
	// tmux commands. The executor commits launch-bound override proofs
	// immediately before replacing itself with the validated shell command.
	HarnessProtocol = "__exec-harness"
	// AgyProtocol is the launch protocol token intercepted by agm before Cobra,
	// configuration, telemetry, or debug logging starts. It carries only
	// non-secret AGY metadata; Google/Gemini authentication is captured inside
	// the executor environment handoff.
	AgyProtocol = "__exec-agy"
	// ExpiryProtocol is the private cleanup protocol used by a detached,
	// credential-free helper to expire an unconsumed launch handoff.
	ExpiryProtocol = "__expire-harness-handoff"
	// CodexWorkerWriteRootsEnv carries the host-authorized write roots consumed
	// by Codex's system-managed worker boundary hook. It is transported through
	// the private handoff so a long-lived tmux server cannot drop or stale it.
	CodexWorkerWriteRootsEnv = "AGM_WORKER_WRITE_ROOTS_JSON"
	// remoteCodexReasoningEffort prevents a restored remote thread from
	// reusing a persisted effort value unsupported by the current Codex runtime.
	remoteCodexReasoningEffort = `model_reasoning_effort="xhigh"`
)

var (
	lookPathInEnvironment           = resolveExecutableInEnvironment
	replaceProcess                  = syscall.Exec
	changeDirectory                 = os.Chdir
	resolveClaudeOAuth              = auth.ResolveOAuthToken
	codexHookOverrides              = codexhooks.LaunchConfigOverrides
	verifyCodexHookTrustAttestation = func(attestation codexhooks.Attestation, workDir string) error {
		return codexhooks.Verify(context.Background(), attestation, workDir)
	}
	commitLaunchOverrideProofs = commitAuthenticatedLaunchOverrideProofs
	recordLaunchSpawn          = func() error {
		return circuitbreaker.NewFileSpawnTimer().RecordSpawn(time.Now())
	}
)

type reserveOverrideClaim func(override.Request) (*override.Reservation, error)
type checkLaunchAdmission func() circuitbreaker.CheckResult

func commitAuthenticatedLaunchOverrideProofs(
	sessionName string,
	proofs ...override.AuthorizationProof,
) error {
	reservations, err := reserveExecutorLaunchOverrides(
		sessionName,
		proofs,
		currentLaunchAdmission,
		override.Reserve,
	)
	if err != nil {
		return err
	}
	_, err = override.CommitAll(reservations...)
	return err
}

func reserveExecutorLaunchOverrides(
	sessionName string,
	proofs []override.AuthorizationProof,
	check checkLaunchAdmission,
	reserve reserveOverrideClaim,
) ([]*override.Reservation, error) {
	// Ordinary harness launches carry no override claims and remain governed by
	// the parent launch path. A trusted hook handoff always carries its required
	// hook-trust claim, so it cannot use this boundary to suppress the executor's
	// live admission checks.
	if len(proofs) == 0 {
		return nil, nil
	}
	hasAdmissionBrake := slices.ContainsFunc(proofs, func(proof override.AuthorizationProof) bool {
		return proof.Kind == override.KindAdmissionBrake
	})
	initialAdmission := check()
	if _, err := validateExecutorAdmissionClaim(initialAdmission, hasAdmissionBrake); err != nil {
		return nil, err
	}

	type authenticatedReservation struct {
		kind        override.Kind
		reservation *override.Reservation
	}
	authenticated := make([]authenticatedReservation, 0, len(proofs))
	seen := make(map[override.Kind]struct{}, len(proofs))
	for _, proof := range proofs {
		if _, duplicate := seen[proof.Kind]; duplicate {
			return nil, fmt.Errorf("private harness launch repeats override kind %q", proof.Kind)
		}
		seen[proof.Kind] = struct{}{}
		switch proof.Kind {
		case override.KindAdmissionBrake, override.KindCodexHookTrust,
			override.KindSupervisorOAuthCheck:
		default:
			return nil, fmt.Errorf("private harness launch contains unsupported override kind %q", proof.Kind)
		}
		if proof.Session != sessionName {
			return nil, errors.New("private harness launch override claim is for another session")
		}
		reservation, err := reserve(override.Request{
			Kind:    proof.Kind,
			Reason:  proof.Reason,
			Actor:   proof.Actor,
			Session: proof.Session,
			Subject: proof.Subject,
		})
		if err != nil {
			return nil, fmt.Errorf("re-authorize private harness launch override %q: %w", proof.Kind, err)
		}
		authenticated = append(authenticated, authenticatedReservation{
			kind: proof.Kind, reservation: reservation,
		})
	}

	finalAdmission := check()
	crossesAdmissionBrake, err := validateExecutorAdmissionClaim(finalAdmission, hasAdmissionBrake)
	if err != nil {
		return nil, err
	}
	reservations := make([]*override.Reservation, 0, len(authenticated))
	for _, item := range authenticated {
		if item.kind == override.KindAdmissionBrake && !crossesAdmissionBrake {
			continue
		}
		reservations = append(reservations, item.reservation)
	}
	return reservations, nil
}

func validateExecutorAdmissionClaim(
	result circuitbreaker.CheckResult,
	hasAdmissionBrake bool,
) (bool, error) {
	if err := validateExecutorAdmissionResult(result); err != nil {
		return false, err
	}
	crossesAdmissionBrake := circuitbreaker.RequiresAdmissionBrakeOverride(result)
	if crossesAdmissionBrake && !hasAdmissionBrake {
		return false, errors.New("private harness launch requires an admission-brake authorization claim")
	}
	return crossesAdmissionBrake, nil
}

func validateExecutorAdmissionResult(result circuitbreaker.CheckResult) error {
	if result.Allowed || circuitbreaker.RequiresAdmissionBrakeOverride(result) {
		return nil
	}
	return fmt.Errorf(
		"private harness launch denied by live circuit breakers: %s",
		circuitbreaker.FormatDenied(result),
	)
}

var currentLaunchAdmission = func() circuitbreaker.CheckResult {
	return circuitbreaker.Check(
		circuitbreaker.DefaultConfig(),
		circuitbreaker.DefaultLoadReader(),
		circuitbreaker.TmuxWorkerCounter{},
		circuitbreaker.NewFileSpawnTimer(),
		circuitbreaker.DefaultMemReader(),
		circuitbreaker.WithDiskReader(circuitbreaker.DefaultDiskReader()),
		circuitbreaker.WithProcCounter(circuitbreaker.DefaultProcCounter()),
		circuitbreaker.WithBrakeReader(circuitbreaker.DefaultBrakeReader()),
	)
}

var codexAllowedEnvironment = []string{
	"HOME", "PATH", "PWD", "SHELL", "USER", "LOGNAME",
	"TMPDIR", "TMP", "TEMP",
	"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "NO_COLOR",
	"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES",
	"TMUX", "TMUX_PANE",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
	"CODEX_HOME", "CODEX_SQLITE_HOME", "CODEX_ACCESS_TOKEN",
	"CODEX_CA_CERTIFICATE", "SSL_CERT_FILE", "RUST_LOG",
	"OPENAI_API_KEY",
	"AGM_HOME", "AGM_CONFIG_DIR", "AGM_DB_PATH", "AGM_SESSIONS_DIR",
	"AGM_STATE_DIR", "AGM_TMUX_SOCKET", "AGM_BUS_SOCKET", "AGM_TEAM",
	CodexWorkerWriteRootsEnv,
	"WORKSPACE",
}

var agyAllowedEnvironment = []string{
	"HOME", "PATH", "PWD", "SHELL", "USER", "LOGNAME",
	"TMPDIR", "TMP", "TEMP",
	"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "NO_COLOR",
	"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES",
	"TMUX", "TMUX_PANE",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
	"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_CLOUD_PROJECT", "GCP_PROJECT",
	"CLOUDSDK_CONFIG", "GEMINI_API_KEY", "GOOGLE_API_KEY",
	"AGM_HOME", "AGM_CONFIG_DIR", "AGM_DB_PATH", "AGM_SESSIONS_DIR",
	"AGM_STATE_DIR", "AGM_TMUX_SOCKET", "AGM_BUS_SOCKET", "AGM_TEAM",
	"WORKSPACE", "AGM_AUTONOMOUS",
}

// paneRuntimeEnvironment names terminal state that belongs to the target pane,
// not the process that prepared a private handoff. In particular, a non-TTY
// caller can report TERM=dumb even though tmux has created a fully capable
// tmux-256color pane. Restoring the caller value would make interactive
// harnesses reject their actual terminal.
var paneRuntimeEnvironment = map[string]bool{
	"TMUX": true, "TMUX_PANE": true,
	"TERM": true, "COLORTERM": true,
	"TERM_PROGRAM": true, "TERM_PROGRAM_VERSION": true,
}

var claudeHandoffEnvironment = map[string]bool{
	auth.OAuthEnvVar: true, "ANTHROPIC_API_KEY": true,
	"OTEL_EXPORTER_OTLP_ENDPOINT": true, "OTEL_EXPORTER_OTLP_HEADERS": true,
}

// CodexLaunch contains the non-secret metadata AGM may place in a tmux launch
// command. The executor constructs the real Codex argv after parsing it.
type CodexLaunch struct {
	Executable             string
	HandoffPath            string
	SessionName            string
	Model                  string
	WorkDir                string
	Sandbox                string
	Approval               string
	AddDirs                []string
	ResumeID               string
	Remote                 bool
	RemoteResume           bool
	BypassHookTrust        bool
	HookRoot               string
	HookTrustReason        string
	HookTrustActor         string
	HookTrustSubject       string
	HookTrustSourceRepo    string
	HookTrustSourceCommit  string
	HookTrustDigest        string
	HookTrustProof         override.AuthorizationProof
	Persistent             bool
	DeferUntilProducerExit bool
}

// ClaudeLaunch contains the non-secret metadata AGM may place in a tmux launch
// command. OAuth values are resolved only inside the executor.
type ClaudeLaunch struct {
	Executable             string
	Binary                 string
	HandoffPath            string
	SessionName            string
	SessionID              string
	ResumeID               string
	WorkDir                string
	Model                  string
	AddDirs                []string
	AutoMode               bool
	Permission             string
	MaxBudgetUSD           float64
	ExtraArgs              []string
	DisableOAuth           bool
	ForwardTelemetry       bool
	Persistent             bool
	DeferUntilProducerExit bool
}

// AgyLaunch contains the non-secret metadata AGM may place in a tmux launch
// command. Google and Gemini authentication values are transported through the
// private child environment rather than shell text.
type AgyLaunch struct {
	Executable             string
	HandoffPath            string
	SessionName            string
	Model                  string
	WorkDir                string
	Permission             string
	AddDirs                []string
	ConversationID         string
	Persistent             bool
	DeferUntilProducerExit bool
}

// IsProtocol reports whether arg names one of the private launch protocols.
func IsProtocol(arg string) bool {
	return arg == CodexProtocol || arg == ClaudeProtocol ||
		arg == HarnessProtocol || arg == AgyProtocol || arg == ExpiryProtocol
}

// BuildCodexCommand returns the token-free shell command pasted into tmux.
func BuildCodexCommand(launch CodexLaunch) string {
	var b strings.Builder
	b.WriteString(privateCommandPrefix(launch.Executable, CodexProtocol))
	if launch.HandoffPath != "" {
		appendShellFlag(&b, "--handoff", launch.HandoffPath)
	}
	appendShellFlag(&b, "--session", launch.SessionName)
	if launch.Model != "" {
		appendShellFlag(&b, "--model", launch.Model)
	}
	appendShellFlag(&b, "--workdir", launch.WorkDir)
	appendShellFlag(&b, "--sandbox", launch.Sandbox)
	if launch.Approval != "" {
		appendShellFlag(&b, "--approval", launch.Approval)
	}
	for _, dir := range launch.AddDirs {
		appendShellFlag(&b, "--add-dir", dir)
	}
	if launch.ResumeID != "" {
		appendShellFlag(&b, "--resume-id", launch.ResumeID)
	}
	if launch.Remote {
		b.WriteString(" --remote")
	}
	if launch.RemoteResume {
		b.WriteString(" --remote-resume")
	}
	if launch.BypassHookTrust {
		b.WriteString(" --bypass-hook-trust")
		appendShellFlag(&b, "--hook-root", launch.HookRoot)
	}
	if !launch.Persistent {
		b.WriteString(" && exit")
	}
	return b.String()
}

// BuildClaudeCommand returns the token-free shell command pasted into tmux.
func BuildClaudeCommand(launch ClaudeLaunch) string {
	var b strings.Builder
	b.WriteString(privateCommandPrefix(launch.Executable, ClaudeProtocol))
	if launch.HandoffPath != "" {
		appendShellFlag(&b, "--handoff", launch.HandoffPath)
	}
	if launch.Binary != "" {
		appendShellFlag(&b, "--binary", launch.Binary)
	}
	appendShellFlag(&b, "--session", launch.SessionName)
	if launch.SessionID != "" {
		appendShellFlag(&b, "--session-id", launch.SessionID)
	}
	if launch.ResumeID != "" {
		appendShellFlag(&b, "--resume-id", launch.ResumeID)
	}
	if launch.WorkDir != "" {
		appendShellFlag(&b, "--workdir", launch.WorkDir)
	}
	if launch.Model != "" {
		appendShellFlag(&b, "--model", launch.Model)
	}
	for _, dir := range launch.AddDirs {
		appendShellFlag(&b, "--add-dir", dir)
	}
	if launch.AutoMode {
		b.WriteString(" --auto-mode")
	}
	if launch.Permission != "" {
		appendShellFlag(&b, "--permission", launch.Permission)
	}
	if launch.MaxBudgetUSD > 0 {
		appendShellFlag(&b, "--max-budget-usd", fmt.Sprintf("%.2f", launch.MaxBudgetUSD))
	}
	for _, arg := range launch.ExtraArgs {
		appendShellFlag(&b, "--extra-arg", arg)
	}
	if launch.DisableOAuth {
		b.WriteString(" --disable-oauth")
	}
	if launch.ForwardTelemetry {
		b.WriteString(" --forward-telemetry")
	}
	if !launch.Persistent {
		b.WriteString(" && exit")
	}
	return b.String()
}

func buildClaudeInvocationArgs(launch ClaudeLaunch) []string {
	args := []string{ClaudeProtocol}
	if launch.HandoffPath != "" {
		args = append(args, "--handoff", launch.HandoffPath)
	}
	if launch.Binary != "" {
		args = append(args, "--binary", launch.Binary)
	}
	args = append(args, "--session", launch.SessionName)
	if launch.SessionID != "" {
		args = append(args, "--session-id", launch.SessionID)
	}
	if launch.ResumeID != "" {
		args = append(args, "--resume-id", launch.ResumeID)
	}
	if launch.WorkDir != "" {
		args = append(args, "--workdir", launch.WorkDir)
	}
	if launch.Model != "" {
		args = append(args, "--model", launch.Model)
	}
	for _, dir := range launch.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if launch.AutoMode {
		args = append(args, "--auto-mode")
	}
	if launch.Permission != "" {
		args = append(args, "--permission", launch.Permission)
	}
	if launch.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", launch.MaxBudgetUSD))
	}
	for _, arg := range launch.ExtraArgs {
		args = append(args, "--extra-arg", arg)
	}
	if launch.DisableOAuth {
		args = append(args, "--disable-oauth")
	}
	if launch.ForwardTelemetry {
		args = append(args, "--forward-telemetry")
	}
	return args
}

// BuildHarnessCommand returns the token-free executor command pasted into tmux.
func BuildHarnessCommand(
	executable, handoffPath, sessionName string,
	persistent bool,
) string {
	var b strings.Builder
	b.WriteString(privateCommandPrefix(executable, HarnessProtocol))
	appendShellFlag(&b, "--handoff", handoffPath)
	appendShellFlag(&b, "--session", sessionName)
	if !persistent {
		b.WriteString(" && exit")
	}
	return b.String()
}

// BuildAgyCommand returns the token-free shell command pasted into tmux.
func BuildAgyCommand(launch AgyLaunch) string {
	var b strings.Builder
	b.WriteString(privateCommandPrefix(launch.Executable, AgyProtocol))
	if launch.HandoffPath != "" {
		appendShellFlag(&b, "--handoff", launch.HandoffPath)
	}
	appendShellFlag(&b, "--session", launch.SessionName)
	if launch.Model != "" {
		appendShellFlag(&b, "--model", launch.Model)
	}
	appendShellFlag(&b, "--workdir", launch.WorkDir)
	if launch.Permission != "" {
		appendShellFlag(&b, "--permission", launch.Permission)
	}
	if launch.ConversationID != "" {
		appendShellFlag(&b, "--conversation", launch.ConversationID)
	}
	for _, dir := range launch.AddDirs {
		appendShellFlag(&b, "--add-dir", dir)
	}
	if !launch.Persistent {
		b.WriteString(" && exit")
	}
	return b.String()
}

func appendShellFlag(b *strings.Builder, name, value string) {
	b.WriteByte(' ')
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(shellquote.Quote(value))
}

func privateCommandPrefix(executable, protocol string) string {
	resolved := privateExecutable(executable)
	if resolved == "agm" {
		return "agm " + protocol
	}
	return shellquote.Quote(resolved) + " " + protocol
}

// Run validates a private protocol request and replaces the current AGM
// process with the fixed harness executable. A successful call does not return.
func Run(protocol string, args []string) error {
	switch protocol {
	case CodexProtocol:
		return runCodex(args)
	case ClaudeProtocol:
		return runClaude(args)
	case HarnessProtocol:
		return runHarness(args)
	case AgyProtocol:
		return runAgy(args)
	case ExpiryProtocol:
		return runExpiry(args)
	default:
		return fmt.Errorf("unsupported private harness protocol %q", protocol)
	}
}

func runHarness(args []string) error {
	request, err := parseHarness(args)
	if err != nil {
		return err
	}
	handoff, err := consumeHandoff(request.HandoffPath, HarnessProtocol, "")
	if err != nil {
		return err
	}
	if handoff.HarnessSessionName != request.SessionName {
		return errors.New("private harness handoff is for another session")
	}
	if err := commitLaunchOverrideProofs(
		request.SessionName,
		handoff.OverrideProofs...,
	); err != nil {
		return fmt.Errorf("commit harness launch override transaction: %w", err)
	}
	if handoff.RecordSpawn {
		if err := recordLaunchSpawn(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "agm: warning: failed to record successful harness spawn: %v\n", err)
		}
	}
	if err := replaceProcess(
		"/bin/sh",
		[]string{"sh", "-c", handoff.HarnessCommand},
		os.Environ(),
	); err != nil {
		return fmt.Errorf("execute harness command: %w", err)
	}
	return errors.New("harness executor returned unexpectedly")
}

func runExpiry(args []string) error {
	var path, expiresAtValue string
	producerLeaseFD := -1
	set := flag.NewFlagSet(ExpiryProtocol, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&path, "handoff", "", "")
	set.StringVar(&expiresAtValue, "expires-at", "", "")
	set.IntVar(&producerLeaseFD, "producer-lease-fd", -1, "")
	if err := set.Parse(args); err != nil {
		return fmt.Errorf("invalid private handoff expiration request: %w", err)
	}
	if set.NArg() != 0 || path == "" || expiresAtValue == "" {
		return errors.New("invalid private handoff expiration request")
	}
	if err := validateText("handoff", path); err != nil {
		return err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresAtValue)
	if err != nil {
		return errors.New("invalid private handoff expiration deadline")
	}
	now := time.Now()
	if expiresAt.After(now.Add(handoffMaxAge + time.Minute)) {
		return errors.New("private handoff expiration deadline exceeds the maximum lifetime")
	}
	if producerLeaseFD != -1 {
		if producerLeaseFD != 3 {
			return errors.New("invalid private handoff producer lease descriptor")
		}
		lease := os.NewFile(uintptr(producerLeaseFD), "private-handoff-producer-lease")
		if lease == nil {
			return errors.New("open private handoff producer lease")
		}
		defer func() { _ = lease.Close() }()
		return expireDeferredHandoff(path, lease, handoffMaxAge, time.Minute)
	}
	return expireHandoff(path, expiresAt, time.Now, time.Sleep)
}

func runCodex(args []string) error {
	request, err := parseCodex(args)
	if err != nil {
		return err
	}
	environ := CodexEnvironment(os.Environ(), request.SessionName)
	if request.BypassHooks && request.HandoffPath == "" {
		return errors.New("invalid Codex launch request: hook-trust bypass requires a prepared private handoff")
	}
	if request.HandoffPath != "" {
		var binding *codexLaunchBinding
		if request.BypassHooks {
			bound := bindCodexLaunch(request.launch())
			binding = &bound
		}
		handoff, handoffErr := consumeHandoff(
			request.HandoffPath, CodexProtocol, request.HookRoot, binding,
		)
		if handoffErr != nil {
			return handoffErr
		}
		if request.BypassHooks {
			request.HookTrustReason = handoff.CodexLaunch.HookTrustReason
			request.HookTrustActor = handoff.CodexLaunch.HookTrustActor
			request.HookTrustSourceRepo = handoff.CodexLaunch.HookTrustSourceRepo
			request.HookTrustSourceCommit = handoff.CodexLaunch.HookTrustSourceCommit
			request.HookTrustDigest = handoff.CodexLaunch.HookTrustDigest
			request.HookTrustProof = handoff.CodexLaunch.HookTrustProof
		}
		request.OverrideProofs = append([]override.AuthorizationProof(nil), handoff.OverrideProofs...)
		request.RecordSpawn = handoff.RecordSpawn
		environ = CodexEnvironment(handoff.Environment, request.SessionName)
		environ = overlayEnvironment(environ, selectedEnvironment(os.Environ(), paneRuntimeEnvironment))
	}
	// The caller snapshot can come from a process outside the target pane.
	// Keep the child's logical working directory aligned with the validated
	// -C target instead of forwarding a stale caller PWD.
	environ = overlayEnvironment(environ, []string{"PWD=" + request.WorkDir})
	if request.BypassHooks {
		attestation := codexhooks.Attestation{
			SourceRepo:   request.HookTrustSourceRepo,
			SourceCommit: request.HookTrustSourceCommit,
			Digest:       request.HookTrustDigest,
			HookRoot:     request.HookRoot,
		}
		if err := verifyCodexHookTrustAttestation(attestation, request.WorkDir); err != nil {
			return fmt.Errorf("revalidate approved Codex hook source: %w", err)
		}
		environ = overlayEnvironment(environ, []string{"AGM_CODEX_HOOK_ROOT=" + request.HookRoot})
		overrides, overrideErr := codexHookOverrides(
			request.HookRoot, request.HookTrustDigest, request.WorkDir,
		)
		if overrideErr != nil {
			return fmt.Errorf("prepare immutable Codex hook configuration: %w", overrideErr)
		}
		request.ConfigOverrides = overrides
	}
	path, err := lookPathInEnvironment("codex", environ)
	if err != nil {
		return fmt.Errorf("resolve codex executable: %w", err)
	}
	argv := append([]string{"codex"}, request.argv()...)
	// Commit every prepared launch override as one transaction only after all
	// fallible attestation, helper, configuration, and executable checks have
	// succeeded. This is the final userspace boundary before exec.
	if err := commitLaunchOverrideProofs(request.SessionName, request.OverrideProofs...); err != nil {
		return fmt.Errorf("commit Codex launch override transaction: %w", err)
	}
	if request.RecordSpawn {
		if err := recordLaunchSpawn(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "agm: warning: failed to record successful Codex spawn: %v\n", err)
		}
	}
	if err := replaceProcess(path, argv, environ); err != nil {
		return fmt.Errorf("execute codex: %w", err)
	}
	return errors.New("codex executor returned unexpectedly")
}

func runClaude(args []string) error {
	request, err := parseClaude(args)
	if err != nil {
		return err
	}
	parent := os.Environ()
	token := ""
	if request.HandoffPath != "" {
		handoff, handoffErr := consumeHandoff(request.HandoffPath, ClaudeProtocol, "")
		if handoffErr != nil {
			return handoffErr
		}
		expected := bindClaudeLaunch(request.launch())
		if handoff.ClaudeLaunch == nil ||
			!handoff.ClaudeLaunch.matches(expected) {
			return errors.New("private Claude handoff does not authorize the requested launch")
		}
		parent = removeEnvironment(parent, claudeHandoffEnvironment)
		parent = overlayEnvironment(parent, handoff.Environment)
		token = environmentMap(handoff.Environment)[auth.OAuthEnvVar]
		request.OverrideProofs = append([]override.AuthorizationProof(nil), handoff.OverrideProofs...)
		request.RecordSpawn = handoff.RecordSpawn
	} else if request.Binary != "" || len(request.ExtraArgs) != 0 {
		return errors.New("custom Claude executable and arguments require a prepared private handoff")
	}
	if token == "" && !request.DisableOAuth && request.HandoffPath == "" {
		token = resolveClaudeOAuth()
	}
	argv := append([]string{"claude"}, request.argv()...)
	env := ClaudeEnvironment(parent, request.launch(), token)
	if request.WorkDir != "" {
		if err := changeDirectory(request.WorkDir); err != nil {
			return fmt.Errorf("enter Claude working directory: %w", err)
		}
		env = overlayEnvironment(env, []string{"PWD=" + request.WorkDir})
	}
	binary := request.Binary
	if binary == "" {
		binary = "claude"
	}
	path, err := lookPathInEnvironment(binary, env)
	if err != nil {
		return fmt.Errorf("resolve claude executable: %w", err)
	}
	argv[0] = binary
	if err := commitLaunchOverrideProofs(request.SessionName, request.OverrideProofs...); err != nil {
		return fmt.Errorf("commit Claude launch override transaction: %w", err)
	}
	if request.RecordSpawn {
		if err := recordLaunchSpawn(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "agm: warning: failed to record successful Claude spawn: %v\n", err)
		}
	}
	if err := replaceProcess(path, argv, env); err != nil {
		return fmt.Errorf("execute claude: %w", err)
	}
	return errors.New("claude executor returned unexpectedly")
}

type harnessRequest struct {
	HandoffPath string
	SessionName string
}

func parseHarness(args []string) (harnessRequest, error) {
	var request harnessRequest
	set := flag.NewFlagSet(HarnessProtocol, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&request.HandoffPath, "handoff", "", "")
	set.StringVar(&request.SessionName, "session", "", "")
	if err := set.Parse(args); err != nil {
		return harnessRequest{}, fmt.Errorf("invalid private harness launch request: %w", err)
	}
	if set.NArg() != 0 {
		return harnessRequest{}, errors.New("invalid private harness launch request: positional arguments are not allowed")
	}
	if err := validateText("handoff", request.HandoffPath); err != nil {
		return harnessRequest{}, err
	}
	if !filepath.IsAbs(request.HandoffPath) ||
		filepath.Clean(request.HandoffPath) != request.HandoffPath {
		return harnessRequest{}, errors.New("invalid private harness launch request: handoff must be a clean absolute path")
	}
	if err := validateText("session", request.SessionName); err != nil {
		return harnessRequest{}, err
	}
	return request, nil
}

func runAgy(args []string) error {
	request, err := parseAgy(args)
	if err != nil {
		return err
	}
	environ := AgyEnvironment(os.Environ(), request.SessionName)
	var handoff launchHandoff
	if request.HandoffPath != "" {
		consumed, handoffErr := consumeHandoff(request.HandoffPath, AgyProtocol, "")
		if handoffErr != nil {
			return handoffErr
		}
		handoff = consumed
		environ = AgyEnvironment(handoff.Environment, request.SessionName)
		environ = overlayEnvironment(environ, selectedEnvironment(os.Environ(), paneRuntimeEnvironment))
	}
	if err := changeDirectory(request.WorkDir); err != nil {
		return fmt.Errorf("enter AGY working directory: %w", err)
	}
	environ = overlayEnvironment(environ, []string{"PWD=" + request.WorkDir})
	path, err := lookPathInEnvironment("agy", environ)
	if err != nil {
		return fmt.Errorf("resolve agy executable: %w", err)
	}
	argv := append([]string{"agy"}, request.argv()...)
	if err := commitLaunchOverrideProofs(request.SessionName, handoff.OverrideProofs...); err != nil {
		return fmt.Errorf("commit AGY launch override transaction: %w", err)
	}
	if handoff.RecordSpawn {
		if err := recordLaunchSpawn(); err != nil {
			return fmt.Errorf("record AGY launch: %w", err)
		}
	}
	if err := replaceProcess(path, argv, environ); err != nil {
		return fmt.Errorf("execute agy: %w", err)
	}
	return errors.New("agy executor returned unexpectedly")
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type codexRequest struct {
	HandoffPath           string
	SessionName           string
	Model                 string
	WorkDir               string
	Sandbox               string
	Approval              string
	AddDirs               stringList
	ResumeID              string
	Remote                bool
	RemoteResume          bool
	BypassHooks           bool
	HookRoot              string
	HookTrustReason       string
	HookTrustActor        string
	HookTrustSourceRepo   string
	HookTrustSourceCommit string
	HookTrustDigest       string
	HookTrustProof        override.AuthorizationProof
	OverrideProofs        []override.AuthorizationProof
	RecordSpawn           bool
	ConfigOverrides       []string
}

func parseCodex(args []string) (codexRequest, error) {
	var request codexRequest
	set := flag.NewFlagSet(CodexProtocol, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&request.HandoffPath, "handoff", "", "")
	set.StringVar(&request.SessionName, "session", "", "")
	set.StringVar(&request.Model, "model", "", "")
	set.StringVar(&request.WorkDir, "workdir", "", "")
	set.StringVar(&request.Sandbox, "sandbox", "", "")
	set.StringVar(&request.Approval, "approval", "", "")
	set.Var(&request.AddDirs, "add-dir", "")
	set.StringVar(&request.ResumeID, "resume-id", "", "")
	set.BoolVar(&request.Remote, "remote", false, "")
	set.BoolVar(&request.RemoteResume, "remote-resume", false, "")
	set.BoolVar(&request.BypassHooks, "bypass-hook-trust", false, "")
	set.StringVar(&request.HookRoot, "hook-root", "", "")
	if err := set.Parse(args); err != nil {
		return codexRequest{}, fmt.Errorf("invalid Codex launch request: %w", err)
	}
	if set.NArg() != 0 {
		return codexRequest{}, errors.New("invalid Codex launch request: positional arguments are not allowed")
	}
	if err := validateCodexRequest(request); err != nil {
		return codexRequest{}, err
	}
	return request, nil
}

func validateCodexRequest(request codexRequest) error {
	for _, field := range []struct{ name, value string }{
		{"session", request.SessionName}, {"model", request.Model}, {"workdir", request.WorkDir},
	} {
		if err := validateText(field.name, field.value); err != nil {
			return err
		}
	}
	if !oneOf(request.Sandbox, "read-only", "workspace-write", "danger-full-access") {
		return fmt.Errorf("invalid Codex sandbox %q", request.Sandbox)
	}
	if request.Approval != "" && !oneOf(request.Approval, "untrusted", "on-request", "never") {
		return fmt.Errorf("invalid Codex approval policy %q", request.Approval)
	}
	if err := validateTextList("add-dir", request.AddDirs); err != nil {
		return err
	}
	if request.Remote && request.ResumeID == "" {
		return errors.New("invalid Codex launch request: remote resume requires a session id")
	}
	if request.RemoteResume && !request.Remote {
		return errors.New("invalid Codex launch request: remote resume marker requires remote launch")
	}
	for _, field := range []struct{ name, value string }{
		{"resume-id", request.ResumeID}, {"handoff", request.HandoffPath}, {"hook-root", request.HookRoot},
	} {
		if err := validateOptionalText(field.name, field.value); err != nil {
			return err
		}
	}
	if request.BypassHooks {
		if !filepath.IsAbs(request.HookRoot) || filepath.Clean(request.HookRoot) != request.HookRoot {
			return errors.New("invalid Codex launch request: hook root must be a clean absolute path")
		}
	} else if request.HookRoot != "" {
		return errors.New("invalid Codex launch request: hook root requires hook-trust bypass")
	}
	return nil
}

func (r codexRequest) argv() []string {
	args := make([]string, 0, 14)
	if r.Remote {
		args = append(args, "resume", "--remote", "unix://")
		if r.RemoteResume {
			args = append(args, "-c", remoteCodexReasoningEffort)
		}
	}
	args = append(args, "-m", r.Model, "-C", r.WorkDir, "-s", r.Sandbox)
	for _, dir := range r.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	for _, override := range r.ConfigOverrides {
		args = append(args, "-c", override)
	}
	if r.Approval != "" {
		args = append(args, "-a", r.Approval)
	}
	if r.ResumeID != "" {
		if !r.Remote {
			args = append(args, "resume")
		}
		args = append(args, r.ResumeID)
	}
	return args
}

func (r codexRequest) launch() CodexLaunch {
	return CodexLaunch{
		HandoffPath:     r.HandoffPath,
		SessionName:     r.SessionName,
		Model:           r.Model,
		WorkDir:         r.WorkDir,
		Sandbox:         r.Sandbox,
		Approval:        r.Approval,
		AddDirs:         append([]string(nil), r.AddDirs...),
		ResumeID:        r.ResumeID,
		Remote:          r.Remote,
		RemoteResume:    r.RemoteResume,
		BypassHookTrust: r.BypassHooks,
		HookRoot:        r.HookRoot,
	}
}

type claudeRequest struct {
	HandoffPath      string
	Binary           string
	SessionName      string
	SessionID        string
	ResumeID         string
	WorkDir          string
	Model            string
	AddDirs          stringList
	AutoMode         bool
	Permission       string
	MaxBudgetUSD     float64
	ExtraArgs        stringList
	DisableOAuth     bool
	ForwardTelemetry bool
	OverrideProofs   []override.AuthorizationProof
	RecordSpawn      bool
}

func parseClaude(args []string) (claudeRequest, error) {
	var request claudeRequest
	var maxBudget string
	set := flag.NewFlagSet(ClaudeProtocol, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&request.HandoffPath, "handoff", "", "")
	set.StringVar(&request.Binary, "binary", "", "")
	set.StringVar(&request.SessionName, "session", "", "")
	set.StringVar(&request.SessionID, "session-id", "", "")
	set.StringVar(&request.ResumeID, "resume-id", "", "")
	set.StringVar(&request.WorkDir, "workdir", "", "")
	set.StringVar(&request.Model, "model", "", "")
	set.Var(&request.AddDirs, "add-dir", "")
	set.BoolVar(&request.AutoMode, "auto-mode", false, "")
	set.StringVar(&request.Permission, "permission", "", "")
	set.StringVar(&maxBudget, "max-budget-usd", "", "")
	set.Var(&request.ExtraArgs, "extra-arg", "")
	set.BoolVar(&request.DisableOAuth, "disable-oauth", false, "")
	set.BoolVar(&request.ForwardTelemetry, "forward-telemetry", false, "")
	if err := set.Parse(args); err != nil {
		return claudeRequest{}, fmt.Errorf("invalid Claude launch request: %w", err)
	}
	if set.NArg() != 0 {
		return claudeRequest{}, errors.New("invalid Claude launch request: positional arguments are not allowed")
	}
	if err := validateClaudeRequest(&request, maxBudget); err != nil {
		return claudeRequest{}, err
	}
	return request, nil
}

func validateClaudeRequest(r *claudeRequest, maxBudget string) error {
	if err := validateOptionalText("handoff", r.HandoffPath); err != nil {
		return err
	}
	if err := validateText("session", r.SessionName); err != nil {
		return err
	}
	for _, field := range []struct{ name, value string }{
		{"binary", r.Binary}, {"session-id", r.SessionID}, {"resume-id", r.ResumeID},
		{"workdir", r.WorkDir}, {"model", r.Model},
	} {
		if err := validateOptionalText(field.name, field.value); err != nil {
			return err
		}
	}
	if err := validateTextList("add-dir", r.AddDirs); err != nil {
		return err
	}
	if err := validateTextList("extra-arg", r.ExtraArgs); err != nil {
		return err
	}
	if r.Permission != "" && !oneOf(r.Permission, "auto", "plan", "default") {
		return fmt.Errorf("invalid Claude permission mode %q", r.Permission)
	}
	budget, err := parsePositiveBudget(maxBudget)
	if err != nil {
		return err
	}
	r.MaxBudgetUSD = budget
	return nil
}

func parsePositiveBudget(value string) (float64, error) {
	if value == "" {
		return 0, nil
	}
	budget, err := strconv.ParseFloat(value, 64)
	if err != nil || budget <= 0 || math.IsNaN(budget) || math.IsInf(budget, 0) {
		return 0, fmt.Errorf("invalid Claude max budget %q", value)
	}
	return budget, nil
}

func (r claudeRequest) argv() []string {
	args := make([]string, 0, 12)
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	for _, dir := range r.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if r.AutoMode {
		args = append(args, "--enable-auto-mode")
	}
	if r.Permission != "" {
		args = append(args, "--permission-mode", r.Permission)
	}
	if r.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", r.MaxBudgetUSD))
	}
	if r.ResumeID != "" {
		args = append(args, "--resume", r.ResumeID)
	}
	args = append(args, r.ExtraArgs...)
	return args
}

func (r claudeRequest) launch() ClaudeLaunch {
	return ClaudeLaunch{
		HandoffPath:      r.HandoffPath,
		Binary:           r.Binary,
		SessionName:      r.SessionName,
		SessionID:        r.SessionID,
		ResumeID:         r.ResumeID,
		WorkDir:          r.WorkDir,
		Model:            r.Model,
		AddDirs:          append([]string(nil), r.AddDirs...),
		AutoMode:         r.AutoMode,
		Permission:       r.Permission,
		MaxBudgetUSD:     r.MaxBudgetUSD,
		ExtraArgs:        append([]string(nil), r.ExtraArgs...),
		DisableOAuth:     r.DisableOAuth,
		ForwardTelemetry: r.ForwardTelemetry,
	}
}

type agyRequest struct {
	HandoffPath    string
	SessionName    string
	Model          string
	WorkDir        string
	Permission     string
	AddDirs        stringList
	ConversationID string
}

func parseAgy(args []string) (agyRequest, error) {
	var request agyRequest
	set := flag.NewFlagSet(AgyProtocol, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&request.HandoffPath, "handoff", "", "")
	set.StringVar(&request.SessionName, "session", "", "")
	set.StringVar(&request.Model, "model", "", "")
	set.StringVar(&request.WorkDir, "workdir", "", "")
	set.StringVar(&request.Permission, "permission", "", "")
	set.StringVar(&request.ConversationID, "conversation", "", "")
	set.Var(&request.AddDirs, "add-dir", "")
	if err := set.Parse(args); err != nil {
		return agyRequest{}, fmt.Errorf("invalid AGY launch request: %w", err)
	}
	if set.NArg() != 0 {
		return agyRequest{}, errors.New("invalid AGY launch request: positional arguments are not allowed")
	}
	if err := validateAgyRequest(request); err != nil {
		return agyRequest{}, err
	}
	return request, nil
}

func validateAgyRequest(request agyRequest) error {
	for _, field := range []struct{ name, value string }{
		{"session", request.SessionName}, {"workdir", request.WorkDir},
	} {
		if err := validateText(field.name, field.value); err != nil {
			return err
		}
	}
	if request.Permission != "" && !oneOf(request.Permission, "auto", "default", "plan", "dangerously-skip-permissions") {
		return fmt.Errorf("invalid AGY permission mode %q", request.Permission)
	}
	if err := validateTextList("add-dir", request.AddDirs); err != nil {
		return err
	}
	for _, field := range []struct{ name, value string }{
		{"handoff", request.HandoffPath}, {"conversation", request.ConversationID},
	} {
		if err := validateOptionalText(field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

func (r agyRequest) argv() []string {
	args := make([]string, 0, 8+len(r.AddDirs)*2)
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	switch r.Permission {
	case "auto", "dangerously-skip-permissions":
		args = append(args, "--dangerously-skip-permissions")
	case "plan":
		args = append(args, "--mode", "plan")
	}
	if r.ConversationID != "" {
		args = append(args, "--conversation", r.ConversationID)
	}
	for _, dir := range r.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	return args
}

func validateText(name, value string) error {
	if value == "" {
		return fmt.Errorf("invalid harness launch request: %s is required", name)
	}
	return tmux.ValidatePastedText(name, value)
}

func validateOptionalText(name, value string) error {
	return tmux.ValidatePastedText(name, value)
}

func validateTextList(name string, values []string) error {
	for _, value := range values {
		if err := validateText(name, value); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePastedText rejects terminal controls and invalid UTF-8 before a
// caller-controlled value is interpolated into a command pasted into tmux.
// Empty optional fields are permitted.
func ValidatePastedText(name, value string) error {
	return validateOptionalText(name, value)
}

// ValidatePastedTextList applies ValidatePastedText to a repeated field.
func ValidatePastedTextList(name string, values []string) error {
	return validateTextList(name, values)
}

func oneOf(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

// CodexEnvironment applies the deny-by-default environment contract for the
// interactive Codex child. The input is explicit so BDD and regression tests
// never need to inspect the developer's real environment.
func CodexEnvironment(parent []string, sessionName string) []string {
	values := environmentMap(parent)
	env := make([]string, 0, len(codexAllowedEnvironment)+1)
	for _, name := range codexAllowedEnvironment {
		if value, ok := values[name]; ok {
			env = append(env, name+"="+value)
		}
	}
	env = append(env, "AGM_SESSION_NAME="+sessionName)
	return env
}

// ClaudeEnvironment injects runtime-only Claude metadata and OAuth without
// placing either credential values or telemetry endpoints in command text.
func ClaudeEnvironment(parent []string, launch ClaudeLaunch, oauthToken string) []string {
	parentValues := environmentMap(parent)
	remove := map[string]bool{
		"CLAUDECODE":                          true,
		"AGM_SESSION_NAME":                    true,
		"ENGRAM_SESSION_ID":                   true,
		"CLAUDE_CODE_ENABLE_TELEMETRY":        true,
		"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": true,
		"OTEL_TRACES_EXPORTER":                true,
		"OTEL_EXPORTER_OTLP_PROTOCOL":         true,
		"OTEL_EXPORTER_OTLP_ENDPOINT":         true,
		"OTEL_EXPORTER_OTLP_HEADERS":          true,
		auth.OAuthEnvVar:                      true,
	}
	if oauthToken != "" {
		remove["ANTHROPIC_API_KEY"] = true
	}
	env := filterEnvironment(parent, remove)
	if oauthToken != "" && !launch.DisableOAuth {
		env = append(env, auth.OAuthEnvVar+"="+oauthToken)
	}
	env = append(env, "AGM_SESSION_NAME="+launch.SessionName)
	if launch.ForwardTelemetry {
		if launch.SessionID != "" {
			env = append(env, "ENGRAM_SESSION_ID="+launch.SessionID)
		}
		if endpoint := parentValues["OTEL_EXPORTER_OTLP_ENDPOINT"]; endpoint != "" {
			env = append(env,
				"OTEL_EXPORTER_OTLP_ENDPOINT="+endpoint,
				"CLAUDE_CODE_ENABLE_TELEMETRY=1",
				"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1",
				"OTEL_TRACES_EXPORTER=otlp",
				"OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
			)
			if headers := parentValues["OTEL_EXPORTER_OTLP_HEADERS"]; headers != "" {
				env = append(env, "OTEL_EXPORTER_OTLP_HEADERS="+headers)
			}
		}
	}
	return env
}

// AgyEnvironment applies the deny-by-default environment contract for AGY.
// Google and Gemini authentication are caller-scoped, so a restarted AGY
// supervisor cannot inherit a stale long-lived tmux server environment.
func AgyEnvironment(parent []string, sessionName string) []string {
	values := environmentMap(parent)
	env := make([]string, 0, len(agyAllowedEnvironment)+1)
	for _, name := range agyAllowedEnvironment {
		if value, ok := values[name]; ok {
			env = append(env, name+"="+value)
		}
	}
	env = append(env, "AGM_SESSION_NAME="+sessionName)
	return env
}

func environmentMap(environ []string) map[string]string {
	values := make(map[string]string, len(environ))
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" {
			values[name] = value
		}
	}
	return values
}

func filterEnvironment(environ []string, remove map[string]bool) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		if ok && !remove[name] {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
