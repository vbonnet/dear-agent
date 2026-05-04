package main

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// bucketize is the trend math; pure unit tests skip the JSON-RPC layer.

func TestBucketize_FixedGrid(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	got := bucketize(since, 72*time.Hour, 24*time.Hour, nil)
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	for i := 0; i < 3; i++ {
		want := since.Add(time.Duration(i) * 24 * time.Hour)
		if !got[i].Start.Equal(want) {
			t.Errorf("bucket[%d].Start = %v, want %v", i, got[i].Start, want)
		}
		if got[i].Count != 0 {
			t.Errorf("bucket[%d].Count = %d, want 0 (no signals)", i, got[i].Count)
		}
	}
}

func TestBucketize_AssignsToCorrectBucket(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	signals := []aggregator.Signal{
		{Kind: aggregator.KindLintTrend, Subject: "a", Value: 1, CollectedAt: since.Add(1 * time.Hour)},
		{Kind: aggregator.KindLintTrend, Subject: "b", Value: 2, CollectedAt: since.Add(25 * time.Hour)},
		{Kind: aggregator.KindLintTrend, Subject: "c", Value: 3, CollectedAt: since.Add(49 * time.Hour)},
	}
	got := bucketize(since, 72*time.Hour, 24*time.Hour, signals)
	if got[0].Count != 1 || got[1].Count != 1 || got[2].Count != 1 {
		t.Errorf("counts = [%d %d %d], want [1 1 1]", got[0].Count, got[1].Count, got[2].Count)
	}
}

func TestBucketize_AggregatesMultipleSignalsInBucket(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	signals := []aggregator.Signal{
		{Kind: aggregator.KindLintTrend, Subject: "a", Value: 4, CollectedAt: since.Add(1 * time.Hour)},
		{Kind: aggregator.KindLintTrend, Subject: "b", Value: 10, CollectedAt: since.Add(2 * time.Hour)},
		{Kind: aggregator.KindLintTrend, Subject: "c", Value: 16, CollectedAt: since.Add(3 * time.Hour)},
	}
	got := bucketize(since, 24*time.Hour, 24*time.Hour, signals)
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
	if got[0].Count != 3 {
		t.Errorf("Count=%d, want 3", got[0].Count)
	}
	if got[0].Mean != 10 {
		t.Errorf("Mean=%v, want 10 (avg 4,10,16)", got[0].Mean)
	}
	if got[0].Min != 4 || got[0].Max != 16 {
		t.Errorf("Min/Max = %v/%v, want 4/16", got[0].Min, got[0].Max)
	}
}

func TestBucketize_SkipsOutOfRange(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	signals := []aggregator.Signal{
		// Before window:
		{Kind: aggregator.KindLintTrend, Subject: "old", Value: 99, CollectedAt: since.Add(-time.Hour)},
		// After window:
		{Kind: aggregator.KindLintTrend, Subject: "new", Value: 99, CollectedAt: since.Add(100 * time.Hour)},
	}
	got := bucketize(since, 24*time.Hour, 24*time.Hour, signals)
	if got[0].Count != 0 {
		t.Errorf("Count=%d, want 0 (out-of-range signals dropped)", got[0].Count)
	}
}

func TestBucketize_ZeroOrNegativeBucketReturnsNil(t *testing.T) {
	since := time.Now().UTC()
	if got := bucketize(since, 24*time.Hour, 0, nil); got != nil {
		t.Errorf("zero bucket should return nil, got %v", got)
	}
	if got := bucketize(since, 0, 24*time.Hour, nil); got != nil {
		t.Errorf("zero window should return nil, got %v", got)
	}
}

// Window not divisible by bucket: ceil to next bucket so the last
// bucket covers the partial tail.
func TestBucketize_PartialFinalBucket(t *testing.T) {
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	got := bucketize(since, 25*time.Hour, 24*time.Hour, nil)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2 (25h ceil-divided by 24h)", len(got))
	}
	// Final bucket's End should be clamped to since+window, not since+2*bucket.
	wantEnd := since.Add(25 * time.Hour)
	if !got[1].End.Equal(wantEnd) {
		t.Errorf("bucket[1].End = %v, want %v (clamped to window end)", got[1].End, wantEnd)
	}
}
