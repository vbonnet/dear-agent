package main

// tools_test.go — coverage for the unit-testable layer of tools.go that
// the 2026-05-27 audit flagged as 0%. The MCP tool handlers themselves
// reach into Dolt via newMCPOpContext, so they aren't exercisable in
// pure unit tests; what *is* testable in isolation is the request/
// response shaping (mcpSuccess, mcpError), the trace-context injector,
// and the forwardToEngramMCP HTTP shim. Those are the surfaces that
// would silently break a Discord/Desktop client without anyone
// noticing — so they get tests now.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// extractText pulls the single TextContent payload out of a
// CallToolResult; every helper in tools.go emits exactly one TextContent
// so this is the natural shape to assert on.
func extractText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if r == nil {
		t.Fatal("nil CallToolResult")
		return ""
	}
	if len(r.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(r.Content))
		return ""
	}
	tc, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] = %T, want *mcp.TextContent", r.Content[0])
		return ""
	}
	return tc.Text
}

type blockingKillTmux struct {
	*session.MockTmux
	probeStarted chan struct{}
	releaseProbe chan struct{}
	probeOnce    sync.Once
	killCalls    int
}

func (m *blockingKillTmux) HasSession(name string) (bool, error) {
	m.probeOnce.Do(func() {
		close(m.probeStarted)
		<-m.releaseProbe
	})
	return m.MockTmux.HasSession(name)
}

func (m *blockingKillTmux) KillSession(name string) error {
	m.killCalls++
	return m.MockTmux.KillSession(name)
}

func TestMcpSuccess_EncodesResultAsJSON(t *testing.T) {
	payload := map[string]any{
		"status": "active",
		"count":  3,
	}
	r := mcpSuccess(payload)
	if r.IsError {
		t.Error("mcpSuccess result has IsError=true; want false")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(extractText(t, r)), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["status"] != "active" {
		t.Errorf("status = %v, want active", got["status"])
	}
	// JSON numbers round-trip as float64 — the value is what matters here.
	if got["count"].(float64) != 3 {
		t.Errorf("count = %v, want 3", got["count"])
	}
}

func TestMcpSuccess_NilResultIsValidJSON(t *testing.T) {
	// json.Marshal(nil) -> "null". mcpSuccess must not panic and the
	// MCP client must receive a well-formed result.
	r := mcpSuccess(nil)
	if got := extractText(t, r); got != "null" {
		t.Errorf("mcpSuccess(nil) text = %q, want null", got)
	}
}

func TestMcpError_PlainErrorUsesMessage(t *testing.T) {
	r := mcpError(errors.New("boom"))
	if !r.IsError {
		t.Fatal("mcpError result has IsError=false; want true")
	}
	if got := extractText(t, r); got != "boom" {
		t.Errorf("text = %q, want %q", got, "boom")
	}
}

func TestMcpError_OpErrorEmitsRFC7807JSON(t *testing.T) {
	// An ops.OpError must serialize as its RFC 7807 JSON, not its
	// stringified Error() — that's the contract MCP clients depend on
	// to switch on `code`/`type` rather than parse the message string.
	opErr := ops.ErrInvalidInput("identifier", "Session identifier is required.")
	r := mcpError(opErr)
	if !r.IsError {
		t.Fatal("IsError=false; want true")
	}
	text := extractText(t, r)
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("OpError text should be JSON, got %q: %v", text, err)
	}
	if env["code"] != ops.ErrCodeInvalidInput {
		t.Errorf("code = %v, want %s", env["code"], ops.ErrCodeInvalidInput)
	}
	if env["type"] != "input/invalid" {
		t.Errorf("type = %v, want input/invalid", env["type"])
	}
}

func TestMcpError_WrappedOpErrorStillJSON(t *testing.T) {
	// errors.As must find the wrapped OpError so callers can wrap
	// ops errors with context (`fmt.Errorf("dolt connect: %w", err)`)
	// without losing the RFC 7807 payload.
	wrapped := errors.Join(errors.New("context"), ops.ErrInvalidInput("x", "y"))
	r := mcpError(wrapped)
	text := extractText(t, r)
	if !strings.Contains(text, ops.ErrCodeInvalidInput) {
		t.Errorf("wrapped OpError lost the JSON shape: %s", text)
	}
}

