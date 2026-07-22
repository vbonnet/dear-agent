package circuitbreaker

import (
	"os"
	"path/filepath"
	"runtime"
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

type stubWorkers struct {
	count int
	err   error
}

func (s stubWorkers) CountWorkers() (int, error) { return s.count, s.err }

type stubTimer struct {
	t        time.Time
	err      error
	recorded time.Time
}

func (s *stubTimer) LastSpawnTime() (time.Time, error) { return s.t, s.err }
func (s *stubTimer) RecordSpawn(t time.Time) error     { s.recorded = t; return nil }

type stubMem struct {
	pct float64
	err error
}

func (s stubMem) FreeMemPct() (float64, error) { return s.pct, s.err }

// noMem is a nil MemReader used when memory gate should be skipped.
var noMem MemReader = nil

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
				noMem,
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
				noMem,
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

// --- Gate 3: Memory ---

func TestCheckMemory(t *testing.T) {
	cfg := Config{MaxWorkers: 10, MaxLoad5: 100, MinFreeMemPct: 10, MinSpawnInterval: 0}

	tests := []struct {
		name    string
		freePct float64
		mr      MemReader
		wantOK  bool
	}{
		{"nil reader fails open", 0, nil, true},
		{"well above threshold", 50, stubMem{pct: 50}, true},
		{"at threshold", 10, stubMem{pct: 10}, true},
		{"just below threshold", 9.9, stubMem{pct: 9.9}, false},
		{"critically low", 1, stubMem{pct: 1}, false},
		{"reader error fails closed", 0, stubMem{err: os.ErrPermission}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Check(cfg,
				stubLoad{load: 1},
				stubWorkers{count: 0},
				&stubTimer{err: os.ErrNotExist},
				tt.mr,
			)
			var gate GateResult
			for _, g := range r.Gates {
				if g.Gate == "memory" {
					gate = g
				}
			}
			if gate.Passed != tt.wantOK {
				t.Errorf("memory gate: passed=%v, want=%v (msg: %s)", gate.Passed, tt.wantOK, gate.Message)
			}
		})
	}
}

// --- Gate 4: SpawnStagger ---

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
				noMem,
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

func TestCheckSpawnStaggerDistinguishesGovernorPause(t *testing.T) {
	cfg := Config{MaxWorkers: 10, MaxLoad5: 100, MinSpawnInterval: 2 * time.Minute}
	future := time.Now().Add(2 * time.Minute).Truncate(time.Second)
	resumeAt := future.Add(cfg.MinSpawnInterval)
	timer := FileSpawnTimer{Dir: t.TempDir()}
	if err := timer.RecordSpawn(future); err != nil {
		t.Fatalf("record governor hold: %v", err)
	}

	gate := checkSpawnStagger(cfg, timer)
	if gate.Passed {
		t.Fatal("future governor hold passed the spawn stagger gate")
	}
	for _, want := range []string{
		"spawns paused by resource governor",
		"admission resumes automatically at " + resumeAt.Format(time.RFC3339),
		"after the governor hold and 2m spawn safety interval",
		"remaining",
	} {
		if !strings.Contains(gate.Message, want) {
			t.Errorf("message %q does not contain %q", gate.Message, want)
		}
	}
	if strings.Contains(gate.Message, "last spawn was") {
		t.Errorf("governor hold misreported as a recent spawn: %q", gate.Message)
	}
}

func TestCheckSpawnStaggerGovernorPauseReportsFullAdmissionWindow(t *testing.T) {
	cfg := Config{MaxWorkers: 10, MaxLoad5: 100, MinSpawnInterval: 2 * time.Minute}
	future := time.Now().Add(30 * time.Second).Truncate(time.Second)

	gate := checkSpawnStagger(cfg, &stubTimer{t: future})
	if gate.Passed {
		t.Fatal("future governor hold passed the spawn stagger gate")
	}
	wantResumeAt := future.Add(cfg.MinSpawnInterval).Format(time.RFC3339)
	if !strings.Contains(gate.Message, wantResumeAt) {
		t.Fatalf("governor diagnostic %q does not report effective resume %s", gate.Message, wantResumeAt)
	}
	if strings.Contains(gate.Message, "at "+future.Format(time.RFC3339)+" (") {
		t.Fatalf("governor diagnostic reports the hold timestamp as the full admission expiry: %q", gate.Message)
	}
}

