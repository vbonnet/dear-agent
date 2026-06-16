package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
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

	// Model is the model to use (e.g. "sonnet", "opus"). Defaults to "sonnet".
	Model string `json:"model,omitempty"`

	// Harness is the agent harness (default: "claude-code").
	Harness string `json:"harness,omitempty"`
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
	name    string
	harness string
	model   string
}

// isSafeNameRune returns true for characters safe in tmux session names and shell arguments.
func isSafeNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

// isValidModelChar returns true for characters allowed in model identifiers.
func isValidModelChar(r rune) bool {
	return isSafeNameRune(r) || r == '.'
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

	p := &createSessionParams{harness: req.Harness, model: req.Model}
	if p.harness == "" {
		p.harness = "claude-code"
	}
	switch p.harness {
	case "claude-code", "gemini-cli", "codex-cli":
	default:
		return nil, ErrInvalidInput("harness", fmt.Sprintf("Unsupported harness: %s. Supported: claude-code, gemini-cli, codex-cli", p.harness))
	}

	if p.model == "" {
		p.model = "sonnet"
	}
	for _, r := range p.model {
		if !isValidModelChar(r) {
			return nil, ErrInvalidInput("model", "Model contains invalid characters (only alphanumeric, hyphens, underscores, and dots are allowed).")
		}
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

	harnessCmd := buildHarnessCommand(params.harness, params.model, params.name, req.Cwd)
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
func buildHarnessCommand(harness, model, sessionName, workDir string) string {
	switch harness {
	case "claude-code":
		oauthArg := ""
		if token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); token != "" {
			oauthArg = fmt.Sprintf(" CLAUDE_CODE_OAUTH_TOKEN='%s'", shellQuote(token))
		}
		return fmt.Sprintf("env AGM_SESSION_NAME='%s'%s claude --model '%s' --add-dir '%s' --enable-auto-mode && exit",
			shellQuote(sessionName), oauthArg, shellQuote(model), shellQuote(workDir))
	case "gemini-cli":
		return fmt.Sprintf("gemini -m '%s' && exit", shellQuote(model))
	case "codex-cli":
		return fmt.Sprintf("codex --model '%s' && exit", shellQuote(model))
	default:
		return fmt.Sprintf("echo 'Unknown harness: %s' && exit 1", shellQuote(harness))
	}
}
