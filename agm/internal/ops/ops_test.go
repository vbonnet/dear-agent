package ops

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// mockTmux implements the tmux interfaces needed by ops.
type mockTmux struct {
	sessions map[string]bool
}

func newMockTmux(sessions ...string) *mockTmux {
	m := &mockTmux{sessions: make(map[string]bool)}
	for _, s := range sessions {
		m.sessions[s] = true
	}
	return m
}

func (m *mockTmux) HasSession(name string) (bool, error) {
	return m.sessions[name], nil
}

func (m *mockTmux) ListSessions() ([]string, error) {
	var names []string
	for name := range m.sessions {
		names = append(names, name)
	}
	return names, nil
}

func (m *mockTmux) ListSessionsWithInfo() ([]session.SessionInfo, error) {
	var infos []session.SessionInfo
	for name := range m.sessions {
		infos = append(infos, session.SessionInfo{
			Name:            name,
			AttachedClients: 0,
		})
	}
	return infos, nil
}

func (m *mockTmux) ListClients(string) ([]session.ClientInfo, error) {
	return nil, nil
}

func (m *mockTmux) CreateSession(name, workdir string) error { return nil }
func (m *mockTmux) AttachSession(name string) error          { return nil }
func (m *mockTmux) SendKeys(session, keys string) error      { return nil }

// testCtx creates an OpContext with mock storage and tmux.
func testCtx(sessions []*manifest.Manifest, tmuxSessions ...string) *OpContext {
	mock := dolt.NewMockAdapter()
	for _, s := range sessions {
		_ = mock.CreateSession(s)
	}
	return &OpContext{
		Storage: mock,
		Tmux:    newMockTmux(tmuxSessions...),
	}
}

func newManifest(id, name, project string) *manifest.Manifest {
	return &manifest.Manifest{
		SessionID: id,
		Name:      name,
		Harness:   "claude-code",
		State:     "DONE",
		Context: manifest.Context{
			Project: project,
		},
		Tmux: manifest.Tmux{
			SessionName: name,
		},
		CreatedAt: time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC),
	}
}

// --- ListSessions tests ---

func TestListSessions_Empty(t *testing.T) {
	ctx := testCtx(nil)
	result, err := ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 sessions, got %d", result.Total)
	}
	if result.Operation != "list_sessions" {
		t.Errorf("expected operation list_sessions, got %s", result.Operation)
	}
}

func TestListSessions_ReturnsAll(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "session-1", "~/project-a"),
		newManifest("id-2", "session-2", "~/project-b"),
	}
	ctx := testCtx(sessions, "session-1")

	result, err := ListSessions(ctx, &ListSessionsRequest{Status: "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 2 {
		t.Errorf("expected 2 sessions, got %d", result.Total)
	}

	// session-1 should be active (tmux running), session-2 stopped
	for _, s := range result.Sessions {
		if s.Name == "session-1" && s.Status != "active" {
			t.Errorf("session-1 should be active, got %s", s.Status)
		}
		if s.Name == "session-2" && s.Status != "stopped" {
			t.Errorf("session-2 should be stopped, got %s", s.Status)
		}
	}
}

