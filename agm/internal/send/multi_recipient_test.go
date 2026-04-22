package send

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// mockResolver implements SessionResolver for testing
type mockResolver struct {
	sessions map[string]*manifest.Manifest
	listErr  error
}

func (m *mockResolver) ResolveIdentifier(identifier string) (*manifest.Manifest, error) {
	if session, ok := m.sessions[identifier]; ok {
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", identifier)
}

func (m *mockResolver) ListAllSessions() ([]*manifest.Manifest, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var sessions []*manifest.Manifest
	for _, s := range m.sessions {
		// Only return non-archived sessions
		if s.Lifecycle != manifest.LifecycleArchived {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		sessions: map[string]*manifest.Manifest{
			"session1": {
				SessionID: "id1",
				Name:      "session1",
				Tmux:      manifest.Tmux{SessionName: "session1"},
				Lifecycle: "",
			},
			"session2": {
				SessionID: "id2",
				Name:      "session2",
				Tmux:      manifest.Tmux{SessionName: "session2"},
				Lifecycle: "",
			},
			"session3": {
				SessionID: "id3",
				Name:      "session3",
				Tmux:      manifest.Tmux{SessionName: "session3"},
				Lifecycle: "",
			},
			"research-1": {
				SessionID: "id4",
				Name:      "research-1",
				Tmux:      manifest.Tmux{SessionName: "research-1"},
				Lifecycle: "",
			},
			"research-2": {
				SessionID: "id5",
				Name:      "research-2",
				Tmux:      manifest.Tmux{SessionName: "research-2"},
				Lifecycle: "",
			},
			"test-session": {
				SessionID: "id6",
				Name:      "test-session",
				Tmux:      manifest.Tmux{SessionName: "test-session"},
				Lifecycle: "",
			},
			"archived-session": {
				SessionID: "id7",
				Name:      "archived-session",
				Tmux:      manifest.Tmux{SessionName: "archived-session"},
				Lifecycle: manifest.LifecycleArchived,
			},
		},
	}
}

func TestParseRecipients_SingleDirect(t *testing.T) {
	spec, err := ParseRecipients([]string{"session1"}, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Type != "direct" {
		t.Errorf("expected type 'direct', got '%s'", spec.Type)
	}

	if len(spec.Recipients) != 1 {
		t.Errorf("expected 1 recipient, got %d", len(spec.Recipients))
	}

	if spec.Recipients[0] != "session1" {
		t.Errorf("expected recipient 'session1', got '%s'", spec.Recipients[0])
	}
}

func TestParseRecipients_CommaList(t *testing.T) {
	spec, err := ParseRecipients([]string{"session1,session2,session3"}, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Type != "comma_list" {
		t.Errorf("expected type 'comma_list', got '%s'", spec.Type)
	}

	if len(spec.Recipients) != 3 {
		t.Errorf("expected 3 recipients, got %d", len(spec.Recipients))
	}

	expected := []string{"session1", "session2", "session3"}
	for i, exp := range expected {
		if spec.Recipients[i] != exp {
			t.Errorf("expected recipient[%d] '%s', got '%s'", i, exp, spec.Recipients[i])
		}
	}
}

func TestParseRecipients_GlobPattern(t *testing.T) {
	spec, err := ParseRecipients([]string{"*research*"}, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Type != "glob" {
		t.Errorf("expected type 'glob', got '%s'", spec.Type)
	}

	if len(spec.Recipients) != 1 {
		t.Errorf("expected 1 recipient (pattern), got %d", len(spec.Recipients))
	}

	if spec.Recipients[0] != "*research*" {
		t.Errorf("expected pattern '*research*', got '%s'", spec.Recipients[0])
	}
}

func TestParseRecipients_ToFlag(t *testing.T) {
	spec, err := ParseRecipients([]string{}, "session1,session2", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Type != "comma_list" {
		t.Errorf("expected type 'comma_list', got '%s'", spec.Type)
	}

	if len(spec.Recipients) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(spec.Recipients))
	}
}

func TestParseRecipients_ToFlagPriority(t *testing.T) {
	// --to flag takes priority over positional arg
	spec, err := ParseRecipients([]string{"session1"}, "session2,session3", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Type != "comma_list" {
		t.Errorf("expected type 'comma_list', got '%s'", spec.Type)
	}

	if len(spec.Recipients) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(spec.Recipients))
	}

	if spec.Raw != "session2,session3" {
		t.Errorf("expected raw 'session2,session3', got '%s'", spec.Raw)
	}
}

func TestParseRecipients_NoInput(t *testing.T) {
	_, err := ParseRecipients([]string{}, "", "", false)
	if err == nil {
		t.Fatal("expected error for no input, got nil")
	}
}

func TestResolveRecipients_SingleDirect(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:       "direct",
		Recipients: []string{"session1"},
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Recipients) != 1 {
		t.Errorf("expected 1 recipient, got %d", len(resolved.Recipients))
	}

	if resolved.Recipients[0] != "session1" {
		t.Errorf("expected 'session1', got '%s'", resolved.Recipients[0])
	}
}

func TestResolveRecipients_CommaList(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:       "comma_list",
		Recipients: []string{"session1", "session2", "session3"},
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Recipients) != 3 {
		t.Errorf("expected 3 recipients, got %d", len(resolved.Recipients))
	}
}

func TestResolveRecipients_CommaListWithDuplicates(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:       "comma_list",
		Recipients: []string{"session1", "session1", "session2"},
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Recipients) != 2 {
		t.Errorf("expected 2 recipients (deduplicated), got %d", len(resolved.Recipients))
	}
}