func TestInjectTraceContext_EmptyWhenNoSpan(t *testing.T) {
	// Without a propagator that emits traceparent, the meta map should
	// be empty. Other agents on the bus rely on "no traceparent →
	// start a new trace", not "always present".
	meta := injectTraceContext(context.Background())
	if _, has := meta["traceparent"]; has {
		t.Errorf("meta has traceparent without an active span: %v", meta)
	}
}

func TestInjectTraceContext_PropagatesTraceparent(t *testing.T) {
	// Install the W3C trace-context propagator, seed the carrier with
	// a traceparent header, extract back into a ctx, then assert the
	// injector emits the same header into the _meta map. This is the
	// exact round-trip forwardToEngramMCP relies on.
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	want := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	carrier := propagation.MapCarrier{"traceparent": want}
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), carrier)

	meta := injectTraceContext(ctx)
	if got := meta["traceparent"]; got != want {
		t.Errorf("traceparent = %v, want %s", got, want)
	}
}

func TestMCPCreateSessionRuntimeWaitsForAgyBeforePrompt(t *testing.T) {
	type callerContextKey struct{}
	tmuxMock := session.NewMockTmux()
	callerCtx := context.WithValue(t.Context(), callerContextKey{}, "caller")
	var waited bool
	runtime := &mcpCreateSessionRuntime{
		tmux:     tmuxMock,
		lookPath: func(string) (string, error) { return "/usr/local/bin/agy", nil },
		sleep:    func(time.Duration) {},
		waitForAgyPrompt: func(ctx context.Context, sessionName string, timeout time.Duration) error {
			if ctx != callerCtx {
				t.Fatalf("readiness context identity changed")
			}
			if sessionName != "mcp-agy" || timeout != 90*time.Second {
				t.Fatalf("readiness wait = %q/%s, want mcp-agy/90s", sessionName, timeout)
			}
			if len(tmuxMock.SentCommands) != 1 || strings.Contains(tmuxMock.SentCommands[0], "startup prompt") {
				t.Fatalf("commands before readiness = %v, want launch only", tmuxMock.SentCommands)
			}
			waited = true
			return nil
		},
	}

	result, err := runtime.Launch(callerCtx, ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash-low", SessionName: "mcp-agy", WorkDir: "/tmp/mcp-agy",
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !waited {
		t.Fatal("AGY readiness wait was not called")
	}
	if result.ModeAppliedAtStartup {
		t.Fatal("default AGY launch unexpectedly applied auto mode")
	}
	if result.Readiness != "" {
		t.Fatalf("AGY readiness disposition = %q, want shared verification", result.Readiness)
	}
	if err := runtime.Complete(callerCtx, ops.CreateSessionCompletion{
		Manifest: &manifest.Manifest{Name: "mcp-agy", Harness: "agy"}, Prompt: "startup prompt",
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(tmuxMock.SentCommands) != 2 || tmuxMock.SentCommands[1] != "startup prompt" {
		t.Fatalf("commands after completion = %v, want launch then startup prompt", tmuxMock.SentCommands)
	}
}

func TestMCPCreateSessionRuntimeBootstrapsAgyIdentityPromptExactlyOnce(t *testing.T) {
	tmuxMock := session.NewMockTmux()
	runtime := &mcpCreateSessionRuntime{
		tmux:             tmuxMock,
		lookPath:         func(string) (string, error) { return "/usr/local/bin/agy", nil },
		sleep:            func(time.Duration) {},
		waitForAgyPrompt: func(context.Context, string, time.Duration) error { return nil },
	}
	ctx := t.Context()
	launch, err := runtime.Launch(ctx, ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash-low", SessionName: "mcp-agy-lazy", WorkDir: "/tmp/mcp-agy-lazy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.BootstrapAgyCreateIdentity(ctx, ops.AgyCreateIdentityBootstrap{
		SessionName: "mcp-agy-lazy", Prompt: "persist once",
	}); err != nil {
		t.Fatal(err)
	}
	launch.PromptDelivered = true
	if err := runtime.Complete(ctx, ops.CreateSessionCompletion{
		Manifest: &manifest.Manifest{Name: "mcp-agy-lazy"}, Prompt: "persist once", Launch: launch,
	}); err != nil {
		t.Fatal(err)
	}
	if len(tmuxMock.SentCommands) != 2 || tmuxMock.SentCommands[1] != "persist once" {
		t.Fatalf("MCP commands = %v, want bare launch and one identity-bootstrap prompt", tmuxMock.SentCommands)
	}
	if strings.Contains(tmuxMock.SentCommands[0], "persist once") || strings.Contains(tmuxMock.SentCommands[0], "--prompt-interactive") {
		t.Fatalf("MCP launch leaked startup prompt into process arguments: %q", tmuxMock.SentCommands[0])
	}
	if got := tmuxMock.AtomicInputChecks; !reflect.DeepEqual(got, []string{"mcp-agy-lazy:agy"}) {
		t.Fatalf("AGY identity bootstrap atomic checks = %v, want [mcp-agy-lazy:agy]", got)
	}
}

func TestMCPCreateSessionRuntimeAgyIdentityBootstrapFailsClosedWhenComposerIsNotReady(t *testing.T) {
	tmuxMock := session.NewMockTmux()
	tmuxMock.InputReadiness = session.InputReadiness{State: "PERMISSION", PaneID: "%0"}
	runtime := newMCPCreateSessionRuntime(tmuxMock)

	err := runtime.BootstrapAgyCreateIdentity(t.Context(), ops.AgyCreateIdentityBootstrap{
		SessionName: "mcp-agy-permission", Prompt: "must not authorize",
	})
	if err == nil || !strings.Contains(err.Error(), "PERMISSION") {
		t.Fatalf("BootstrapAgyCreateIdentity() error = %v, want permission readiness failure", err)
	}
	if len(tmuxMock.SentCommands) != 0 {
		t.Fatalf("AGY permission prompt received bootstrap input: %v", tmuxMock.SentCommands)
	}
}

func TestMCPCreateSessionRuntimeCannotBypassSharedCodexReadiness(t *testing.T) {
	t.Parallel()

	tmuxMock := session.NewMockTmux()
	tmuxMock.WaitForHarnessReadyError = errors.New("Codex composer was not observed")
	store := dolt.NewMockAdapter()
	runtime := newMCPCreateSessionRuntime(tmuxMock)

	_, err := ops.CreateSessionWithContext(t.Context(), &ops.OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime,
	}, &ops.CreateSessionRequest{
		Cwd: t.TempDir(), Title: "mcp-codex-readiness", Harness: "codex-cli",
		Model: "5.5", Prompt: "must not send", SkipCodexRemoteControl: true,
		Caller: ops.CreateSessionCaller{Surface: ops.CreateSurfaceMCP},
	})
	if err == nil || !strings.Contains(err.Error(), "composer was not observed") {
		t.Fatalf("CreateSessionWithContext() error = %v, want shared readiness failure", err)
	}
	if got := tmuxMock.WaitedHarnesses; !reflect.DeepEqual(got, []string{"mcp-codex-readiness:codex-cli"}) {
		t.Fatalf("shared readiness waits = %v, want MCP Codex wait", got)
	}
	if got := tmuxMock.SentCommands; len(got) != 1 || strings.Contains(got[0], "must not send") {
		t.Fatalf("commands before readiness failure = %v, want launch only", got)
	}
	registered, listErr := store.ListSessions(&dolt.SessionFilter{})
	if listErr != nil {
		t.Fatalf("ListSessions: %v", listErr)
	}
	if len(registered) != 0 {
		t.Fatalf("registrations before readiness = %d, want 0", len(registered))
	}
	if tmuxMock.Sessions["mcp-codex-readiness"] {
		t.Fatal("MCP Codex tmux session survived readiness rollback")
	}
}

func TestMCPCreateSessionRuntimeCannotBypassSharedAgyReadiness(t *testing.T) {
	t.Parallel()

	tmuxMock := session.NewMockTmux()
	tmuxMock.WaitForHarnessReadyError = errors.New("AGY process-backed composer was not observed")
	store := dolt.NewMockAdapter()
	runtime := &mcpCreateSessionRuntime{
		tmux: tmuxMock,
		lookPath: func(string) (string, error) {
			return "/usr/local/bin/agy", nil
		},
		sleep:            func(time.Duration) {},
		waitForAgyPrompt: func(context.Context, string, time.Duration) error { return nil },
	}

	_, err := ops.CreateSessionWithContext(t.Context(), &ops.OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime,
	}, &ops.CreateSessionRequest{
		Cwd: t.TempDir(), Title: "mcp-agy-readiness", Harness: "agy",
		Model: "3.5-flash-low", Prompt: "must not send",
		Caller: ops.CreateSessionCaller{Surface: ops.CreateSurfaceMCP},
	})
	if err == nil || !strings.Contains(err.Error(), "process-backed composer was not observed") {
		t.Fatalf("CreateSessionWithContext() error = %v, want shared AGY readiness failure", err)
	}
	if got := tmuxMock.WaitedHarnesses; !reflect.DeepEqual(got, []string{"mcp-agy-readiness:agy"}) {
		t.Fatalf("shared readiness waits = %v, want MCP AGY wait", got)
	}
	if got := tmuxMock.SentCommands; len(got) != 1 || strings.Contains(got[0], "must not send") {
		t.Fatalf("commands before readiness failure = %v, want launch only", got)
	}
	registered, listErr := store.ListSessions(&dolt.SessionFilter{})
	if listErr != nil {
		t.Fatalf("ListSessions: %v", listErr)
	}
	if len(registered) != 0 {
		t.Fatalf("registrations before readiness = %d, want 0", len(registered))
	}
	if tmuxMock.Sessions["mcp-agy-readiness"] {
		t.Fatal("MCP AGY tmux session survived readiness rollback")
	}
}

func TestMCPCreateSessionRuntimeStopsBeforePromptAfterCancellation(t *testing.T) {
	t.Parallel()

	tmuxMock := session.NewMockTmux()
	runtime := &mcpCreateSessionRuntime{tmux: tmuxMock}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := runtime.Complete(ctx, ops.CreateSessionCompletion{
		Manifest: &manifest.Manifest{Name: "mcp-agy", Harness: "agy"}, Prompt: "must not run",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete error = %v, want context.Canceled", err)
	}
	if len(tmuxMock.SentCommands) != 0 {
		t.Fatalf("commands after cancellation = %v, want none", tmuxMock.SentCommands)
	}
}

func TestMCPCreateSessionRuntimeRevalidatesStartupPromptAtomically(t *testing.T) {
	t.Parallel()

	tmuxMock := session.NewMockTmux()
	tmuxMock.InputReadiness = session.InputReadiness{Ready: false, State: "BUSY", PaneID: "%9"}
	runtime := &mcpCreateSessionRuntime{tmux: tmuxMock}
	err := runtime.Complete(t.Context(), ops.CreateSessionCompletion{
		Manifest: &manifest.Manifest{Name: "mcp-codex", Harness: "codex-cli"},
		Prompt:   "must not run",
	})
	if err == nil || !strings.Contains(err.Error(), "harness input is BUSY") {
		t.Fatalf("Complete() error = %v, want atomic BUSY rejection", err)
	}
	if got := tmuxMock.AtomicInputChecks; !reflect.DeepEqual(got, []string{"mcp-codex:codex-cli"}) {
		t.Fatalf("atomic input checks = %v, want MCP Codex revalidation", got)
	}
	if len(tmuxMock.SentCommands) != 0 || len(tmuxMock.ExactPaneDeliveries) != 0 {
		t.Fatalf("startup prompt sent after readiness changed: commands=%v panes=%v", tmuxMock.SentCommands, tmuxMock.ExactPaneDeliveries)
	}
}

func TestKillSessionToolExecutesAndVerifiesSharedMutation(t *testing.T) {
	store := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SessionID: "mcp-kill-id",
		Name:      "display-name",
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	m.Tmux.SessionName = "exact-tmux-target"
	if err := store.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	tmuxClient := session.NewMockTmux()
	tmuxClient.Sessions["exact-tmux-target"] = true
	tmuxClient.Sessions["exact-tmux-target-child"] = true

	cli := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
		addKillSessionToolWithFactory(server, func() (*ops.OpContext, func(), error) {
			return &ops.OpContext{Storage: store, Tmux: tmuxClient}, func() {}, nil
		})
	})
	res, err := cli.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "agm_kill_session",
		Arguments: map[string]any{
			"identifier":      m.SessionID,
			"confirmed_stuck": true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool result is an error: %s", extractText(t, res))
	}
	var payload ops.KillSessionResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if payload.SessionID != m.SessionID || payload.TmuxSessionName != "exact-tmux-target" {
		t.Fatalf("kill result = %+v, want exact stable-ID target", payload)
	}
	if tmuxClient.Sessions["exact-tmux-target"] {
		t.Fatal("exact tmux target remains after successful MCP kill")
	}
	if !tmuxClient.Sessions["exact-tmux-target-child"] {
		t.Fatal("prefix-related tmux session was mutated")
	}
}

func TestKillSessionToolForwardsRecentActivityForce(t *testing.T) {
	store := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SessionID: "mcp-force-id",
		Name:      "recent-stopped-session",
		UpdatedAt: time.Now(),
	}
	m.Tmux.SessionName = "missing-tmux-target"
	if err := store.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	tmuxClient := session.NewMockTmux()
	cli := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
		addKillSessionToolWithFactory(server, func() (*ops.OpContext, func(), error) {
			return &ops.OpContext{Storage: store, Tmux: tmuxClient}, func() {}, nil
		})
	})

	res, err := cli.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "agm_kill_session",
		Arguments: map[string]any{
			"identifier": m.SessionID,
			"force":      true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool result is an error: %s", extractText(t, res))
	}
	var payload ops.KillSessionResult
	if err := json.Unmarshal([]byte(extractText(t, res)), &payload); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !payload.RecentlyActive || payload.WasRunning {
		t.Fatalf("kill result = %+v, want recently active stopped target", payload)
	}
}

