package ops

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestCheckSessionHealth_NilRequest(t *testing.T) {
	ctx := testCtx(nil)
	_, err := CheckSessionHealth(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeInvalidInput {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidInput, opErr.Code)
	}
}

func TestCheckSessionHealth_EmptyIdentifier(t *testing.T) {
	ctx := testCtx(nil)
	_, err := CheckSessionHealth(ctx, &SessionHealthRequest{})
	if err == nil {
		t.Fatal("expected error for empty identifier without --all")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeInvalidInput {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidInput, opErr.Code)
	}
}

func TestCheckSessionHealth_NotFound(t *testing.T) {
	ctx := testCtx(nil)
	_, err := CheckSessionHealth(ctx, &SessionHealthRequest{Identifier: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionNotFound {
		t.Errorf("expected code %s, got %s", ErrCodeSessionNotFound, opErr.Code)
	}
}

func TestCheckSessionHealth_SingleSession(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	m := newManifest("id-1", "worker-1", "~/project")
	m.State = "WORKING"
	m.CreatedAt = time.Now().Add(-1 * time.Hour)
	m.UpdatedAt = time.Now().Add(-2 * time.Minute)
	ctx := testCtx([]*manifest.Manifest{m}, "worker-1")

	result, err := CheckSessionHealth(ctx, &SessionHealthRequest{Identifier: "worker-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Operation != "session_health" {
		t.Errorf("expected operation session_health, got %s", result.Operation)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 session, got %d", result.Total)
	}

	d := result.Sessions[0]
	if d.Name != "worker-1" {
		t.Errorf("expected name worker-1, got %s", d.Name)
	}
	if d.ID != "id-1" {
		t.Errorf("expected ID id-1, got %s", d.ID)
	}
	if d.Status != "active" {
		t.Errorf("expected active status, got %s", d.Status)
	}
	if d.State != "WORKING" {
		t.Errorf("expected WORKING state, got %s", d.State)
	}
	if d.StartedAt == "" {
		t.Error("expected non-empty StartedAt")
	}
	if d.Duration == "" {
		t.Error("expected non-empty Duration")
	}
	if d.Health == "" {
		t.Error("expected non-empty Health")
	}
}

func TestCheckSessionHealth_AllSessions(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	now := time.Now()
	sessions := []*manifest.Manifest{
		{
			SessionID: "id-1",
			Name:      "active-1",
			State:     "WORKING",
			Harness:   "claude-code",
			Context:   manifest.Context{Project: "~/proj-a"},
			Tmux:      manifest.Tmux{SessionName: "active-1"},
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Minute),
		},
		{
			SessionID: "id-2",
			Name:      "stopped-1",
			State:     "DONE",
			Harness:   "claude-code",
			Context:   manifest.Context{Project: "~/proj-b"},
			Tmux:      manifest.Tmux{SessionName: "stopped-1"},
			CreatedAt: now.Add(-3 * time.Hour),
			UpdatedAt: now.Add(-30 * time.Minute),
		},
		{
			SessionID: "id-3",
			Name:      "archived-1",
			State:     "DONE",
			Harness:   "claude-code",
			Lifecycle: manifest.LifecycleArchived,
			Context:   manifest.Context{Project: "~/proj-c"},
			Tmux:      manifest.Tmux{SessionName: "archived-1"},
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now.Add(-12 * time.Hour),
		},
	}
	ctx := testCtx(sessions, "active-1")

	result, err := CheckSessionHealth(ctx, &SessionHealthRequest{All: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should exclude archived sessions
	if result.Total != 2 {
		t.Errorf("expected 2 sessions (excluding archived), got %d", result.Total)
	}

	// Verify both non-archived sessions are present
	names := map[string]bool{}
	for _, d := range result.Sessions {
		names[d.Name] = true
	}
	if !names["active-1"] {
		t.Error("expected active-1 in results")
	}
	if !names["stopped-1"] {
		t.Error("expected stopped-1 in results")
	}
	if names["archived-1"] {
		t.Error("archived-1 should be excluded")
	}
}

func TestCheckSessionHealth_StoppedSession(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	m := newManifest("id-1", "stopped-worker", "~/project")
	m.State = "DONE"
	// tmux does not include this session → it's stopped
	ctx := testCtx([]*manifest.Manifest{m})

	result, err := CheckSessionHealth(ctx, &SessionHealthRequest{Identifier: "stopped-worker"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d := result.Sessions[0]
	if d.Status != "stopped" {
		t.Errorf("expected stopped status, got %s", d.Status)
	}
	if d.Health != "stopped" {
		t.Errorf("expected stopped health, got %s", d.Health)
	}
}

func TestCheckSessionHealth_WithTrustScore(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	// Record some trust events
	for i := 0; i < 3; i++ {
		_, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "trust-worker",
			EventType:   "success",
		})
		if err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	m := newManifest("id-1", "trust-worker", "~/project")
	m.State = "WORKING"
	ctx := testCtx([]*manifest.Manifest{m}, "trust-worker")

	result, err := CheckSessionHealth(ctx, &SessionHealthRequest{Identifier: "trust-worker"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	d := result.Sessions[0]
	if d.TrustScore == nil {
		t.Fatal("expected trust score to be populated")
	}
	// 50 (base) + 3*5 = 65
	if *d.TrustScore != 65 {
		t.Errorf("expected trust score 65, got %d", *d.TrustScore)
	}
}

func TestCheckSessionHealth_ByID(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()

	m := newManifest("unique-id-abc", "my-worker", "~/project")
	ctx := testCtx([]*manifest.Manifest{m}, "my-worker")

	result, err := CheckSessionHealth(ctx, &SessionHealthRequest{Identifier: "unique-id-abc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Sessions[0].ID != "unique-id-abc" {
		t.Errorf("expected ID unique-id-abc, got %s", result.Sessions[0].ID)
	}
}

// --- assessHealth tests ---

func TestAssessHealth_Healthy(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "2m",
		State:               "WORKING",
		ErrorLines:          0,
		CPUPct:              25.0,
		MemoryMB:            512,
	}
	health, warnings := assessHealth(d, "active")
	if health != "healthy" {
		t.Errorf("expected healthy, got %s", health)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestAssessHealth_StoppedSession(t *testing.T) {
	d := SessionHealthDetail{}
	health, warnings := assessHealth(d, "stopped")
	if health != "stopped" {
		t.Errorf("expected stopped, got %s", health)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got %v", warnings)
	}
}

func TestAssessHealth_ArchivedSession(t *testing.T) {
	d := SessionHealthDetail{}
	health, _ := assessHealth(d, "archived")
	if health != "archived" {
		t.Errorf("expected archived, got %s", health)
	}
}

func TestAssessHealth_WarningStaleUpdate(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "15m",
		State:               "WORKING",
	}
	health, warnings := assessHealth(d, "active")
	if health != "warning" {
		t.Errorf("expected warning, got %s", health)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestAssessHealth_CriticalStaleUpdate(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "45m",
		State:               "WORKING",
	}
	health, _ := assessHealth(d, "active")
	if health != "critical" {
		t.Errorf("expected critical, got %s", health)
	}
}

func TestAssessHealth_PermissionPrompt(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "1m",
		State:               "PERMISSION_PROMPT",
	}
	health, warnings := assessHealth(d, "active")
	if health != "warning" {
		t.Errorf("expected warning, got %s", health)
	}
	found := false
	for _, w := range warnings {
		if w == "Session waiting on permission prompt" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected permission prompt warning in %v", warnings)
	}
}

func TestAssessHealth_HighErrorRate(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "1m",
		State:               "WORKING",
		ErrorLines:          15,
	}
	health, _ := assessHealth(d, "active")
	if health != "critical" {
		t.Errorf("expected critical for high error rate, got %s", health)
	}
}

func TestAssessHealth_ElevatedErrorRate(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "1m",
		State:               "WORKING",
		ErrorLines:          5,
	}
	health, _ := assessHealth(d, "active")
	if health != "warning" {
		t.Errorf("expected warning for elevated error rate, got %s", health)
	}
}

func TestAssessHealth_HighCPU(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "1m",
		State:               "WORKING",
		CPUPct:              95.0,
	}
	health, _ := assessHealth(d, "active")
	if health != "warning" {
		t.Errorf("expected warning for high CPU, got %s", health)
	}
}

func TestAssessHealth_HighMemory(t *testing.T) {
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "1m",
		State:               "WORKING",
		MemoryMB:            5000,
	}
	health, _ := assessHealth(d, "active")
	if health != "warning" {
		t.Errorf("expected warning for high memory, got %s", health)
	}
}

func TestAssessHealth_LowTrustScore(t *testing.T) {
	score := 10
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "1m",
		State:               "WORKING",
		TrustScore:          &score,
	}
	health, warnings := assessHealth(d, "active")
	if health != "warning" {
		t.Errorf("expected warning for low trust score, got %s", health)
	}
	if len(warnings) == 0 {
		t.Error("expected trust score warning")
	}
}

func TestAssessHealth_MultipleWarnings(t *testing.T) {
	score := 5
	d := SessionHealthDetail{
		TimeSinceLastUpdate: "35m", // critical
		State:               "PERMISSION_PROMPT",
		ErrorLines:          5,  // warning
		CPUPct:              95, // warning
		TrustScore:          &score,
	}
	health, warnings := assessHealth(d, "active")
	if health != "critical" {
		t.Errorf("expected critical (worst of all), got %s", health)
	}
	// Should have multiple warnings
	if len(warnings) < 3 {
		t.Errorf("expected at least 3 warnings, got %d: %v", len(warnings), warnings)
	}
}

// --- formatDuration tests ---

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 15*time.Minute, "2h15m"},
		{2 * time.Hour, "2h"},
		{48 * time.Hour, "2d"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.input)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- parseDurationFromFormatted tests ---

func TestParseDurationFromFormatted(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"2h15m", 2*time.Hour + 15*time.Minute},
		{"1d3h20m", 27*time.Hour + 20*time.Minute},
		{"3d", 72 * time.Hour},
	}
	for _, tt := range tests {
		got := parseDurationFromFormatted(tt.input)
		if got != tt.want {
			t.Errorf("parseDurationFromFormatted(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseDurationFromFormatted_Roundtrip(t *testing.T) {
	durations := []time.Duration{
		45 * time.Second,
		7 * time.Minute,
		3*time.Hour + 45*time.Minute,
	}
	for _, d := range durations {
		formatted := formatDuration(d)
		parsed := parseDurationFromFormatted(formatted)
		if parsed != d {
			t.Errorf("roundtrip failed for %v: formatted=%q parsed=%v", d, formatted, parsed)
		}
	}
}
