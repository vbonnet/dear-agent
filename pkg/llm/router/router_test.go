package router

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
)

// fakeProvider is the test seam for the router. It records calls and
// returns canned responses or errors per-instance.
type fakeProvider struct {
	name string

	mu    sync.Mutex
	calls int
	resp  *provider.GenerateResponse
	err   error

	// errOnce, if true, fails the first call and succeeds afterwards.
	// Useful for checking that the router falls through to the next
	// candidate on failure but the breaker doesn't stay open forever.
	errOnce bool
}

func (f *fakeProvider) Name() string                  { return f.name }
func (f *fakeProvider) Capabilities() provider.Capabilities { return provider.Capabilities{} }

func (f *fakeProvider) Generate(_ context.Context, req *provider.GenerateRequest) (*provider.GenerateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		err := f.err
		if f.errOnce {
			f.err = nil
		}
		return nil, err
	}
	if f.resp != nil {
		// Echo the model so tests can assert the resolver/router set it.
		clone := *f.resp
		clone.Model = req.Model
		return &clone, nil
	}
	return &provider.GenerateResponse{Text: f.name + " ok", Model: req.Model}, nil
}

func newRouter(t *testing.T, cfg *Config, providers map[string]provider.Provider) *Router {
	t.Helper()
	r, err := New(Options{
		Config: cfg,
		Factory: func(family, model string) (provider.Provider, error) {
			key := family + "|" + model
			p, ok := providers[key]
			if !ok {
				return nil, errors.New("no provider registered for " + key)
			}
			return p, nil
		},
		// Trip after a single failure so tests don't need to send N
		// failures to observe breaker behaviour.
		CircuitBreaker: provider.CircuitBreakerConfig{FailureThreshold: 1},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func TestRouter_New_RequiresConfig(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error when Config is nil")
	}
}

func TestRouter_Generate_PrimarySucceeds(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7", Secondary: "gpt-4o"},
	}}
	primary := &fakeProvider{name: "anthropic"}
	secondary := &fakeProvider{name: "openai"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": primary,
		"openai|gpt-4o":             secondary,
	})

	resp, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "claude-opus-4-7" {
		t.Errorf("response model = %q, want claude-opus-4-7", resp.Model)
	}
	if primary.calls != 1 {
		t.Errorf("primary calls = %d, want 1", primary.calls)
	}
	if secondary.calls != 0 {
		t.Errorf("secondary calls = %d, want 0", secondary.calls)
	}
}

func TestRouter_Generate_FallsThroughOnError(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {
			Primary:   "claude-opus-4-7",
			Secondary: "gpt-5-pro",
			Tertiary:  "gemini-2.5-flash",
		},
	}}
	primary := &fakeProvider{name: "anthropic", err: errors.New("boom")}
	secondary := &fakeProvider{name: "openai", err: errors.New("also boom")}
	tertiary := &fakeProvider{name: "gemini"}

	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": primary,
		"openai|gpt-5-pro":          secondary,
		"gemini|gemini-2.5-flash":   tertiary,
	})

	resp, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "gemini-2.5-flash" {
		t.Errorf("response model = %q, want gemini-2.5-flash", resp.Model)
	}
	if primary.calls != 1 || secondary.calls != 1 || tertiary.calls != 1 {
		t.Errorf("call counts = primary:%d secondary:%d tertiary:%d (want 1,1,1)",
			primary.calls, secondary.calls, tertiary.calls)
	}
}

func TestRouter_Generate_AllFail(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7", Secondary: "gpt-4o"},
	}}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": &fakeProvider{name: "anthropic", err: errors.New("rate limited")},
		"openai|gpt-4o":             &fakeProvider{name: "openai", err: errors.New("server down")},
	})

	_, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"})
	if err == nil {
		t.Fatal("expected error when all candidates fail")
	}
	// The error message should mention both candidates and the role.
	msg := err.Error()
	for _, want := range []string{"research", "claude-opus-4-7", "gpt-4o", "exhausted"} {
		if !contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestRouter_Generate_ContextCancelledStopsImmediately(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7", Secondary: "gpt-4o"},
	}}
	primary := &fakeProvider{name: "anthropic", err: context.Canceled}
	secondary := &fakeProvider{name: "openai"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-opus-4-7": primary,
		"openai|gpt-4o":             secondary,
	})

	_, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if secondary.calls != 0 {
		t.Errorf("secondary should NOT be called on cancellation, got %d calls", secondary.calls)
	}
}

