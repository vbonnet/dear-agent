package permissionparity

import "github.com/vbonnet/dear-agent/agm/internal/permissionparity/piadapter"

// PiDecisionAction is the native authorization outcome expected by Pi.
type PiDecisionAction = piadapter.DecisionAction

const (
	// PiAllow authorizes a Pi tool call without prompting.
	PiAllow = piadapter.Allow
	// PiAsk delegates a Pi tool call to the interactive user.
	PiAsk = piadapter.Ask
	// PiBlock denies a Pi tool call.
	PiBlock = piadapter.Block
)

// PiToolCall is the harness-neutral projection of a Pi tool_call event.
type PiToolCall = piadapter.ToolCall

// PiDecision is an authorization result with a user-facing reason.
type PiDecision = piadapter.Decision

// DecidePiToolCall applies AGM mode and pre-approved policy semantics.
func DecidePiToolCall(mode string, allow []string, call PiToolCall, interactive bool) PiDecision {
	return piadapter.DecideToolCall(mode, allow, call, interactive)
}

// PiPolicyAllows reports whether a Pi call matches one resolved AGM entry.
func PiPolicyAllows(allow []string, call PiToolCall) bool {
	return piadapter.PolicyAllows(allow, call)
}

// EnsurePiAuthorizationExtension atomically installs the embedded Pi bridge.
func EnsurePiAuthorizationExtension(root string) (string, error) {
	return piadapter.EnsureExtension(root)
}
