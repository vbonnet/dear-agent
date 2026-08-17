package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// GetCompletionRelayTarget is the op that reports which Dispatch session
// completions are currently relayed to.
var GetCompletionRelayTarget = codegen.Op{
	Name:         "get_completion_relay_target",
	Description:  "Read the live AGM completion relay target",
	Category:     codegen.CategoryRead,
	RequestType:  "GetCompletionRelayTargetInput",
	ResponseType: "CompletionRelayTargetResult",
	HandlerFunc:  "GetCompletionRelayTarget",
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_get_completion_relay_target",
		Description: "Read the live AGM completion relay target. Use before relying on completion notifications from AGM-created sessions.",
	},
}

// SetCompletionRelayTarget is the op that points completion relay at a
// live Dispatch session.
var SetCompletionRelayTarget = codegen.Op{
	Name:         "set_completion_relay_target",
	Description:  "Set the live AGM completion relay target",
	Category:     codegen.CategoryMutation,
	RequestType:  "SetCompletionRelayTargetInput",
	ResponseType: "CompletionRelayTargetResult",
	HandlerFunc:  "SetCompletionRelayTarget",
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_set_completion_relay_target",
		Description: "Set the live Dispatch/AGM session that receives completion relays from the watcher. Takes effect without restarting the watcher.",
	},
}

// GetQuotaStatus is the op that reports the recorded provider quota state.
var GetQuotaStatus = codegen.Op{
	Name:         "get_quota_status",
	Description:  "Read the latest provider quota status captured by CodexBar",
	Category:     codegen.CategoryRead,
	RequestType:  "QuotaStatusInput",
	ResponseType: "QuotaStatusResult",
	HandlerFunc:  "GetQuotaStatus",
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_get_quota_status",
		Description: "Read the latest provider quota status captured by CodexBar. Use to pace Dispatch work when a provider is throttled or near-empty.",
	},
}

// GetCompletionRelayTargetInput is the (empty) request for
// GetCompletionRelayTarget.
type GetCompletionRelayTargetInput struct{}

// SetCompletionRelayTargetInput carries the Dispatch session to relay to.
type SetCompletionRelayTargetInput struct {
	SessionID string `json:"session_id" ef:"session_id,pos=0,required" desc:"Live Dispatch/AGM session ID or name that should receive completion relays (required)"`
}

// QuotaStatusInput selects which provider's quota state to report.
type QuotaStatusInput struct {
	Provider string `json:"provider,omitempty" ef:"provider" desc:"Provider quota to read. Defaults to codex."`
}