func TestRouter_Generate_DefaultRole(t *testing.T) {
	cfg := &Config{Version: 1, DefaultRole: "orchestrator", Roles: map[string]RoleSpec{
		"orchestrator": {Primary: "claude-sonnet-4-6"},
	}}
	prov := &fakeProvider{name: "anthropic"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"anthropic|claude-sonnet-4-6": prov,
	})

	if _, err := r.Generate(context.Background(), "", &provider.GenerateRequest{Prompt: "hi"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if prov.calls != 1 {
		t.Errorf("prov.calls = %d, want 1", prov.calls)
	}
}

func TestRouter_Generate_NoRoleAndNoDefault(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "gpt-4o"},
	}}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"openai|gpt-4o": &fakeProvider{name: "openai"},
	})
	if _, err := r.Generate(context.Background(), "", &provider.GenerateRequest{Prompt: "hi"}); err == nil {
		t.Fatal("expected error when no role and no default")
	}
}

func TestRouter_Generate_UnknownRole(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{}}
	r := newRouter(t, cfg, nil)
	if _, err := r.Generate(context.Background(), "ghost", &provider.GenerateRequest{Prompt: "hi"}); err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestRouter_Generate_NilRequest(t *testing.T) {
	cfg := &Config{Version: 1, DefaultRole: "research", Roles: map[string]RoleSpec{
		"research": {Primary: "gpt-4o"},
	}}
	r := newRouter(t, cfg, nil)
	if _, err := r.Generate(context.Background(), "research", nil); err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestRouter_GenerateForModel_RoutesDirectly(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{}}
	prov := &fakeProvider{name: "openai"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"openai|gpt-4o": prov,
	})
	resp, err := r.GenerateForModel(context.Background(), "gpt-4o", &provider.GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("GenerateForModel: %v", err)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", resp.Model)
	}
	if prov.calls != 1 {
		t.Errorf("calls = %d, want 1", prov.calls)
	}
}

func TestRouter_GenerateForModel_RejectsEmptyID(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{}}
	r := newRouter(t, cfg, nil)
	if _, err := r.GenerateForModel(context.Background(), "", &provider.GenerateRequest{Prompt: "hi"}); err == nil {
		t.Fatal("expected error on empty model id")
	}
}

func TestRouter_ProviderCachedAcrossCalls(t *testing.T) {
	cfg := &Config{Version: 1, DefaultRole: "research", Roles: map[string]RoleSpec{
		"research": {Primary: "gpt-4o"},
	}}
	var built int
	r, err := New(Options{
		Config: cfg,
		Factory: func(family, model string) (provider.Provider, error) {
			built++
			return &fakeProvider{name: family}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if built != 1 {
		t.Errorf("factory called %d times, want 1 (provider should be cached)", built)
	}
}

func TestRouter_HasRoleAndDefaultRole(t *testing.T) {
	cfg := &Config{Version: 1, DefaultRole: "research", Roles: map[string]RoleSpec{
		"research":     {Primary: "gpt-4o"},
		"implementer":  {Primary: "claude-opus-4-7"},
	}}
	r, err := New(Options{Config: cfg, Factory: func(_, _ string) (provider.Provider, error) {
		return &fakeProvider{}, nil
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !r.HasRole("research") {
		t.Error("HasRole(research) = false")
	}
	if r.HasRole("ghost") {
		t.Error("HasRole(ghost) = true")
	}
	if r.HasRole("") {
		t.Error("HasRole(\"\") = true")
	}
	if r.DefaultRole() != "research" {
		t.Errorf("DefaultRole = %q, want research", r.DefaultRole())
	}
}

func TestRouter_RoleWithUnresolvableModelFallsThrough(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "totally-made-up-model", Secondary: "gpt-4o"},
	}}
	good := &fakeProvider{name: "openai"}
	r := newRouter(t, cfg, map[string]provider.Provider{
		"openai|gpt-4o": good,
	})
	resp, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o (fall-through after resolve failure)", resp.Model)
	}
	if good.calls != 1 {
		t.Errorf("openai calls = %d, want 1", good.calls)
	}
}

// contains is a tiny strings.Contains shim so this file doesn't need to
// import the strings package just for the assertion.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
