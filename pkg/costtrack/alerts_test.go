package costtrack

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// newTestLogger returns a slog.Logger writing to the buffer with the level
// captured in each line as a JSON field.
func newTestLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func TestAlertManager_FiresAtEachThreshold(t *testing.T) {
	var buf bytes.Buffer
	mgr := NewAlertManager(newTestLogger(&buf))
	statuses := []BudgetStatus{
		{Period: BudgetDaily, Limit: 100, Spent: 95, Percent: 95},
	}

	fired := mgr.CheckAndAlert("proj", "model", statuses)

	// 95% crosses the 50/75/90 thresholds. 100 has not been hit yet.
	if len(fired) != 3 {
		t.Fatalf("expected 3 alerts at 50/75/90, got %d: %v", len(fired), fired)
	}
	for _, want := range []string{"50%", "75%", "90%"} {
		found := false
		for _, msg := range fired {
			if strings.Contains(msg, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in fired messages: %v", want, fired)
		}
	}
}

func TestAlertManager_NoDuplicateAlerts(t *testing.T) {
	var buf bytes.Buffer
	mgr := NewAlertManager(newTestLogger(&buf))
	statuses := []BudgetStatus{
		{Period: BudgetDaily, Limit: 100, Spent: 60, Percent: 60},
	}

	first := mgr.CheckAndAlert("proj", "model", statuses)
	second := mgr.CheckAndAlert("proj", "model", statuses)
	third := mgr.CheckAndAlert("proj", "model", statuses)

	if len(first) != 1 {
		t.Fatalf("first call should fire one alert (50%%), got %d: %v", len(first), first)
	}
	if len(second) != 0 || len(third) != 0 {
		t.Fatalf("subsequent calls should not duplicate alerts, got %v / %v", second, third)
	}
}

func TestAlertManager_SeparateKeysFireSeparately(t *testing.T) {
	var buf bytes.Buffer
	mgr := NewAlertManager(newTestLogger(&buf))
	statuses := []BudgetStatus{
		{Period: BudgetDaily, Limit: 100, Spent: 60, Percent: 60},
	}

	a := mgr.CheckAndAlert("alpha", "opus", statuses)
	b := mgr.CheckAndAlert("beta", "opus", statuses) // different project → fresh alerts
	c := mgr.CheckAndAlert("alpha", "sonnet", statuses)

	if len(a) != 1 || len(b) != 1 || len(c) != 1 {
		t.Fatalf("each project/model pair should fire its own 50%% alert; got %d/%d/%d", len(a), len(b), len(c))
	}
}

func TestAlertManager_LogLevelMatchesThreshold(t *testing.T) {
	var buf bytes.Buffer
	mgr := NewAlertManager(newTestLogger(&buf))
	// 100% crosses all four thresholds → Info(50), Info(75), Warn(90), Error(100).
	mgr.CheckAndAlert("proj", "model", []BudgetStatus{
		{Period: BudgetDaily, Limit: 100, Spent: 100, Percent: 100},
	})

	out := buf.String()
	checks := []struct {
		threshold string
		level     string
	}{
		{"50%", `"level":"INFO"`},
		{"75%", `"level":"INFO"`},
		{"90%", `"level":"WARN"`},
		{"100%", `"level":"ERROR"`},
	}
	for _, c := range checks {
		// Find the line containing the threshold and verify it has the right level.
		var matchLine string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, c.threshold+" threshold") {
				matchLine = line
				break
			}
		}
		if matchLine == "" {
			t.Errorf("no log line for %s found in: %s", c.threshold, out)
			continue
		}
		if !strings.Contains(matchLine, c.level) {
			t.Errorf("%s alert: want %s, got: %s", c.threshold, c.level, matchLine)
		}
	}
}

func TestAlertManager_Reset(t *testing.T) {
	var buf bytes.Buffer
	mgr := NewAlertManager(newTestLogger(&buf))
	statuses := []BudgetStatus{{Period: BudgetDaily, Limit: 100, Spent: 60, Percent: 60}}

	if got := mgr.CheckAndAlert("proj", "model", statuses); len(got) != 1 {
		t.Fatalf("first fire expected 1, got %d", len(got))
	}
	if got := mgr.CheckAndAlert("proj", "model", statuses); len(got) != 0 {
		t.Fatalf("duplicate suppressed expected 0, got %d", len(got))
	}

	mgr.Reset()

	if got := mgr.CheckAndAlert("proj", "model", statuses); len(got) != 1 {
		t.Fatalf("after Reset, expected 1 fresh alert, got %d", len(got))
	}
}

func TestAlertManager_ResetForPeriod_OnlyClearsThatPeriod(t *testing.T) {
	var buf bytes.Buffer
	mgr := NewAlertManager(newTestLogger(&buf))

	mgr.CheckAndAlert("proj", "model", []BudgetStatus{
		{Period: BudgetDaily, Limit: 100, Spent: 60, Percent: 60},
		{Period: BudgetMonthly, Limit: 1000, Spent: 600, Percent: 60},
	})

	// Reset daily; monthly key must survive.
	mgr.ResetForPeriod(BudgetDaily)

	fresh := mgr.CheckAndAlert("proj", "model", []BudgetStatus{
		{Period: BudgetDaily, Limit: 100, Spent: 60, Percent: 60},
		{Period: BudgetMonthly, Limit: 1000, Spent: 600, Percent: 60},
	})
	// Daily re-fires (1), monthly stays suppressed (0). Total = 1.
	if len(fresh) != 1 {
		t.Fatalf("ResetForPeriod(daily) should re-fire only daily; got %d: %v", len(fresh), fresh)
	}
	if !strings.Contains(fresh[0], "daily") {
		t.Fatalf("re-fired alert should be daily, got %q", fresh[0])
	}
}

func TestAlertManager_BelowFirstThresholdDoesNotFire(t *testing.T) {
	var buf bytes.Buffer
	mgr := NewAlertManager(newTestLogger(&buf))
	// 49% < first threshold (50%); no alerts expected.
	fired := mgr.CheckAndAlert("proj", "model", []BudgetStatus{
		{Period: BudgetDaily, Limit: 100, Spent: 49, Percent: 49},
	})
	if len(fired) != 0 {
		t.Fatalf("expected zero alerts below 50%%, got %d: %v", len(fired), fired)
	}
}

func TestContainsPeriod(t *testing.T) {
	tests := []struct {
		key    string
		period BudgetPeriod
		want   bool
	}{
		{"proj:model:daily:50", BudgetDaily, true},
		{"proj:model:weekly:75", BudgetWeekly, true},
		{"proj:model:monthly:100", BudgetMonthly, true},
		{"proj:model:daily:50", BudgetWeekly, false},
		{"proj:model:weekly:75", BudgetDaily, false},
	}
	for _, tt := range tests {
		if got := containsPeriod(tt.key, tt.period); got != tt.want {
			t.Errorf("containsPeriod(%q, %q) = %v, want %v", tt.key, tt.period, got, tt.want)
		}
	}
}
