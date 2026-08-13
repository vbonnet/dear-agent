package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/dispatchstate"
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

func listSessionsRequestFromMCP(input ListSessionsInput) (*ops.ListSessionsRequest, []string) {
	return &ops.ListSessionsRequest{
		Status:  input.Filters.Status,
		Harness: input.Filters.AgentType,
		Limit:   input.Filters.Limit,
	}, input.Fields
}

func searchSessionsRequestFromMCP(input SearchSessionsInput) *ops.SearchSessionsRequest {
	return &ops.SearchSessionsRequest{
		Query:  input.Query,
		Status: input.Filters.Status,
		Limit:  input.Filters.Limit,
	}
}

func getSessionRequestFromMCP(input GetSessionInput) *ops.GetSessionRequest {
	return &ops.GetSessionRequest{Identifier: input.Identifier}
}

// --- Tool registration ---

type mcpOpContextFactory func() (*ops.OpContext, func(), error)

type listSessionsOperation func(*ops.OpContext, *ops.ListSessionsRequest) (*ops.ListSessionsResult, error)
type searchSessionsOperation func(*ops.OpContext, *ops.SearchSessionsRequest) (*ops.SearchSessionsResult, error)
type getSessionOperation func(*ops.OpContext, *ops.GetSessionRequest) (*ops.GetSessionResult, error)
type archiveSessionOperation func(*ops.OpContext, *ops.ArchiveSessionRequest) (*ops.ArchiveSessionResult, error)
type killSessionOperation func(*ops.OpContext, *ops.KillSessionRequest) (*ops.KillSessionResult, error)

func addListSessionsTool(server *mcp.Server, _ *Config) {
	addListSessionsToolWithDependencies(server, newMCPOpContext, ops.ListSessions)
}

