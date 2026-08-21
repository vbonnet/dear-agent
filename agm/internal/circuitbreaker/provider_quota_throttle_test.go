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

// The ledger has to survive across separate calls the way separate agm
// process invocations would use it — nothing here holds anything open
// between calls, so this is exactly the cross-process scenario
// quota.Breaker.Admit's in-memory counter cannot cover.
func TestRecordThrottledAdmissionPersistsAcrossCalls(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()

	for i := range 3 {
		if err := recordThrottledAdmission("openai", now); err != nil {
			t.Fatalf("recordThrottledAdmission #%d: %v", i, err)
		}
	}

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

	if err := recordThrottledAdmission("openai", now.Add(-90*time.Minute)); err != nil {
		t.Fatalf("record old: %v", err)
	}
	if err := recordThrottledAdmission("openai", now.Add(-10*time.Minute)); err != nil {
		t.Fatalf("record recent: %v", err)
	}

	count, err := recentThrottledAdmissions("openai", now)
	if err != nil {
		t.Fatalf("recentThrottledAdmissions: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (the 90-minute-old entry must not count)", count)
	}
}

func TestRecordThrottledAdmissionKeepsFamiliesSeparate(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()

	for range 2 {
		if err := recordThrottledAdmission("openai", now); err != nil {
			t.Fatalf("record openai: %v", err)
		}
	}
	if err := recordThrottledAdmission("anthropic", now); err != nil {
		t.Fatalf("record anthropic: %v", err)
	}

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

func TestRecordProviderQuotaAdmissionIgnoresAnEmptyFamily(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := RecordProviderQuotaAdmission("", time.Now()); err != nil {
		t.Errorf("want no error for an empty family, got %v", err)
	}
	count, err := recentThrottledAdmissions("", time.Now())
	if err != nil {
		t.Fatalf("recentThrottledAdmissions: %v", err)
	}
	if count != 0 {
		t.Errorf("an empty family must never be recorded, count = %d", count)
	}
}

// This is the finding's actual scenario: a family the published state
// marks Throttled, with the persistent ledger already holding the
// hourly allowance's worth of admissions. checkProviderQuota must refuse
// the spawn instead of letting the throttled band admit an unbounded
// burst.
func TestCheckProviderQuotaEnforcesTheHourlyThrottleAllowance(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	now := time.Now()
	for i := range quota.DefaultThrottledSpawnsPerHour {
		if err := recordThrottledAdmission("openai", now); err != nil {
			t.Fatalf("record #%d: %v", i, err)
		}
	}

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
	if err := recordThrottledAdmission("openai", now); err != nil {
		t.Fatalf("record: %v", err)
	}

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
