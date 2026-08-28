package gateway_test

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/gateway"
)

func TestListHandlerRejectsInvalidRunState(t *testing.T) {
	state := withDB(t)
	gw := gateway.New(gateway.WorkflowHandlers(state.DB(), nil))
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
