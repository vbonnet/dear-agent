package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreAppendAndQuery(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Millisecond)
	r := Record{
		Timestamp: now,
		Metric:    MetricTestPassRateDelta,
		Category:  CategoryOutcomeQuality,
		Value:     0.15,
		SessionID: "sess-1",
		Labels:    map[string]string{"foo": "bar"},
	}
	require.NoError(t, store.Append(r))

	records, err := store.Query(QueryFilter{})
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, MetricTestPassRateDelta, records[0].Metric)
	assert.InDelta(t, 0.15, records[0].Value, 0.001)
	assert.Equal(t, "sess-1", records[0].SessionID)
	assert.Equal(t, "bar", records[0].Labels["foo"])
}

func TestStoreQueryFilter(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	records := []Record{
		{Timestamp: base, Metric: MetricTestPassRateDelta, Category: CategoryOutcomeQuality, Value: 0.1},
		{Timestamp: base.Add(time.Hour), Metric: MetricHookBypassRate, Category: CategoryHealth, Value: 0.5},
		{Timestamp: base.Add(2 * time.Hour), Metric: MetricTestPassRateDelta, Category: CategoryOutcomeQuality, Value: 0.2},
	}
	for _, r := range records {
		require.NoError(t, store.Append(r))
	}

	// Filter by metric
	result, err := store.Query(QueryFilter{Metric: MetricTestPassRateDelta})
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Filter by time range
	result, err = store.Query(QueryFilter{Since: base.Add(30 * time.Minute)})
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Filter by metric + time
	result, err = store.Query(QueryFilter{
		Metric: MetricTestPassRateDelta,
		Since:  base.Add(30 * time.Minute),
	})
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.InDelta(t, 0.2, result[0].Value, 0.001)
}

func TestStoreQueryEmpty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	records, err := store.Query(QueryFilter{Metric: MetricHookBypassRate})
	require.NoError(t, err)
	assert.Nil(t, records)
}

func TestStoreSummarize(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	for _, v := range []float64{0.1, 0.3, 0.2, 0.4} {
		require.NoError(t, store.Append(Record{
			Timestamp: time.Now().UTC(),
			Metric:    MetricSessionSuccessRate,
			Category:  CategoryHealth,
			Value:     v,
		}))
	}

	sum, err := store.Summarize(QueryFilter{Metric: MetricSessionSuccessRate})
	require.NoError(t, err)
	require.NotNil(t, sum)
	assert.Equal(t, 4, sum.Count)
	assert.InDelta(t, 0.25, sum.Mean, 0.001)
	assert.InDelta(t, 0.1, sum.Min, 0.001)
	assert.InDelta(t, 0.4, sum.Max, 0.001)
	assert.InDelta(t, 0.4, sum.Latest, 0.001)
}

func TestStoreSummarizeEmpty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	sum, err := store.Summarize(QueryFilter{Metric: MetricHookBypassRate})
	require.NoError(t, err)
	assert.Nil(t, sum)
}
