package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// CreateSessionRequest defines the input for creating a new AGM session.
type CreateSessionRequest struct {
	// Cwd is the working directory for the session (required).
	Cwd string `json:"cwd"`

	// Prompt is the initial prompt to send after the session starts (required).
	Prompt string `json:"prompt"`

	// Title overrides the auto-generated session name. If empty, a name is
	// derived from the cwd directory name.
	Title string `json:"title,omitempty"`

	// Model is the model to use (e.g. "sonnet", "opus"). Defaults to the
	// selected harness's default model, or "sonnet" when the harness requires
	// interactive model selection.
	Model string `json:"model,omitempty"`

	// Harness is the agent harness (default: "claude-code").
	Harness string `json:"harness,omitempty"`

	// Persistent omits the "&&  exit" suffix from the harness launch command.
	// Use for long-lived supervisor sessions (vroom-meta-orchestrator, etc.)
	// that must survive their Claude turn/loop ending. One-shot workers should
	// leave this false so the shell exits cleanly when the task completes.
	Persistent bool `json:"persistent,omitempty"`
}

// CreateSessionResult is the output of CreateSession.
type CreateSessionResult struct {
	Operation string `json:"operation"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model"`
	Harness   string `json:"harness"`
	Created   bool   `json:"created"`
}

// createSessionParams holds the validated+defaulted parameters for session creation.
type createSessionParams struct {
	name       string
	harness    string
	model      string
	persistent bool
}

// isSafeNameRune returns true for characters safe in tmux session names and shell arguments.
func isSafeNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

// resolveSessionName derives a safe session name from the cwd base, or validates user-provided title.
func resolveSessionName(title, cwd string) (string, error) {
	if title != "" {
		for _, r := range title {
			if !isSafeNameRune(r) {
				return "", ErrInvalidInput("title", "Session title contains invalid characters (only alphanumeric, hyphens, and underscores are allowed).")
			}
		}
		return title, nil
	}
	base := filepath.Base(cwd)
	var safe []rune
	for _, r := range base {
		if isSafeNameRune(r) {
			safe = append(safe, r)
		} else {
			safe = append(safe, '-')
		}
	}
	return "mcp-" + string(safe), nil
}

func defaultModelForMCPSession(harness string) string {
	if model, ok := agent.DefaultModelForHarness(harness); ok {
		return model
	}
	return "sonnet"
}

func supportedMCPHarnessesMessage() string {
	return strings.Join(agent.KnownHarnesses(), ", ")
}

// validateCreateRequest validates the request and returns resolved defaults.
func validateCreateRequest(ctx *OpContext, req *CreateSessionRequest) (*createSessionParams, error) {
	if req == nil || req.Cwd == "" {
		return nil, ErrInvalidInput("cwd", "Working directory (cwd) is required.")
	}
	if !filepath.IsAbs(req.Cwd) {
		return nil, ErrInvalidInput("cwd", fmt.Sprintf("Working directory must be an absolute path: %s", req.Cwd))
	}
	if req.Prompt == "" {
		return nil, ErrInvalidInput("prompt", "Prompt is required.")
	}
	info, err := os.Stat(req.Cwd)
	if err != nil {
		return nil, ErrInvalidInput("cwd", fmt.Sprintf("Working directory does not exist: %s", req.Cwd))
	}
	if !info.IsDir() {
		return nil, ErrInvalidInput("cwd", fmt.Sprintf("Path is not a directory: %s", req.Cwd))
	}
	if ctx.Tmux == nil {
		return nil, ErrTmuxNotRunning()
	}

	p := &createSessionParams{harness: agent.NormalizeHarnessName(req.Harness), model: req.Model, persistent: req.Persistent}
	if p.harness == "" {
		p.harness = "claude-code"
	}
	if err := agent.ValidateHarnessName(p.harness); err != nil {
		return nil, ErrInvalidInput("harness", fmt.Sprintf("Unsupported harness: %s. Supported: %s", p.harness, supportedMCPHarnessesMessage()))
	}

	if p.model == "" {
		p.model = defaultModelForMCPSession(p.harness)
	}
	if err := agent.ValidateModel(p.harness, p.model); err != nil {
		return nil, ErrInvalidInput("model", err.Error())
	}

	name, err := resolveSessionName(req.Title, req.Cwd)
	if err != nil {
		return nil, err
	}
	p.name = name
	return p, nil
}

