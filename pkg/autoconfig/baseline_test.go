package autoconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBaseline_AddSession(t *testing.T) {
	b := &Baseline{
		ProjectHash: "test",
		WindowSize:  3,
	}

	sessions := []SessionSummary{
		{SessionID: "s1", TotalCostUSD: 1.0, TokenEfficiency: 0.5, PhaseScores: map[string]float64{"plan": 0.9}},
		{SessionID: "s2", TotalCostUSD: 2.0, TokenEfficiency: 0.6, PhaseScores: map[string]float64{"plan": 0.8}},
		{SessionID: "s3", TotalCostUSD: 3.0, TokenEfficiency: 0.7, PhaseScores: map[string]float64{"plan": 0.7}},
	}

	for _, s := range sessions {
		b.AddSession(s)
	}

	if len(b.Sessions) != 3 {
		t.Fatalf("sessions = %d, want 3", len(b.Sessions))
	}

	// Check averages.
	wantAvgCost := 2.0
	if b.AvgCostUSD != wantAvgCost {
		t.Errorf("avg cost = %f, want %f", b.AvgCostUSD, wantAvgCost)
	}

	wantAvgEff := 0.6
	if b.AvgTokenEfficiency != wantAvgEff {
		t.Errorf("avg efficiency = %f, want %f", b.AvgTokenEfficiency, wantAvgEff)
	}

	// Verify phase scores (use tolerance for floating-point division).
	planScore := b.AvgPhaseScores["plan"]
	if planScore < 0.799 || planScore > 0.801 {
		t.Errorf("avg plan score = %f, want ~0.8", planScore)
	}
}

func TestBaseline_WindowTrimming(t *testing.T) {
	b := &Baseline{
		ProjectHash: "test",
		WindowSize:  2,
	}

	b.AddSession(SessionSummary{SessionID: "s1", TotalCostUSD: 1.0})
	b.AddSession(SessionSummary{SessionID: "s2", TotalCostUSD: 2.0})
	b.AddSession(SessionSummary{SessionID: "s3", TotalCostUSD: 3.0})

	if len(b.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(b.Sessions))
	}
	if b.Sessions[0].SessionID != "s2" {
		t.Errorf("oldest session = %q, want %q", b.Sessions[0].SessionID, "s2")
	}

	// Average should be of s2+s3 only.
	if b.AvgCostUSD != 2.5 {
		t.Errorf("avg cost = %f, want 2.5", b.AvgCostUSD)
	}
}

func TestBaseline_SaveAndLoad(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	hash := ProjectHash("myproject")

	b := &Baseline{
		ProjectHash: hash,
		WindowSize:  DefaultWindowSize,
	}
	b.AddSession(SessionSummary{
		SessionID:       "s1",
		TotalCostUSD:    1.5,
		TokenEfficiency: 0.4,
		Timestamp:       time.Now(),
	})

	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify file exists.
	path, _ := BaselinePath(hash)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Load and verify.
	loaded, err := LoadBaseline(hash)
	if err != nil {
		t.Fatal(err)
	}

	if len(loaded.Sessions) != 1 {
		t.Fatalf("loaded sessions = %d, want 1", len(loaded.Sessions))
	}
	if loaded.AvgCostUSD != 1.5 {
		t.Errorf("loaded avg cost = %f, want 1.5", loaded.AvgCostUSD)
	}
}

func TestBaseline_LoadNonExistent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	b, err := LoadBaseline("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Sessions) != 0 {
		t.Errorf("sessions = %d, want 0", len(b.Sessions))
	}
}

func TestProjectHash(t *testing.T) {
	h1 := ProjectHash("project-a")
	h2 := ProjectHash("project-b")
	h3 := ProjectHash("project-a")

	if h1 == h2 {
		t.Error("different projects should have different hashes")
	}
	if h1 != h3 {
		t.Error("same project should have same hash")
	}
	if len(h1) != 16 { // 8 bytes hex-encoded
		t.Errorf("hash length = %d, want 16", len(h1))
	}
}

func TestPercentile(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}

	p50 := percentile(vals, 0.50)
	if p50 != 3 {
		t.Errorf("p50 = %f, want 3", p50)
	}

	p90 := percentile(vals, 0.90)
	// p90 of [1,2,3,4,5] = 4.6
	if p90 < 4.5 || p90 > 4.7 {
		t.Errorf("p90 = %f, want ~4.6", p90)
	}

	empty := percentile(nil, 0.50)
	if empty != 0 {
		t.Errorf("empty p50 = %f, want 0", empty)
	}
}

func TestBaselinePath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	path, err := BaselinePath("abc123")
	if err != nil {
		t.Fatal(err)
	}

	expected := filepath.Join(tmpHome, ".engram", "baselines", "abc123.json")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}
