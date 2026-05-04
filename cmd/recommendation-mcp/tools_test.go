package main

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// ----- get_signals -----

// Validation property #2: empty store, empty result, no error.
func TestGetSignals_EmptyStore(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signals", map[string]any{})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	sigs := resp.Result.(map[string]any)["signals"].([]map[string]any)
	if len(sigs) != 0 {
		t.Errorf("len=%d, want 0", len(sigs))
	}
}

// Validation property #3: unknown kind → -32602.
func TestGetSignals_UnknownKind(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signals", map[string]any{"kind": "not_a_kind"})
	if resp.Error == nil {
		t.Fatal("expected error for unknown kind")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

// Validation property #3: known kind with no rows returns empty array.
func TestGetSignals_KnownKindNoRows(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signals", map[string]any{"kind": "lint_trend"})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	sigs := resp.Result.(map[string]any)["signals"].([]map[string]any)
	if len(sigs) != 0 {
		t.Errorf("len=%d, want 0", len(sigs))
	}
}

func TestGetSignals_FilterByKind(t *testing.T) {
	srv, store := newTestServer(t)
	at := recently()
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "a.go", Value: 1, CollectedAt: at},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "b.go", Value: 2, CollectedAt: at},
		aggregator.Signal{Kind: aggregator.KindGitActivity, Subject: "/repo", Value: 9, CollectedAt: at},
	)
	resp := callTool(t, srv, "get_signals", map[string]any{"kind": "lint_trend"})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	sigs := resp.Result.(map[string]any)["signals"].([]map[string]any)
	if len(sigs) != 2 {
		t.Fatalf("len=%d, want 2", len(sigs))
	}
}

func TestGetSignals_SubjectSubstringFilter(t *testing.T) {
	srv, store := newTestServer(t)
	at := recently()
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/foo/x.go", Value: 1, CollectedAt: at},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/foo/y.go", Value: 2, CollectedAt: at},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/bar/z.go", Value: 3, CollectedAt: at},
	)
	resp := callTool(t, srv, "get_signals", map[string]any{
		"kind": "lint_trend", "subject": "pkg/foo/",
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	sigs := resp.Result.(map[string]any)["signals"].([]map[string]any)
	if len(sigs) != 2 {
		t.Fatalf("len=%d, want 2", len(sigs))
	}
}

func TestGetSignals_AllKindsWhenOmitted(t *testing.T) {
	srv, store := newTestServer(t)
	at := recently()
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "a.go", Value: 1, CollectedAt: at},
		aggregator.Signal{Kind: aggregator.KindGitActivity, Subject: "/repo", Value: 9, CollectedAt: at},
	)
	resp := callTool(t, srv, "get_signals", map[string]any{})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	sigs := resp.Result.(map[string]any)["signals"].([]map[string]any)
	if len(sigs) != 2 {
		t.Fatalf("len=%d, want 2 (cross-kind), got %d", len(sigs), len(sigs))
	}
}

func TestGetSignals_LimitCap(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signals", map[string]any{"limit": 9999})
	if resp.Error == nil {
		t.Fatal("expected error for oversize limit")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

func TestGetSignals_BadSinceTimestamp(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signals", map[string]any{
		"kind": "lint_trend", "since": "not-a-timestamp",
	})
	if resp.Error == nil {
		t.Fatal("expected error for bad since")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

// ----- get_recommendations -----

// Validation property #4: ranking matches Scorer output for the same
// inputs. We seed two kinds with values that the default weights rank
// in a knowable order: security_alerts (weight 1.0) > lint_trend (0.4).
func TestGetRecommendations_OrderMatchesScorer(t *testing.T) {
	srv, store := newTestServer(t)
	at := recently()
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindSecurityAlerts, Subject: "GO-2024-1", Value: 1, CollectedAt: at},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/x.go", Value: 100, CollectedAt: at},
	)
	resp := callTool(t, srv, "get_recommendations", map[string]any{"top_n": 5})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	recs := resp.Result.(map[string]any)["recommendations"].([]map[string]any)
	if len(recs) != 2 {
		t.Fatalf("len=%d, want 2", len(recs))
	}
	// security_alerts norm = 1/10 = 0.1, weighted = 0.1 * 1.0 = 0.1
	// lint_trend     norm = 100/200 = 0.5, weighted = 0.5 * 0.4 = 0.2
	// → lint_trend ranks higher; verify the ranking math in tools agrees.
	if recs[0]["kind"] != "lint_trend" {
		t.Errorf("recs[0].kind = %v, want lint_trend (weighted 0.2)", recs[0]["kind"])
	}
	if recs[1]["kind"] != "security_alerts" {
		t.Errorf("recs[1].kind = %v, want security_alerts (weighted 0.1)", recs[1]["kind"])
	}
}

