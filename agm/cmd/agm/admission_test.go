package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// Pi authenticates independently of the CLI-subscription accounts CodexBar
// meters (agm/docs/PI-HARNESS.md), so the provider-quota gate must not
// attribute a Pi spawn to whatever family its model alias happens to
// resolve to — that would gate (or admit) it on an unrelated subscription's
// headroom (codex review on #1218).
func TestProviderQuotaFamilyResolverLeavesPiUnmapped(t *testing.T) {
	for _, harness := range []string{"pi", "pi-cli", "Pi-CLI"} {
		if got := providerQuotaFamilyResolver(harness)("sonnet"); got != "" {
			t.Errorf("providerQuotaFamilyResolver(%q)(\"sonnet\") = %q, want \"\" (Pi's billing identity is not established)", harness, got)
		}
	}
}

// Every other harness must keep resolving normally — this is a narrow
// exception for Pi's documented independent credentials, not a general
// weakening of the gate.
func TestProviderQuotaFamilyResolverResolvesOtherHarnessesNormally(t *testing.T) {
	tests := []struct {
		harness string
		model   string
		want    string
	}{
		{harness: "claude-code", model: "sonnet", want: "anthropic"},
		{harness: "codex-cli", model: "5.5", want: "openai"},
	}
	for _, tt := range tests {
		if got := providerQuotaFamilyResolver(tt.harness)(tt.model); got != tt.want {
			t.Errorf("providerQuotaFamilyResolver(%q)(%q) = %q, want %q", tt.harness, tt.model, got, tt.want)
		}
	}
}

// publishAnthropicState writes a minimal published quota state with one
// "anthropic" provider entry in the given breaker state, so
// reserveProviderQuotaAdmissionIfWired's fresh SpawnGate.AllowSpawn call
// has something real to evaluate.
func publishAnthropicState(t *testing.T, breakerState quota.BreakerState) {
	t.Helper()
	statePath, err := quota.DefaultStateFilePath()
	if err != nil {
		t.Fatalf("DefaultStateFilePath: %v", err)
	}
	state := &quota.State{
		Version:     quota.StateFileVersion,
		GeneratedAt: time.Now().UTC(),
		WrittenAt:   time.Now().UTC(),
		Source:      "test",
		Providers: []quota.ProviderState{{
			Family:       "anthropic",
			SourceID:     "anthropic",
			Readable:     true,
			BreakerState: string(breakerState),
			Reason:       "test reading",
		}},
	}
	if err := quota.WriteStateFile(statePath, state); err != nil {
		t.Fatalf("WriteStateFile: %v", err)
	}
}

func readThrottleLedgerCount(t *testing.T, family string) int {
	t.Helper()
	ledgerPath, err := quota.DefaultThrottleLedgerPath()
	if err != nil {
		t.Fatalf("DefaultThrottleLedgerPath: %v", err)
	}
	data, err := os.ReadFile(ledgerPath)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read throttle ledger: %v", err)
	}
	var entries map[string][]time.Time
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse throttle ledger: %v", err)
	}
	return len(entries[family])
}

// The persistent throttle admission must actually be reserved by the real
// spawn boundary. Repo-wide search found admission.afterAuthorization was
// never invoked by any launch path — submitHarnessLaunch's working routes
// commit through launch.BindOverrideReservations instead — so recording
// there was dead code and the ledger stayed empty in production regardless
// of how well-tested the ledger's own read/write functions were (codex
// review on #1218, third pass). beforeSpawn now calls
// reserveProviderQuotaAdmissionIfWired directly, which this checks against
// the on-disk ledger — the actual observable contract — rather than a call
// graph that could silently stop matching production wiring again the same
// way enforceCircuitBreakers's admission.beforeSpawn does now.
func TestReserveProviderQuotaAdmissionIfWiredWritesTheLedgerWhenThrottled(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	publishAnthropicState(t, quota.BreakerThrottled)

	family, err := reserveProviderQuotaAdmissionIfWired("claude-code", "sonnet", time.Now())
	if err != nil {
		t.Fatalf("reserveProviderQuotaAdmissionIfWired: %v", err)
	}
	if family != "anthropic" {
		t.Errorf("reservedFamily = %q, want anthropic", family)
	}
	if got := readThrottleLedgerCount(t, "anthropic"); got != 1 {
		t.Errorf("ledger entries for anthropic = %d, want 1", got)
	}
}