func TestListSessions_LimitValidation(t *testing.T) {
	ctx := testCtx(nil)
	_, err := ListSessions(ctx, &ListSessionsRequest{Limit: 1001})
	if err == nil {
		t.Fatal("expected error for limit > 1000")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeInvalidInput {
		t.Errorf("expected code %s, got %s", ErrCodeInvalidInput, opErr.Code)
	}
}

func TestListSessions_InvalidStatus(t *testing.T) {
	ctx := testCtx(nil)
	_, err := ListSessions(ctx, &ListSessionsRequest{Status: "bogus"})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestListSessions_DefaultLimit(t *testing.T) {
	ctx := testCtx(nil)
	result, err := ListSessions(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 100 {
		t.Errorf("expected default limit 100, got %d", result.Limit)
	}
}

func TestListSessions_ExcludeStoppedHidesOfflineSessions(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "running-session", "~/project-a"),
		newManifest("id-2", "stopped-session", "~/project-b"),
		newManifest("id-3", "also-running", "~/project-c"),
	}
	// Only running-session and also-running have tmux sessions
	ctx := testCtx(sessions, "running-session", "also-running")

	result, err := ListSessions(ctx, &ListSessionsRequest{
		Status:         "active",
		ExcludeStopped: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("expected 2 sessions (stopped hidden), got %d", result.Total)
	}
	for _, s := range result.Sessions {
		if s.Status == "stopped" {
			t.Errorf("stopped session %q should be excluded", s.Name)
		}
	}
}

func TestListSessions_IncludesStoppedByDefault(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "running-session", "~/project-a"),
		newManifest("id-2", "stopped-session", "~/project-b"),
	}
	ctx := testCtx(sessions, "running-session")

	result, err := ListSessions(ctx, &ListSessionsRequest{
		Status:         "active",
		ExcludeStopped: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("expected 2 sessions (stopped included), got %d", result.Total)
	}

	foundStopped := false
	for _, s := range result.Sessions {
		if s.Name == "stopped-session" && s.Status == "stopped" {
			foundStopped = true
		}
	}
	if !foundStopped {
		t.Error("stopped-session should be included when ExcludeStopped is false")
	}
}

func TestListSessions_AllShowsStoppedAndArchived(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "running-session", "~/project-a"),
		newManifest("id-2", "stopped-session", "~/project-b"),
		newManifest("id-3", "archived-session", "~/project-c"),
	}
	sessions[2].Lifecycle = manifest.LifecycleArchived

	ctx := testCtx(sessions, "running-session")

	// With status=all and ExcludeStopped=false, everything is visible
	result, err := ListSessions(ctx, &ListSessionsRequest{
		Status:         "all",
		ExcludeStopped: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("expected 3 sessions (all shown), got %d", result.Total)
	}
}

// --- GetSession tests ---

func TestGetSession_ByID(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("abc-123", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")

	result, err := GetSession(ctx, &GetSessionRequest{Identifier: "abc-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Session.ID != "abc-123" {
		t.Errorf("expected ID abc-123, got %s", result.Session.ID)
	}
	if result.Session.Name != "my-session" {
		t.Errorf("expected name my-session, got %s", result.Session.Name)
	}
	if result.Session.Status != "active" {
		t.Errorf("expected active status, got %s", result.Session.Status)
	}
}

func TestGetSession_ByName(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("abc-123", "my-session", "~/project"),
	}
	ctx := testCtx(sessions, "my-session")

	result, err := GetSession(ctx, &GetSessionRequest{Identifier: "my-session"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Session.ID != "abc-123" {
		t.Errorf("expected ID abc-123, got %s", result.Session.ID)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	ctx := testCtx(nil)
	_, err := GetSession(ctx, &GetSessionRequest{Identifier: "nonexistent"})
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
	if len(opErr.Suggestions) == 0 {
		t.Error("expected suggestions in error")
	}
}

func TestGetSession_EmptyIdentifier(t *testing.T) {
	ctx := testCtx(nil)
	_, err := GetSession(ctx, &GetSessionRequest{Identifier: ""})
	if err == nil {
		t.Fatal("expected error for empty identifier")
	}
}

// --- GetStatus tests ---

func TestGetStatus_Summary(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "running", "/project"),
		newManifest("id-2", "stopped-session", "/project"),
	}
	ctx := testCtx(sessions, "running")

	result, err := GetStatus(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary.Total != 2 {
		t.Errorf("expected 2 total, got %d", result.Summary.Total)
	}
	if result.Summary.Active != 1 {
		t.Errorf("expected 1 active, got %d", result.Summary.Active)
	}
	if result.Summary.Stopped != 1 {
		t.Errorf("expected 1 stopped, got %d", result.Summary.Stopped)
	}
}

// --- Error format tests ---

func TestOpError_RFC7807Format(t *testing.T) {
	err := ErrSessionNotFound("test-session")

	data := err.JSON()
	var parsed map[string]interface{}
	if jsonErr := json.Unmarshal(data, &parsed); jsonErr != nil {
		t.Fatalf("error JSON is not valid: %v", jsonErr)
	}

	// Verify required RFC 7807 fields
	requiredFields := []string{"status", "type", "code", "title", "detail", "suggestions"}
	for _, field := range requiredFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("missing required RFC 7807 field: %s", field)
		}
	}

	if parsed["code"] != ErrCodeSessionNotFound {
		t.Errorf("expected code %s, got %v", ErrCodeSessionNotFound, parsed["code"])
	}

	suggestions, ok := parsed["suggestions"].([]interface{})
	if !ok || len(suggestions) == 0 {
		t.Error("expected non-empty suggestions array")
	}
}

func TestOpError_ErrorString(t *testing.T) {
	err := ErrSessionNotFound("xyz")
	str := err.Error()
	if str == "" {
		t.Error("error string should not be empty")
	}
}

// --- Orphan tmux session tests ---

func TestListSessions_DetectsOrphans(t *testing.T) {
	// AGM knows about session-1, but tmux has session-1 AND orphan-worker
	sessions := []*manifest.Manifest{
		newManifest("id-1", "session-1", "~/project-a"),
	}
	ctx := testCtx(sessions, "session-1", "orphan-worker")

	result, err := ListSessions(ctx, &ListSessionsRequest{Status: "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.OrphanTmuxSessions) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(result.OrphanTmuxSessions))
	}
	if result.OrphanTmuxSessions[0] != "orphan-worker" {
		t.Errorf("expected orphan name orphan-worker, got %s", result.OrphanTmuxSessions[0])
	}
}

func TestListSessions_NoOrphansWhenClean(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "session-1", "~/project-a"),
		newManifest("id-2", "session-2", "~/project-b"),
	}
	ctx := testCtx(sessions, "session-1", "session-2")

	result, err := ListSessions(ctx, &ListSessionsRequest{Status: "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.OrphanTmuxSessions) != 0 {
		t.Errorf("expected 0 orphans, got %d: %v", len(result.OrphanTmuxSessions), result.OrphanTmuxSessions)
	}
}

func TestListSessions_MultipleOrphansSorted(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "known-session", "~/project"),
	}
	ctx := testCtx(sessions, "known-session", "zebra-orphan", "alpha-orphan", "middle-orphan")

	result, err := ListSessions(ctx, &ListSessionsRequest{Status: "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.OrphanTmuxSessions) != 3 {
		t.Fatalf("expected 3 orphans, got %d", len(result.OrphanTmuxSessions))
	}
	// Verify sorted order
	expected := []string{"alpha-orphan", "middle-orphan", "zebra-orphan"}
	for i, name := range expected {
		if result.OrphanTmuxSessions[i] != name {
			t.Errorf("orphan[%d]: expected %s, got %s", i, name, result.OrphanTmuxSessions[i])
		}
	}
}

