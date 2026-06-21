package safegit

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func makeChecksJSON(checks []ghCheck) []byte {
	b, _ := json.Marshal(checks)
	return b
}

func TestParseFailingChecks_ExtractsRunID(t *testing.T) {
	data := makeChecksJSON([]ghCheck{
		{Name: "Build", State: "success", Link: "https://github.com/o/r/actions/runs/111/job/9"},
		{Name: "TestFullLifecycle", State: "failure", Link: "https://github.com/o/r/actions/runs/222/job/8"},
		{Name: "Pending", State: "pending", Link: ""},
	})
	failing, err := parseFailingChecks(data)
	if err != nil {
		t.Fatalf("parseFailingChecks: %v", err)
	}
	if len(failing) != 1 {
		t.Fatalf("want 1 failing check, got %d: %+v", len(failing), failing)
	}
	if failing[0].Name != "TestFullLifecycle" || failing[0].RunID != "222" {
		t.Errorf("unexpected failing check: %+v", failing[0])
	}
}

func TestParseFailingChecks_NoFailures(t *testing.T) {
	data := makeChecksJSON([]ghCheck{
		{Name: "Build", State: "success"},
		{Name: "Lint", State: "SKIPPED"},
	})
	failing, err := parseFailingChecks(data)
	if err != nil {
		t.Fatalf("parseFailingChecks: %v", err)
	}
	if len(failing) != 0 {
		t.Errorf("expected no failing checks, got %+v", failing)
	}
}

// mockRerun records calls and optionally returns an error.
type mockRerun struct {
	calls   []string
	failNow bool
}

func (m *mockRerun) fn(runID, repo string) error {
	m.calls = append(m.calls, runID)
	if m.failNow {
		return fmt.Errorf("boom")
	}
	return nil
}

func TestEvaluateFlakeValve_FirstFailureReruns(t *testing.T) {
	cfg := &Config{FlakyChecks: []FlakyCheck{{Name: "TestFullLifecycle", MaxRetries: 1}}}
	failing := []failedCheck{{Name: "TestFullLifecycle", State: "failure", RunID: "222"}}
	state := map[string]int{}
	mr := &mockRerun{}

	err := evaluateFlakeValve(failing, cfg, "o/r", state, mr.fn)
	if err == nil || !strings.Contains(err.Error(), "reran") {
		t.Fatalf("first failure should rerun and report awaiting, got: %v", err)
	}
	if len(mr.calls) != 1 || mr.calls[0] != "222" {
		t.Errorf("expected one rerun of run 222, got %v", mr.calls)
	}
	if state["TestFullLifecycle"] != 1 {
		t.Errorf("retry count should be 1, got %d", state["TestFullLifecycle"])
	}
}

func TestEvaluateFlakeValve_SecondFailureIsRealBlock(t *testing.T) {
	cfg := &Config{FlakyChecks: []FlakyCheck{{Name: "TestFullLifecycle", MaxRetries: 1}}}
	failing := []failedCheck{{Name: "TestFullLifecycle", State: "failure", RunID: "222"}}
	state := map[string]int{"TestFullLifecycle": 1} // already retried once
	mr := &mockRerun{}

	err := evaluateFlakeValve(failing, cfg, "o/r", state, mr.fn)
	if err == nil || !strings.Contains(err.Error(), "real failure") {
		t.Fatalf("second failure should be a real block, got: %v", err)
	}
	if len(mr.calls) != 0 {
		t.Errorf("must not rerun after retries exhausted, got %v", mr.calls)
	}
}

func TestEvaluateFlakeValve_NonFlakyIsRealBlock(t *testing.T) {
	cfg := &Config{FlakyChecks: []FlakyCheck{{Name: "TestFullLifecycle", MaxRetries: 1}}}
	failing := []failedCheck{{Name: "Build", State: "failure", RunID: "999"}}
	mr := &mockRerun{}

	err := evaluateFlakeValve(failing, cfg, "o/r", map[string]int{}, mr.fn)
	if err == nil || !strings.Contains(err.Error(), "Build") {
		t.Fatalf("non-flaky failure should block immediately, got: %v", err)
	}
	if len(mr.calls) != 0 {
		t.Errorf("non-flaky check must not be rerun, got %v", mr.calls)
	}
}

func TestEvaluateFlakeValve_NoFailuresPasses(t *testing.T) {
	cfg := &Config{FlakyChecks: []FlakyCheck{{Name: "X", MaxRetries: 1}}}
	if err := evaluateFlakeValve(nil, cfg, "o/r", map[string]int{}, (&mockRerun{}).fn); err != nil {
		t.Fatalf("no failing checks should pass, got: %v", err)
	}
}

func TestEvaluateFlakeValve_RerunErrorIsBlock(t *testing.T) {
	cfg := &Config{FlakyChecks: []FlakyCheck{{Name: "Flaky", MaxRetries: 1}}}
	failing := []failedCheck{{Name: "Flaky", State: "failure", RunID: "5"}}
	mr := &mockRerun{failNow: true}

	err := evaluateFlakeValve(failing, cfg, "o/r", map[string]int{}, mr.fn)
	if err == nil || !strings.Contains(err.Error(), "rerun failed") {
		t.Fatalf("a failed rerun should block, got: %v", err)
	}
}

func TestEvaluateFlakeValve_FlakyButNoRunID(t *testing.T) {
	cfg := &Config{FlakyChecks: []FlakyCheck{{Name: "Flaky", MaxRetries: 1}}}
	failing := []failedCheck{{Name: "Flaky", State: "failure", RunID: ""}}
	mr := &mockRerun{}

	err := evaluateFlakeValve(failing, cfg, "o/r", map[string]int{}, mr.fn)
	if err == nil || !strings.Contains(err.Error(), "cannot rerun") {
		t.Fatalf("flaky check with no run ID should block, got: %v", err)
	}
	if len(mr.calls) != 0 {
		t.Errorf("must not call rerun without a run ID, got %v", mr.calls)
	}
}

func TestMatchFlaky_Substring(t *testing.T) {
	cfg := &Config{FlakyChecks: []FlakyCheck{{Name: "FullLifecycle", MaxRetries: 1}}}
	if matchFlaky(cfg, "TestFullLifecycle (ubuntu)") == nil {
		t.Error("substring should match")
	}
	if matchFlaky(cfg, "Build") != nil {
		t.Error("unrelated check should not match")
	}
	if matchFlaky(nil, "x") != nil {
		t.Error("nil config matches nothing")
	}
}

func TestBuildRerunArgs(t *testing.T) {
	args := BuildRerunArgs("12345", "vbonnet/dear-agent")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "gh run rerun 12345") {
		t.Errorf("unexpected args: %v", args)
	}
	if !strings.Contains(joined, "--failed") {
		t.Error("--failed must be present so green jobs are not re-run")
	}
	if !strings.Contains(joined, "--repo vbonnet/dear-agent") {
		t.Error("--repo must be present")
	}
}

func TestBuildRerunArgs_PanicsOnEmptyRunID(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("BuildRerunArgs must panic on empty runID")
		}
	}()
	BuildRerunArgs("", "o/r")
}
