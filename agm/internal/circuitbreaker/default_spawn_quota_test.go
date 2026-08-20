// This file is an external test package on purpose. The harness catalog in
// agm/internal/agent reaches circuitbreaker through harnessexec, so an
// in-package test cannot import it. That dependency direction is also why
// DefaultProviderQuotaGate takes the family resolver rather than building one.
package circuitbreaker_test

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// publishState points the default state-file resolver at a temp file holding
// the given providers.
func publishState(t *testing.T, providers ...quota.ProviderState) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path, err := quota.DefaultStateFilePath()
	if err != nil {
		t.Fatalf("DefaultStateFilePath: %v", err)
	}
	err = quota.WriteStateFile(path, &quota.State{
		Version:     quota.StateFileVersion,
		GeneratedAt: time.Now().UTC(),
		WrittenAt:   time.Now().UTC(),
		Source:      "test",
		Providers:   providers,
	})
	if err != nil {
		t.Fatalf("WriteStateFile: %v", err)
	}
}

func exhausted(family string, resets time.Time) quota.ProviderState {
	return quota.ProviderState{
		Family:            family,
		SourceID:          family,
		Readable:          true,
		RoutingClass:      "avoid",
		BreakerState:      string(quota.BreakerOpen),
		RemainingPercent:  0.5,
		ConstrainedWindow: "Weekly",
		ResetsAt:          &resets,
		Reason:            "Weekly has 0.5% left, at or below the halt floor",
	}
}

// defaultGate builds the production gate exactly as the admission path does.
func defaultGate(harness string) circuitbreaker.ProviderQuotaGate {
	return circuitbreaker.DefaultProviderQuotaGate(func(model string) string {
		return agent.ModelFamilyForHarnessModel(harness, model)
	})
}

// This is the defect. A spawn that names its model by the harness default
// alias — which is every launch that does not pass --model — was allowed
// straight through an exhausted provider, because the alias matched no vendor
// substring and the gate scored it "not mapped to a metered provider". The
// guardrail was inert on the only path most spawns ever take.
func TestQuotaGateRefusesTheDefaultSpawnOfEveryHarness(t *testing.T) {
	for harness, alias := range agent.HarnessDefaults {
		family := agent.ModelFamilyForHarnessModel(harness, alias)
		if family == "" {
			t.Errorf("harness %s: default model %q maps to no provider family", harness, alias)
			continue
		}
		t.Run(harness, func(t *testing.T) {
			publishState(t, exhausted(family, time.Now().Add(3*time.Hour).UTC()))
			got := defaultGate(harness).AllowSpawn(alias)
			if got.Allowed {
				t.Fatalf("default spawn %s/%s was allowed against an exhausted %s budget: %s",
					harness, alias, family, got.Reason)
			}
			if got.Family != family {
				t.Errorf("Family = %q, want %q", got.Family, family)
			}
			if !got.Evaluated {
				t.Error("a refusal must be an evaluated decision")
			}
		})
	}
}

// `agm supervisor run` pins "sonnet-200k", which is outside HarnessDefaults
// and was equally unmapped. Supervisors are the fleet's highest-volume
// spawner, so this path escaping the guardrail is the expensive half.
func TestQuotaGateRefusesTheSupervisorDefaultSpawn(t *testing.T) {
	publishState(t, exhausted("anthropic", time.Now().Add(3*time.Hour).UTC()))
	got := defaultGate("claude-code").AllowSpawn("sonnet-200k")
	if got.Allowed {
		t.Fatalf("supervisor default spawn was allowed against an exhausted budget: %s", got.Reason)
	}
}

// A cross-harness tier alias bills the harness it actually launches on, not
// the vendor its name evokes: "sonnet" on codex-cli runs gpt-5.5.
func TestQuotaGateAttributesACrossHarnessAliasToTheLaunchingProvider(t *testing.T) {
	publishState(t, exhausted("openai", time.Now().Add(3*time.Hour).UTC()))
	got := defaultGate("codex-cli").AllowSpawn("sonnet")
	if got.Family != "openai" {
		t.Fatalf("Family = %q, want openai — codex-cli's \"sonnet\" launches gpt-5.5", got.Family)
	}
	if got.Allowed {
		t.Errorf("want a refusal against the exhausted OpenAI budget: %s", got.Reason)
	}
}

// The fix must be a working guardrail, not a blanket refusal.
func TestQuotaGateAdmitsTheDefaultSpawnWhenTheBudgetIsHealthy(t *testing.T) {
	publishState(t, quota.ProviderState{
		Family:           "anthropic",
		SourceID:         "anthropic",
		Readable:         true,
		RoutingClass:     "healthy",
		BreakerState:     string(quota.BreakerClosed),
		RemainingPercent: 82,
		Reason:           "Weekly has 82.0% left",
	})
	got := defaultGate("claude-code").AllowSpawn("sonnet")
	if !got.Allowed {
		t.Fatalf("healthy budget refused the default spawn: %s", got.Reason)
	}
	if !got.Evaluated {
		t.Error("a pass from a fresh readable reading must be Evaluated")
	}
}

// With no published reading the gate still fails open, but must not report
// that as a measured pass.
func TestQuotaGateMarksAFailOpenAsUnevaluated(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	var warnings []string
	gate := &quota.SpawnGate{
		FamilyForModel: func(model string) string {
			return agent.ModelFamilyForHarnessModel("claude-code", model)
		},
		Warn: func(msg string) { warnings = append(warnings, msg) },
	}
	got := gate.AllowSpawn("sonnet")
	if !got.Allowed {
		t.Fatalf("want a fail-open pass with no published reading: %s", got.Reason)
	}
	if got.Evaluated {
		t.Error("a spawn allowed without a reading must not report Evaluated")
	}
	if got.Family != "anthropic" {
		t.Errorf("Family = %q, want anthropic: the fail-open must still name the provider it could not check", got.Family)
	}
	if len(warnings) != 1 {
		t.Fatalf("want exactly one warning, got %d: %v", len(warnings), warnings)
	}
}
