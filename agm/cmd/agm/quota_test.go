package main

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// A published state older than --max-age is unevaluated, not evidence that
// a provider is still halted. --check must fail open past that age, the
// same way SpawnGate does, or an orchestrator polling this command can
// keep admissions halted indefinitely off a BreakerState the refresh job
// stopped updating long ago (codex review on #1218).
func TestCheckQuotaMeterStateFailsOpenOnAStaleReading(t *testing.T) {
	now := time.Now()
	state := &quota.State{GeneratedAt: now.Add(-2 * time.Hour)}
	providers := []quota.ProviderState{{Family: "openai", BreakerState: string(quota.BreakerOpen), Reason: "spent"}}

	err := checkQuotaMeterState(state, providers, 90*time.Minute, now)
	if err != nil {
		t.Fatalf("want nil (stale reading must not gate), got %v", err)
	}
}

func TestCheckQuotaMeterStateRefusesOnAFreshOpenBreaker(t *testing.T) {
	now := time.Now()
	state := &quota.State{GeneratedAt: now.Add(-5 * time.Minute)}
	providers := []quota.ProviderState{{Family: "openai", BreakerState: string(quota.BreakerOpen), Reason: "spent"}}

	err := checkQuotaMeterState(state, providers, 90*time.Minute, now)
	if err == nil {
		t.Fatal("want an error, a fresh open breaker must gate --check")
	}
	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.code != ExitStateConflict {
		t.Errorf("err = %v, want an exitError with code ExitStateConflict", err)
	}
}

func TestCheckQuotaMeterStatePassesWhenNothingIsOpen(t *testing.T) {
	now := time.Now()
	state := &quota.State{GeneratedAt: now.Add(-5 * time.Minute)}
	providers := []quota.ProviderState{{Family: "openai", BreakerState: string(quota.BreakerThrottled), Reason: "close to the floor"}}

	if err := checkQuotaMeterState(state, providers, 90*time.Minute, now); err != nil {
		t.Errorf("want nil (throttled is not a --check failure), got %v", err)
	}
}

func TestCheckQuotaMeterStateIgnoresAgeWhenMaxAgeDisabled(t *testing.T) {
	now := time.Now()
	state := &quota.State{GeneratedAt: now.Add(-1000 * time.Hour)}
	providers := []quota.ProviderState{{Family: "openai", BreakerState: string(quota.BreakerOpen), Reason: "spent"}}

	// maxAge <= 0 means "accept any age" for the freshness check itself,
	// mirroring quota.SpawnGate.MaxAge's own zero/negative convention —
	// so an ancient reading must still be able to gate --check when the
	// operator has explicitly disabled the staleness cutoff.
	err := checkQuotaMeterState(state, providers, -1, now)
	if err == nil {
		t.Fatal("want an error: maxAge<=0 disables the staleness check, not the breaker check")
	}
}

func TestRequireAuditedCodexBarVersionRefusesBelowTheFloor(t *testing.T) {
	err := requireAuditedCodexBarVersion(&quota.Snapshot{SourceVersion: "0.48.9"})
	if err == nil {
		t.Fatal("want an error for a below-floor codexbar version")
	}
}

func TestRequireAuditedCodexBarVersionAllowsTheFloorAndAbove(t *testing.T) {
	for _, v := range []string{quota.MinAuditedCodexBarVersion, "0.49.2", "1.0.0"} {
		if err := requireAuditedCodexBarVersion(&quota.Snapshot{SourceVersion: v}); err != nil {
			t.Errorf("version %q: want no error, got %v", v, err)
		}
	}
}

// An empty SourceVersion must refuse to publish, not pass through. This is
// the one place in the package where "no evidence" must NOT default to
// "don't gate": the whole point of the check is to keep an unauditable
// build's readings from ever reaching the published state at all, so
// missing evidence of meeting the floor must be treated the same as
// evidence of missing it (codex review on #1218, second pass).
func TestRequireAuditedCodexBarVersionRefusesAnEmptyVersion(t *testing.T) {
	if err := requireAuditedCodexBarVersion(&quota.Snapshot{}); err == nil {
		t.Fatal("want an error for an empty codexbar version, not a silent pass")
	}
}

func TestRequireAuditedCodexBarVersionAllowsANilSnapshot(t *testing.T) {
	// Defensive only: meter.Refresh never returns a nil snapshot on
	// success, so this path exists to make the nil check's intent
	// explicit rather than to document a real caller.
	if err := requireAuditedCodexBarVersion(nil); err != nil {
		t.Errorf("nil snapshot: want no error, got %v", err)
	}
}
