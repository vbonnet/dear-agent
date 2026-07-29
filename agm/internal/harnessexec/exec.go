// Package harnessexec owns AGM's private, argument-only launch protocol for
// interactive harnesses whose credentials must not appear in tmux commands.
package harnessexec

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/shellquote"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
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
	// ExpiryProtocol is the private cleanup protocol used by a detached,
	// credential-free helper to expire an unconsumed launch handoff.
	ExpiryProtocol = "__expire-harness-handoff"
)

var (
	lookPathInEnvironment = resolveExecutableInEnvironment
	replaceProcess        = syscall.Exec
	changeDirectory       = os.Chdir
	resolveClaudeOAuth    = auth.ResolveOAuthToken
)

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
	"WORKSPACE",
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
	Persistent             bool
	DeferUntilProducerExit bool
}

// ClaudeLaunch contains the non-secret metadata AGM may place in a tmux launch
// command. OAuth values are resolved only inside the executor.
type ClaudeLaunch struct {
	Executable             string
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
	DisableOAuth           bool
	ForwardTelemetry       bool
	Persistent             bool
	DeferUntilProducerExit bool
}

// IsProtocol reports whether arg names one of the private launch protocols.
func IsProtocol(arg string) bool {
	return arg == CodexProtocol || arg == ClaudeProtocol || arg == ExpiryProtocol
}

// BuildCodexCommand returns the token-free shell command pasted into tmux.
func BuildCodexCommand(launch CodexLaunch) string {
	var b strings.Builder
	b.WriteString(privateCommandPrefix(launch.Executable, CodexProtocol))
	if launch.HandoffPath != "" {
		appendShellFlag(&b, "--handoff", launch.HandoffPath)
	}
	appendShellFlag(&b, "--session", launch.SessionName)
	appendShellFlag(&b, "--model", launch.Model)
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
	case ExpiryProtocol:
		return runExpiry(args)
	default:
		return fmt.Errorf("unsupported private harness protocol %q", protocol)
	}
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
	if request.HandoffPath != "" {
		handoff, handoffErr := consumeHandoff(request.HandoffPath, CodexProtocol)
		if handoffErr != nil {
			return handoffErr
		}
		environ = CodexEnvironment(handoff.Environment, request.SessionName)
		environ = overlayEnvironment(environ, selectedEnvironment(os.Environ(), paneRuntimeEnvironment))
	}
	// The caller snapshot can come from a process outside the target pane.
	// Keep the child's logical working directory aligned with the validated
	// -C target instead of forwarding a stale caller PWD.
	environ = overlayEnvironment(environ, []string{"PWD=" + request.WorkDir})
	path, err := lookPathInEnvironment("codex", environ)
	if err != nil {
		return fmt.Errorf("resolve codex executable: %w", err)
	}
	argv := append([]string{"codex"}, request.argv()...)
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
		handoff, handoffErr := consumeHandoff(request.HandoffPath, ClaudeProtocol)
		if handoffErr != nil {
			return handoffErr
		}
		parent = removeEnvironment(parent, claudeHandoffEnvironment)
		parent = overlayEnvironment(parent, handoff.Environment)
		token = environmentMap(handoff.Environment)[auth.OAuthEnvVar]
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
	path, err := lookPathInEnvironment("claude", env)
	if err != nil {
		return fmt.Errorf("resolve claude executable: %w", err)
	}
	if err := replaceProcess(path, argv, env); err != nil {
		return fmt.Errorf("execute claude: %w", err)
	}
	return errors.New("claude executor returned unexpectedly")
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type codexRequest struct {
	HandoffPath string
	SessionName string
	Model       string
	WorkDir     string
	Sandbox     string
	Approval    string
	AddDirs     stringList
	ResumeID    string
	Remote      bool
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
	for _, field := range []struct{ name, value string }{
		{"resume-id", request.ResumeID}, {"handoff", request.HandoffPath},
	} {
		if err := validateOptionalText(field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

func (r codexRequest) argv() []string {
	args := make([]string, 0, 12+len(r.AddDirs)*2)
	if r.Remote {
		args = append(args, "resume", "--remote", "unix://")
	}
	args = append(args, "-m", r.Model, "-C", r.WorkDir, "-s", r.Sandbox)
	for _, dir := range r.AddDirs {
		args = append(args, "--add-dir", dir)
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

type claudeRequest struct {
	HandoffPath      string
	SessionName      string
	SessionID        string
	ResumeID         string
	WorkDir          string
	Model            string
	AddDirs          stringList
	AutoMode         bool
	Permission       string
	MaxBudgetUSD     float64
	DisableOAuth     bool
	ForwardTelemetry bool
}

func parseClaude(args []string) (claudeRequest, error) {
	var request claudeRequest
	var maxBudget string
	set := flag.NewFlagSet(ClaudeProtocol, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&request.HandoffPath, "handoff", "", "")
	set.StringVar(&request.SessionName, "session", "", "")
	set.StringVar(&request.SessionID, "session-id", "", "")
	set.StringVar(&request.ResumeID, "resume-id", "", "")
	set.StringVar(&request.WorkDir, "workdir", "", "")
	set.StringVar(&request.Model, "model", "", "")
	set.Var(&request.AddDirs, "add-dir", "")
	set.BoolVar(&request.AutoMode, "auto-mode", false, "")
	set.StringVar(&request.Permission, "permission", "", "")
	set.StringVar(&maxBudget, "max-budget-usd", "", "")
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
		{"session-id", r.SessionID}, {"resume-id", r.ResumeID},
		{"workdir", r.WorkDir}, {"model", r.Model},
	} {
		if err := validateOptionalText(field.name, field.value); err != nil {
			return err
		}
	}
	if err := validateTextList("add-dir", r.AddDirs); err != nil {
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
	args := make([]string, 0, 12+len(r.AddDirs)*2)
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
	return args
}

func (r claudeRequest) launch() ClaudeLaunch {
	return ClaudeLaunch{
		HandoffPath:      r.HandoffPath,
		SessionName:      r.SessionName,
		SessionID:        r.SessionID,
		ResumeID:         r.ResumeID,
		WorkDir:          r.WorkDir,
		Model:            r.Model,
		AddDirs:          append([]string(nil), r.AddDirs...),
		AutoMode:         r.AutoMode,
		Permission:       r.Permission,
		MaxBudgetUSD:     r.MaxBudgetUSD,
		DisableOAuth:     r.DisableOAuth,
		ForwardTelemetry: r.ForwardTelemetry,
	}
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
