package circuitbreaker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- test doubles ---

type stubLoad struct{ load float64; err error }

func (s stubLoad) Load5() (float64, error) { return s.load, s.err }

type stubWorkers struct{ count int; err error }

func (s stubWorkers) CountWorkers() (int, error) { return s.count, s.err }

type stubTimer struct{ t time.Time; err error; recorded time.Time }

func (s *stubTimer) LastSpawnTime() (time.Time, error) { return s.t, s.err }
func (s *stubTimer) RecordSpawn(t time.Time) error     { s.recorded = t; return nil }

// --- DEARLevel ---

func TestClassifyLoad(t *testing.T) {
	tests := []struct {
		load float64
		want DEARLevel
	}{
		{0, DEARGreen},
		{10, DEARGreen},
		{39.9, DEARGreen},
		{40, DEARYellow},
		{50, DEARYellow},
		{60, DEARYellow},
		{60.1, DEARRed},
		{80, DEARRed},
		{100, DEARRed},
		{100.1, DEAREmergency},
		{226, DEAREmergency},
	}
	for _, tt := range tests {
		got := ClassifyLoad(tt.load)
		if got != tt.want {
			t.Errorf("ClassifyLoad(%.1f) = %s, want %s", tt.load, got, tt.want)
		}
	}
}

// --- Gate 1: MaxWorkers ---

func TestCheckMaxWorkers(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	tests := []struct {
		name    string
		workers int
		wantOK  bool
	}{
		{"zero workers", 0, true},
		{"under limit", 2, true},
		{"at limit", 3, false},
		{"over limit", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Check(cfg,
				stubLoad{load: 1},
				stubWorkers{count: tt.workers},
				&stubTimer{err: os.ErrNotExist},
			)
			// Find the max_workers gate
			var gate GateResult
			for _, g := range r.Gates {
				if g.Gate == "max_workers" {
					gate = g
				}
			}
			if gate.Passed != tt.wantOK {
				t.Errorf("max_workers gate: passed=%v, want=%v (msg: %s)", gate.Passed, tt.wantOK, gate.Message)
			}
		})
	}
}

// --- Gate 2: CPULoad ---

func TestCheckCPULoad(t *testing.T) {
	cfg := Config{MaxWorkers: 10, MaxLoad5: 50, MinSpawnInterval: 0}

	tests := []struct {
		name   string
		load   float64
		wantOK bool
	}{
		{"low load", 5, true},
		{"at threshold", 50, true},
		{"above threshold", 50.1, false},
		{"extreme", 226, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Check(cfg,
				stubLoad{load: tt.load},
				stubWorkers{count: 0},
				&stubTimer{err: os.ErrNotExist},
			)
			var gate GateResult
			for _, g := range r.Gates {
				if g.Gate == "cpu_load" {
					gate = g
				}
			}
			if gate.Passed != tt.wantOK {
				t.Errorf("cpu_load gate: passed=%v, want=%v (msg: %s)", gate.Passed, tt.wantOK, gate.Message)
			}
		})
	}
}

// --- Gate 3: SpawnStagger ---

func TestCheckSpawnStagger(t *testing.T) {
	cfg := Config{MaxWorkers: 10, MaxLoad5: 100, MinSpawnInterval: 2 * time.Minute}

	tests := []struct {
		name   string
		last   time.Time
		err    error
		wantOK bool
	}{
		{"no previous spawn", time.Time{}, os.ErrNotExist, true},
		{"spawned 3 min ago", time.Now().Add(-3 * time.Minute), nil, true},
		{"spawned 2 min ago", time.Now().Add(-2 * time.Minute), nil, true},
		{"spawned 1 min ago", time.Now().Add(-1 * time.Minute), nil, false},
		{"spawned 30s ago", time.Now().Add(-30 * time.Second), nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Check(cfg,
				stubLoad{load: 1},
				stubWorkers{count: 0},
				&stubTimer{t: tt.last, err: tt.err},
			)
			var gate GateResult
			for _, g := range r.Gates {
				if g.Gate == "spawn_stagger" {
					gate = g
				}
			}
			if gate.Passed != tt.wantOK {
				t.Errorf("spawn_stagger gate: passed=%v, want=%v (msg: %s)", gate.Passed, tt.wantOK, gate.Message)
			}
		})
	}
}