func TestListSessions_NoOrphansWhenNoTmux(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "session-1", "~/project"),
	}
	mock := dolt.NewMockAdapter()
	for _, s := range sessions {
		_ = mock.CreateSession(s)
	}
	ctx := &OpContext{
		Storage: mock,
		Tmux:    nil, // no tmux client
	}

	result, err := ListSessions(ctx, &ListSessionsRequest{Status: "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.OrphanTmuxSessions) != 0 {
		t.Errorf("expected 0 orphans with nil tmux, got %d", len(result.OrphanTmuxSessions))
	}
}

func TestFindOrphanTmuxSessions_MatchesByTmuxSessionName(t *testing.T) {
	// Manifest has a different Tmux.SessionName than Name
	m := newManifest("id-1", "agm-name", "~/project")
	m.Tmux.SessionName = "tmux-name"

	tmuxSessions := []session.SessionInfo{
		{Name: "tmux-name"},
		{Name: "orphan-session"},
	}

	orphans := findOrphanTmuxSessions([]*manifest.Manifest{m}, tmuxSessions)
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	if orphans[0] != "orphan-session" {
		t.Errorf("expected orphan-session, got %s", orphans[0])
	}
}

func TestFindOrphanTmuxSessions_EmptyTmux(t *testing.T) {
	manifests := []*manifest.Manifest{newManifest("id-1", "s1", "~/p")}
	orphans := findOrphanTmuxSessions(manifests, nil)
	if len(orphans) != 0 {
		t.Errorf("expected 0 orphans for nil tmux sessions, got %d", len(orphans))
	}
}

// --- FieldMask tests ---

func TestApplyFieldMask_Empty(t *testing.T) {
	input := map[string]string{"a": "1", "b": "2"}
	result, err := ApplyFieldMask(input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 fields, got %d", len(parsed))
	}
}

func TestApplyFieldMask_Filters(t *testing.T) {
	input := map[string]string{"a": "1", "b": "2", "c": "3"}
	result, err := ApplyFieldMask(input, []string{"a", "c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 fields, got %d", len(parsed))
	}
	if parsed["a"] != "1" || parsed["c"] != "3" {
		t.Errorf("unexpected values: %v", parsed)
	}
	if _, ok := parsed["b"]; ok {
		t.Error("field 'b' should be filtered out")
	}
}