// A healthy or halted family must never touch the ledger: the throttled
// band is the only one the hourly allowance governs (a halted family
// already refused upstream, and a healthy one has nothing to throttle).
func TestReserveProviderQuotaAdmissionIfWiredSkipsWhenNotThrottled(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	publishAnthropicState(t, quota.BreakerClosed)

	family, err := reserveProviderQuotaAdmissionIfWired("claude-code", "sonnet", time.Now())
	if err != nil {
		t.Fatalf("reserveProviderQuotaAdmissionIfWired: %v", err)
	}
	if family != "" {
		t.Errorf("reservedFamily = %q, want empty — nothing was reserved", family)
	}
	if got := readThrottleLedgerCount(t, "anthropic"); got != 0 {
		t.Errorf("a healthy family must not touch the ledger, got %d entries", got)
	}
}

// An empty model or a harness whose billing identity isn't established
// (pi-cli) must reserve nothing — there is no family to attribute the
// admission to.
func TestReserveProviderQuotaAdmissionIfWiredSkipsWhenThereIsNothingToReserve(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	publishAnthropicState(t, quota.BreakerThrottled)
	now := time.Now()

	if family, err := reserveProviderQuotaAdmissionIfWired("claude-code", "", now); err != nil || family != "" {
		t.Fatalf("empty model: family=%q err=%v, want (\"\", nil)", family, err)
	}
	if family, err := reserveProviderQuotaAdmissionIfWired("pi-cli", "sonnet", now); err != nil || family != "" {
		t.Fatalf("pi-cli: family=%q err=%v, want (\"\", nil)", family, err)
	}
	if got := readThrottleLedgerCount(t, "anthropic"); got != 0 {
		t.Errorf("want no ledger entries written, got %d", got)
	}
}

// Once the hourly allowance is spent, reserveProviderQuotaAdmissionIfWired
// must refuse — this is the final, authoritative boundary beforeSpawn
// relies on to close the race an earlier advisory check alone cannot.
func TestReserveProviderQuotaAdmissionIfWiredRefusesOnceTheAllowanceIsSpent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	publishAnthropicState(t, quota.BreakerThrottled)
	now := time.Now()

	for i := range quota.DefaultThrottledSpawnsPerHour {
		if _, err := reserveProviderQuotaAdmissionIfWired("claude-code", "sonnet", now); err != nil {
			t.Fatalf("reservation #%d: want allowed, got %v", i, err)
		}
	}
	if _, err := reserveProviderQuotaAdmissionIfWired("claude-code", "sonnet", now); err == nil {
		t.Fatal("want a refusal once the hourly allowance is spent")
	}
}

// A launch whose reservation succeeded but then definitely failed before
// real work started must release the reservation, or repeated failed
// attempts would consume the entire hourly allowance and block the next
// real launch for an hour even though no work ran (codex review on
// #1218, fourth pass). This exercises the same release path
// admission.go's beforeSpawn wires up via circuitbreaker.ReleaseThrottledAdmission.
func TestReleasedReservationFreesTheAllowanceForTheNextAttempt(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	publishAnthropicState(t, quota.BreakerThrottled)
	now := time.Now()

	for i := range quota.DefaultThrottledSpawnsPerHour {
		if _, err := reserveProviderQuotaAdmissionIfWired("claude-code", "sonnet", now); err != nil {
			t.Fatalf("reservation #%d: want allowed, got %v", i, err)
		}
	}
	if got := readThrottleLedgerCount(t, "anthropic"); got != quota.DefaultThrottledSpawnsPerHour {
		t.Fatalf("ledger count = %d, want %d before release", got, quota.DefaultThrottledSpawnsPerHour)
	}

	// Simulate the last of those reservations belonging to a launch that
	// then definitely failed (FinalizeLaunch/submission), the same as
	// admission.go's onAbort closure would do.
	if err := circuitbreaker.ReleaseThrottledAdmission("anthropic", now); err != nil {
		t.Fatalf("ReleaseThrottledAdmission: %v", err)
	}
	if got := readThrottleLedgerCount(t, "anthropic"); got != quota.DefaultThrottledSpawnsPerHour-1 {
		t.Errorf("ledger count after release = %d, want %d", got, quota.DefaultThrottledSpawnsPerHour-1)
	}

	// The freed allowance must be usable by the next attempt.
	if _, err := reserveProviderQuotaAdmissionIfWired("claude-code", "sonnet", now); err != nil {
		t.Errorf("reservation after release: want allowed, got %v", err)
	}
}
