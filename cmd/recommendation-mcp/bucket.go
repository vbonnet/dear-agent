package main

import (
	"time"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// bucketResult is one row of get_signal_trends. Mean is undefined when
// Count is zero; we report 0 for empty buckets so the JSON shape stays
// consistent (clients can disambiguate via Count).
type bucketResult struct {
	Start time.Time
	End   time.Time
	Count int
	Mean  float64
	Min   float64
	Max   float64
}

// bucketize groups signals into evenly-spaced time buckets starting at
// `since` and ending at `since + window`. Bucket boundaries align to
// `since` (not to wall-clock midnight) so the caller controls the
// zero-point — see ADR-016 §D4.
//
// Empty buckets are emitted with Count=0 so the consumer can chart
// "we lost collection here" without re-creating the time grid.
func bucketize(since time.Time, window, bucket time.Duration, signals []aggregator.Signal) []bucketResult {
	if bucket <= 0 || window <= 0 {
		return nil
	}
	n := int(window / bucket)
	if window%bucket != 0 {
		n++
	}
	out := make([]bucketResult, n)
	sums := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i].Start = since.Add(time.Duration(i) * bucket)
		end := out[i].Start.Add(bucket)
		if end.After(since.Add(window)) {
			end = since.Add(window)
		}
		out[i].End = end
	}
	for _, sig := range signals {
		if sig.CollectedAt.Before(since) {
			continue
		}
		offset := sig.CollectedAt.Sub(since)
		idx := int(offset / bucket)
		if idx < 0 || idx >= n {
			continue
		}
		out[idx].Count++
		sums[idx] += sig.Value
		if out[idx].Count == 1 {
			out[idx].Min = sig.Value
			out[idx].Max = sig.Value
		} else {
			if sig.Value < out[idx].Min {
				out[idx].Min = sig.Value
			}
			if sig.Value > out[idx].Max {
				out[idx].Max = sig.Value
			}
		}
	}
	for i := range out {
		if out[i].Count > 0 {
			out[i].Mean = sums[i] / float64(out[i].Count)
		}
	}
	return out
}
