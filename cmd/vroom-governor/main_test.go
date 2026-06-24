package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPauseSpawns_WritesFile(t *testing.T) {
	dir := t.TempDir()
	spawnFile := filepath.Join(dir, "last-spawn.txt")

	if err := pauseSpawns(spawnFile, 30*time.Second); err != nil {
		t.Fatalf("pauseSpawns: %v", err)
	}

	data, err := os.ReadFile(spawnFile)
	if err != nil {
		t.Fatalf("reading spawn file: %v", err)
	}

	ts, err := time.Parse(time.RFC3339, string(data[:len(data)-1])) // strip newline
	if err != nil {
		t.Fatalf("parse timestamp %q: %v", data, err)
	}

	if ts.Before(time.Now()) {
		t.Errorf("expected future timestamp, got %v", ts)
	}
}

func TestPauseSpawns_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	spawnFile := filepath.Join(dir, "nested", "subdir", "last-spawn.txt")

	if err := pauseSpawns(spawnFile, 30*time.Second); err != nil {
		t.Fatalf("pauseSpawns should create parent dirs: %v", err)
	}

	if _, err := os.Stat(spawnFile); err != nil {
		t.Errorf("spawn file not created: %v", err)
	}
}

func TestBuildReason(t *testing.T) {
	tests := []struct {
		loadHigh bool
		load     float64
		maxLoad  float64
		memLow   bool
		memPct   float64
		minFree  float64
		contains []string
	}{
		{true, 9.5, 8.0, false, 20, 10, []string{"load"}},
		{false, 3.0, 8.0, true, 7.5, 10, []string{"free RAM"}},
		{true, 9.5, 8.0, true, 7.5, 10, []string{"load", "free RAM"}},
	}

	for _, tt := range tests {
		got := buildReason(tt.loadHigh, tt.load, tt.maxLoad, tt.memLow, tt.memPct, tt.minFree)
		for _, want := range tt.contains {
			if !containsStr(got, want) {
				t.Errorf("buildReason(...) = %q, want it to contain %q", got, want)
			}
		}
	}
}

func TestSpawnFilePath_DefaultsToHome(t *testing.T) {
	t.Setenv("AGM_CONFIG_DIR", "")
	p := spawnFilePath()
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".agm", "last-spawn.txt")
	if p != want {
		t.Errorf("spawnFilePath() = %q, want %q", p, want)
	}
}

func TestSpawnFilePath_EnvOverride(t *testing.T) {
	t.Setenv("AGM_CONFIG_DIR", "/tmp/custom-agm")
	p := spawnFilePath()
	want := "/tmp/custom-agm/last-spawn.txt"
	if p != want {
		t.Errorf("spawnFilePath() = %q, want %q", p, want)
	}
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