func TestCheckSpawnStaggerRetainsRecentSpawnDiagnostic(t *testing.T) {
	cfg := Config{MaxWorkers: 10, MaxLoad5: 100, MinSpawnInterval: 2 * time.Minute}

	gate := checkSpawnStagger(cfg, &stubTimer{t: time.Now().Add(-30 * time.Second)})
	if gate.Passed {
		t.Fatal("recent spawn passed the spawn stagger gate")
	}
	if !strings.Contains(gate.Message, "spawn too soon: last spawn was") {
		t.Errorf("recent spawn diagnostic changed unexpectedly: %q", gate.Message)
	}
	if strings.Contains(gate.Message, "resource governor") {
		t.Errorf("recent spawn misreported as a governor hold: %q", gate.Message)
	}
}

// --- Combined: all gates ---

func TestCheckAllGatesPass(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinFreeMemPct: 10, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 10},
		stubWorkers{count: 1},
		&stubTimer{t: time.Now().Add(-5 * time.Minute)},
		stubMem{pct: 50},
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
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinFreeMemPct: 10, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 80},
		stubWorkers{count: 5},
		&stubTimer{t: time.Now().Add(-30 * time.Second)},
		stubMem{pct: 2},
	)

	if r.Allowed {
		t.Error("expected Allowed=false, got true")
	}

	// All four gates should have failed
	for _, g := range r.Gates {
		if g.Passed {
			t.Errorf("expected gate %s to fail, but it passed: %s", g.Gate, g.Message)
		}
	}
}

// A load probe that cannot answer is itself a saturation signal: on 2026-07-18
// the host's probes and remediation were being killed while the mesh kept
// spawning (ce-93lw.18). Refusing is the point.
func TestCheckFailClosed_LoadError(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{err: os.ErrPermission},
		stubWorkers{count: 0},
		&stubTimer{err: os.ErrNotExist},
		noMem,
	)

	if r.Allowed {
		t.Error("should fail closed when the load reader errors")
	}
}

func TestCheckFailClosed_LoadError_OverrideAllowsSpawn(t *testing.T) {
	t.Setenv(allowUnverifiedEnv, "1")
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{err: os.ErrPermission},
		stubWorkers{count: 0},
		&stubTimer{err: os.ErrNotExist},
		noMem,
	)

	if !r.Allowed {
		t.Errorf("%s=1 should allow a spawn with an unreadable load probe: %s",
			allowUnverifiedEnv, FormatDenied(r))
	}
	if g := findGate(r, "cpu_load"); !strings.Contains(g.Message, allowUnverifiedEnv) {
		t.Errorf("cpu_load message must name the override that let it pass, got %q", g.Message)
	}
}

// The override buys tolerance for an unreadable probe, never for a reading that
// actually breaches the threshold.
func TestOverrideDoesNotPassRealBreach(t *testing.T) {
	t.Setenv(allowUnverifiedEnv, "1")
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinFreeMemPct: 10, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 999},
		stubWorkers{count: 0},
		&stubTimer{err: os.ErrNotExist},
		stubMem{pct: 1},
	)

	if r.Allowed {
		t.Error("override must not pass a real load/memory threshold breach")
	}
	if g := findGate(r, "cpu_load"); g.Passed {
		t.Error("cpu_load should still fail on a real breach under the override")
	}
	if g := findGate(r, "memory"); g.Passed {
		t.Error("memory should still fail on a real breach under the override")
	}
}

func TestCheckFailOpen_WorkerCountError(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 1},
		stubWorkers{err: os.ErrPermission},
		&stubTimer{err: os.ErrNotExist},
		noMem,
	)

	if !r.Allowed {
		t.Error("should fail open when worker counter errors")
	}
}

func TestCheckFailClosed_MemError(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinFreeMemPct: 10, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 1},
		stubWorkers{count: 0},
		&stubTimer{err: os.ErrNotExist},
		stubMem{err: os.ErrPermission},
	)

	if r.Allowed {
		t.Error("should fail closed when the memory reader errors")
	}
}

