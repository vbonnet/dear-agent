package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// injectTraceContext injects W3C trace context into an MCP _meta map.
func injectTraceContext(ctx context.Context) map[string]interface{} {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	meta := map[string]interface{}{}
	if tp := carrier.Get("traceparent"); tp != "" {
		meta["traceparent"] = tp
	}
	return meta
}

// --- Input/Output structures ---

type ListSessionsInput struct {
	Filters struct {
		Status    string `json:"status,omitempty" jsonschema:"Filter by status: active (default), archived, or all"`
		AgentType string `json:"agent_type,omitempty" jsonschema:"Filter by harness: claude-code, codex-cli, agy, opencode-cli, pi-cli, gemini-cli, or all"`
		Limit     int    `json:"limit,omitempty" jsonschema:"Maximum sessions to return (1-1000, default 100)"`
	} `json:"filters"`
	Fields []string `json:"fields,omitempty" jsonschema:"Field mask: only return these fields (e.g. [id, name, status]). Omit for all fields."`
}

type SearchSessionsInput struct {
	Query   string `json:"query" jsonschema:"Search query for session names (case-insensitive)"`
	Filters struct {
		Status string `json:"status,omitempty" jsonschema:"Filter by status: active (default), archived, or all"`
		Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results (1-50, default 10)"`
	} `json:"filters"`
}

type GetSessionInput struct {
	Identifier string `json:"identifier" jsonschema:"Session ID, name, or UUID prefix"`
}

// --- Tool registration ---

func addListSessionsTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_list_sessions",
		Description: "List AGM sessions. Use when you need to see all active sessions or find sessions by status/type.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input ListSessionsInput) (*mcp.CallToolResult, any, error) {
		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Fields = input.Fields

		result, opErr := ops.ListSessions(opCtx, &ops.ListSessionsRequest{
			Status:  input.Filters.Status,
			Harness: input.Filters.AgentType,
			Limit:   input.Filters.Limit,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addSearchSessionsTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_search_sessions",
		Description: "Search AGM sessions by name. Use when you need to find a specific session by partial name match.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input SearchSessionsInput) (*mcp.CallToolResult, any, error) {
		if input.Query == "" {
			return mcpError(ops.ErrInvalidInput("query", "Search query is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()

		result, opErr := ops.SearchSessions(opCtx, &ops.SearchSessionsRequest{
			Query:  input.Query,
			Status: input.Filters.Status,
			Limit:  input.Filters.Limit,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addGetSessionMetadataTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_get_session_metadata",
		Description: "Get detailed metadata for an AGM session. Use when you need full session details by ID or name.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input GetSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()

		result, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
			Identifier: input.Identifier,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

// --- MCP helpers ---

// newMCPOpContext creates an OpContext with Dolt storage for MCP tool handlers.
// Returns a cleanup function that must be deferred.
func newMCPOpContext() (*ops.OpContext, func(), error) {
	doltCfg, err := dolt.DefaultConfig()
	if err != nil {
		return nil, func() {}, fmt.Errorf("dolt config: %w", err)
	}

	adapter, err := dolt.New(doltCfg)
	if err != nil {
		return nil, func() {}, fmt.Errorf("dolt connect: %w", err)
	}

	cleanup := func() { _ = adapter.Close() }

	return &ops.OpContext{
		Storage:    adapter,
		OutputMode: "json",
	}, cleanup, nil
}

// mcpSuccess formats an ops result as an MCP success response.
func mcpSuccess(result any) *mcp.CallToolResult {
	data, _ := json.Marshal(result)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}
}

// mcpError formats an error as an MCP error response.
// If the error is an OpError, includes the full RFC 7807 JSON.
func mcpError(err error) *mcp.CallToolResult {
	var text string
	opErr := &ops.OpError{}
	if errors.As(err, &opErr) {
		text = string(opErr.JSON())
	} else {
		text = err.Error()
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

// --- Mutation tools ---

type ArchiveSessionInput struct {
	Identifier string `json:"identifier" jsonschema:"Session ID, name, or tmux session name to archive"`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"Preview the archive without executing. Returns what would happen."`
}

type KillSessionInput struct {
	Identifier     string `json:"identifier" jsonschema:"Session ID, name, or tmux session name to kill"`
	Force          bool   `json:"force,omitempty" jsonschema:"Bypass the recent-activity safety check."`
	ConfirmedStuck bool   `json:"confirmed_stuck,omitempty" jsonschema:"Confirm that a live harness is stuck and may be killed. Required for an active session."`
	DryRun         bool   `json:"dry_run,omitempty" jsonschema:"Preview the kill without executing. Returns what would happen."`
}

type CreateSessionInput struct {
	Cwd     string `json:"cwd" jsonschema:"Absolute path to the working directory for the new session (required)"`
	Prompt  string `json:"prompt" jsonschema:"Initial prompt to send to the session after startup (required)"`
	Title   string `json:"title,omitempty" jsonschema:"Session name. If omitted, derived from cwd directory name."`
	Model   string `json:"model,omitempty" jsonschema:"Model to use (e.g. sonnet, 5.5, 5.6, 3.5-flash, z-ai/glm-5.2). Defaults to the selected harness default."`
	Harness string `json:"harness,omitempty" jsonschema:"Agent harness: claude-code, codex-cli, agy, opencode-cli, pi-cli, or deprecated gemini-cli. Defaults to claude-code."`
}

type SendMessageInput struct {
	SessionID string `json:"session_id" jsonschema:"Session ID, name, or UUID prefix of the target session (required)"`
	Message   string `json:"message" jsonschema:"Message text to send to the session (required)"`
}

func addArchiveSessionTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_archive_session",
		Description: "Archive an AGM session by marking it as archived. Use when a session is no longer needed and should be hidden from the active list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ArchiveSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Context = ctx
		opCtx.DryRun = input.DryRun

		result, opErr := ops.ArchiveSession(opCtx, &ops.ArchiveSessionRequest{
			Identifier: input.Identifier,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addKillSessionTool(server *mcp.Server, _ *Config) {
	addKillSessionToolWithFactory(server, newMCPOpContextWithTmux)
}

type mcpOpContextFactory func() (*ops.OpContext, func(), error)

// addKillSessionToolWithFactory keeps dependency construction private while
// allowing the complete MCP transport and shared mutation contract to be
// exercised with deterministic storage and tmux adapters.
func addKillSessionToolWithFactory(server *mcp.Server, newOpContext mcpOpContextFactory) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_kill_session",
		Description: "Kill the tmux session for an AGM session. Use when a session is stuck or unresponsive and needs to be force-stopped.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input KillSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Context = ctx
		opCtx.DryRun = input.DryRun

		result, opErr := ops.KillSession(opCtx, &ops.KillSessionRequest{
			Identifier:     input.Identifier,
			Force:          input.Force,
			ConfirmedStuck: input.ConfirmedStuck,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

// --- Session lifecycle tools ---

// newMCPOpContextWithTmux creates an OpContext with both Dolt storage and a
// real Tmux interface. Used by tools that need to create sessions or send
// messages (as opposed to read-only tools that only query Dolt).
func newMCPOpContextWithTmux() (*ops.OpContext, func(), error) {
	opCtx, cleanup, err := newMCPOpContext()
	if err != nil {
		return nil, cleanup, err
	}
	opCtx.Tmux = session.NewRealTmux()
	opCtx.CreationRuntime = newMCPCreateSessionRuntime(opCtx.Tmux)
	return opCtx, cleanup, nil
}

type mcpCreateSessionRuntime struct {
	tmux             session.TmuxInterface
	lookPath         func(string) (string, error)
	waitForAgyPrompt func(context.Context, string, time.Duration) error
	sleep            func(time.Duration)
}

func newMCPCreateSessionRuntime(tmuxClient session.TmuxInterface) *mcpCreateSessionRuntime {
	return &mcpCreateSessionRuntime{
		tmux:             tmuxClient,
		lookPath:         exec.LookPath,
		waitForAgyPrompt: tmux.WaitForAgyPrompt,
		sleep:            time.Sleep,
	}
}

func (r *mcpCreateSessionRuntime) Launch(ctx context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
	if spec.Harness == "agy" {
		if _, err := r.lookPath("agy"); err != nil {
			return ops.CreateSessionLaunchResult{}, fmt.Errorf("find AGY executable: %w", err)
		}
	}
	launch := ops.BuildHarnessLaunchCommand(spec)
	result := ops.CreateSessionLaunchResult{ModeAppliedAtStartup: launch.ModeAppliedAtStartup}
	if err := r.tmux.SendKeys(spec.SessionName, launch.Command); err != nil {
		return result, err
	}
	if spec.Harness != "agy" {
		return result, nil
	}
	r.sleep(500 * time.Millisecond)
	if err := r.waitForAgyPrompt(ctx, spec.SessionName, 90*time.Second); err != nil {
		return result, fmt.Errorf("wait for AGY readiness: %w", err)
	}
	// The native wait handles AGY onboarding, but it does not prove that the
	// process rendering the composer is still alive. Leave the result unverified
	// so the shared process-and-composer gate owns the final readiness verdict.
	return result, nil
}

func (r *mcpCreateSessionRuntime) Complete(ctx context.Context, completion ops.CreateSessionCompletion) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if completion.Prompt == "" || completion.Launch.PromptDelivered {
		return nil
	}
	return r.sendPromptAtomically(ctx, completion.Manifest.Name, completion.Manifest.Harness, completion.Prompt, "startup prompt delivery")
}

func (r *mcpCreateSessionRuntime) sendPromptAtomically(ctx context.Context, sessionName, harness, prompt, operation string) error {
	sender, ok := r.tmux.(session.AtomicInputSender)
	if !ok {
		return fmt.Errorf("MCP tmux runtime does not expose atomic input delivery")
	}
	readiness, err := sender.SendKeysIfInputReady(ctx, sessionName, harness, prompt, session.InputDeliveryOptions{})
	if err != nil {
		return fmt.Errorf("revalidate MCP %s: %w", operation, err)
	}
	if !readiness.Ready {
		return fmt.Errorf("revalidate MCP %s: harness input is %s", operation, readiness.State)
	}
	if readiness.PaneID == "" {
		return fmt.Errorf("revalidate MCP %s: harness returned no verified pane", operation)
	}
	return nil
}

func (r *mcpCreateSessionRuntime) BootstrapAgyCreateIdentity(ctx context.Context, input ops.AgyCreateIdentityBootstrap) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.sendPromptAtomically(ctx, input.SessionName, "agy", input.Prompt, "AGY identity bootstrap prompt")
}

// mcpTracer is the OTel tracer for MCP tool handlers.
var mcpTracer = otel.Tracer("agm-mcp-server")

func addCreateSessionTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_create_session",
		Description: "Create a new AGM session (tmux + harness + manifest). Use when you need to spawn a new agent harness session programmatically.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateSessionInput) (*mcp.CallToolResult, any, error) {
		_, span := mcpTracer.Start(ctx, "agm_create_session",
			trace.WithAttributes(
				attribute.String("agm.tool", "agm_create_session"),
				attribute.String("agm.cwd", input.Cwd),
				attribute.String("agm.model", input.Model),
				attribute.String("agm.harness", input.Harness),
			))
		defer span.End()

		if input.Cwd == "" {
			return mcpError(ops.ErrInvalidInput("cwd", "Working directory (cwd) is required.")), nil, nil
		}
		if input.Prompt == "" {
			return mcpError(ops.ErrInvalidInput("prompt", "Prompt is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContextWithTmux()
		if err != nil {
			span.RecordError(err)
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Context = ctx

		result, opErr := ops.CreateSessionWithContext(ctx, opCtx, createSessionRequestFromMCP(input))
		if opErr != nil {
			span.RecordError(opErr)
			return mcpError(opErr), nil, nil
		}

		span.SetAttributes(
			attribute.String("agm.session_id", result.SessionID),
			attribute.String("agm.session_name", result.Name),
		)

		return mcpSuccess(result), result, nil
	})
}

func createSessionRequestFromMCP(input CreateSessionInput) *ops.CreateSessionRequest {
	return &ops.CreateSessionRequest{
		Cwd:                input.Cwd,
		Prompt:             input.Prompt,
		Title:              input.Title,
		Model:              input.Model,
		Harness:            input.Harness,
		Caller:             ops.CreateSessionCaller{Surface: ops.CreateSurfaceMCP},
		ForwardClaudeOAuth: true,
	}
}

func addSendMessageTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_send_message",
		Description: "Send a message to a running AGM session. Use when you need to deliver a prompt or instruction to an existing session.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SendMessageInput) (*mcp.CallToolResult, any, error) {
		_, span := mcpTracer.Start(ctx, "agm_send_message",
			trace.WithAttributes(
				attribute.String("agm.tool", "agm_send_message"),
				attribute.String("agm.session_id", input.SessionID),
				attribute.Int("agm.message_length", len(input.Message)),
			))
		defer span.End()

		if input.SessionID == "" {
			return mcpError(ops.ErrInvalidInput("session_id", "Session identifier is required.")), nil, nil
		}
		if input.Message == "" {
			return mcpError(ops.ErrInvalidInput("message", "Message text is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContextWithTmux()
		if err != nil {
			span.RecordError(err)
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Context = ctx
		result, opErr := ops.SendMessage(opCtx, &ops.SendMessageRequest{
			Recipient: input.SessionID,
			Message:   input.Message,
		})
		if opErr != nil {
			span.RecordError(opErr)
			return mcpError(opErr), nil, nil
		}

		span.SetAttributes(
			attribute.Bool("agm.delivered", result.Delivered),
			attribute.String("agm.recipient", result.Recipient),
		)

		return mcpSuccess(result), result, nil
	})
}

// --- Schema introspection ---

type ListOpsInput struct{}

func addListOpsTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_list_ops",
		Description: "List all available AGM operations. Use for schema discovery and to see what tools are available.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ ListOpsInput) (*mcp.CallToolResult, any, error) {
		result := ops.ListOps()
		return mcpSuccess(result), result, nil
	})
}

// --- Wayfinder MCP tool forwarding (Phase 7.1) ---

type ListWayfinderSessionsInput struct {
	StatusFilter string `json:"status_filter,omitempty" jsonschema:"Filter by status: active, completed, failed, abandoned"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum sessions to return (max 1000, default 100)"`
}

type GetWayfinderSessionInput struct {
	SessionID string `json:"session_id" jsonschema:"Session UUID"`
}

func addListWayfinderSessionsTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "engram_list_wayfinder_sessions",
		Description: "List Wayfinder sessions from Engram. Use when checking status of SDLC projects.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input ListWayfinderSessionsInput) (*mcp.CallToolResult, any, error) {
		sessions, err := listWayfinderSessions(cfg.WayfinderDir, input.StatusFilter, input.Limit)
		if err != nil {
			return mcpError(err), nil, nil
		}
		result := map[string]any{"sessions": sessions, "count": len(sessions)}
		return mcpSuccess(result), result, nil
	})
}

func addGetWayfinderSessionTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "engram_get_wayfinder_session",
		Description: "Get detailed Wayfinder session info by ID. Use when you need phase status for a specific project.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input GetWayfinderSessionInput) (*mcp.CallToolResult, any, error) {
		if input.SessionID == "" {
			return mcpError(ops.ErrInvalidInput("session_id", "Session ID is required.")), nil, nil
		}
		detail, err := getWayfinderSessionDetail(cfg.WayfinderDir, input.SessionID)
		if err != nil {
			return mcpError(err), nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: detail}},
		}, nil, nil
	})
}

