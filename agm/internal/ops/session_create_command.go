package ops

import (
	"fmt"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// HarnessLaunchSpec is the harness-neutral launch contract used by every
// creation surface. Surface adapters may still own interactive readiness and
// presentation, but they must not assemble a second launch command.
type HarnessLaunchSpec struct {
	Harness          string
	Model            string
	SessionName      string
	SessionID        string
	WorkDir          string
	Persistent       bool
	PermissionMode   string
	DisableAutoMode  bool
	DisableOAuth     bool
	OAuthToken       string
	MaxBudgetUSD     float64
	ExtraAddDirs     []string
	ForwardTelemetry bool
	Codex            *manifest.Codex
	Pi               *manifest.Pi
	PiLaunchID       string
	PiExtension      string
	PiPolicyJSON     string
	PiPolicyFile     string
}

// HarnessLaunchCommand is the command plus the startup-policy outcome needed
// by post-create handling.
type HarnessLaunchCommand struct {
	Command              string
	ModeAppliedAtStartup bool
}

// BuildHarnessLaunchCommand builds the one canonical shell command for a
// supported AGM harness.
func BuildHarnessLaunchCommand(spec HarnessLaunchSpec) HarnessLaunchCommand {
	exitSuffix := launchparity.ExitSuffix(spec.Persistent)
	switch agent.NormalizeHarnessName(spec.Harness) {
	case "claude-code":
		return buildClaudeLaunchCommand(spec, exitSuffix)
	case "codex-cli":
		return buildCodexLaunchCommand(spec, exitSuffix)
	case "agy":
		return buildAgyLaunchCommand(spec, exitSuffix)
	case "pi-cli":
		return buildPiLaunchCommand(spec)
	case "opencode-cli":
		return HarnessLaunchCommand{Command: fmt.Sprintf("cd %s && opencode attach%s", launchparity.ShellQuote(spec.WorkDir), exitSuffix)}
	case "gemini-cli":
		resolvedModel := agent.ResolveModelFullName("gemini-cli", spec.Model)
		return HarnessLaunchCommand{Command: fmt.Sprintf("gemini -m %s%s", launchparity.ShellQuote(resolvedModel), exitSuffix)}
	default:
		return HarnessLaunchCommand{Command: fmt.Sprintf("echo %s && exit 1", launchparity.ShellQuote("Unknown harness: "+spec.Harness))}
	}
}

func buildPiLaunchCommand(spec HarnessLaunchSpec) HarnessLaunchCommand {
	nativeID := spec.SessionID
	sessionDir := ""
	codingAgentDir := ""
	if spec.Pi != nil {
		if spec.Pi.SessionID != "" {
			nativeID = spec.Pi.SessionID
		}
		sessionDir = spec.Pi.SessionDir
		codingAgentDir = spec.Pi.CodingAgentDir
	}
	launchID := spec.PiLaunchID
	if launchID == "" {
		launchID = launchparity.NewPiLaunchID()
	}
	command := launchparity.BuildPiCommand(launchparity.PiCommandSpec{
		WorkDir: spec.WorkDir, ResolvedModel: agent.ResolveModelFullName("pi-cli", spec.Model),
		SessionName: spec.SessionName, SessionID: nativeID, LaunchID: launchID, SessionDir: sessionDir,
		CodingAgentDir: codingAgentDir,
		PermissionMode: spec.PermissionMode, PermissionExtension: spec.PiExtension,
		PermissionPolicyFile: spec.PiPolicyFile, Persistent: spec.Persistent,
	})
	return HarnessLaunchCommand{Command: command.Command, ModeAppliedAtStartup: command.ModeAppliedAtStartup}
}

func buildClaudeLaunchCommand(spec HarnessLaunchSpec, exitSuffix string) HarnessLaunchCommand {
	resolvedModel := agent.ResolveModelFullName("claude-code", spec.Model)
	oauthToken := spec.OAuthToken
	if oauthToken == "" && !spec.DisableOAuth {
		oauthToken = auth.ResolveOAuthToken()
	}
	envUnset := "-u CLAUDECODE"
	oauthArg := ""
	if oauthToken != "" {
		envUnset += " -u ANTHROPIC_API_KEY"
		oauthArg = " CLAUDE_CODE_OAUTH_TOKEN=" + launchparity.ShellQuote(oauthToken)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "env %s AGM_SESSION_NAME=%s", envUnset, launchparity.ShellQuote(spec.SessionName))
	if spec.ForwardTelemetry {
		appendTelemetryEnv(&b, spec.SessionID)
	}
	b.WriteString(oauthArg)
	fmt.Fprintf(&b, " claude --model %s --add-dir %s", launchparity.ShellQuote(resolvedModel), launchparity.ShellQuote(spec.WorkDir))
	if !spec.DisableAutoMode {
		b.WriteString(" --enable-auto-mode")
	}
	for _, dir := range spec.ExtraAddDirs {
		fmt.Fprintf(&b, " --add-dir %s", launchparity.ShellQuote(dir))
	}
	modeApplied := false
	if spec.PermissionMode == "auto" || spec.PermissionMode == "plan" || spec.PermissionMode == "default" {
		fmt.Fprintf(&b, " --permission-mode %s", spec.PermissionMode)
		modeApplied = true
	}
	if spec.MaxBudgetUSD > 0 {
		fmt.Fprintf(&b, " --max-budget-usd %.2f", spec.MaxBudgetUSD)
	}
	b.WriteString(exitSuffix)
	return HarnessLaunchCommand{Command: b.String(), ModeAppliedAtStartup: modeApplied}
}

func buildCodexLaunchCommand(spec HarnessLaunchSpec, exitSuffix string) HarnessLaunchCommand {
	resolvedModel := agent.ResolveModelFullName("codex-cli", spec.Model)
	sandboxMode := launchparity.CodexSandboxMode(spec.PermissionMode)
	var b strings.Builder
	fmt.Fprintf(&b, "env -u CLAUDECODE AGM_SESSION_NAME=%s codex", launchparity.ShellQuote(spec.SessionName))
	if spec.Codex != nil && spec.Codex.SessionID != "" {
		b.WriteString(" resume --remote unix://")
	}
	fmt.Fprintf(&b, " -m %s -C %s -s %s", launchparity.ShellQuote(resolvedModel), launchparity.ShellQuote(spec.WorkDir), sandboxMode)
	for _, dir := range spec.ExtraAddDirs {
		fmt.Fprintf(&b, " --add-dir %s", launchparity.ShellQuote(dir))
	}
	modeApplied := false
	if flag := launchparity.CodexPermissionModeFlag(spec.PermissionMode); flag != "" {
		b.WriteString(" " + flag)
		modeApplied = true
	}
	if spec.Codex != nil && spec.Codex.SessionID != "" {
		fmt.Fprintf(&b, " %s", launchparity.ShellQuote(spec.Codex.SessionID))
	}
	b.WriteString(exitSuffix)
	return HarnessLaunchCommand{Command: b.String(), ModeAppliedAtStartup: modeApplied}
}

func buildAgyLaunchCommand(spec HarnessLaunchSpec, _ string) HarnessLaunchCommand {
	return buildAgyCommand(spec, "")
}

// BuildAgyResumeCommand builds AGY's native cold-resume command from the same
// model, permission, directory, quoting, and persistence policy used at create.
func BuildAgyResumeCommand(spec HarnessLaunchSpec, conversationID string) HarnessLaunchCommand {
	return buildAgyCommand(spec, conversationID)
}

func buildAgyCommand(spec HarnessLaunchSpec, conversationID string) HarnessLaunchCommand {
	resolvedModel := agent.ResolveModelFullName("agy", spec.Model)
	command := launchparity.BuildAgyCommand(launchparity.AgyCommandSpec{
		WorkDir:        spec.WorkDir,
		ResolvedModel:  resolvedModel,
		PermissionMode: spec.PermissionMode,
		ConversationID: conversationID,
		ExtraAddDirs:   spec.ExtraAddDirs,
		Persistent:     spec.Persistent,
	})
	return HarnessLaunchCommand{Command: command.Command, ModeAppliedAtStartup: command.ModeAppliedAtStartup}
}

func appendTelemetryEnv(b *strings.Builder, sessionID string) {
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		fmt.Fprintf(b, " OTEL_EXPORTER_OTLP_ENDPOINT=%s", launchparity.ShellQuote(endpoint))
		b.WriteString(" CLAUDE_CODE_ENABLE_TELEMETRY=1")
		b.WriteString(" CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1")
		b.WriteString(" OTEL_TRACES_EXPORTER=otlp")
		b.WriteString(" OTEL_EXPORTER_OTLP_PROTOCOL=grpc")
	}
	if sessionID != "" {
		fmt.Fprintf(b, " ENGRAM_SESSION_ID=%s", launchparity.ShellQuote(sessionID))
	}
}