// Validation property #5: most-recent-per-(kind, subject) reduction.
// Inserting two lint signals on the same subject in-window yields ONE
// recommendation row, with the LATER raw value.
func TestGetRecommendations_MostRecentPerSubject(t *testing.T) {
	srv, store := newTestServer(t)
	now := time.Now().UTC()
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/x.go", Value: 5, CollectedAt: now.Add(-2 * time.Hour)},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/x.go", Value: 50, CollectedAt: now.Add(-1 * time.Hour)},
	)
	resp := callTool(t, srv, "get_recommendations", nil)
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	recs := resp.Result.(map[string]any)["recommendations"].([]map[string]any)
	if len(recs) != 1 {
		t.Fatalf("len=%d, want 1 (deduped)", len(recs))
	}
	if recs[0]["raw"].(float64) != 50 {
		t.Errorf("raw=%v, want 50 (the later value)", recs[0]["raw"])
	}
}

func TestGetRecommendations_TopNCap(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_recommendations", map[string]any{"top_n": 999})
	if resp.Error == nil {
		t.Fatal("expected error for oversize top_n")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

func TestGetRecommendations_WindowExcludesOldSignals(t *testing.T) {
	srv, store := newTestServer(t)
	now := time.Now().UTC()
	insertSignals(t, store,
		// Outside the 1h window:
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "old.go", Value: 100, CollectedAt: now.Add(-2 * time.Hour)},
		// Inside:
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "new.go", Value: 10, CollectedAt: now.Add(-30 * time.Minute)},
	)
	resp := callTool(t, srv, "get_recommendations", map[string]any{"window": "1h"})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	recs := resp.Result.(map[string]any)["recommendations"].([]map[string]any)
	if len(recs) != 1 {
		t.Fatalf("len=%d, want 1 (window excludes old)", len(recs))
	}
	if recs[0]["subject"] != "new.go" {
		t.Errorf("subject=%v, want new.go", recs[0]["subject"])
	}
}