// defaultEngramMCPURL is the documented out-of-the-box URL for the
// Engram MCP server. Surface here so tests can assert on a constant
// instead of inferring it from a real network error.
const defaultEngramMCPURL = "http://localhost:8081"

// forwardToEngramMCP forwards MCP tool call to Engram MCP server via HTTP
func forwardToEngramMCP(ctx context.Context, toolName string, arguments map[string]interface{}, cfg *Config) (string, error) {
	engramURL := cfg.EngramMCPURL
	if engramURL == "" {
		engramURL = defaultEngramMCPURL
	}

	// Inject W3C trace context into _meta for downstream propagation
	meta := injectTraceContext(ctx)

	mcpParams := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}
	if len(meta) > 0 {
		mcpParams["_meta"] = meta
	}

	mcpRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  mcpParams,
	}

	requestBody, err := json.Marshal(mcpRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodPost, engramURL, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w (is Engram MCP server running at %s?)", err, engramURL)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	var mcpResponse struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(responseBody, &mcpResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if mcpResponse.Error.Code != 0 {
		return "", fmt.Errorf("Engram MCP error %d: %s", mcpResponse.Error.Code, mcpResponse.Error.Message) //nolint:staticcheck // proper noun
	}

	if len(mcpResponse.Result.Content) == 0 {
		return "", fmt.Errorf("empty response from Engram MCP server")
	}

	return mcpResponse.Result.Content[0].Text, nil
}
