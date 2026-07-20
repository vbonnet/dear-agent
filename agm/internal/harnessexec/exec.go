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
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode"

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
)

var (
	lookPath           = exec.LookPath
	replaceProcess     = syscall.Exec
	resolveClaudeOAuth = auth.ResolveOAuthToken
)

// CodexLaunch contains the non-secret metadata AGM may place in a tmux launch
// command. The executor constructs the real Codex argv after parsing it.
type CodexLaunch struct {
	SessionName string
	Model       string
	WorkDir     string
	Sandbox     string
	Approval    string
	AddDirs     []string
	ResumeID    string
	Remote      bool
	Persistent  bool
}

// ClaudeLaunch contains the non-secret metadata AGM may place in a tmux launch
// command. OAuth values are resolved only inside the executor.
type ClaudeLaunch struct {
	SessionName      string
	SessionID        string
	Model            string
	AddDirs          []string
	AutoMode         bool
	Permission       string
	MaxBudgetUSD     float64
	DisableOAuth     bool
	ForwardTelemetry bool
	Persistent       bool
}

// IsProtocol reports whether arg names one of the private launch protocols.
func IsProtocol(arg string) bool {
	return arg == CodexProtocol || arg == ClaudeProtocol
}

// BuildCodexCommand returns the token-free shell command pasted into tmux.
func BuildCodexCommand(launch CodexLaunch) string {
	var b strings.Builder
	b.WriteString("agm " + CodexProtocol)
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
	b.WriteString("agm " + ClaudeProtocol)
	appendShellFlag(&b, "--session", launch.SessionName)
	if launch.SessionID != "" {
		appendShellFlag(&b, "--session-id", launch.SessionID)
	}
	appendShellFlag(&b, "--model", launch.Model)
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
	b.WriteString(shellQuote(value))
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// Run validates a private protocol request and replaces the current AGM
// process with the fixed harness executable. A successful call does not return.
func Run(protocol string, args []string) error {
	switch protocol {
	case CodexProtocol:
		request, err := parseCodex(args)
		if err != nil {
			return err
		}
		path, err := lookPath("codex")
		if err != nil {
			return fmt.Errorf("resolve codex executable: %w", err)
		}
		argv := append([]string{"codex"}, request.argv()...)
		if err := replaceProcess(path, argv, CodexEnvironment(os.Environ(), request.SessionName)); err != nil {
			return fmt.Errorf("execute codex: %w", err)
		}
		return errors.New("codex executor returned unexpectedly")
	case ClaudeProtocol:
		request, err := parseClaude(args)
		if err != nil {
			return err
		}
		path, err := lookPath("claude")
		if err != nil {
			return fmt.Errorf("resolve claude executable: %w", err)
		}
		token := ""
		if !request.DisableOAuth {
			token = resolveClaudeOAuth()
		}
		argv := append([]string{"claude"}, request.argv()...)
		env := ClaudeEnvironment(os.Environ(), request.launch(), token)
		if err := replaceProcess(path, argv, env); err != nil {
			return fmt.Errorf("execute claude: %w", err)
		}
		return errors.New("claude executor returned unexpectedly")
	default:
		return fmt.Errorf("unsupported private harness protocol %q", protocol)
	}
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type codexRequest struct {
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
	if err := validateText("session", request.SessionName); err != nil {
		return codexRequest{}, err
	}
	if err := validateText("model", request.Model); err != nil {
		return codexRequest{}, err
	}
	if err := validateText("workdir", request.WorkDir); err != nil {
		return codexRequest{}, err
	}
	if !oneOf(request.Sandbox, "read-only", "workspace-write", "danger-full-access") {
		return codexRequest{}, fmt.Errorf("invalid Codex sandbox %q", request.Sandbox)
	}
	if request.Approval != "" && !oneOf(request.Approval, "untrusted", "on-request", "never") {
		return codexRequest{}, fmt.Errorf("invalid Codex approval policy %q", request.Approval)
	}
	for _, dir := range request.AddDirs {
		if err := validateText("add-dir", dir); err != nil {
			return codexRequest{}, err
		}
	}
	if request.Remote && request.ResumeID == "" {
		return codexRequest{}, errors.New("invalid Codex launch request: remote resume requires a session id")
	}
	if request.ResumeID != "" {
		if err := validateText("resume-id", request.ResumeID); err != nil {
			return codexRequest{}, err
		}
	}
	return request, nil
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
	SessionName      string
	SessionID        string
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
	set.StringVar(&request.SessionName, "session", "", "")
	set.StringVar(&request.SessionID, "session-id", "", "")
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
	if err := validateText("session", r.SessionName); err != nil {
		return err
	}
	if err := validateText("model", r.Model); err != nil {
		return err
	}
	if r.SessionID != "" {
		if err := validateText("session-id", r.SessionID); err != nil {
			return err
		}
	}
	for _, dir := range r.AddDirs {
		if err := validateText("add-dir", dir); err != nil {
			return err
		}
	}
	if r.Permission != "" && !oneOf(r.Permission, "auto", "plan", "default") {
		return fmt.Errorf("invalid Claude permission mode %q", r.Permission)
	}
	if maxBudget != "" {
		budget, err := strconv.ParseFloat(maxBudget, 64)
		if err != nil || budget <= 0 || math.IsNaN(budget) || math.IsInf(budget, 0) {
			return fmt.Errorf("invalid Claude max budget %q", maxBudget)
		}
		r.MaxBudgetUSD = budget
	}
	return nil
}

func (r claudeRequest) argv() []string {
	args := []string{"--model", r.Model}
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
	return args
}

func (r claudeRequest) launch() ClaudeLaunch {
	return ClaudeLaunch{
		SessionName:      r.SessionName,
		SessionID:        r.SessionID,
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
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("invalid harness launch request: %s contains control characters", name)
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

// CodexEnvironment applies the deny-by-default environment contract for the
// interactive Codex child. The input is explicit so BDD and regression tests
// never need to inspect the developer's real environment.
func CodexEnvironment(parent []string, sessionName string) []string {
	values := environmentMap(parent)
	allowed := []string{
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
		"AGM_SESSION_BACKEND", "WORKSPACE",
	}
	env := make([]string, 0, len(allowed)+1)
	for _, name := range allowed {
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
	remove := map[string]bool{
		"CLAUDECODE":                          true,
		"AGM_SESSION_NAME":                    true,
		"ENGRAM_SESSION_ID":                   true,
		"CLAUDE_CODE_ENABLE_TELEMETRY":        true,
		"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": true,
		"OTEL_TRACES_EXPORTER":                true,
		"OTEL_EXPORTER_OTLP_PROTOCOL":         true,
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
		if _, ok := environmentMap(parent)["OTEL_EXPORTER_OTLP_ENDPOINT"]; ok {
			env = append(env,
				"CLAUDE_CODE_ENABLE_TELEMETRY=1",
				"CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1",
				"OTEL_TRACES_EXPORTER=otlp",
				"OTEL_EXPORTER_OTLP_PROTOCOL=grpc",
			)
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