// --- Combined: all gates ---

func TestCheckAllGatesPass(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 10},
		stubWorkers{count: 1},
		&stubTimer{t: time.Now().Add(-5 * time.Minute)},
	)

	if !r.Allowed {
		t.Errorf("expected Allowed=true, got false")
		for _, g := range r.Gates {
			t.Logf("  gate %s: passed=%v msg=%s", g.Gate, g.Passed, g.Message)
		}
	}
	if r.Level != DEARGreen {
		t.Errorf("expected DEAR level GREEN, got %s", r.Level)
	}
}

func TestCheckAllGatesFail(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 80},
		stubWorkers{count: 5},
		&stubTimer{t: time.Now().Add(-30 * time.Second)},
	)

	if r.Allowed {
		t.Error("expected Allowed=false, got true")
	}

	// All three gates should have failed
	for _, g := range r.Gates {
		if g.Passed {
			t.Errorf("expected gate %s to fail, but it passed: %s", g.Gate, g.Message)
		}
	}
}

func TestCheckFailOpen_LoadError(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{err: os.ErrPermission},
		stubWorkers{count: 0},
		&stubTimer{err: os.ErrNotExist},
	)

	if !r.Allowed {
		t.Error("should fail open when load reader errors")
	}
}

func TestCheckFailOpen_WorkerCountError(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 1},
		stubWorkers{err: os.ErrPermission},
		&stubTimer{err: os.ErrNotExist},
	)

	if !r.Allowed {
		t.Error("should fail open when worker counter errors")
	}
}

// --- FormatDenied ---

func TestFormatDenied(t *testing.T) {
	r := CheckResult{
		Allowed: false,
		Level:   DEARRed,
		Gates: []GateResult{
			{Gate: "max_workers", Passed: false, Message: "worker limit reached: 3/3"},
			{Gate: "cpu_load", Passed: true, Message: "ok"},
		},
	}

	msg := FormatDenied(r)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	if !contains(msg, "max_workers") {
		t.Errorf("expected max_workers in message, got: %s", msg)
	}
	if contains(msg, "cpu_load") {
		t.Errorf("should not include passing gates in denied message")
	}
}

// --- DefaultConfig env override ---

func TestDefaultConfig_EnvOverride(t *testing.T) {
	t.Setenv("AGM_MAX_WORKERS", "7")
	cfg := DefaultConfig()
	if cfg.MaxWorkers != 7 {
		t.Errorf("expected MaxWorkers=7, got %d", cfg.MaxWorkers)
	}
}

func TestDefaultConfig_InvalidEnv(t *testing.T) {
	t.Setenv("AGM_MAX_WORKERS", "abc")
	cfg := DefaultConfig()
	if cfg.MaxWorkers != 3 {
		t.Errorf("expected MaxWorkers=3 (default), got %d", cfg.MaxWorkers)
	}
}

// --- FileSpawnTimer ---

func TestFileSpawnTimer(t *testing.T) {
	dir := t.TempDir()
	timer := FileSpawnTimer{Dir: dir}

	// No file yet — should error
	_, err := timer.LastSpawnTime()
	if err == nil {
		t.Fatal("expected error when no spawn file exists")
	}

	// Record spawn
	now := time.Now().Truncate(time.Second)
	if err := timer.RecordSpawn(now); err != nil {
		t.Fatalf("RecordSpawn: %v", err)
	}

	// Read it back
	got, err := timer.LastSpawnTime()
	if err != nil {
		t.Fatalf("LastSpawnTime: %v", err)
	}
	if !got.Equal(now) {
		t.Errorf("LastSpawnTime = %v, want %v", got, now)
	}

	// Verify file location
	if _, err := os.Stat(filepath.Join(dir, lastSpawnFile)); err != nil {
		t.Errorf("spawn file not found at expected path: %v", err)
	}
}

// --- formatDuration ---

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "0s"},
		{5 * time.Second, "5s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m30s"},
		{2 * time.Minute, "2m"},
		{2*time.Minute + 15*time.Second, "2m15s"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