// CreateSession creates a new AGM session: tmux session + harness startup +
// manifest registration in Dolt. This is the ops-layer equivalent of the CLI
// `agm session new` flow, but without interactive prompts.
func CreateSession(ctx *OpContext, req *CreateSessionRequest) (*CreateSessionResult, error) {
	params, err := validateCreateRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	exists, err := ctx.Tmux.HasSession(params.name)
	if err != nil {
		return nil, ErrStorageError("tmux.HasSession", err)
	}
	if exists {
		return nil, &OpError{
			Status: 409,
			Type:   "session/exists",
			Code:   ErrCodeSessionExists,
			Title:  "Session already exists",
			Detail: fmt.Sprintf("A tmux session named %q already exists.", params.name),
			Suggestions: []string{
				"Use a different title.",
				fmt.Sprintf("Archive the existing session: agm session archive %s", params.name),
			},
			Parameters: map[string]string{"title": params.name},
		}
	}

	if err := ctx.Tmux.CreateSession(params.name, req.Cwd); err != nil {
		return nil, ErrStorageError("tmux.CreateSession", err)
	}

	harnessCmd := buildHarnessCommand(params.harness, params.model, params.name, req.Cwd, params.persistent)
	if err := ctx.Tmux.SendKeys(params.name, harnessCmd); err != nil {
		return nil, ErrStorageError("tmux.SendKeys(harness)", err)
	}

	sessionID := uuid.New().String()
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          params.name,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: req.Cwd,
			Tags:    []string{"source:mcp"},
		},
		Tmux:    manifest.Tmux{SessionName: params.name},
		Harness: params.harness,
		Model:   params.model,
	}
	if ctx.Storage != nil {
		if err := ctx.Storage.CreateSession(m); err != nil {
			return nil, ErrStorageError("dolt.CreateSession", err)
		}
	}

	if req.Prompt != "" {
		if err := ctx.Tmux.SendKeys(params.name, req.Prompt); err != nil {
			return nil, ErrStorageError("tmux.SendKeys(prompt)", err)
		}
	}

	return &CreateSessionResult{
		Operation: "create_session",
		SessionID: sessionID,
		Name:      params.name,
		Cwd:       req.Cwd,
		Model:     params.model,
		Harness:   params.harness,
		Created:   true,
	}, nil
}

// shellQuote escapes a string for safe use inside single quotes in a shell command.
func shellQuote(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// buildHarnessCommand constructs the shell command to start the given harness.
// When persistent is true the "&&  exit" suffix is omitted so that the tmux
// pane's shell survives after the harness process exits — required for
// long-lived supervisor sessions (vroom-meta-orchestrator, etc.) that must
// restart or be re-driven without losing the pane.  One-shot workers should
// pass persistent=false to get the clean-teardown behaviour.
func buildHarnessCommand(harness, model, sessionName, workDir string, persistent bool) string {
	exitSuffix := " && exit"
	if persistent {
		exitSuffix = ""
	}
	switch harness {
	case "claude-code":
		// Resolve the freshest OAuth token (live credentials file preferred
		// over the possibly-stale CLAUDE_CODE_OAUTH_TOKEN env var) so the
		// spawned worker doesn't 401 on a refreshed token (ce-dzhz).
		oauthArg := ""
		// envUnset always force-unsets CLAUDECODE (for consistency with the CLI
		// spawn path in new_harness.go, so the spawned worker can't detect a
		// nested Claude Code session) and additionally unsets a stray metered
		// ANTHROPIC_API_KEY whenever we inject an OAuth token, so it can't shadow
		// the Max-plan token and route the worker through the metered API (ce-84l2).
		envUnset := "-u CLAUDECODE "
		if token := auth.ResolveOAuthToken(); token != "" {
			oauthArg = fmt.Sprintf(" CLAUDE_CODE_OAUTH_TOKEN='%s'", shellQuote(token))
			envUnset = "-u CLAUDECODE -u ANTHROPIC_API_KEY "
		}
		return fmt.Sprintf("env %sAGM_SESSION_NAME='%s'%s claude --model '%s' --add-dir '%s' --enable-auto-mode%s",
			envUnset, shellQuote(sessionName), oauthArg, shellQuote(model), shellQuote(workDir), exitSuffix)
	case "gemini-cli":
		return fmt.Sprintf("gemini -m '%s'%s", shellQuote(model), exitSuffix)
	case "codex-cli":
		resolvedModel := agent.ResolveModelFullName("codex-cli", model)
		return fmt.Sprintf("env -u CLAUDECODE AGM_SESSION_NAME='%s' codex -m '%s' -C '%s' -s workspace-write%s",
			shellQuote(sessionName), shellQuote(resolvedModel), shellQuote(workDir), exitSuffix)
	case "agy":
		return fmt.Sprintf("cd '%s' && agy --add-dir '%s'%s",
			shellQuote(workDir), shellQuote(workDir), exitSuffix)
	case "opencode-cli":
		return fmt.Sprintf("cd '%s' && opencode attach%s", shellQuote(workDir), exitSuffix)
	default:
		return fmt.Sprintf("echo 'Unknown harness: %s' && exit 1", shellQuote(harness))
	}
}
