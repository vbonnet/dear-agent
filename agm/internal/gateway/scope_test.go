package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAllowed_AllowPolicy_NoDenylist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyAllow, nil, nil, testLogger)
	assert.True(t, scope.isAllowed("any_tool"))
	assert.True(t, scope.isAllowed("another_tool"))
}

func TestIsAllowed_AllowPolicy_WithDenylist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyAllow, nil, []string{"blocked"}, testLogger)
	assert.True(t, scope.isAllowed("ok_tool"))
	assert.False(t, scope.isAllowed("blocked"))
}

func TestIsAllowed_DenyPolicy_NoAllowlist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyDeny, nil, nil, testLogger)
	assert.False(t, scope.isAllowed("any_tool"))
}

func TestIsAllowed_DenyPolicy_WithAllowlist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyDeny, []string{"allowed"}, nil, testLogger)
	assert.True(t, scope.isAllowed("allowed"))
	assert.False(t, scope.isAllowed("other"))
}

func TestIsAllowed_DenylistOverridesAllowlist(t *testing.T) {
	// Tool is in both lists — deny wins
	scope := NewScopeEnforcer(PolicyAllow, []string{"tool"}, []string{"tool"}, testLogger)
	assert.False(t, scope.isAllowed("tool"))
}

func TestIsAllowed_DenyPolicy_DenylistOverridesAllowlist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyDeny, []string{"tool"}, []string{"tool"}, testLogger)
	assert.False(t, scope.isAllowed("tool"))
}

func TestScope_EmptyToolName_Passthrough(t *testing.T) {
	scope := NewScopeEnforcer(PolicyDeny, nil, nil, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	// initReq has no CallToolParams, so extractToolName returns ""
	result, err := handler(context.Background(), "tools/call", initReq())
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestScope_MultipleAllowlist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyDeny, []string{"read", "write", "list"}, nil, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	tests := []struct {
		tool    string
		allowed bool
	}{
		{"read", true},
		{"write", true},
		{"list", true},
		{"delete", false},
		{"admin", false},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := handler(context.Background(), "tools/call", toolCallReq(tt.tool))
			if tt.allowed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not permitted")
			}
		})
	}
}

func TestScope_MultipleDenylist(t *testing.T) {
	scope := NewScopeEnforcer(PolicyAllow, nil, []string{"drop_table", "rm_rf", "format_disk"}, testLogger)
	mw := scope.Middleware()
	handler := mw(successHandler)

	tests := []struct {
		tool    string
		allowed bool
	}{
		{"safe_tool", true},
		{"drop_table", false},
		{"rm_rf", false},
		{"format_disk", false},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := handler(context.Background(), "tools/call", toolCallReq(tt.tool))
			if tt.allowed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestExtractToolName_CallToolParams(t *testing.T) {
	req := toolCallReq("my_tool")
	assert.Equal(t, "my_tool", extractToolName(req))
}

func TestExtractToolName_NonCallToolParams(t *testing.T) {
	req := initReq()
	assert.Equal(t, "", extractToolName(req))
}

func TestScopePolicy_Constants(t *testing.T) {
	assert.Equal(t, ScopePolicy("allow"), PolicyAllow)
	assert.Equal(t, ScopePolicy("deny"), PolicyDeny)
}
