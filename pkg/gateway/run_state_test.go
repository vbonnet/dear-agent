package gateway_test

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/gateway"
)

func TestListHandlerRejectsInvalidRunState(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	if err := state.Close(); err != nil {
		t.Fatalf("close test database: %v", err)
	}

	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdList,
		Args: map[string]any{"state": "typo"},
	})
	if resp.Err == nil || resp.Err.Code != gateway.CodeInvalidArgs {
		t.Fatalf("want CodeInvalidArgs, got %+v", resp.Err)
	}
}

func TestListHandlerPreservesEmptyRunStateFilter(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
	resp := gw.Dispatch(context.Background(), gateway.Command{
		Type: gateway.CmdList,
		Args: map[string]any{"state": ""},
	})
	if resp.Err != nil {
		t.Fatalf("empty state filter: %v", resp.Err)
	}
}

// CmdList documents state as an optional string. A present value of another
// type must be refused: treating it as absent silently widened the result
// set to every run.
func TestListHandlerRejectsNonStringRunState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "number", value: 123},
		{name: "object", value: map[string]any{"eq": "running"}},
		{name: "array", value: []any{"running"}},
		{name: "bool", value: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := withDB(t)
			gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
			resp := gw.Dispatch(context.Background(), gateway.Command{
				Type: gateway.CmdList,
				Args: map[string]any{"state": tc.value},
			})
			if resp.Err == nil || resp.Err.Code != gateway.CodeInvalidArgs {
				t.Fatalf("state=%v: want CodeInvalidArgs, got %+v", tc.value, resp.Err)
			}
		})
	}
}
