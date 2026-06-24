package circuitbreaker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- test doubles ---

type stubLoad struct {
	load float64
	err  error
}

func (s stubLoad) Load5() (float64, error) { return s.load, s.err }

type stubTimer struct {
	t        time.Time
	err      error
	recorded time.Time
}

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

// --- Gate 1: CPULoad ---

func TestCheckCPULoad(t *testing.T) {
	cfg := Config{MaxLoad5: 50, MinSpawnInterval: 0}

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
				&stubTimer{err: os.ErrNotExist},
			)
			gate := findGate(r, "cpu_load")
			if gate.Passed != tt.wantOK {
				t.Errorf("cpu_load gate: passed=%v, want=%v (msg: %s)", gate.Passed, tt.wantOK, gate.Message)
			}
			assertNoMaxWorkersGate(t, r)
		})
	}
}

// --- Gate 2: SpawnStagger ---

func TestCheckSpawnStagger(t *testing.T) {
	cfg := Config{MaxLoad5: 100, MinSpawnInterval: 2 * time.Minute}

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
				&stubTimer{t: tt.last, err: tt.err},
			)
			gate := findGate(r, "spawn_stagger")
			if gate.Passed != tt.wantOK {
				t.Errorf("spawn_stagger gate: passed=%v, want=%v (msg: %s)", gate.Passed, tt.wantOK, gate.Message)
			}
			assertNoMaxWorkersGate(t, r)
		})
	}
}

// --- Combined: all gates ---

func TestCheckAllGatesPass(t *testing.T) {
	cfg := Config{MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 10},
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
	assertNoMaxWorkersGate(t, r)
}

func TestCheckAllGatesFail(t *testing.T) {
	cfg := Config{MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 80},
		&stubTimer{t: time.Now().Add(-30 * time.Second)},
	)

	if r.Allowed {
		t.Error("expected Allowed=false, got true")
	}

	// Both remaining gates should have failed.
	for _, g := range r.Gates {
		if g.Passed {
			t.Errorf("expected gate %s to fail, but it passed: %s", g.Gate, g.Message)
		}
	}
	assertNoMaxWorkersGate(t, r)
}

func TestCheckDoesNotEmitWorkerCapGate(t *testing.T) {
	cfg := Config{MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 10},
		&stubTimer{t: time.Now().Add(-5 * time.Minute)},
	)

	assertNoMaxWorkersGate(t, r)
	if len(r.Gates) != 2 {
		t.Fatalf("expected exactly CPU and stagger gates, got %d: %#v", len(r.Gates), r.Gates)
	}
}

func TestCheckFailOpen_LoadError(t *testing.T) {
	cfg := Config{MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{err: os.ErrPermission},
		&stubTimer{err: os.ErrNotExist},
	)

	if !r.Allowed {
		t.Error("should fail open when load reader errors")
	}
}

// --- FormatDenied ---

func TestFormatDenied(t *testing.T) {
	r := CheckResult{
		Allowed: false,
		Level:   DEARRed,
		Gates: []GateResult{
			{Gate: "spawn_stagger", Passed: false, Message: "spawn too soon"},
			{Gate: "cpu_load", Passed: true, Message: "ok"},
		},
	}

	msg := FormatDenied(r)
	if msg == "" {
		t.Fatal("expected non-empty message")
	}
	if !strings.Contains(msg, "spawn_stagger") {
		t.Errorf("expected spawn_stagger in message, got: %s", msg)
	}
	if strings.Contains(msg, "cpu_load") {
		t.Errorf("should not include passing gates in denied message")
	}
}

func TestFormatDenied_EmptyLevelFallsBackToUnknown(t *testing.T) {
	r := CheckResult{
		Allowed: false,
		Level:   "", // probe never ran / load classification missing
		Gates: []GateResult{
			{Gate: "spawn_stagger", Passed: false, Message: "spawn too soon"},
		},
	}

	msg := FormatDenied(r)
	if !strings.Contains(msg, "load level: unknown") {
		t.Errorf("expected empty load level to render as 'unknown', got: %s", msg)
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

func findGate(r CheckResult, name string) GateResult {
	for _, g := range r.Gates {
		if g.Gate == name {
			return g
		}
	}
	return GateResult{}
}

func assertNoMaxWorkersGate(t *testing.T, r CheckResult) {
	t.Helper()
	for _, g := range r.Gates {
		if g.Gate == "max_workers" {
			t.Fatalf("worker-count cap gate must not be present: %#v", r.Gates)
		}
	}
}
