package circuitbreaker

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

type stubQuotaGate struct {
	decision quota.SpawnDecision
	asked    string
	calls    int
}

func (g *stubQuotaGate) AllowSpawn(model string) quota.SpawnDecision {
	g.asked = model
	g.calls++
	return g.decision
}

func TestProviderQuotaGatePassesWhenAllowed(t *testing.T) {
	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed: true, Family: "gemini", Reason: "Gemini Models has 98.8% left",
	}}
	got := checkProviderQuota(gate, "gemini-3.1-pro")
	if !got.Passed {
		t.Errorf("Passed = false, want true (%s)", got.Message)
	}
	if got.Gate != providerQuotaGateName {
		t.Errorf("Gate = %q, want %q", got.Gate, providerQuotaGateName)
	}
	if gate.asked != "gemini-3.1-pro" {
		t.Errorf("gate asked about %q", gate.asked)
	}
}

func TestProviderQuotaGateRefusesAnExhaustedProvider(t *testing.T) {
	resets := time.Now().Add(4 * time.Hour).UTC()
	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed:  false,
		Family:   "openai",
		State:    quota.BreakerOpen,
		Reason:   "Weekly has 1.0% left, at or below the 3.0% halt floor",
		ResetsAt: resets,
	}}
	got := checkProviderQuota(gate, "gpt-5.5-pro")
	if got.Passed {
		t.Fatal("want the gate to refuse")
	}
	for _, want := range []string{"openai", "1.0%", resets.Format(time.RFC3339), "agm quota", quota.SpawnGateOverrideEnvVar} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("message missing %q: %s", want, got.Message)
		}
	}
}

func TestCheckRunsTheQuotaGateOnlyWhenWired(t *testing.T) {
	gate := &stubQuotaGate{decision: quota.SpawnDecision{Allowed: true, Family: "openai"}}
	cfg := Config{}

	result := Check(cfg, stubLoad{}, stubWorkers{}, &stubTimer{}, nil)
	if gateRan(result, providerQuotaGateName) {
		t.Error("the quota gate must not run unless it is wired")
	}
	if gate.calls != 0 {
		t.Error("unwired gate was consulted")
	}

	result = Check(cfg, stubLoad{}, stubWorkers{}, &stubTimer{}, nil,
		WithProviderQuota(gate, "gpt-5.5-pro"))
	if !gateRan(result, providerQuotaGateName) {
		t.Fatal("the quota gate did not run when wired")
	}
	if gate.calls != 1 {
		t.Errorf("gate consulted %d times, want 1", gate.calls)
	}
}

func TestCheckRefusesWhenTheQuotaGateRefuses(t *testing.T) {
	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed: false, Family: "openai", Reason: "spent",
	}}
	result := Check(Config{}, stubLoad{}, stubWorkers{}, &stubTimer{}, nil,
		WithProviderQuota(gate, "gpt-5.5-pro"))
	if result.Allowed {
		t.Fatal("want the spawn refused")
	}
	denied := FormatDenied(result)
	if !strings.Contains(denied, providerQuotaGateName) {
		t.Errorf("FormatDenied should name the gate: %s", denied)
	}
}

// An empty model means the caller could not say which budget this spawn
// draws down, so the gate has nothing to check and must stay out of the way.
func TestWithProviderQuotaIgnoresAnEmptyModelOrNilGate(t *testing.T) {
	gate := &stubQuotaGate{decision: quota.SpawnDecision{Allowed: false, Reason: "spent"}}

	result := Check(Config{}, stubLoad{}, stubWorkers{}, &stubTimer{}, nil,
		WithProviderQuota(gate, ""))
	if !result.Allowed {
		t.Error("an empty model must leave the gate off")
	}

	result = Check(Config{}, stubLoad{}, stubWorkers{}, &stubTimer{}, nil,
		WithProviderQuota(nil, "gpt-5.5-pro"))
	if !result.Allowed {
		t.Error("a nil gate must leave the gate off")
	}
}

// This gate deliberately inverts the fail-closed rule the resource gates
// follow. The production gate reads a file that will usually be absent.
func TestDefaultProviderQuotaGateAllowsWithNoPublishedReading(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	got := checkProviderQuota(DefaultProviderQuotaGate(nil), "claude-opus-4-8")
	if !got.Passed {
		t.Errorf("want a pass with no published reading, got %q", got.Message)
	}
}

// A pass the guardrail could not evaluate must not read like a measured one.
// Telling the two apart in the log is what makes an inert gate visible.
func TestProviderQuotaGateLabelsAnUnevaluatedPass(t *testing.T) {
	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed:   true,
		Reason:    "model \"sonnet\" is not mapped to a metered provider",
		Evaluated: false,
	}}
	got := checkProviderQuota(gate, "sonnet")
	if !got.Passed {
		t.Fatal("an unevaluated decision still passes")
	}
	if !strings.Contains(got.Message, "not evaluated") {
		t.Errorf("message does not mark the pass as unevaluated: %s", got.Message)
	}
}

func TestProviderQuotaGateDoesNotLabelAnEvaluatedPass(t *testing.T) {
	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed:   true,
		Family:    "anthropic",
		Reason:    "Weekly has 82.0% left",
		Evaluated: true,
	}}
	got := checkProviderQuota(gate, "sonnet")
	if strings.Contains(got.Message, "not evaluated") {
		t.Errorf("a measured pass must not be labelled unevaluated: %s", got.Message)
	}
}