// A nil MemReader (non-Darwin) is a gate that was never wired, not a gate that
// failed. Nothing was asked, so nothing went unanswered.
func TestCheckMemory_NilReaderSkipsGate(t *testing.T) {
	cfg := Config{MaxWorkers: 3, MaxLoad5: 50, MinFreeMemPct: 10, MinSpawnInterval: 2 * time.Minute}

	r := Check(cfg,
		stubLoad{load: 1},
		stubWorkers{count: 0},
		&stubTimer{err: os.ErrNotExist},
		noMem,
	)

	if !r.Allowed {
		t.Errorf("an unwired memory reader must not refuse a spawn: %s", FormatDenied(r))
	}
	if g := findGate(r, "memory"); !g.Passed {
		t.Errorf("memory gate should be skipped, got %q", g.Message)
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

func TestFormatDenied_EmptyLevelFallsBackToUnknown(t *testing.T) {
	r := CheckResult{
		Allowed: false,
		Level:   "", // probe never ran / load classification missing
		Gates: []GateResult{
			{Gate: "max_workers", Passed: false, Message: "worker limit reached: 3/3"},
		},
	}

	msg := FormatDenied(r)
	if !contains(msg, "load level: unknown") {
		t.Errorf("expected empty load level to render as 'unknown', got: %s", msg)
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
	if cfg.MaxWorkers != 0 {
		t.Errorf("expected MaxWorkers=0 (default: no cap), got %d", cfg.MaxWorkers)
	}
}

func TestDefaultConfig_DefaultIsZero(t *testing.T) {
	// The default MaxWorkers is 0 (dynamic — no hard cap).
	// Workers are bounded by CPU load and spawn stagger gates instead.
	t.Setenv("AGM_MAX_WORKERS", "")
	cfg := DefaultConfig()
	if cfg.MaxWorkers != 0 {
		t.Errorf("expected MaxWorkers=0 (dynamic), got %d", cfg.MaxWorkers)
	}
}

func TestDefaultConfig_MaxLoad5Default(t *testing.T) {
	t.Setenv("AGM_MAX_LOAD5", "")
	cfg := DefaultConfig()
	want := float64(runtime.NumCPU()) * 2
	if cfg.MaxLoad5 != want {
		t.Errorf("expected MaxLoad5=%.0f (2×NumCPU), got %.1f", want, cfg.MaxLoad5)
	}
}

func TestDefaultConfig_MaxLoad5EnvOverride(t *testing.T) {
	t.Setenv("AGM_MAX_LOAD5", "24.5")
	cfg := DefaultConfig()
	if cfg.MaxLoad5 != 24.5 {
		t.Errorf("expected MaxLoad5=24.5, got %.1f", cfg.MaxLoad5)
	}
}

func TestDefaultConfig_MaxLoad5InvalidEnv(t *testing.T) {
	t.Setenv("AGM_MAX_LOAD5", "bad")
	cfg := DefaultConfig()
	want := float64(runtime.NumCPU()) * 2
	if cfg.MaxLoad5 != want {
		t.Errorf("expected MaxLoad5=%.0f (default) on invalid env, got %.1f", want, cfg.MaxLoad5)
	}
}

func TestDefaultConfig_MinFreeMemPctDefault(t *testing.T) {
	t.Setenv("AGM_MIN_FREE_MEM_PCT", "")
	cfg := DefaultConfig()
	if cfg.MinFreeMemPct != 10 {
		t.Errorf("expected MinFreeMemPct=10, got %.1f", cfg.MinFreeMemPct)
	}
}

func TestDefaultConfig_MinFreeMemPctEnvOverride(t *testing.T) {
	t.Setenv("AGM_MIN_FREE_MEM_PCT", "15")
	cfg := DefaultConfig()
	if cfg.MinFreeMemPct != 15 {
		t.Errorf("expected MinFreeMemPct=15, got %.1f", cfg.MinFreeMemPct)
	}
}

func TestDefaultConfig_MinFreeMemPctZeroAllowed(t *testing.T) {
	// 0 means disable the gate.
	t.Setenv("AGM_MIN_FREE_MEM_PCT", "0")
	cfg := DefaultConfig()
	if cfg.MinFreeMemPct != 0 {
		t.Errorf("expected MinFreeMemPct=0, got %.1f", cfg.MinFreeMemPct)
	}
}

func TestDynamicMode_MaxWorkersDisabled(t *testing.T) {
	// MaxWorkers=0 → gate passes regardless of worker count.
	cfg := Config{MaxWorkers: 0, MaxLoad5: 100, MinSpawnInterval: 0}

	for _, count := range []int{0, 1, 5, 10, 100} {
		r := Check(cfg,
			stubLoad{load: 1},
			stubWorkers{count: count},
			&stubTimer{err: os.ErrNotExist},
			noMem,
		)
		var gate GateResult
		for _, g := range r.Gates {
			if g.Gate == "max_workers" {
				gate = g
			}
		}
		if !gate.Passed {
			t.Errorf("max_workers gate should pass with MaxWorkers=0 and count=%d, got: %s", count, gate.Message)
		}
		if !r.Allowed {
			t.Errorf("spawn should be allowed with MaxWorkers=0 and count=%d", count)
		}
	}
}

func TestDynamicMode_CPUStillGates(t *testing.T) {
	// Even with MaxWorkers=0, a high CPU load should block spawning.
	cfg := Config{MaxWorkers: 0, MaxLoad5: 50, MinSpawnInterval: 0}

	r := Check(cfg,
		stubLoad{load: 80},
		stubWorkers{count: 100},
		&stubTimer{err: os.ErrNotExist},
		noMem,
	)

	if r.Allowed {
		t.Error("spawn should be blocked by CPU gate even when MaxWorkers=0")
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
