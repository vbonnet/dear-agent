package main

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestMCPHandlersRouteMappedRequestsToOperations(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		opCtx := &ops.OpContext{}
		var gotRequest ops.ListSessionsRequest
		var gotFields []string
		cleanupCalls := 0
		client := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
			addListSessionsToolWithDependencies(server, func() (*ops.OpContext, func(), error) {
				return opCtx, func() { cleanupCalls++ }, nil
			}, func(gotCtx *ops.OpContext, request *ops.ListSessionsRequest) (*ops.ListSessionsResult, error) {
				gotRequest = *request
				gotFields = slices.Clone(gotCtx.Fields)
				return &ops.ListSessionsResult{Operation: "list_sessions"}, nil
			})
		})
		callMCPContractTool(t, client, "agm_list_sessions", map[string]any{
			"filters": map[string]any{
				"status":     "list-status-sentinel",
				"agent_type": "list-agent-sentinel",
				"limit":      137,
			},
			"fields": []string{"list-field-one-sentinel", "list-field-two-sentinel"},
		})
		wantRequest := ops.ListSessionsRequest{
			Status: "list-status-sentinel", Harness: "list-agent-sentinel", Limit: 137,
		}
		if !reflect.DeepEqual(gotRequest, wantRequest) {
			t.Fatalf("list operation request = %+v, want %+v", gotRequest, wantRequest)
		}
		wantFields := []string{"list-field-one-sentinel", "list-field-two-sentinel"}
		if !slices.Equal(gotFields, wantFields) || opCtx.Context == nil || cleanupCalls != 1 {
			t.Fatalf("list operation fields/context/cleanup = %v/%v/%d, want %v/non-nil/1",
				gotFields, opCtx.Context, cleanupCalls, wantFields)
		}
	})

	t.Run("search", func(t *testing.T) {
		opCtx := &ops.OpContext{}
		var gotRequest ops.SearchSessionsRequest
		cleanupCalls := 0
		client := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
			addSearchSessionsToolWithDependencies(server, func() (*ops.OpContext, func(), error) {
				return opCtx, func() { cleanupCalls++ }, nil
			}, func(_ *ops.OpContext, request *ops.SearchSessionsRequest) (*ops.SearchSessionsResult, error) {
				gotRequest = *request
				return &ops.SearchSessionsResult{Operation: "search_sessions"}, nil
			})
		})
		callMCPContractTool(t, client, "agm_search_sessions", map[string]any{
			"query": "search-query-sentinel",
			"filters": map[string]any{
				"status": "search-status-sentinel",
				"limit":  29,
			},
		})
		want := ops.SearchSessionsRequest{
			Query: "search-query-sentinel", Status: "search-status-sentinel", Limit: 29,
		}
		if gotRequest != want || opCtx.Context == nil || cleanupCalls != 1 {
			t.Fatalf("search operation request/context/cleanup = %+v/%v/%d, want %+v/non-nil/1",
				gotRequest, opCtx.Context, cleanupCalls, want)
		}
	})

	t.Run("get", func(t *testing.T) {
		opCtx := &ops.OpContext{}
		var gotIdentifier string
		cleanupCalls := 0
		client := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
			addGetSessionMetadataToolWithDependencies(server, func() (*ops.OpContext, func(), error) {
				return opCtx, func() { cleanupCalls++ }, nil
			}, func(_ *ops.OpContext, request *ops.GetSessionRequest) (*ops.GetSessionResult, error) {
				gotIdentifier = request.Identifier
				return &ops.GetSessionResult{Operation: "get_session"}, nil
			})
		})
		callMCPContractTool(t, client, "agm_get_session_metadata", map[string]any{
			"identifier": "get-identifier-sentinel",
		})
		if gotIdentifier != "get-identifier-sentinel" || opCtx.Context == nil || cleanupCalls != 1 {
			t.Fatalf("get operation identifier/context/cleanup = %q/%v/%d, want sentinel/non-nil/1",
				gotIdentifier, opCtx.Context, cleanupCalls)
		}
	})

	t.Run("archive", func(t *testing.T) {
		opCtx := &ops.OpContext{}
		var gotRequest ops.ArchiveSessionRequest
		var gotContext context.Context
		gotDryRun := false
		cleanupCalls := 0
		client := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
			addArchiveSessionToolWithDependencies(server, func() (*ops.OpContext, func(), error) {
				return opCtx, func() { cleanupCalls++ }, nil
			}, func(gotCtx *ops.OpContext, request *ops.ArchiveSessionRequest) (*ops.ArchiveSessionResult, error) {
				gotRequest = *request
				gotContext = gotCtx.Context
				gotDryRun = gotCtx.DryRun
				return &ops.ArchiveSessionResult{Operation: "archive_session"}, nil
			})
		})
		callMCPContractTool(t, client, "agm_archive_session", map[string]any{
			"identifier": "archive-identifier-sentinel",
			"dry_run":    true,
		})
		if gotRequest.Identifier != "archive-identifier-sentinel" || gotContext == nil || !gotDryRun || cleanupCalls != 1 {
			t.Fatalf("archive operation request/context/dry-run/cleanup = %+v/%v/%t/%d",
				gotRequest, gotContext, gotDryRun, cleanupCalls)
		}
	})

	t.Run("kill", func(t *testing.T) {
		type observedKill struct {
			request ops.KillSessionRequest
			context context.Context
			dryRun  bool
		}
		opCtx := &ops.OpContext{}
		var calls []observedKill
		cleanupCalls := 0
		client := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
			addKillSessionToolWithDependencies(server, func() (*ops.OpContext, func(), error) {
				return opCtx, func() { cleanupCalls++ }, nil
			}, func(gotCtx *ops.OpContext, request *ops.KillSessionRequest) (*ops.KillSessionResult, error) {
				calls = append(calls, observedKill{request: *request, context: gotCtx.Context, dryRun: gotCtx.DryRun})
				return &ops.KillSessionResult{Operation: "kill_session"}, nil
			})
		})
		callMCPContractTool(t, client, "agm_kill_session", map[string]any{
			"identifier": "kill-force-identifier-sentinel",
			"force":      true,
		})
		callMCPContractTool(t, client, "agm_kill_session", map[string]any{
			"identifier":      "kill-stuck-dry-run-identifier-sentinel",
			"confirmed_stuck": true,
			"dry_run":         true,
		})
		wantRequests := []ops.KillSessionRequest{
			{Identifier: "kill-force-identifier-sentinel", Force: true},
			{Identifier: "kill-stuck-dry-run-identifier-sentinel", ConfirmedStuck: true},
		}
		if len(calls) != 2 || calls[0].context == nil || calls[1].context == nil ||
			calls[0].dryRun || !calls[1].dryRun || cleanupCalls != 2 ||
			!reflect.DeepEqual([]ops.KillSessionRequest{calls[0].request, calls[1].request}, wantRequests) {
			t.Fatalf("kill operation calls/cleanup = %+v/%d, want requests=%+v dry-runs=false,true",
				calls, cleanupCalls, wantRequests)
		}
	})
}