func TestResolveRecipients_CommaListNotFound(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:       "comma_list",
		Recipients: []string{"session1", "nonexistent", "session2"},
	}

	_, err := ResolveRecipients(spec, resolver)
	if err == nil {
		t.Fatal("expected error for nonexistent session, got nil")
	}

	if err.Error() != "recipient 'nonexistent' not found: session not found: nonexistent" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveRecipients_GlobPattern(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:       "glob",
		Recipients: []string{"research-*"},
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Recipients) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(resolved.Recipients))
	}

	// Should match research-1 and research-2
	matches := make(map[string]bool)
	for _, r := range resolved.Recipients {
		matches[r] = true
	}

	if !matches["research-1"] || !matches["research-2"] {
		t.Errorf("expected research-1 and research-2, got %v", resolved.Recipients)
	}
}

func TestResolveRecipients_GlobNoMatches(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:       "glob",
		Recipients: []string{"nonexistent-*"},
	}

	_, err := ResolveRecipients(spec, resolver)
	if err == nil {
		t.Fatal("expected error for no matches, got nil")
	}

	if err.Error() != "no sessions match pattern: nonexistent-*" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveRecipients_NilSpec(t *testing.T) {
	resolver := newMockResolver()
	_, err := ResolveRecipients(nil, resolver)
	if err == nil {
		t.Fatal("expected error for nil spec, got nil")
	}
}

func TestResolveRecipients_NilResolver(t *testing.T) {
	spec := &RecipientSpec{
		Type:       "direct",
		Recipients: []string{"session1"},
	}
	_, err := ResolveRecipients(spec, nil)
	if err == nil {
		t.Fatal("expected error for nil resolver, got nil")
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern  string
		name     string
		expected bool
	}{
		{"*research*", "research-1", true},
		{"*research*", "my-research-project", true},
		{"*research*", "session1", false},
		{"test-*", "test-session", true},
		{"test-*", "my-test", false},
		{"session?", "session1", true},
		{"session?", "session12", false},
		{"*", "anything", true},
	}

	for _, tt := range tests {
		result := matchGlob(tt.pattern, tt.name)
		if result != tt.expected {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, result, tt.expected)
		}
	}
}

func TestResolveGlob_SkipsArchived(t *testing.T) {
	resolver := newMockResolver()
	sessions, _ := resolver.ListAllSessions()

	// Pattern that would match archived-session
	matches := resolveGlob("archived-*", sessions)

	// Should not include archived sessions
	if len(matches) != 0 {
		t.Errorf("expected 0 matches (archived should be skipped), got %d: %v", len(matches), matches)
	}
}

func TestResolveGlob_WildcardAll(t *testing.T) {
	resolver := newMockResolver()
	sessions, _ := resolver.ListAllSessions()

	// Pattern that matches all
	matches := resolveGlob("*", sessions)

	// Should match all non-archived sessions (6 sessions)
	// session1, session2, session3, research-1, research-2, test-session
	if len(matches) != 6 {
		t.Errorf("expected 6 matches, got %d: %v", len(matches), matches)
	}
}

