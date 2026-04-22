package gateway

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

func TestAuditLogger_LogsSuccessAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	audit := NewAuditLogger(logger)
	mw := audit.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "tools/call", toolCallReq("test_tool"))
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "gateway.audit")
	assert.Contains(t, output, "status=ok")
	assert.Contains(t, output, "method=tools/call")
	assert.Contains(t, output, "tool=test_tool")
}

func TestAuditLogger_LogsErrorAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	audit := NewAuditLogger(logger)
	mw := audit.Middleware()
	handler := mw(failHandler)

	_, err := handler(context.Background(), "tools/call", toolCallReq("broken_tool"))
	assert.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "gateway.audit")
	assert.Contains(t, output, "status=error")
	assert.Contains(t, output, "tool=broken_tool")
	assert.Contains(t, output, "handler error")
}

func TestAuditLogger_NoToolNameForNonToolMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	audit := NewAuditLogger(logger)
	mw := audit.Middleware()
	handler := mw(successHandler)

	_, err := handler(context.Background(), "initialize", initReq())
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "method=initialize")
	assert.Contains(t, output, "status=ok")
	// Should NOT contain tool= since it's not a tools/call
	assert.NotContains(t, output, "tool=")
}

func TestAuditLogger_IncludesDuration(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	audit := NewAuditLogger(logger)
	mw := audit.Middleware()
	handler := mw(successHandler)

	_, _ = handler(context.Background(), "tools/call", toolCallReq("test"))

	output := buf.String()
	assert.Contains(t, output, "duration=")
}

func TestAuditLogger_PreservesHandlerError(t *testing.T) {
	audit := NewAuditLogger(testLogger)
	mw := audit.Middleware()

	customErr := fmt.Errorf("specific error message")
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, customErr
	})

	_, err := handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.ErrorIs(t, err, customErr)
}

func TestAuditLogger_PreservesHandlerResult(t *testing.T) {
	audit := NewAuditLogger(testLogger)
	mw := audit.Middleware()

	expected := &mcp.CallToolResult{}
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return expected, nil
	})

	result, err := handler(context.Background(), "tools/call", toolCallReq("tool"))
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestNewAuditLogger(t *testing.T) {
	audit := NewAuditLogger(testLogger)
	assert.NotNil(t, audit)
	assert.Equal(t, testLogger, audit.logger)
}
