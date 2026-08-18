package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

func TestPriceFor(t *testing.T) {
	tests := []struct {
		model string
		want  Pricing
	}{
		{"claude-opus-4-8", opusPricing},
		{"claude-opus-5", opus5Pricing},
		{"claude-sonnet-4-6", sonnetPricing},
		{"claude-haiku-4-5-20251001", haikuPricing},
		{"claude-fable-5", fablePricing},
		{"<synthetic>", Pricing{}},
		{"", Pricing{}},
	}
	for _, tt := range tests {
		if got := PriceFor(tt.model); got != tt.want {
			t.Errorf("PriceFor(%q) = %+v, want %+v", tt.model, got, tt.want)
		}
	}
}

func TestOpus5PricingUsesCanonicalCosttrackRate(t *testing.T) {
	got := PriceFor("claude-opus-5")
	want := costtrack.PricingTable["claude-opus-5"]
	if got.InputPerM != want.Input || got.OutputPerM != want.Output || got.CacheReadPerM != want.CacheRead || got.CacheWritePerM != want.CacheWrite {
		t.Fatalf("PriceFor(claude-opus-5) = %+v, want canonical pricing %+v", got, want)
	}
}

func TestTokensCost(t *testing.T) {
	// 1M input + 1M output on Opus = $15 + $75 = $90.
	tok := Tokens{Input: 1_000_000, Output: 1_000_000}
	if got := tok.Cost(opusPricing); got != 90.0 {
		t.Errorf("Cost = %v, want 90.0", got)
	}
	// Cache read 1M on Sonnet = $0.30.
	tok = Tokens{CacheRead: 1_000_000}
	if got := tok.Cost(sonnetPricing); got != 0.30 {
		t.Errorf("CacheRead cost = %v, want 0.30", got)
	}
}

func TestTokensTotal(t *testing.T) {
	tok := Tokens{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}
	if got := tok.Total(); got != 10 {
		t.Errorf("Total = %d, want 10", got)
	}
}

func TestSessionType(t *testing.T) {
	worker := Entry{Cwd: "/Users/x/.agm/sandboxes/abc/upper"}
	if got := worker.SessionType(); got != "worker" {
		t.Errorf("worker SessionType = %q, want worker", got)
	}
	sup := Entry{Cwd: "/Users/x/src/engram-research"}
	if got := sup.SessionType(); got != "supervisor" {
		t.Errorf("supervisor SessionType = %q, want supervisor", got)
	}
}

// writeJSONL writes lines to a temp jsonl file under a project subdir.
func writeJSONL(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	sub := filepath.Join(dir, "proj-"+name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(sub, name+".jsonl")
	var content strings.Builder
	for _, l := range lines {
		content.WriteString(l)
		content.WriteString("\n")
	}
	if err := os.WriteFile(p, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func assistantLine(ts, model, sessionID, cwd, reqID, msgID string, in, out, cr, cc int64) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"sessionId":%q,"cwd":%q,"requestId":%q,"isSidechain":false,"message":{"id":%q,"model":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d}}}`,
		ts, sessionID, cwd, reqID, msgID, model, in, out, cr, cc)
}

func TestCollectAndBuild(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	old := now.Add(-72 * time.Hour).Format(time.RFC3339)

	lines := []string{
		// Opus worker, recent.
		assistantLine(recent, "claude-opus-4-8", "s1", "/home/u/.agm/sandboxes/x/upper", "r1", "m1", 1_000_000, 1_000_000, 0, 0),
		// Sonnet supervisor, recent.
		assistantLine(recent, "claude-sonnet-4-6", "s2", "/home/u/src/repo", "r2", "m2", 1_000_000, 0, 0, 0),
		// Old entry — filtered out by --since 24h.
		assistantLine(old, "claude-opus-4-8", "s3", "/home/u/src/repo", "r3", "m3", 5_000_000, 5_000_000, 0, 0),
		// Duplicate of r1/m1 — must be de-duped.
		assistantLine(recent, "claude-opus-4-8", "s1", "/home/u/.agm/sandboxes/x/upper", "r1", "m1", 1_000_000, 1_000_000, 0, 0),
		// Non-assistant line — ignored.
		`{"type":"user","timestamp":"` + recent + `","message":{"role":"user"}}`,
		// Zero-usage assistant — ignored.
		assistantLine(recent, "claude-opus-4-8", "s4", "/home/u/src/repo", "r4", "m4", 0, 0, 0, 0),
	}
	writeJSONL(t, dir, "test", lines)

	since := now.Add(-24 * time.Hour)
	entries, err := Collect(dir, since)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (dedup + since + zero-usage filtering)", len(entries))
	}

	r := Build(entries, 24, 0)

	// Opus: $90, Sonnet: $3 → total $93.
	if r.TotalCostUSD != 93.0 {
		t.Errorf("TotalCostUSD = %v, want 93.0", r.TotalCostUSD)
	}
	if r.TotalTokens != 3_000_000 {
		t.Errorf("TotalTokens = %d, want 3000000", r.TotalTokens)
	}
	if r.TopModel != "claude-opus-4-8" {
		t.Errorf("TopModel = %q, want claude-opus-4-8", r.TopModel)
	}
	// Burn rate over a 24h window equals total cost.
	if r.BurnRateDailyUSD != 93.0 {
		t.Errorf("BurnRateDailyUSD = %v, want 93.0", r.BurnRateDailyUSD)
	}
	if r.Sessions["worker"].CostUSD != 90.0 {
		t.Errorf("worker cost = %v, want 90.0", r.Sessions["worker"].CostUSD)
	}
	if r.Sessions["supervisor"].CostUSD != 3.0 {
		t.Errorf("supervisor cost = %v, want 3.0", r.Sessions["supervisor"].CostUSD)
	}
}

func TestBuildAlert(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{{
		Timestamp: now,
		Model:     "claude-opus-4-8",
		Cwd:       "/x/.agm/sandboxes/a/upper",
		Tokens:    Tokens{Input: 1_000_000, Output: 1_000_000}, // $90
	}}

	// 1h window → projected daily = $90 * 24 = $2160.
	r := Build(entries, 1, 100)
	if !r.Alert {
		t.Errorf("expected alert: burn %v should exceed threshold 100", r.BurnRateDailyUSD)
	}
	if r.AlertThresholdUSD != 100 {
		t.Errorf("AlertThresholdUSD = %v, want 100", r.AlertThresholdUSD)
	}

	// High threshold → no alert.
	r = Build(entries, 1, 100000)
	if r.Alert {
		t.Errorf("did not expect alert with high threshold")
	}

	// Zero threshold → alerting disabled.
	r = Build(entries, 1, 0)
	if r.Alert {
		t.Errorf("zero threshold must disable alerting")
	}
}

func TestCollectEmptyRoot(t *testing.T) {
	entries, err := Collect(t.TempDir(), time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("Collect on empty dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}