func TestParseRecipients_CommaListWithWhitespace(t *testing.T) {
	spec, err := ParseRecipients([]string{"session1, session2 , session3"}, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spec.Recipients) != 3 {
		t.Errorf("expected 3 recipients, got %d", len(spec.Recipients))
	}

	// Should trim whitespace
	expected := []string{"session1", "session2", "session3"}
	for i, exp := range expected {
		if spec.Recipients[i] != exp {
			t.Errorf("expected recipient[%d] '%s', got '%s'", i, exp, spec.Recipients[i])
		}
	}
}

func TestParseRecipients_EmptyCommaListEntry(t *testing.T) {
	spec, err := ParseRecipients([]string{"session1,,session2"}, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should skip empty entries
	if len(spec.Recipients) != 2 {
		t.Errorf("expected 2 recipients (empty skipped), got %d", len(spec.Recipients))
	}
}

func TestParseRecipients_AllFlag(t *testing.T) {
	spec, err := ParseRecipients([]string{}, "", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Type != "glob" {
		t.Errorf("expected type 'glob', got '%s'", spec.Type)
	}

	if len(spec.Recipients) != 1 || spec.Recipients[0] != "*" {
		t.Errorf("expected pattern '*', got %v", spec.Recipients)
	}

	if spec.Raw != "*" {
		t.Errorf("expected raw '*', got '%s'", spec.Raw)
	}
}

func TestParseRecipients_AllFlagIgnoresArgs(t *testing.T) {
	// --all should take precedence and ignore positional args
	spec, err := ParseRecipients([]string{"session1"}, "", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Type != "glob" || spec.Recipients[0] != "*" {
		t.Errorf("--all should override args, got type=%s recipients=%v", spec.Type, spec.Recipients)
	}
}

func TestResolveRecipients_AllFlag(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:       "glob",
		Recipients: []string{"*"},
		Raw:        "*",
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should match all 6 non-archived sessions
	// (existing test TestResolveGlob_WildcardAll validates this)
	if len(resolved.Recipients) != 6 {
		t.Errorf("expected 6 recipients, got %d", len(resolved.Recipients))
	}
}

func TestResolveRecipients_ExcludeSender(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:          "glob",
		Recipients:    []string{"*"},
		Raw:           "*",
		ExcludeSender: "session1",
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should match 5 non-archived sessions (6 minus sender)
	if len(resolved.Recipients) != 5 {
		t.Errorf("expected 5 recipients (sender excluded), got %d: %v", len(resolved.Recipients), resolved.Recipients)
	}

	// Verify sender is not in the list
	for _, r := range resolved.Recipients {
		if r == "session1" {
			t.Error("sender 'session1' should be excluded from recipients")
		}
	}
}

func TestResolveRecipients_ExcludeSender_CommaList(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:          "comma_list",
		Recipients:    []string{"session1", "session2", "session3"},
		ExcludeSender: "session2",
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved.Recipients) != 2 {
		t.Errorf("expected 2 recipients (sender excluded), got %d", len(resolved.Recipients))
	}

	for _, r := range resolved.Recipients {
		if r == "session2" {
			t.Error("sender 'session2' should be excluded from recipients")
		}
	}
}

func TestResolveRecipients_ExcludeSender_AllExcluded(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:          "direct",
		Recipients:    []string{"session1"},
		ExcludeSender: "session1",
	}

	_, err := ResolveRecipients(spec, resolver)
	if err == nil {
		t.Fatal("expected error when all recipients excluded, got nil")
	}

	if !strings.Contains(err.Error(), "excluding sender") {
		t.Errorf("expected error about excluding sender, got: %v", err)
	}
}

func TestResolveRecipients_ExcludeSender_NoExclude(t *testing.T) {
	resolver := newMockResolver()
	spec := &RecipientSpec{
		Type:          "glob",
		Recipients:    []string{"*"},
		Raw:           "*",
		ExcludeSender: "", // Empty = no exclusion (--include-self)
	}

	resolved, err := ResolveRecipients(spec, resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 6 non-archived sessions should be included
	if len(resolved.Recipients) != 6 {
		t.Errorf("expected 6 recipients (no exclusion), got %d", len(resolved.Recipients))
	}
}
