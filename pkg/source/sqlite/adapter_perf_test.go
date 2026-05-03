package sqlite_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
	sqliteadapter "github.com/vbonnet/dear-agent/pkg/source/sqlite"
)

// TestAdapter_Perf_Fetch10K validates the BACKLOG ship criterion:
// Fetch P95 < 50 ms on a 10K-row corpus. The test seeds 10K rows with
// distinct content, runs 50 sample queries, and asserts on the 95th
// percentile latency. Skipped under -short.
func TestAdapter_Perf_Fetch10K(t *testing.T) {
	if testing.Short() {
		t.Skip("perf test skipped under -short")
	}
	dir := t.TempDir()
	a, err := sqliteadapter.Open(filepath.Join(dir, "sources.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = a.Close() }()
	ctx := context.Background()

	// Seed 10K rows. Each row contains a unique token (tok_<i>) so we
	// can exercise the FTS index, plus a shared token "corpus" so a
	// broad query also returns results.
	const N = 10000
	for i := 0; i < N; i++ {
		_, err := a.Add(ctx, source.Source{
			URI:       fmt.Sprintf("doc-%05d", i),
			Title:     fmt.Sprintf("Document %d", i),
			Snippet:   fmt.Sprintf("Snippet for tok_%d", i),
			Content:   []byte(fmt.Sprintf("corpus tok_%d body of document %d", i, i)),
			IndexedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Second),
			Metadata:  source.Metadata{Cues: []string{"perf"}, WorkItem: "perf-run/n"},
		})
		if err != nil {
			t.Fatalf("Add %d: %v", i, err)
		}
	}

	const samples = 50
	latencies := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		q := fmt.Sprintf("tok_%d", (i*173)%N)
		start := time.Now()
		got, err := a.Fetch(ctx, source.FetchQuery{Query: q, K: 10})
		dur := time.Since(start)
		if err != nil {
			t.Fatalf("Fetch %s: %v", q, err)
		}
		if len(got) == 0 {
			t.Fatalf("Fetch %s returned 0", q)
		}
		latencies = append(latencies, dur)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[(len(latencies)*95)/100]
	t.Logf("Fetch 10K p95=%s p50=%s", p95, latencies[len(latencies)/2])
	if p95 > 50*time.Millisecond {
		t.Fatalf("Fetch p95 = %s, want < 50ms", p95)
	}
}
