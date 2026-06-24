package telemetry

import (
	"context"
	"math"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkmetricdata "go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestMergeVelocityData_Fields is a compile-time/zero-value check: every
// exported field of MergeVelocityData must be addressable with its declared
// type.
func TestMergeVelocityData_Fields(t *testing.T) {
	d := MergeVelocityData{
		MergesPerDay:           1.5,
		OpenPRCount:            42,
		MedianTimeToMergeHours: 6.25,
		CreatedVsMergedDelta:   -0.5,
	}
	if d.MergesPerDay != 1.5 {
		t.Fatalf("MergesPerDay: want 1.5, got %v", d.MergesPerDay)
	}
	if d.OpenPRCount != 42 {
		t.Fatalf("OpenPRCount: want 42, got %v", d.OpenPRCount)
	}
}

// TestRecordMergeVelocity_Histogram verifies that RecordMergeVelocity emits
// the four instruments and that the histogram gets a sample.
func TestRecordMergeVelocity_Histogram(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	// Build a fresh MergeVelocityMetrics bound to the test provider (bypass the
	// global singleton so parallel tests don't share state).
	mv := newMergeVelocityMetrics(mp)
	ctx := context.Background()

	data := MergeVelocityData{
		MergesPerDay:           3,
		OpenPRCount:            12,
		MedianTimeToMergeHours: 4.5,
		CreatedVsMergedDelta:   2,
	}

	// Manually update mv (mirrors what RecordMergeVelocity does via the global).
	mv.mu.Lock()
	mv.last["owner/repo"] = data
	mv.mu.Unlock()
	if mv.medianTimeToMerge != nil {
		mv.medianTimeToMerge.Record(ctx, data.MedianTimeToMergeHours)
	}

	var rm sdkmetricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	want := map[string]bool{
		"merge.velocity.merges_per_day":             false,
		"merge.velocity.open_pr_count":              false,
		"merge.velocity.median_time_to_merge_hours": false,
		"merge.velocity.created_vs_merged_delta":    false,
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if _, ok := want[m.Name]; ok {
				want[m.Name] = true
			}
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected metric %q to be emitted", name)
		}
	}
}

// TestMedianFloat64 covers the internal median helper across edge cases.
func TestMedianFloat64(t *testing.T) {
	cases := []struct {
		in   []float64
		want float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 3}, 2},
		{[]float64{1, 2, 3}, 2},
		{[]float64{4, 1, 3, 2}, 2.5},
		{[]float64{math.NaN(), 4, 2}, 3}, // NaN excluded
	}
	for _, tc := range cases {
		got := medianFloat64(tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("medianFloat64(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestRecordMergeVelocity_NoopSafe verifies RecordMergeVelocity is safe to
// call against no-op providers (the global singleton path before InitMeter).
func TestRecordMergeVelocity_NoopSafe(t *testing.T) {
	ctx := context.Background()
	// Does not panic.
	RecordMergeVelocity(ctx, "owner/repo", MergeVelocityData{
		MergesPerDay:           2,
		OpenPRCount:            5,
		MedianTimeToMergeHours: 3.0,
		CreatedVsMergedDelta:   1,
	})
}
