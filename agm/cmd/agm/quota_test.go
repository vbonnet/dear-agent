package main

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// A published state older than --max-age is unevaluated, not evidence that
// a provider is still halted — but "unevaluated" must not read as exit 0
// either, since that is indistinguishable from a fresh reading positively
// confirming nothing is halted. --check on a stale reading reports exit 1
// ("no usable reading"), never the frozen open-breaker's exit 4 and never
// a silent success (codex review on #1218, third pass corrects the
// original fix, which returned nil here).
func TestCheckQuotaMeterStateReportsUnevaluatedOnAStaleReading(t *testing.T) {
	now := time.Now()
	state := &quota.State{GeneratedAt: now.Add(-2 * time.Hour)}
	providers := []quota.ProviderState{{Family: "openai", BreakerState: string(quota.BreakerOpen), Reason: "spent"}}

	err := checkQuotaMeterState(state, providers, 90*time.Minute, now)
	if err == nil {
		t.Fatal("want an error: a stale reading must not report as a confirmed-healthy exit 0")
	}
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		t.Errorf("want a plain error (exit 1, generic), not an exitError with code %d — a stale reading is not a state conflict", exitErr.code)
	}
}

// An undated reading (GeneratedAt zero) must report the same as a stale
// one, not as a confirmed-healthy exit 0: State.Age reports zero age for
// it by design, which would otherwise pass the maxAge comparison
// unconditionally (codex review on #1218, fourth pass).
func TestCheckQuotaMeterStateReportsUnevaluatedOnAnUndatedReading(t *testing.T) {
	now := time.Now()
	state := &quota.State{} // GeneratedAt zero
	providers := []quota.ProviderState{{Family: "openai", BreakerState: string(quota.BreakerOpen), Reason: "spent"}}

	err := checkQuotaMeterState(state, providers, 90*time.Minute, now)
	if err == nil {
		t.Fatal("want an error: an undated reading must not report as a confirmed-healthy exit 0")
	}
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		t.Errorf("want a plain error (exit 1, generic), not an exitError with code %d", exitErr.code)
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
