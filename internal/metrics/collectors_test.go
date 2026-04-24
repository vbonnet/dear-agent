package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCollector(t *testing.T) (*Collector, *Store) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	return NewCollector(store), store
}

func TestRecordTestPassRate_Improvement(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordTestPassRate(TestPassRateEvent{
		SessionID:   "s1",
		TestsBefore: 8, TotalBefore: 10,
		TestsAfter: 10, TotalAfter: 10,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricTestPassRateDelta})
	require.NoError(t, err)
	require.Len(t, records, 1)
	// delta = 1.0 - 0.8 = 0.2
	assert.InDelta(t, 0.2, records[0].Value, 0.001)
	assert.Equal(t, CategoryOutcomeQuality, records[0].Category)
	assert.Equal(t, "8", records[0].Labels["tests_before"])
}

func TestRecordTestPassRate_Regression(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordTestPassRate(TestPassRateEvent{
		SessionID:   "s1",
		TestsBefore: 10, TotalBefore: 10,
		TestsAfter: 7, TotalAfter: 10,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricTestPassRateDelta})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.InDelta(t, -0.3, records[0].Value, 0.001)
}

func TestRecordTestPassRate_NoTestsBefore(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordTestPassRate(TestPassRateEvent{
		SessionID:   "s1",
		TestsBefore: 0, TotalBefore: 0,
		TestsAfter: 5, TotalAfter: 10,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricTestPassRateDelta})
	require.NoError(t, err)
	assert.InDelta(t, 0.5, records[0].Value, 0.001)
}

func TestRecordFalseCompletion_Valid(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordFalseCompletion(CompletionEvent{
		SessionID:    "s1",
		WorkItemID:   "wi-1",
		ClaimedDone:  true,
		TestsPass:    true,
		CommitsExist: true,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricFalseCompletionRate})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.InDelta(t, 0.0, records[0].Value, 0.001) // valid completion
}

func TestRecordFalseCompletion_TestsFail(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordFalseCompletion(CompletionEvent{
		SessionID:    "s1",
		WorkItemID:   "wi-2",
		ClaimedDone:  true,
		TestsPass:    false,
		CommitsExist: true,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricFalseCompletionRate})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, records[0].Value, 0.001) // false completion
}

func TestRecordFalseCompletion_MissingCommits(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordFalseCompletion(CompletionEvent{
		SessionID:    "s1",
		WorkItemID:   "wi-3",
		ClaimedDone:  true,
		TestsPass:    true,
		CommitsExist: false,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricFalseCompletionRate})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, records[0].Value, 0.001) // false completion
}

func TestRecordFalseCompletion_NotClaimed(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordFalseCompletion(CompletionEvent{
		SessionID:    "s1",
		WorkItemID:   "wi-4",
		ClaimedDone:  false,
		TestsPass:    false,
		CommitsExist: false,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricFalseCompletionRate})
	require.NoError(t, err)
	assert.InDelta(t, 0.0, records[0].Value, 0.001) // not claimed, so not false
}

func TestRecordHookEvent_Caught(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordHookEvent(HookEvent{
		SessionID: "s1",
		HookName:  "bash-blocker",
		PatternID: "compound-cmd",
		Blocked:   true,
		Bypassed:  false,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricHookBypassRate})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.InDelta(t, 0.0, records[0].Value, 0.001)
	assert.Equal(t, CategoryHealth, records[0].Category)
}

func TestRecordHookEvent_Bypassed(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordHookEvent(HookEvent{
		SessionID: "s1",
		HookName:  "bash-blocker",
		PatternID: "compound-cmd",
		Blocked:   false,
		Bypassed:  true,
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricHookBypassRate})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, records[0].Value, 0.001)
}

func TestRecordSessionOutcome_Completed(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordSessionOutcome(SessionOutcome{
		SessionID:   "s1",
		SessionName: "fix-bug",
		Outcome:     "completed",
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricSessionSuccessRate})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.InDelta(t, 1.0, records[0].Value, 0.001)
	assert.Equal(t, "completed", records[0].Labels["outcome"])
}

func TestRecordSessionOutcome_Failed(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordSessionOutcome(SessionOutcome{
		SessionID: "s2",
		Outcome:   "failed",
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricSessionSuccessRate})
	require.NoError(t, err)
	assert.InDelta(t, 0.0, records[0].Value, 0.001)
}

func TestRecordSessionOutcome_Abandoned(t *testing.T) {
	c, store := newTestCollector(t)

	err := c.RecordSessionOutcome(SessionOutcome{
		SessionID: "s3",
		Outcome:   "abandoned",
	})
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricSessionSuccessRate})
	require.NoError(t, err)
	assert.InDelta(t, 0.0, records[0].Value, 0.001)
}

func TestCollectorEndToEnd(t *testing.T) {
	c, store := newTestCollector(t)

	// Record multiple events across all metrics
	require.NoError(t, c.RecordTestPassRate(TestPassRateEvent{
		SessionID: "s1", TestsBefore: 8, TotalBefore: 10, TestsAfter: 10, TotalAfter: 10,
	}))
	require.NoError(t, c.RecordFalseCompletion(CompletionEvent{
		SessionID: "s1", WorkItemID: "wi-1", ClaimedDone: true, TestsPass: true, CommitsExist: true,
	}))
	require.NoError(t, c.RecordHookEvent(HookEvent{
		SessionID: "s1", HookName: "bash-blocker", Blocked: true,
	}))
	require.NoError(t, c.RecordSessionOutcome(SessionOutcome{
		SessionID: "s1", Outcome: "completed",
	}))

	// All 4 records present
	all, err := store.Query(QueryFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 4)

	// Each metric queryable individually
	for _, m := range []MetricName{MetricTestPassRateDelta, MetricFalseCompletionRate, MetricHookBypassRate, MetricSessionSuccessRate} {
		records, err := store.Query(QueryFilter{Metric: m})
		require.NoError(t, err)
		assert.Len(t, records, 1, "expected 1 record for %s", m)
	}
}
