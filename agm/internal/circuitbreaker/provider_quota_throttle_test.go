package circuitbreaker

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

func TestRecentThrottledAdmissionsIsZeroWithNoLedger(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	count, err := recentThrottledAdmissions("openai", time.Now())
	if err != nil {
		t.Fatalf("recentThrottledAdmissions: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 with no ledger written yet", count)
	}
}

func reserveN(t *testing.T, family string, n int, now time.Time) {
	t.Helper()
	for i := range n {
		allowed, err := reserveThrottledAdmission(family, 1000, now)
		if err != nil {
			t.Fatalf("reserveThrottledAdmission #%d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("reserveThrottledAdmission #%d: refused under a limit of 1000", i)
		}
	}
}

// The ledger has to survive across separate calls the way separate agm
// process invocations would use it — nothing here holds anything open
// between calls, so this is exactly the cross-process scenario
// quota.Breaker.Admit's in-memory counter cannot cover.
func TestReserveThrottledAdmissionPersistsAcrossCalls(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()

	reserveN(t, "openai", 3, now)

	count, err := recentThrottledAdmissions("openai", now)
	if err != nil {
		t.Fatalf("recentThrottledAdmissions: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestRecentThrottledAdmissionsPrunesEntriesOlderThanAnHour(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()

	reserveN(t, "openai", 1, now.Add(-90*time.Minute))
	reserveN(t, "openai", 1, now.Add(-10*time.Minute))

	count, err := recentThrottledAdmissions("openai", now)
	if err != nil {
		t.Fatalf("recentThrottledAdmissions: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (the 90-minute-old entry must not count)", count)
	}
}

func TestReserveThrottledAdmissionKeepsFamiliesSeparate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()

	reserveN(t, "openai", 2, now)
	reserveN(t, "anthropic", 1, now)

	openaiCount, err := recentThrottledAdmissions("openai", now)
	if err != nil {
		t.Fatalf("recentThrottledAdmissions openai: %v", err)
	}
	if openaiCount != 2 {
		t.Errorf("openai count = %d, want 2", openaiCount)
	}
	anthropicCount, err := recentThrottledAdmissions("anthropic", now)
	if err != nil {
		t.Fatalf("recentThrottledAdmissions anthropic: %v", err)
	}
	if anthropicCount != 1 {
		t.Errorf("anthropic count = %d, want 1", anthropicCount)
	}
}

func TestReserveThrottledAdmissionIgnoresAnEmptyFamily(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	allowed, err := ReserveThrottledAdmission("", 4, time.Now())
	if err != nil {
		t.Errorf("want no error for an empty family, got %v", err)
	}
	if !allowed {
		t.Error("an empty family must never be refused")
	}
	count, err := recentThrottledAdmissions("", time.Now())
	if err != nil {
		t.Fatalf("recentThrottledAdmissions: %v", err)
	}
	if count != 0 {
		t.Errorf("an empty family must never be recorded, count = %d", count)
	}
}

// This is the concurrency finding's actual scenario: two "processes"
// (sequential calls here, since the ledger is the only shared state)
// both attempt the last reservation under a limit of 1. Only one may
// succeed — the check and the append must be atomic under one lock, not
// a separate read followed by a separate write, or both could observe
// "under the limit" and both would be admitted (codex review on #1218,
// fourth pass).
func TestReserveThrottledAdmissionIsAtomicAtTheLimit(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()

	first, err := ReserveThrottledAdmission("openai", 1, now)
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if !first {
		t.Fatal("first reservation under limit 1 must be allowed")
	}

	second, err := ReserveThrottledAdmission("openai", 1, now)
	if err != nil {
		t.Fatalf("second reserve: %v", err)
	}
	if second {
		t.Fatal("second reservation must be refused once the limit is spent — this is the race the atomic reserve exists to close")
	}

	count, err := recentThrottledAdmissions("openai", now)
	if err != nil {
		t.Fatalf("recentThrottledAdmissions: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want exactly 1 — the refused reservation must not have been recorded", count)
	}
}

// This is the finding's original scenario: a family the published state
// marks Throttled, with the persistent ledger already holding the
// hourly allowance's worth of admissions. checkProviderQuota's advisory
// check must refuse the spawn early, before ReserveThrottledAdmission's
// atomic check ever runs.
func TestCheckProviderQuotaEnforcesTheHourlyThrottleAllowance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()
	reserveN(t, "openai", quota.DefaultThrottledSpawnsPerHour, now)

	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed:   true,
		Family:    "openai",
		State:     quota.BreakerThrottled,
		Reason:    "Weekly has 12.0% left, at or below the 15.0% throttle floor",
		Evaluated: true,
	}}
	got := checkProviderQuota(gate, "gpt-5.5-pro")
	if got.Passed {
		t.Fatal("want the gate to refuse once the hourly throttle allowance is spent")
	}
	for _, want := range []string{"openai", "throttled", "allowance"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("message missing %q: %s", want, got.Message)
		}
	}
}

// Below the allowance, a throttled family must still admit — throttled
// means rate-limited, not halted.
func TestCheckProviderQuotaAllowsThrottledBelowTheAllowance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()
	reserveN(t, "openai", 1, now)

	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed:   true,
		Family:    "openai",
		State:     quota.BreakerThrottled,
		Reason:    "Weekly has 12.0% left, at or below the 15.0% throttle floor",
		Evaluated: true,
	}}
	got := checkProviderQuota(gate, "gpt-5.5-pro")
	if !got.Passed {
		t.Errorf("want the gate to pass below the hourly allowance, got %q", got.Message)
	}
}

// A healthy family must never touch the ledger at all — only the
// throttled band is rate-limited.
func TestCheckProviderQuotaIgnoresTheLedgerWhenHealthy(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	gate := &stubQuotaGate{decision: quota.SpawnDecision{
		Allowed:   true,
		Family:    "openai",
		State:     quota.BreakerClosed,
		Reason:    "Weekly has 82.0% left",
		Evaluated: true,
	}}
	got := checkProviderQuota(gate, "gpt-5.5-pro")
	if !got.Passed {
		t.Errorf("want a healthy family to pass unconditionally, got %q", got.Message)
	}
}