func TestKillSessionToolCancellationStopsBeforeMutation(t *testing.T) {
	store := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SessionID: "mcp-cancel-id",
		Name:      "cancel-display",
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	m.Tmux.SessionName = "cancel-target"
	if err := store.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	tmuxClient := &blockingKillTmux{
		MockTmux:     session.NewMockTmux(),
		probeStarted: make(chan struct{}),
		releaseProbe: make(chan struct{}),
	}
	tmuxClient.Sessions["cancel-target"] = true
	handlerDone := make(chan struct{})
	opCtx := &ops.OpContext{Storage: store, Tmux: tmuxClient}
	cli := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
		addKillSessionToolWithFactory(server, func() (*ops.OpContext, func(), error) {
			return opCtx, func() { close(handlerDone) }, nil
		})
	})

	requestCtx, cancel := context.WithCancel(t.Context())
	callDone := make(chan error, 1)
	go func() {
		_, err := cli.CallTool(requestCtx, &mcp.CallToolParams{
			Name: "agm_kill_session",
			Arguments: map[string]any{
				"identifier":      m.SessionID,
				"confirmed_stuck": true,
			},
		})
		callDone <- err
	}()
	<-tmuxClient.probeStarted
	cancel()
	select {
	case <-opCtx.Context.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("MCP handler context did not observe client cancellation")
	}
	close(tmuxClient.releaseProbe)

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("MCP kill handler did not finish after cancellation")
	}
	select {
	case <-callDone:
	case <-time.After(2 * time.Second):
		t.Fatal("MCP client call did not finish after cancellation")
	}
	if tmuxClient.killCalls != 0 || !tmuxClient.Sessions["cancel-target"] {
		t.Fatalf("canceled MCP request mutated tmux: killCalls=%d sessions=%v", tmuxClient.killCalls, tmuxClient.Sessions)
	}
}