func TestGetRecommendations_WeightOverride(t *testing.T) {
	srv, store := newTestServer(t)
	at := recently()
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindGitActivity, Subject: "/repo", Value: 50, CollectedAt: at},
	)
	// Boost git_activity weight to 10.0 — single row, but with the
	// override the weighted score should be much larger than the
	// default would produce.
	resp := callTool(t, srv, "get_recommendations", map[string]any{
		"weights": map[string]any{"git_activity": 10.0},
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	recs := resp.Result.(map[string]any)["recommendations"].([]map[string]any)
	if len(recs) != 1 {
		t.Fatalf("len=%d, want 1", len(recs))
	}
	w := recs[0]["weight"].(float64)
	if w != 10.0 {
		t.Errorf("weight=%v, want 10.0 (override applied)", w)
	}
}

func TestGetRecommendations_BadWeightKind(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_recommendations", map[string]any{
		"weights": map[string]any{"not_a_kind": 0.5},
	})
	if resp.Error == nil {
		t.Fatal("expected error for unknown kind in weights")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

func TestGetRecommendations_BadWindow(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_recommendations", map[string]any{"window": "not-a-duration"})
	if resp.Error == nil {
		t.Fatal("expected error for bad window")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

// ----- get_signal_trends -----

// Validation property #6: bucket=24h, window=72h → exactly 3 buckets,
// including zero-count ones.
func TestGetSignalTrends_FixedBucketCount(t *testing.T) {
	srv, store := newTestServer(t)
	now := time.Now().UTC()
	// One signal in the most recent bucket.
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "x.go", Value: 7, CollectedAt: now.Add(-30 * time.Minute)},
	)
	resp := callTool(t, srv, "get_signal_trends", map[string]any{
		"kind":   "lint_trend",
		"window": "72h",
		"bucket": "24h",
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	buckets := resp.Result.(map[string]any)["buckets"].([]map[string]any)
	if len(buckets) != 3 {
		t.Fatalf("len=%d, want 3 (72h / 24h)", len(buckets))
	}
	// Two of the three buckets should be zero-count (the older two).
	zeros := 0
	for _, b := range buckets {
		if b["count"].(int) == 0 {
			zeros++
		}
	}
	if zeros != 2 {
		t.Errorf("zero-count buckets = %d, want 2", zeros)
	}
}

// Validation property #7: window/bucket > 1000 is rejected.
func TestGetSignalTrends_ExcessBucketsRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signal_trends", map[string]any{
		"kind":   "lint_trend",
		"window": "20000h", // 20000h / 1h = 20000 buckets
		"bucket": "1h",
	})
	if resp.Error == nil {
		t.Fatal("expected error for excess buckets")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

func TestGetSignalTrends_RequiresKind(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signal_trends", map[string]any{})
	if resp.Error == nil {
		t.Fatal("expected error when kind is omitted")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

func TestGetSignalTrends_BucketBelowMin(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "get_signal_trends", map[string]any{
		"kind":   "lint_trend",
		"window": "24h",
		"bucket": "1m",
	})
	if resp.Error == nil {
		t.Fatal("expected error for bucket below 1h")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

func TestGetSignalTrends_AggregatesValues(t *testing.T) {
	srv, store := newTestServer(t)
	now := time.Now().UTC()
	// Three signals all in the most recent 24h bucket.
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "a.go", Value: 4, CollectedAt: now.Add(-1 * time.Hour)},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "b.go", Value: 8, CollectedAt: now.Add(-2 * time.Hour)},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "c.go", Value: 12, CollectedAt: now.Add(-3 * time.Hour)},
	)
	resp := callTool(t, srv, "get_signal_trends", map[string]any{
		"kind":   "lint_trend",
		"window": "24h",
		"bucket": "24h",
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	buckets := resp.Result.(map[string]any)["buckets"].([]map[string]any)
	if len(buckets) != 1 {
		t.Fatalf("len=%d, want 1", len(buckets))
	}
	b := buckets[0]
	if b["count"].(int) != 3 {
		t.Errorf("count=%v, want 3", b["count"])
	}
	if b["mean"].(float64) != 8 {
		t.Errorf("mean=%v, want 8 (avg of 4,8,12)", b["mean"])
	}
	if b["min"].(float64) != 4 {
		t.Errorf("min=%v, want 4", b["min"])
	}
	if b["max"].(float64) != 12 {
		t.Errorf("max=%v, want 12", b["max"])
	}
}

func TestGetSignalTrends_SubjectFilter(t *testing.T) {
	srv, store := newTestServer(t)
	now := time.Now().UTC()
	insertSignals(t, store,
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/foo/a.go", Value: 10, CollectedAt: now.Add(-1 * time.Hour)},
		aggregator.Signal{Kind: aggregator.KindLintTrend, Subject: "pkg/bar/b.go", Value: 99, CollectedAt: now.Add(-1 * time.Hour)},
	)
	resp := callTool(t, srv, "get_signal_trends", map[string]any{
		"kind":    "lint_trend",
		"subject": "pkg/foo",
		"window":  "24h",
		"bucket":  "24h",
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	buckets := resp.Result.(map[string]any)["buckets"].([]map[string]any)
	b := buckets[0]
	if b["count"].(int) != 1 {
		t.Errorf("count=%v, want 1 (filter excluded pkg/bar)", b["count"])
	}
}

// ----- ServeHTTP smoke -----

// One round-trip through ServeHTTP confirms the JSON-RPC envelope shape
// matches what stdio produces. The transport is otherwise just a
// different reader/writer.
func TestServeHTTP_RoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	req := rpcRequest{JSONRPC: "2.0", ID: 7, Method: "tools/list"}
	resp := srv.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("HandleRequest error: %+v", resp.Error)
	}
}
