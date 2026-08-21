package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// quotaConfig is one role whose chain spans all three vendors, which is
// the shape the shipped roles.yaml uses.
func quotaConfig() *Config {
	return &Config{
		Version:     1,
		DefaultRole: "implementer",
		Roles: map[string]RoleSpec{
			"implementer": {
				Primary:   "claude-opus-4-8",
				Secondary: "gpt-5.5-pro",
				Tertiary:  "gemini-3.1-pro",
			},
		},
	}
}

func quotaProviders() map[string]provider.Provider {
	return map[string]provider.Provider{
		"anthropic|claude-opus-4-8": &fakeProvider{name: "anthropic"},
		"openai|gpt-5.5-pro":        &fakeProvider{name: "openai"},
		"gemini|gemini-3.1-pro":     &fakeProvider{name: "gemini"},
	}
}

type fixedReader struct{ snapshot *quota.Snapshot }

func (f fixedReader) Read(context.Context) (*quota.Snapshot, error) { return f.snapshot, nil }

// meterWith builds a warmed meter over a hand-written reading, one
// window per family.
func meterWith(t *testing.T, remaining map[string]float64) *quota.Meter {
	t.Helper()
	snapshot := &quota.Snapshot{Source: "test", GeneratedAt: time.Now()}
	for family, pct := range remaining {
		snapshot.Providers = append(snapshot.Providers, quota.ProviderQuota{
			Family:       family,
			SourceID:     family,
			Availability: quota.AvailabilityOK,
			Windows: []quota.Window{{
				ID:               "weekly",
				Label:            "Weekly",
				RemainingPercent: pct,
				UsedPercent:      100 - pct,
			}},
		})
	}
	meter := quota.New(quota.Options{Reader: fixedReader{snapshot: snapshot}, RefreshInterval: -1})
	if _, err := meter.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh meter: %v", err)
	}
	return meter
}

