package router

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func TestAIExecutor_NodeModelOverridesRole(t *testing.T) {
	cfg := &Config{Version: 1, DefaultRole: "research", Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7"},
	}}
	role := &fakeProvider{name: "anthropic"}
	literal := &fakeProvider{name: "openai"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": role,
		"openai|gpt-4o":             literal,
	})
	exec := NewAIExecutor(r)

	out, err := exec.Generate(
		context.Background(),
		&workflow.AINode{
			Model:  "gpt-4o",
			Role:   "research", // should be ignored when Model is set
			Prompt: "hi",
		},
		nil, nil,
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
	if literal.calls != 1 {
		t.Errorf("literal calls = %d, want 1", literal.calls)
	}
	if role.calls != 0 {
		t.Errorf("role calls = %d, want 0 (Model overrides Role)", role.calls)
	}
}

func TestAIExecutor_NodeRoleRoutesViaConfig(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"implementer": {Primary: "claude-opus-4-7"},
	}}
	prov := &fakeProvider{name: "anthropic"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": prov,
	})
	exec := NewAIExecutor(r)

	if _, err := exec.Generate(context.Background(), &workflow.AINode{
		Role:   "implementer",
		Prompt: "ship it",
	}, nil, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if prov.calls != 1 {
		t.Errorf("calls = %d, want 1", prov.calls)
	}
}

func TestAIExecutor_NoRoleNoModelUsesDefault(t *testing.T) {
	cfg := &Config{Version: 1, DefaultRole: "orchestrator", Roles: map[string]RoleSpec{
		"orchestrator": {Primary: "claude-sonnet-4-6"},
	}}
	prov := &fakeProvider{name: "anthropic"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-sonnet-4-6": prov,
	})
	exec := NewAIExecutor(r)

	if _, err := exec.Generate(context.Background(), &workflow.AINode{
		Prompt: "hello",
	}, nil, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if prov.calls != 1 {
		t.Errorf("calls = %d, want 1", prov.calls)
	}
}

func TestAIExecutor_NoRoleNoDefaultErrors(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7"},
	}}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": &fakeProvider{name: "anthropic"},
	})
	exec := NewAIExecutor(r)

	_, err := exec.Generate(context.Background(), &workflow.AINode{Prompt: "hi"}, nil, nil)
	if err == nil {
		t.Fatal("expected error when neither Model, Role, nor default_role is set")
	}
}

func TestAIExecutor_PropagatesRouterError(t *testing.T) {
	cfg := &Config{Version: 1, DefaultRole: "research", Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7"},
	}}
	want := errors.New("anthropic exploded")
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": &fakeProvider{name: "anthropic", err: want},
	})
	exec := NewAIExecutor(r)

	_, err := exec.Generate(context.Background(), &workflow.AINode{Prompt: "hi"}, nil, nil)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped %v, got %v", want, err)
	}
}
