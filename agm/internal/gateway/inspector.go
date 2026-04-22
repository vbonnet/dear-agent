package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Inspector validates incoming JSON-RPC 2.0 requests.
type Inspector struct {
	logger *slog.Logger
}

// NewInspector creates a new request inspector.
func NewInspector(logger *slog.Logger) *Inspector {
	return &Inspector{logger: logger}
}

// Middleware returns an MCP middleware that validates incoming requests.
// It checks that the method is non-empty and the request has valid params.
func (i *Inspector) Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "" {
				i.logger.WarnContext(ctx, "gateway.inspector: empty method")
				return nil, fmt.Errorf("invalid request: empty method")
			}

			// Validate method name is reasonable (alphanumeric + / + _)
			if !isValidMethod(method) {
				i.logger.WarnContext(ctx, "gateway.inspector: invalid method name", "method", method)
				return nil, fmt.Errorf("invalid request: malformed method %q", method)
			}

			// Validate params can be serialized (catches corrupted state)
			if params := req.GetParams(); params != nil {
				if _, err := json.Marshal(params); err != nil {
					i.logger.WarnContext(ctx, "gateway.inspector: params not serializable", "method", method, "error", err)
					return nil, fmt.Errorf("invalid request: params not serializable: %w", err)
				}
			}

			return next(ctx, method, req)
		}
	}
}

// isValidMethod checks that a method name contains only valid characters.
func isValidMethod(method string) bool {
	for _, r := range method {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '/' || r == '_' || r == '.' || r == '$') {
			return false
		}
	}
	return len(method) > 0 && len(method) <= 256
}