func newQuotaRouter(t *testing.T, meter *quota.Meter, providers map[string]provider.Provider) *Router {
	t.Helper()
	r, err := New(Options{
		Config: quotaConfig(),
		Quota:  meter,
		Factory: func(family, model string) (provider.Provider, error) {
			p, ok := providers[family+"|"+model]
			if !ok {
				return nil, errors.New("no provider registered for " + family + "|" + model)
			}
			return p, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func TestRouterRoutesToTheRoomiestProvider(t *testing.T) {
	providers := quotaProviders()
	// The configured primary (Anthropic) is nearly spent; Gemini has the
	// most headroom of all three, but gemini is excluded from quota
	// promotion (no constructible provider yet — review on #1218), so
	// OpenAI, the roomiest candidate that can actually be promoted, takes
	// the request even though roles.yaml ranks it after Anthropic.
	meter := meterWith(t, map[string]float64{
		"anthropic": 8,
		"openai":    55,
		"gemini":    95,
	})
	r := newQuotaRouter(t, meter, providers)

	resp, err := r.Generate(context.Background(), "implementer", &provider.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "gpt-5.5-pro" {
		t.Errorf("routed to %q, want gpt-5.5-pro", resp.Model)
	}
	if got := providers["anthropic|claude-opus-4-8"].(*fakeProvider).calls; got != 0 {
		t.Errorf("the near-limit provider was called %d times, want 0", got)
	}
	if got := providers["gemini|gemini-3.1-pro"].(*fakeProvider).calls; got != 0 {
		t.Errorf("the unimplemented-provider family was called %d times, want 0", got)
	}
}

func TestRouterKeepsConfiguredOrderWhenQuotaDoesNotDistinguish(t *testing.T) {
	providers := quotaProviders()
	meter := meterWith(t, map[string]float64{
		"anthropic": 90,
		"openai":    88,
		"gemini":    95,
	})
	r := newQuotaRouter(t, meter, providers)

	resp, err := r.Generate(context.Background(), "implementer", &provider.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "claude-opus-4-8" {
		t.Errorf("routed to %q, want the configured primary claude-opus-4-8", resp.Model)
	}
}

// The fail-safe contract at the router boundary: no meter, an unreadable
// meter, and a stale meter must all route exactly as before.
func TestRouterWithoutUsableQuotaRoutesAsConfigured(t *testing.T) {
	unreadable := quota.New(quota.Options{
		Reader: fixedReader{snapshot: &quota.Snapshot{
			Source:      "test",
			GeneratedAt: time.Now(),
			Providers: []quota.ProviderQuota{{
				Family:       "anthropic",
				Availability: quota.AvailabilityAuthRequired,
				Note:         "No Claude session key found in browser cookies.",
			}},
		}},
		RefreshInterval: -1,
	})
	if _, err := unreadable.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	stale := quota.New(quota.Options{
		Reader: fixedReader{snapshot: &quota.Snapshot{
			Source:      "test",
			GeneratedAt: time.Now().Add(-24 * time.Hour),
			Providers: []quota.ProviderQuota{{
				Family:       "anthropic",
				Availability: quota.AvailabilityOK,
				Windows:      []quota.Window{{Label: "Weekly", RemainingPercent: 1}},
			}},
		}},
		Policy:          quota.Policy{MaxSnapshotAge: time.Minute},
		RefreshInterval: -1,
	})
	if _, err := stale.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	tests := []struct {
		name  string
		meter *quota.Meter
	}{
		{name: "no meter", meter: nil},
		{name: "meter with no reader", meter: quota.New(quota.Options{})},
		{name: "provider needs credentials", meter: unreadable},
		{name: "reading past its max age", meter: stale},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newQuotaRouter(t, tc.meter, quotaProviders())
			resp, err := r.Generate(context.Background(), "implementer", &provider.GenerateRequest{})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if resp.Model != "claude-opus-4-8" {
				t.Errorf("routed to %q, want the configured primary claude-opus-4-8", resp.Model)
			}
		})
	}
}

func TestRouterStillFallsThroughAnExhaustedChain(t *testing.T) {
	providers := quotaProviders()
	// Everything is spent. The router must still try, and still succeed,
	// rather than refusing to route.
	meter := meterWith(t, map[string]float64{"anthropic": 0, "openai": 0, "gemini": 0})
	r := newQuotaRouter(t, meter, providers)

	resp, err := r.Generate(context.Background(), "implementer", &provider.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model == "" {
		t.Fatal("want a routed model even when every provider is out of quota")
	}
}

func TestRouterRecordsTheQuotaVerdictInMetadata(t *testing.T) {
	meter := meterWith(t, map[string]float64{"anthropic": 90, "openai": 55, "gemini": 95})
	r := newQuotaRouter(t, meter, quotaProviders())

	resp, err := r.Generate(context.Background(), "implementer", &provider.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := resp.Metadata["router_quota_class"]; got != string(quota.ClassHealthy) {
		t.Errorf("router_quota_class = %v, want healthy", got)
	}
	if got := resp.Metadata["router_quota_family"]; got != "anthropic" {
		t.Errorf("router_quota_family = %v, want anthropic", got)
	}
	if got := resp.Metadata["router_quota_remaining"]; got != 90.0 {
		t.Errorf("router_quota_remaining = %v, want 90", got)
	}
	if got := resp.Metadata["router_quota_window"]; got != "Weekly" {
		t.Errorf("router_quota_window = %v, want Weekly", got)
	}
}

// TestRouterOmitsQuotaMetadataOnCircuitBreakerFallback guards against
// attributing the failed primary candidate's quota verdict to a response
// that the circuit breaker's fallback provider actually served: the
// fallback is a different (family, model) than decisions[i] was evaluated
// against, so its quota class/remaining would describe the wrong provider
// (codex review on #1218).
func TestRouterOmitsQuotaMetadataOnCircuitBreakerFallback(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7"},
	}}
	primary := &fakeProvider{name: "anthropic", err: errors.New("primary failed")}
	fallback := &fakeProvider{name: "openai", resp: &provider.GenerateResponse{Text: "ok"}}
	meter := meterWith(t, map[string]float64{"anthropic": 90})

	r, err := New(Options{
		Config: cfg,
		Quota:  meter,
		Factory: func(_, _ string) (provider.Provider, error) {
			return primary, nil
		},
		CircuitBreaker: provider.CircuitBreakerConfig{
			FailureThreshold: 1,
			FallbackProvider: fallback,
			FallbackModel:    "gpt-4o",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Metadata["router_fallback"] != true {
		t.Fatalf("expected a fallback response, got metadata = %#v", resp.Metadata)
	}
	for key := range resp.Metadata {
		if len(key) > 12 && key[:12] == "router_quota" {
			t.Errorf("fallback response carried the primary candidate's quota metadata: %q = %v", key, resp.Metadata[key])
		}
	}
}

// TestRouterDoesNotSendQuotaMetadataToTheProvider guards the request side
// of the same leak TestRouterOmitsQuotaMetadataOnCircuitBreakerFallback
// guards on the response side. CircuitBreaker.GenerateWithSource forwards
// the router's *GenerateRequest verbatim to its own internal fallback when
// the primary fails, and a real provider (OpenAI) echoes req.Metadata back
// under response Metadata["request_metadata"] — so quota fields merged
// into the outgoing request would ride along into the fallback's response
// and mislabel it with the primary's verdict, even though the top-level
// response metadata correctly omits them on that path (codex review on
// #1218). The fix is to never put quota fields on the request at all;
// this asserts that directly against what each provider actually
// received, independent of what any specific provider does with it.
func TestRouterDoesNotSendQuotaMetadataToTheProvider(t *testing.T) {
	cfg := &Config{Version: 1, Roles: map[string]RoleSpec{
		"research": {Primary: "claude-opus-4-7"},
	}}
	primary := &fakeProvider{name: "anthropic", err: errors.New("primary failed")}
	fallback := &fakeProvider{name: "openai", resp: &provider.GenerateResponse{Text: "ok"}}
	meter := meterWith(t, map[string]float64{"anthropic": 90})

	r, err := New(Options{
		Config: cfg,
		Quota:  meter,
		Factory: func(_, _ string) (provider.Provider, error) {
			return primary, nil
		},
		CircuitBreaker: provider.CircuitBreakerConfig{
			FailureThreshold: 1,
			FallbackProvider: fallback,
			FallbackModel:    "gpt-4o",
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := r.Generate(context.Background(), "research", &provider.GenerateRequest{Prompt: "hi"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for _, p := range []*fakeProvider{primary, fallback} {
		if p.lastReq == nil {
			t.Fatalf("%s: provider was never called", p.name)
		}
		for key := range p.lastReq.Metadata {
			if len(key) > 12 && key[:12] == "router_quota" {
				t.Errorf("%s: request metadata carried a quota field: %q = %v", p.name, key, p.lastReq.Metadata[key])
			}
		}
	}
}

func TestRouterOmitsQuotaMetadataWhenUnmetered(t *testing.T) {
	r := newQuotaRouter(t, nil, quotaProviders())
	resp, err := r.Generate(context.Background(), "implementer", &provider.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for key := range resp.Metadata {
		if len(key) > 12 && key[:12] == "router_quota" {
			t.Errorf("unmetered routing emitted %q", key)
		}
	}
}
