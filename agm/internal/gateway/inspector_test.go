package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

func TestIsValidMethod_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		method string
		valid  bool
	}{
		{"simple", "initialize", true},
		{"with slash", "tools/call", true},
		{"with underscore", "tool_name", true},
		{"with dot", "notifications.progress", true},
		{"with dollar", "$refs/resolve", true},
		{"mixed valid chars", "a/b.c_d$e", true},
		{"empty string", "", false},
		{"space", "tools call", false},
		{"semicolon", "tools;call", false},
		{"dash", "tools-call", false},
		{"newline", "tools\ncall", false},
		{"tab", "tools\tcall", false},
		{"parentheses", "tools()", false},
		{"brackets", "tools[]", false},
		{"equals", "tools=call", false},
		{"pipe", "tools|call", false},
		{"ampersand", "tools&call", false},
		{"unicode", "tools/\u00e9", false},
		{"single char", "a", true},
		{"numbers only", "12345", true},
		{"max length", strings.Repeat("a", 256), true},
		{"over max length", strings.Repeat("a", 257), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidMethod(tt.method), "method: %q", tt.method)
		})
	}
}

func TestInspector_PassesValidRequest(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()

	called := false
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		called = true
		return &mcp.CallToolResult{}, nil
	})

	result, err := handler(context.Background(), "tools/call", toolCallReq("test"))
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, called, "next handler should be called for valid requests")
}

func TestInspector_BlocksEmptyMethod(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()

	called := false
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		called = true
		return &mcp.CallToolResult{}, nil
	})

	_, err := handler(context.Background(), "", toolCallReq("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty method")
	assert.False(t, called, "next handler should not be called for empty method")
}

func TestInspector_BlocksMalformedMethod(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()

	called := false
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		called = true
		return &mcp.CallToolResult{}, nil
	})

	malformedMethods := []string{
		"tools call",          // space
		"method;injection",    // semicolon
		"tools/call\n--drop",  // newline
		"<script>alert</script>", // HTML
	}

	for _, method := range malformedMethods {
		t.Run(method, func(t *testing.T) {
			called = false
			_, err := handler(context.Background(), method, toolCallReq("test"))
			assert.Error(t, err, "method %q should be rejected", method)
			assert.Contains(t, err.Error(), "malformed method")
			assert.False(t, called)
		})
	}
}

func TestInspector_AllowsVariousMethods(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()
	handler := mw(successHandler)

	validMethods := []string{
		"initialize",
		"tools/call",
		"tools/list",
		"notifications/cancelled",
		"$refs/resolve",
		"completion/complete",
	}

	for _, method := range validMethods {
		t.Run(method, func(t *testing.T) {
			result, err := handler(context.Background(), method, toolCallReq("test"))
			assert.NoError(t, err, "method %q should be allowed", method)
			assert.NotNil(t, result)
		})
	}
}

func TestInspector_NilParams(t *testing.T) {
	insp := NewInspector(testLogger)
	mw := insp.Middleware()
	handler := mw(successHandler)

	// ServerRequest with nil params — should pass validation
	req := &mcp.ServerRequest[*mcp.InitializeParams]{
		Params: nil,
	}
	result, err := handler(context.Background(), "initialize", req)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}
