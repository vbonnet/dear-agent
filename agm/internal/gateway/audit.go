package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AuditLogger logs every MCP call with structured slog output.
type AuditLogger struct {
	logger *slog.Logger
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(logger *slog.Logger) *AuditLogger {
	return &AuditLogger{logger: logger}
}

// Middleware returns an MCP middleware that logs all calls.
// This should be the outermost middleware so it captures everything including rejections.
func (a *AuditLogger) Middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()
			toolName := ""
			if method == "tools/call" {
				toolName = extractToolName(req)
			}

			result, err := next(ctx, method, req)

			duration := time.Since(start)

			attrs := []slog.Attr{
				slog.String("method", method),
				slog.Duration("duration", duration),
			}

			if toolName != "" {
				attrs = append(attrs, slog.String("tool", toolName))
			}

			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
				attrs = append(attrs, slog.String("status", "error"))
				a.logger.LogAttrs(ctx, slog.LevelWarn, "gateway.audit", attrs...)
			} else {
				attrs = append(attrs, slog.String("status", "ok"))
				a.logger.LogAttrs(ctx, slog.LevelInfo, "gateway.audit", attrs...)
			}

			return result, err
		}
	}
}
