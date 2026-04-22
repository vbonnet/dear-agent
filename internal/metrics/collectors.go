package metrics

import (
	"fmt"
	"time"
)

// Collector records metric events and persists computed values.
type Collector struct {
	store *Store
}

// NewCollector creates a collector backed by the given store.
func NewCollector(store *Store) *Collector {
	return &Collector{store: store}
}

// RecordTestPassRate computes test pass rate delta and records it.
// Delta = (after_rate - before_rate), where rate = passing/total.
// Positive delta means improvement, negative means regression.
func (c *Collector) RecordTestPassRate(event TestPassRateEvent) error {
	var beforeRate, afterRate float64
	if event.TotalBefore > 0 {
		beforeRate = float64(event.TestsBefore) / float64(event.TotalBefore)
	}
	if event.TotalAfter > 0 {
		afterRate = float64(event.TestsAfter) / float64(event.TotalAfter)
	}
	delta := afterRate - beforeRate

	return c.store.Append(Record{
		Timestamp:   time.Now().UTC(),
		Metric:      MetricTestPassRateDelta,
		Category:    CategoryOutcomeQuality,
		Value:       delta,
		SessionID:   event.SessionID,
		SessionName: event.SessionName,
		Labels: map[string]string{
			"tests_before": itoa(event.TestsBefore),
			"total_before": itoa(event.TotalBefore),
			"tests_after":  itoa(event.TestsAfter),
			"total_after":  itoa(event.TotalAfter),
		},
	})
}

// RecordFalseCompletion records whether a completion claim was valid.
// Value is 1.0 for false completion, 0.0 for valid completion.
func (c *Collector) RecordFalseCompletion(event CompletionEvent) error {
	var value float64
	if event.ClaimedDone && (!event.TestsPass || !event.CommitsExist) {
		value = 1.0 // false completion
	}

	return c.store.Append(Record{
		Timestamp:   time.Now().UTC(),
		Metric:      MetricFalseCompletionRate,
		Category:    CategoryOutcomeQuality,
		Value:       value,
		SessionID:   event.SessionID,
		SessionName: event.SessionName,
		Labels: map[string]string{
			"work_item_id":  event.WorkItemID,
			"claimed_done":  btoa(event.ClaimedDone),
			"tests_pass":    btoa(event.TestsPass),
			"commits_exist": btoa(event.CommitsExist),
		},
	})
}

// RecordHookEvent records a hook execution for bypass rate tracking.
// Value is 1.0 for bypassed violation, 0.0 for caught violation.
// Only records violations (events where something was wrong).
func (c *Collector) RecordHookEvent(event HookEvent) error {
	var value float64
	if event.Bypassed {
		value = 1.0
	}

	return c.store.Append(Record{
		Timestamp:   time.Now().UTC(),
		Metric:      MetricHookBypassRate,
		Category:    CategoryHealth,
		Value:       value,
		SessionID:   event.SessionID,
		SessionName: event.SessionName,
		Labels: map[string]string{
			"hook_name":  event.HookName,
			"pattern_id": event.PatternID,
			"blocked":    btoa(event.Blocked),
		},
	})
}

// RecordSessionOutcome records a session's final outcome.
// Value is 1.0 for success (completed), 0.0 for failure (failed/abandoned).
func (c *Collector) RecordSessionOutcome(event SessionOutcome) error {
	var value float64
	if event.Outcome == "completed" {
		value = 1.0
	}

	return c.store.Append(Record{
		Timestamp:   time.Now().UTC(),
		Metric:      MetricSessionSuccessRate,
		Category:    CategoryHealth,
		Value:       value,
		SessionID:   event.SessionID,
		SessionName: event.SessionName,
		Labels: map[string]string{
			"outcome": event.Outcome,
		},
	})
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func btoa(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