// fakeEngramMCP returns a handler that records the incoming JSON-RPC
// envelope and replies with the given content text (or an error).
func fakeEngramMCP(t *testing.T, responseText string, status int) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var calls []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseText))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestForwardToEngramMCP_HappyPath(t *testing.T) {
	srv, calls := fakeEngramMCP(t, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"text":"ok-payload"}]}}`, http.StatusOK)
	cfg := &Config{EngramMCPURL: srv.URL}

	out, err := forwardToEngramMCP(context.Background(), "engram_list_wayfinder_sessions", map[string]any{"limit": 10}, cfg)
	if err != nil {
		t.Fatalf("forwardToEngramMCP: %v", err)
	}
	if out != "ok-payload" {
		t.Errorf("output = %q, want ok-payload", out)
	}
	if len(*calls) != 1 {
		t.Fatalf("Engram MCP got %d calls, want 1", len(*calls))
	}
	call := (*calls)[0]
	if call["method"] != "tools/call" {
		t.Errorf("method = %v, want tools/call", call["method"])
	}
	params, ok := call["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %T, want map", call["params"])
	}
	if params["name"] != "engram_list_wayfinder_sessions" {
		t.Errorf("tool name = %v, want engram_list_wayfinder_sessions", params["name"])
	}
	args, _ := params["arguments"].(map[string]any)
	if args["limit"].(float64) != 10 {
		t.Errorf("arguments.limit = %v, want 10", args["limit"])
	}
}

func TestForwardToEngramMCP_MCPErrorEnvelope(t *testing.T) {
	// A non-zero `error.code` in the MCP envelope must surface as a
	// forwardToEngramMCP error — otherwise the agent_caller sees an
	// empty string and proceeds as if the call succeeded.
	srv, _ := fakeEngramMCP(t, `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`, http.StatusOK)
	cfg := &Config{EngramMCPURL: srv.URL}

	_, err := forwardToEngramMCP(context.Background(), "missing_tool", nil, cfg)
	if err == nil {
		t.Fatal("forwardToEngramMCP swallowed MCP error envelope")
	}
	if !strings.Contains(err.Error(), "-32601") || !strings.Contains(err.Error(), "method not found") {
		t.Errorf("error %q should include the code and message", err)
	}
}

func TestForwardToEngramMCP_HTTPStatusError(t *testing.T) {
	srv, _ := fakeEngramMCP(t, `boom`, http.StatusInternalServerError)
	cfg := &Config{EngramMCPURL: srv.URL}

	_, err := forwardToEngramMCP(context.Background(), "x", nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("err = %v, want HTTP 500", err)
	}
}

func TestForwardToEngramMCP_EmptyContent(t *testing.T) {
	// A well-formed response with no content slice is a server bug,
	// but the client must report it rather than return ("", nil).
	srv, _ := fakeEngramMCP(t, `{"jsonrpc":"2.0","id":1,"result":{"content":[]}}`, http.StatusOK)
	cfg := &Config{EngramMCPURL: srv.URL}

	_, err := forwardToEngramMCP(context.Background(), "x", nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("err = %v, want empty-response error", err)
	}
}

func TestForwardToEngramMCP_DefaultsToLocalhost(t *testing.T) {
	// An empty EngramMCPURL must fall back to the documented default.
	// Originally tested via a real network call to localhost:8081 — flaky
	// if something IS listening there. Pin the assertion to the constant
	// in tools.go instead (Gemini PR #159 review): the constant is the
	// single source of truth for the out-of-the-box URL, and "the default
	// kicks in when EngramMCPURL is empty" is verified by sourcing the
	// constant in the same line of code the production path reads.
	if defaultEngramMCPURL != "http://localhost:8081" {
		t.Errorf("default URL drifted to %q — update docs/SPEC.md if intentional", defaultEngramMCPURL)
	}
	// Functional check: route to a closed listener address and assert the
	// error names that address. This is hermetic — `127.0.0.1:1` is a
	// reserved port that never accepts connections in test envs.
	cfg := &Config{EngramMCPURL: "http://127.0.0.1:1"}
	_, err := forwardToEngramMCP(context.Background(), "x", nil, cfg)
	if err == nil {
		t.Fatal("forwardToEngramMCP succeeded against a closed port; want connection error")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error %q should mention the URL it tried", err)
	}
}

// TestAddListOpsTool_EndToEnd registers the list-ops tool on a real
// MCP server, drives it via the in-memory transport pair, and asserts
// the response JSON shape. This is the only tool handler in the file
// that doesn't reach Dolt via newMCPOpContext, so it's the one we can
// exercise end-to-end without a database. Catches:
//   - the tool actually gets registered under the right name
//   - the response is well-formed JSON (mcpSuccess path)
//   - the schema-discovery result has the documented `total` and
//     `operations` fields
func TestAddListOpsTool_EndToEnd(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "agm-test", Version: "test"}, nil)
	addListOpsTool(server, &Config{})
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)

	st, ct := mcp.NewInMemoryTransports()
	srvSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer srvSession.Close()
	cliSession, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer cliSession.Close()

	tools, err := cliSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var found *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "agm_list_ops" {
			found = tool
			break
		}
	}
	if found == nil {
		t.Fatalf("agm_list_ops not registered; got %v", tools.Tools)
		return
	}
	if !strings.Contains(found.Description, "schema discovery") {
		t.Errorf("description %q should mention schema discovery", found.Description)
	}

	res, err := cliSession.CallTool(ctx, &mcp.CallToolParams{Name: "agm_list_ops"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool result is an error: %v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatalf("CallTool result has empty content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] = %T, want *mcp.TextContent", res.Content[0])
	}
	var payload struct {
		Operation  string `json:"operation"`
		Total      int    `json:"total"`
		Operations []struct {
			Name string `json:"name"`
		} `json:"operations"`
	}
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Operation != "list_ops" {
		t.Errorf("operation = %q, want list_ops", payload.Operation)
	}
	if payload.Total == 0 || payload.Total != len(payload.Operations) {
		t.Errorf("total=%d, operations=%d (should match and be > 0)", payload.Total, len(payload.Operations))
	}
}

// --- agm_create_session / agm_send_message input validation tests ---
// These exercise the MCP handler layer — input validation runs before
// Dolt or tmux are touched, so no external dependencies needed.

// newTestMCPClient registers the given tools on a fresh in-memory MCP
// server+client pair and returns the connected client session. The caller
// must defer the returned cleanup function.
func newTestMCPClient(t *testing.T, registerFn func(*mcp.Server, *Config)) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "agm-test", Version: "test"}, nil)
	registerFn(server, &Config{})
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)

	st, ct := mcp.NewInMemoryTransports()
	srvSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { srvSession.Close() })
	cliSession, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { cliSession.Close() })
	return cliSession
}

// callToolExpectError calls a tool and asserts the result is an error.
func callToolExpectError(t *testing.T, cli *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cli.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("CallTool(%s) expected error", name)
	}
	return res
}

func TestCreateSessionTool_RejectsEmptyCwd(t *testing.T) {
	cli := newTestMCPClient(t, func(s *mcp.Server, c *Config) { addCreateSessionTool(s, c) })
	res := callToolExpectError(t, cli, "agm_create_session", map[string]any{"prompt": "hello"})
	text, _ := res.Content[0].(*mcp.TextContent)
	if !strings.Contains(text.Text, "cwd") {
		t.Errorf("error should mention 'cwd', got: %s", text.Text)
	}
}

func TestCreateSessionTool_RejectsEmptyPrompt(t *testing.T) {
	cli := newTestMCPClient(t, func(s *mcp.Server, c *Config) { addCreateSessionTool(s, c) })
	callToolExpectError(t, cli, "agm_create_session", map[string]any{"cwd": "/tmp"})
}

func TestSendMessageTool_RejectsEmptySessionID(t *testing.T) {
	cli := newTestMCPClient(t, func(s *mcp.Server, c *Config) { addSendMessageTool(s, c) })
	res := callToolExpectError(t, cli, "agm_send_message", map[string]any{"message": "hello"})
	text, _ := res.Content[0].(*mcp.TextContent)
	if !strings.Contains(text.Text, "session_id") {
		t.Errorf("error should mention 'session_id', got: %s", text.Text)
	}
}

func TestSendMessageTool_RejectsEmptyMessage(t *testing.T) {
	cli := newTestMCPClient(t, func(s *mcp.Server, c *Config) { addSendMessageTool(s, c) })
	callToolExpectError(t, cli, "agm_send_message", map[string]any{"session_id": "some-id"})
}

func TestLifecycleTools_RegisterUnderCorrectNames(t *testing.T) {
	cli := newTestMCPClient(t, func(s *mcp.Server, c *Config) {
		addCreateSessionTool(s, c)
		addSendMessageTool(s, c)
	})

	tools, err := cli.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	names := make(map[string]bool)
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if !names["agm_create_session"] {
		t.Error("agm_create_session not registered")
	}
	if !names["agm_send_message"] {
		t.Error("agm_send_message not registered")
	}
}

func TestCreateSessionInputSchemaDocumentsHarnessParity(t *testing.T) {
	field, ok := reflect.TypeFor[CreateSessionInput]().FieldByName("Harness")
	if !ok {
		t.Fatal("CreateSessionInput.Harness field missing")
	}
	tag := field.Tag.Get("jsonschema")
	for _, want := range []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli", "gemini-cli"} {
		if !strings.Contains(tag, want) {
			t.Errorf("Harness jsonschema tag %q missing %q", tag, want)
		}
	}

	modelField, ok := reflect.TypeFor[CreateSessionInput]().FieldByName("Model")
	if !ok {
		t.Fatal("CreateSessionInput.Model field missing")
	}
	modelTag := modelField.Tag.Get("jsonschema")
	for _, want := range []string{"5.5", "5.6", "3.5-flash", "z-ai/glm-5.2"} {
		if !strings.Contains(modelTag, want) {
			t.Errorf("Model jsonschema tag %q missing %q", modelTag, want)
		}
	}
}

func TestCreateSessionRequestFromMCPDeclaresCaller(t *testing.T) {
	req := createSessionRequestFromMCP(CreateSessionInput{
		Cwd: "/tmp/work", Prompt: "hello", Title: "worker", Harness: "codex-cli", Model: "5.5",
	})
	if req.Caller.Surface != ops.CreateSurfaceMCP {
		t.Fatalf("caller surface = %q, want %q", req.Caller.Surface, ops.CreateSurfaceMCP)
	}
	if !req.ForwardClaudeOAuth {
		t.Fatal("MCP create request must preserve Claude OAuth forwarding")
	}
	if req.Cwd != "/tmp/work" || req.Prompt != "hello" || req.Title != "worker" || req.Harness != "codex-cli" || req.Model != "5.5" {
		t.Fatalf("request mapping lost fields: %+v", req)
	}
}

func TestForwardToEngramMCP_PropagatesTraceparent(t *testing.T) {
	// When the caller's ctx carries a span, the outbound JSON-RPC
	// envelope must include `_meta.traceparent` so Engram can stitch
	// the downstream call into the same trace.
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	srv, calls := fakeEngramMCP(t, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"text":"ok"}]}}`, http.StatusOK)
	cfg := &Config{EngramMCPURL: srv.URL}

	want := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.MapCarrier{"traceparent": want})

	if _, err := forwardToEngramMCP(ctx, "x", nil, cfg); err != nil {
		t.Fatalf("forwardToEngramMCP: %v", err)
	}
	if len(*calls) == 0 {
		t.Fatal("no calls recorded")
	}
	params, _ := (*calls)[0]["params"].(map[string]any)
	meta, _ := params["_meta"].(map[string]any)
	if got := meta["traceparent"]; got != want {
		t.Errorf("outbound traceparent = %v, want %s", got, want)
	}
}