func addListSessionsToolWithDependencies(
	server *mcp.Server,
	newOpContext mcpOpContextFactory,
	listSessions listSessionsOperation,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_list_sessions",
		Description: "List AGM sessions. Use when you need to see all active sessions or find sessions by status/type.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListSessionsInput) (*mcp.CallToolResult, any, error) {
		opCtx, cleanup, err := newOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		req, fields := listSessionsRequestFromMCP(input)
		opCtx.Context = ctx
		opCtx.Fields = fields

		result, opErr := listSessions(opCtx, req)
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addSearchSessionsTool(server *mcp.Server, _ *Config) {
	addSearchSessionsToolWithDependencies(server, newMCPOpContext, ops.SearchSessions)
}

func addSearchSessionsToolWithDependencies(
	server *mcp.Server,
	newOpContext mcpOpContextFactory,
	searchSessions searchSessionsOperation,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_search_sessions",
		Description: "Search AGM sessions by name. Use when you need to find a specific session by partial name match.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SearchSessionsInput) (*mcp.CallToolResult, any, error) {
		if input.Query == "" {
			return mcpError(ops.ErrInvalidInput("query", "Search query is required.")), nil, nil
		}

		opCtx, cleanup, err := newOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Context = ctx

		result, opErr := searchSessions(opCtx, searchSessionsRequestFromMCP(input))
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addGetSessionMetadataTool(server *mcp.Server, _ *Config) {
	addGetSessionMetadataToolWithDependencies(server, newMCPOpContext, ops.GetSession)
}

func addGetSessionMetadataToolWithDependencies(
	server *mcp.Server,
	newOpContext mcpOpContextFactory,
	getSession getSessionOperation,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_get_session_metadata",
		Description: "Get detailed metadata for an AGM session. Use when you need full session details by ID or name.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Context = ctx

		result, opErr := getSession(opCtx, getSessionRequestFromMCP(input))
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

type GetSessionOutputInput struct {
	Identifier string `json:"identifier" jsonschema:"Session ID, name, or UUID prefix"`
	Lines      int    `json:"lines,omitempty" jsonschema:"Trailing pane lines to capture (default 100, max 2000)"`
}

func addGetSessionOutputTool(server *mcp.Server, _ *Config) {
	addGetSessionOutputToolWithDependencies(server, newMCPOpContext, ops.GetSessionOutput)
}

type getSessionOutputOperation func(*ops.OpContext, *ops.GetSessionOutputRequest) (*ops.GetSessionOutputResult, error)

func addGetSessionOutputToolWithDependencies(
	server *mcp.Server,
	newOpContext mcpOpContextFactory,
	getSessionOutput getSessionOutputOperation,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_get_session_output",
		Description: "Read the tail of an AGM session's terminal output — live pane while running, durable final capture after completion. Use to collect a worker's result without attaching to its pane.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetSessionOutputInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Context = ctx

		result, opErr := getSessionOutput(opCtx, &ops.GetSessionOutputRequest{
			Identifier: input.Identifier,
			Lines:      input.Lines,
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
//
// The context carries a real tmux client: session status is derived from tmux
// liveness, so a tmux-less context makes every read tool report live sessions
// as stopped/unknown and orchestrators conclude spawned work never ran
// (ce-0zng9). The MCP server always runs on the host next to the AGM tmux
// socket, so the real client is always available here.
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
		Tmux:       session.NewRealTmux(),
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

func archiveSessionRequestFromMCP(input ArchiveSessionInput) (*ops.ArchiveSessionRequest, bool) {
	return &ops.ArchiveSessionRequest{Identifier: input.Identifier}, input.DryRun
}

func killSessionRequestFromMCP(input KillSessionInput) (*ops.KillSessionRequest, bool) {
	return &ops.KillSessionRequest{
		Identifier:     input.Identifier,
		Force:          input.Force,
		ConfirmedStuck: input.ConfirmedStuck,
	}, input.DryRun
}

type SendMessageInput struct {
	SessionID string `json:"session_id" jsonschema:"Session ID, name, or UUID prefix of the target session (required)"`
	Message   string `json:"message" jsonschema:"Message text to send to the session (required)"`
}

type SetCompletionRelayTargetInput struct {
	SessionID string `json:"session_id" jsonschema:"Live Dispatch/AGM session ID or name that should receive completion relays (required)"`
}

type GetCompletionRelayTargetInput struct{}

type QuotaStatusInput struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider quota to read. Defaults to codex."`
}

func addArchiveSessionTool(server *mcp.Server, _ *Config) {
	addArchiveSessionToolWithFactory(server, newMCPOpContext)
}

func archiveSessionWithOps(opCtx *ops.OpContext, request *ops.ArchiveSessionRequest) (*ops.ArchiveSessionResult, error) {
	return ops.ArchiveSession(opCtx, request)
}

func addArchiveSessionToolWithFactory(server *mcp.Server, newOpContext mcpOpContextFactory) {
	addArchiveSessionToolWithDependencies(server, newOpContext, archiveSessionWithOps)
}

func addArchiveSessionToolWithDependencies(
	server *mcp.Server,
	newOpContext mcpOpContextFactory,
	archiveSession archiveSessionOperation,
) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_archive_session",
		Description: "Archive an AGM session by marking it as archived. Use when a session is no longer needed and should be hidden from the active list.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ArchiveSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		req, dryRun := archiveSessionRequestFromMCP(input)
		opCtx.Context = ctx
		opCtx.DryRun = dryRun

		result, opErr := archiveSession(opCtx, req)
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addKillSessionTool(server *mcp.Server, _ *Config) {
	addKillSessionToolWithFactory(server, newMCPOpContextWithTmux)
}

func killSessionWithOps(opCtx *ops.OpContext, request *ops.KillSessionRequest) (*ops.KillSessionResult, error) {
	return ops.KillSession(opCtx, request)
}

// addKillSessionToolWithFactory keeps dependency construction private while
// allowing the complete MCP transport and shared mutation contract to be
// exercised with deterministic storage and tmux adapters.
func addKillSessionToolWithFactory(server *mcp.Server, newOpContext mcpOpContextFactory) {
	addKillSessionToolWithDependencies(server, newOpContext, killSessionWithOps)
}

func addKillSessionToolWithDependencies(
	server *mcp.Server,
	newOpContext mcpOpContextFactory,
	killSession killSessionOperation,
) {
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
		req, dryRun := killSessionRequestFromMCP(input)
		opCtx.Context = ctx
		opCtx.DryRun = dryRun

		result, opErr := killSession(opCtx, req)
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

// --- Session lifecycle tools ---

// newMCPOpContextWithTmux extends the shared context with the MCP creation
// runtime. Used by tools that create sessions or send messages.
func newMCPOpContextWithTmux() (*ops.OpContext, func(), error) {
	opCtx, cleanup, err := newMCPOpContext()
	if err != nil {
		return nil, cleanup, err
	}
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
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return ops.CreateSessionLaunchResult{}, fmt.Errorf("prepare harness launch: %w", err)
	}
	if launch.BindOverrideReservations != nil {
		if err := launch.FinalizeLaunch(true); err != nil {
			_ = launch.CancelUndelivered()
			return ops.CreateSessionLaunchResult{}, fmt.Errorf("finalize harness launch: %w", err)
		}
	}
	result := ops.CreateSessionLaunchResult{ModeAppliedAtStartup: launch.ModeAppliedAtStartup}
	submissionErr := r.tmux.SendKeys(spec.SessionName, launch.Command)
	if _, err := ops.ResolveHarnessLaunchSubmission(launch, submissionErr); err != nil {
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

		result, opErr := ops.CreateSessionRouted(ctx, opCtx, createSessionRequestFromMCP(input))
		if opErr != nil {
			span.RecordError(opErr)
			return mcpError(opErr), nil, nil
		}

		// Record the harness/model actually created: CreateSessionRouted may have
		// fallen back from agy to codex, so the requested agm.harness (set at span
		// start) is not necessarily what ran.
		span.SetAttributes(
			attribute.String("agm.session_id", result.SessionID),
			attribute.String("agm.session_name", result.Name),
			attribute.String("agm.effective_harness", result.Harness),
			attribute.String("agm.effective_model", result.Model),
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
		// Dispatch-driven (MCP) agy spawns are the ones that hit the Antigravity
		// identity-discovery race under provider throttle. Default them to
		// retry-with-backoff and fall back to codex so a throttled Gemini keeps
		// the pipeline moving instead of dropping the work. Ignored for non-agy
		// harnesses by CreateSessionRouted.
		SpawnRetries:    mcpAgySpawnRetries,
		FallbackHarness: mcpAgyFallbackHarness,
	}
}

// Defaults that make Dispatch-spawned agy (Gemini/Antigravity) sessions
// resilient to the provider-throttle identity-discovery race.
const (
	mcpAgySpawnRetries    = 3
	mcpAgyFallbackHarness = "codex-cli"
)

func addSendMessageTool(server *mcp.Server, _ *Config) {
	addSendMessageToolWithFactory(server, newMCPOpContextWithTmux)
}

// addSendMessageToolWithFactory exercises the complete MCP transport while
// allowing tests to prove that a context carrying tmux still routes pure API
// manifests through the shared adapter transaction first.
func addSendMessageToolWithFactory(server *mcp.Server, newOpContext mcpOpContextFactory) {
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

		opCtx, cleanup, err := newOpContext()
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

func addCompletionRelayTargetTools(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_get_completion_relay_target",
		Description: "Read the live AGM completion relay target. Use before relying on completion notifications from AGM-created sessions.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ GetCompletionRelayTargetInput) (*mcp.CallToolResult, any, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return mcpError(err), nil, nil
		}
		result := dispatchstate.ResolveRelayTarget(home, "", os.Getenv)
		return mcpSuccess(result), result, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_set_completion_relay_target",
		Description: "Set the live Dispatch/AGM session that receives completion relays from the watcher. Takes effect without restarting the watcher.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input SetCompletionRelayTargetInput) (*mcp.CallToolResult, any, error) {
		if input.SessionID == "" {
			return mcpError(ops.ErrInvalidInput("session_id", "Session identifier is required.")), nil, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return mcpError(err), nil, nil
		}
		result, err := dispatchstate.SetRelayTarget(home, input.SessionID)
		if err != nil {
			return mcpError(err), nil, nil
		}
		return mcpSuccess(result), result, nil
	})
}

func addQuotaStatusTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_get_quota_status",
		Description: "Read the latest provider quota status captured by CodexBar. Use to pace Dispatch work when a provider is throttled or near-empty.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input QuotaStatusInput) (*mcp.CallToolResult, any, error) {
		home, err := os.UserHomeDir()
		if err != nil {
			return mcpError(err), nil, nil
		}
		provider := input.Provider
		if provider == "" {
			provider = "codex"
		}
		result := dispatchstate.ReadQuotaStatus(home, provider, time.Now())
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
