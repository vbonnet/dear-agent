package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ScopePolicy determines the default behavior for unlisted tools.
type ScopePolicy string

const (
	PolicyAllow ScopePolicy = "allow" // Allow unless explicitly denied
	PolicyDeny  ScopePolicy = "deny"  // Deny unless explicitly allowed
)

// ScopeEnforcer controls which tools are accessible based on allowlist/denylist config.
type ScopeEnforcer struct {
	policy    ScopePolicy
	allowlist map[string]bool
	denylist  map[string]bool
	logger    *slog.Logger
}

// NewScopeEnforcer creates a scope enforcer with the given policy and lists.
func NewScopeEnforcer(policy ScopePolicy, allowlist, denylist []string, logger *slog.Logger) *ScopeEnforcer {
	allow := make(map[string]bool, len(allowlist))
	for _, t := range allowlist {
		allow[t] = true
	}
	deny := make(map[string]bool, len(denylist))
	for _, t := range denylist {
		deny[t] = true
	}
	return &ScopeEnforcer{
		policy:    policy,
		allowlist: allow,
		denylist:  deny,
		logger:    logger,
	}
}

// Middleware returns an MCP middleware that enforces tool scope.
// Only applies to tools/call and tools/list methods.
func (s *ScopeEnforcer) Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			// Only enforce scope on tool calls
			if method != "tools/call" {
				return next(ctx, method, req)
			}

			// Extract tool name from params
			toolName := extractToolName(req)
			if toolName == "" {
				return next(ctx, method, req)
			}

			if !s.isAllowed(toolName) {
				s.logger.WarnContext(ctx, "gateway.scope: tool denied", "tool", toolName, "policy", s.policy)
				return nil, fmt.Errorf("tool %q is not permitted by gateway scope policy", toolName)
			}

			return next(ctx, method, req)
		}
	}
}

// isAllowed checks if a tool is permitted under the current policy.
func (s *ScopeEnforcer) isAllowed(tool string) bool {
	// Explicit deny always wins
	if s.denylist[tool] {
		return false
	}

	switch s.policy {
	case PolicyDeny:
		// Must be explicitly allowed
		return s.allowlist[tool]
	default: // PolicyAllow
		// Allowed unless denied (already checked above)
		return true
	}
}

// extractToolName attempts to get the tool name from request params.
func extractToolName(req mcp.Request) string {
	params := req.GetParams()
	if params == nil {
		return ""
	}

	// The CallToolParams has a Name field
	type toolParams struct {
		Name string `json:"name"`
	}

	// Try to get name from the params interface
	switch p := params.(type) {
	case *mcp.CallToolParams:
		return p.Name
	default:
		// Fallback: try to find Name via fmt
		s := fmt.Sprintf("%+v", p)
		if strings.Contains(s, "Name:") {
			// Best effort extraction
			return ""
		}
		return ""
	}
}