func callMCPContractTool(t *testing.T, client *mcp.ClientSession, name string, arguments map[string]any) {
	t.Helper()
	result, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if result.IsError {
		t.Fatalf("CallTool(%s) returned an operation error", name)
	}
}

func TestReadMCPHandlersPropagateRequestCancellation(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		arguments map[string]any
		register  func(*mcp.Server, mcpOpContextFactory, func(*ops.OpContext) error)
	}{
		{
			name: "list", tool: "agm_list_sessions",
			arguments: map[string]any{"filters": map[string]any{}},
			register: func(server *mcp.Server, factory mcpOpContextFactory, observe func(*ops.OpContext) error) {
				addListSessionsToolWithDependencies(server, factory,
					func(opCtx *ops.OpContext, _ *ops.ListSessionsRequest) (*ops.ListSessionsResult, error) {
						return nil, observe(opCtx)
					})
			},
		},
		{
			name: "search", tool: "agm_search_sessions",
			arguments: map[string]any{
				"query": "context-sentinel", "filters": map[string]any{},
			},
			register: func(server *mcp.Server, factory mcpOpContextFactory, observe func(*ops.OpContext) error) {
				addSearchSessionsToolWithDependencies(server, factory,
					func(opCtx *ops.OpContext, _ *ops.SearchSessionsRequest) (*ops.SearchSessionsResult, error) {
						return nil, observe(opCtx)
					})
			},
		},
		{
			name: "get", tool: "agm_get_session_metadata",
			arguments: map[string]any{"identifier": "context-sentinel"},
			register: func(server *mcp.Server, factory mcpOpContextFactory, observe func(*ops.OpContext) error) {
				addGetSessionMetadataToolWithDependencies(server, factory,
					func(opCtx *ops.OpContext, _ *ops.GetSessionRequest) (*ops.GetSessionResult, error) {
						return nil, observe(opCtx)
					})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := make(chan struct{})
			observedCancellation := make(chan bool, 1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseBlocker := func() { releaseOnce.Do(func() { close(release) }) }
			cleaned := make(chan struct{})
			var cleanupOnce sync.Once
			opCtx := &ops.OpContext{}
			client := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
				tt.register(server, func() (*ops.OpContext, func(), error) {
					return opCtx, func() { cleanupOnce.Do(func() { close(cleaned) }) }, nil
				}, func(gotCtx *ops.OpContext) error {
					close(started)
					if gotCtx.Context == nil {
						observedCancellation <- false
						return context.Canceled
					}
					select {
					case <-gotCtx.Context.Done():
						observedCancellation <- true
						return gotCtx.Context.Err()
					case <-release:
						observedCancellation <- false
						return context.Canceled
					}
				})
			})
			// Release before MCP session cleanup if an assertion stops the test
			// while a handler is deliberately blocked on the injected context.
			t.Cleanup(releaseBlocker)

			requestCtx, cancel := context.WithCancel(t.Context())
			callDone := make(chan error, 1)
			go func() {
				_, err := client.CallTool(requestCtx, &mcp.CallToolParams{
					Name: tt.tool, Arguments: tt.arguments,
				})
				callDone <- err
			}()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s handler did not reach the injected operation", tt.tool)
			}
			cancel()
			select {
			case observed := <-observedCancellation:
				if !observed {
					t.Fatalf("%s operation did not observe request cancellation", tt.tool)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s operation remained blocked after request cancellation", tt.tool)
			}
			releaseBlocker()
			select {
			case <-cleaned:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s handler did not clean up after cancellation", tt.tool)
			}
			select {
			case <-callDone:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s client call did not finish after cancellation", tt.tool)
			}
		})
	}
}

