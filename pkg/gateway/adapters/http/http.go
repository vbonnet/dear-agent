// Package http is the HTTP adapter for pkg/gateway.
//
// Unlike the cli adapter, this one does not own its own listener: it
// wraps an existing *api.Server (see ADR-013) and routes the server's
// per-request handler calls through the gateway. The HTTP routes,
// caller identity (Tailscale WhoIs), and request validation stay in
// pkg/api; this adapter exists so dispatch policy (rate limits, audit,
// future authorization) lands in one place.
//
// Construct with Wrap, then use the returned *api.Server with whatever
// ListenAndServe / tsnet wiring the cmd binary already has. Run is a
// no-op for now — the HTTP adapter does not consume events; it serves
// synchronous request/response only. A future SSE or websocket path
// would extend Run.
package http

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/api"
	"github.com/vbonnet/dear-agent/pkg/gateway"
)

// Adapter wraps an *api.Server so its per-request runner dispatches
// through the gateway. The adapter is created by Wrap; the returned
// *api.Server is what the cmd binary serves.
type Adapter struct {
	server *api.Server
	gw     *gateway.Gateway
}

// Wrap installs a gateway-backed Runner on srv and returns the
// adapter. The returned *api.Server (Adapter.Server()) is the one to
// hand to http.ListenAndServe / tsnet.
//
// srv.Runner is overwritten — Wrap is the canonical "use the gateway
// from HTTP" constructor, and silently leaving the old Runner would
// produce surprising behaviour. Callers that want both still work
// directly with pkg/api without this adapter.
func Wrap(srv *api.Server, gw *gateway.Gateway) *Adapter {
	if srv == nil {
		panic("gateway/adapters/http: nil server")
	}
	if gw == nil {
		panic("gateway/adapters/http: nil gateway")
	}
	a := &Adapter{server: srv, gw: gw}
	srv.Runner = &gatewayRunner{gw: gw}
	return a
}

// Server returns the wrapped *api.Server. The underlying object is the
// same one passed to Wrap; this is just a typed accessor.
func (a *Adapter) Server() *api.Server { return a.server }

// Name implements gateway.Adapter.
func (*Adapter) Name() string { return "http" }

// Run is a no-op: the HTTP server's lifecycle is owned by the cmd
// binary (it already runs http.Serve / tsnet.Listen). The adapter is
// passive — it just routes synchronous requests through the gateway.
// Run blocks on ctx.Done so callers can include the adapter in a
// uniform "run all adapters" supervisor without special-casing it.
func (a *Adapter) Run(ctx context.Context, _ *gateway.Gateway) error {
	<-ctx.Done()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// gatewayRunner is the api.Runner implementation that translates an
// HTTP /run request into a gateway.CmdRun command. Errors from the
// gateway are mapped back into Go errors that pkg/api surfaces as
// HTTP 500 (the existing handleRun shape).
type gatewayRunner struct {
	gw *gateway.Gateway
}

func (r *gatewayRunner) Run(ctx context.Context, req api.RunRequest, caller api.Caller) (api.RunResponse, error) {
	cmd := gateway.Command{
		Type: gateway.CmdRun,
		Caller: gateway.Caller{
			LoginName: caller.LoginName,
			Display:   caller.Display,
		},
		Args: map[string]any{
			"file":   req.File,
			"inputs": req.Inputs,
		},
	}
	resp := r.gw.Dispatch(ctx, cmd)
	if resp.Err != nil {
		return api.RunResponse{}, fmt.Errorf("gateway: %w", resp.Err)
	}
	return api.RunResponse{
		RunID:    stringField(resp.Body, "run_id"),
		Workflow: stringField(resp.Body, "workflow"),
		PID:      intField(resp.Body, "pid"),
	}, nil
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func intField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
