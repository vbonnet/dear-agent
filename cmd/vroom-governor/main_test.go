package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/admission"
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

// --- admission brake (ce-93lw.18) ---

// TestUnreadableProbeReason pins the signal the governor used to discard. The
// `err == nil &&` guards in tick made a blind governor look exactly like a
// healthy one, so a host whose probes had stopped answering kept admitting work.
func TestUnreadableProbeReason(t *testing.T) {
	loadBoom := errors.New("sysctl vm.loadavg: signal: killed")
	memBoom := errors.New("memory_pressure -Q: context deadline exceeded")

	tests := []struct {
		name     string
		loadErr  error
		memErr   error
		want     bool
		contains []string
	}{
		{"both clean", nil, nil, false, nil},
		{"load unreadable", loadBoom, nil, true, []string{"load probe unreadable", "signal: killed"}},
		{"memory unreadable", nil, memBoom, true, []string{"memory probe unreadable", "deadline"}},
		{"both unreadable", loadBoom, memBoom, true, []string{"signal: killed", "deadline"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unreadableProbeReason(tt.loadErr, tt.memErr)
			if (got != "") != tt.want {
				t.Fatalf("unreadableProbeReason = %q, want engaged=%v", got, tt.want)
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("reason %q missing %q", got, want)
				}
			}
		})
	}
}

func TestApplyBrake_EngagesOnUnreadableProbe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := tickConfig{brakePath: path, brakeTTL: time.Hour}

	applyBrake(cfg, "load probe unreadable: signal: killed", false)

	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	switch {
	case brake == nil:
		t.Fatal("an unreadable probe must engage the brake")
	case brake.Source != brakeSource:
		t.Errorf("Source = %q, want %q", brake.Source, brakeSource)
	case !strings.Contains(brake.Reason, "signal: killed"):
		t.Errorf("Reason = %q, want the probe error", brake.Reason)
	}
}

func TestApplyBrake_ReleasesOnCleanTick(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := tickConfig{brakePath: path, brakeTTL: time.Hour}
	if err := admission.Engage(path, brakeSource, "earlier blindness", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	applyBrake(cfg, "", false)

	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if brake != nil {
		t.Errorf("clean in-threshold tick left the brake engaged: %+v", brake)
	}
}

// An ordinary threshold breach is handled by the last-spawn.txt pause. Clearing
// the brake here would let this governor overrule a brake disk-watchdog engaged
// for an unrelated reason.
func TestApplyBrake_ThresholdBreachDoesNotTouchTheBrake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := tickConfig{brakePath: path, brakeTTL: time.Hour}
	if err := admission.Engage(path, "disk-watchdog", "sweep killed", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	applyBrake(cfg, "", true)

	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	switch {
	case brake == nil:
		t.Fatal("a clean-but-breaching tick must not clear another watchdog's brake")
	case brake.Source != "disk-watchdog":
		t.Errorf("Source = %q, want the disk-watchdog brake preserved", brake.Source)
	}
}

func TestApplyBrake_EmptyPathIsANoOp(t *testing.T) {
	applyBrake(tickConfig{}, "load probe unreadable", false) // must not panic
}

// The governor ticks every 30s and disk-watchdog every 5 minutes. An
// unconditional release here would clear a disk brake almost as fast as the
// watchdog could set one — silently defeating ce-93lw.18 on its likeliest path,
// a host out of disk but not out of CPU.
func TestApplyBrake_CleanTickDoesNotClearAnotherWatchdogsBrake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := tickConfig{brakePath: path, brakeTTL: time.Hour}
	if err := admission.Engage(path, "disk-watchdog", "worktree-sweep remediation failed: signal: killed", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	applyBrake(cfg, "", false) // probes healthy, thresholds fine

	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	switch {
	case brake == nil:
		t.Fatal("a healthy governor tick cleared the disk-watchdog brake")
	case brake.Source != "disk-watchdog":
		t.Errorf("Source = %q, want the disk-watchdog brake preserved", brake.Source)
	}
}

func TestApplyBrake_CleanTickClearsItsOwnBrake(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admission-brake.json")
	cfg := tickConfig{brakePath: path, brakeTTL: time.Hour}
	if err := admission.Engage(path, brakeSource, "load probe unreadable", time.Hour); err != nil {
		t.Fatalf("Engage: %v", err)
	}

	applyBrake(cfg, "", false)

	brake, err := admission.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if brake != nil {
		t.Errorf("governor did not clear its own brake on a clean tick: %+v", brake)
	}
}