type blockingArchiveStorage struct {
	dolt.Storage
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingArchiveStorage) GetSession(identifier string) (*manifest.Manifest, error) {
	s.once.Do(func() {
		close(s.started)
		<-s.release
	})
	return s.Storage.GetSession(identifier)
}

func TestArchiveMCPHandlerPropagatesContextAndDryRun(t *testing.T) {
	store := dolt.NewMockAdapter()
	sessionManifest := dolt.NewTestManifest("archive-contract-id", "archive-contract")
	if err := store.CreateSession(sessionManifest); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	blockingStore := &blockingArchiveStorage{
		Storage: store,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blockingStore.release) }) }
	opCtx := &ops.OpContext{Storage: blockingStore}
	cleaned := make(chan struct{})
	var cleanupOnce sync.Once
	client := newTestMCPClient(t, func(server *mcp.Server, _ *Config) {
		addArchiveSessionToolWithFactory(server, func() (*ops.OpContext, func(), error) {
			return opCtx, func() { cleanupOnce.Do(func() { close(cleaned) }) }, nil
		})
	})
	// Register after the MCP session cleanups so LIFO cleanup releases the
	// injected blocker before session shutdown can wait for the handler.
	t.Cleanup(release)

	requestCtx, cancel := context.WithCancel(t.Context())
	callDone := make(chan error, 1)
	go func() {
		_, err := client.CallTool(requestCtx, &mcp.CallToolParams{
			Name: "agm_archive_session",
			Arguments: map[string]any{
				"identifier": sessionManifest.SessionID,
				"dry_run":    true,
			},
		})
		callDone <- err
	}()
	select {
	case <-blockingStore.started:
	case <-time.After(2 * time.Second):
		t.Fatal("archive handler did not reach injected storage")
	}
	if opCtx.Context == nil || !opCtx.DryRun {
		t.Fatalf("archive OpContext = %+v, want request context and dry_run=true", opCtx)
	}
	cancel()
	select {
	case <-opCtx.Context.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("archive operation context did not observe client cancellation")
	}
	release()
	select {
	case <-cleaned:
	case <-time.After(2 * time.Second):
		t.Fatal("archive handler did not clean up after cancellation")
	}
	select {
	case <-callDone:
	case <-time.After(2 * time.Second):
		t.Fatal("archive client call did not finish after cancellation")
	}
	stored, err := store.GetSession(sessionManifest.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if stored.Lifecycle == manifest.LifecycleArchived {
		t.Fatal("cancelled archive request mutated session lifecycle")
	}
}

func TestMCPDiscoveryMatchesCompiledToolsWithExactGhosts(t *testing.T) {
	tools := registeredMCPTools(t, registerMCPTools)
	compiled := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		logicalName, ok := compiledLogicalNames()[tool.Name]
		if !ok {
			t.Fatalf("DAH-002/discovery-unmapped-compiled-tool: %q", tool.Name)
		}
		if _, duplicate := compiled[logicalName]; duplicate {
			t.Fatalf("DAH-002/discovery-duplicate-compiled-operation: %q", logicalName)
		}
		compiled[logicalName] = struct{}{}
	}

	advertised := make(map[string]struct{})
	for _, operation := range ops.ListOps().Operations {
		if !slices.Contains(strings.Split(operation.Surface, ","), "mcp") {
			continue
		}
		if _, duplicate := advertised[operation.Name]; duplicate {
			t.Fatalf("DAH-002/discovery-duplicate-advertisement: %q", operation.Name)
		}
		advertised[operation.Name] = struct{}{}
	}
	for name := range compiled {
		if _, ok := advertised[name]; !ok {
			t.Fatalf("DAH-002/discovery-missing-advertisement: compiled operation %q", name)
		}
	}

	wantGhosts := map[string]string{
		"get_status":      "DAH-002/discovery-get-status-ghost",
		"list_workspaces": "DAH-002/discovery-list-workspaces-ghost",
	}
	consumed := make(map[string]bool, len(wantGhosts))
	for name := range advertised {
		if _, ok := compiled[name]; ok {
			continue
		}
		id, ok := wantGhosts[name]
		if !ok {
			t.Fatalf("DAH-002/discovery-unaccounted-ghost: %q", name)
		}
		if consumed[name] {
			t.Fatalf("%s: ghost consumed more than once", id)
		}
		consumed[name] = true
	}
	for name, id := range wantGhosts {
		if !consumed[name] {
			t.Fatalf("%s: stale discovery ghost record", id)
		}
	}
}
