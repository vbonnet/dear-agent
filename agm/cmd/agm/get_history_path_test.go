package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type fakeHistoryPathSessionStore struct {
	resolved   *manifest.Manifest
	resolveErr error
	listed     []*manifest.Manifest
	listErr    error
	listCalls  int
}

func (s *fakeHistoryPathSessionStore) ResolveIdentifier(string) (*manifest.Manifest, error) {
	return s.resolved, s.resolveErr
}

func (s *fakeHistoryPathSessionStore) ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error) {
	s.listCalls++
	return s.listed, s.listErr
}

func TestResolveNamedHistoryLocation_AgyUsesManifestConversationID(t *testing.T) {
	conversationID := "117ff898-a964-4a9f-b460-1be4a8a49b17"
	m := &manifest.Manifest{
		Name: "agy-history", Harness: "antigravity",
		Agy: &manifest.Agy{ConversationID: conversationID},
	}
	store := &fakeHistoryPathSessionStore{resolved: m}

	resolved, identifier, err := resolveNamedHistoryLocation("agy-history", store)
	if err != nil {
		t.Fatalf("resolveNamedHistoryLocation: %v", err)
	}
	if resolved != m || identifier != conversationID {
		t.Fatalf("resolved manifest/id = %p/%q, want %p/%q", resolved, identifier, m, conversationID)
	}
	if store.listCalls != 0 {
		t.Fatalf("AGY resolution entered Claude manifest discovery %d time(s)", store.listCalls)
	}
}

func TestResolveNamedHistoryLocation_AgyRequiresConversationID(t *testing.T) {
	store := &fakeHistoryPathSessionStore{resolved: &manifest.Manifest{Name: "agy-history", Harness: "agy"}}

	_, _, err := resolveNamedHistoryLocation("agy-history", store)
	if err == nil || !strings.Contains(err.Error(), "no native conversation ID") {
		t.Fatalf("error = %v, want missing AGY conversation ID", err)
	}
	if store.listCalls != 0 {
		t.Fatalf("missing AGY ID entered Claude manifest discovery %d time(s)", store.listCalls)
	}
}

func TestResolveNamedHistoryLocation_PiUsesManifestNativeID(t *testing.T) {
	m := &manifest.Manifest{
		Name: "pi-history", Harness: "pi-cli",
		Pi: &manifest.Pi{SessionID: "pi-native-history", SessionDir: "/tmp/pi-sessions"},
	}
	store := &fakeHistoryPathSessionStore{resolved: m}

	resolved, identifier, err := resolveNamedHistoryLocation("pi-history", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != m || identifier != "pi-native-history" || store.listCalls != 0 {
		t.Fatalf("resolved manifest/id/listCalls = %p/%q/%d", resolved, identifier, store.listCalls)
	}
}

func TestResolveNamedHistoryLocation_PiRequiresCompleteIdentity(t *testing.T) {
	store := &fakeHistoryPathSessionStore{resolved: &manifest.Manifest{Name: "pi-history", Harness: "pi-cli", Pi: &manifest.Pi{SessionID: "pi-native-history"}}}
	_, _, err := resolveNamedHistoryLocation("pi-history", store)
	if err == nil || !strings.Contains(err.Error(), "incomplete native identity") {
		t.Fatalf("error = %v, want incomplete Pi identity", err)
	}
	if store.listCalls != 0 {
		t.Fatalf("missing Pi identity entered Claude discovery %d time(s)", store.listCalls)
	}
}

func TestResolveNamedHistoryLocation_PropagatesManifestResolutionFailure(t *testing.T) {
	wantErr := errors.New("fixture storage unavailable")
	store := &fakeHistoryPathSessionStore{resolveErr: wantErr}

	_, _, err := resolveNamedHistoryLocation("missing", store)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestResolveNamedHistoryLocation_RejectsNilResolvedManifest(t *testing.T) {
	store := &fakeHistoryPathSessionStore{}

	_, _, err := resolveNamedHistoryLocation("missing", store)
	if err == nil || !strings.Contains(err.Error(), `session "missing" not found`) {
		t.Fatalf("error = %v, want missing session", err)
	}
	if store.listCalls != 0 {
		t.Fatalf("nil resolved manifest entered fallback discovery %d time(s)", store.listCalls)
	}
}

func TestResolveNamedHistoryLocation_SkipsNilListedManifests(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resolved := &manifest.Manifest{Name: "claude-history", Harness: "claude-code"}
	matched := &manifest.Manifest{
		Name: "claude-history", Tmux: manifest.Tmux{SessionName: "claude-history"},
		Claude: manifest.Claude{UUID: "fixture-claude-uuid"},
	}
	store := &fakeHistoryPathSessionStore{
		resolved: resolved,
		listed:   []*manifest.Manifest{nil, matched},
	}

	got, identifier, err := resolveNamedHistoryLocation("claude-history", store)
	if err != nil {
		t.Fatalf("resolveNamedHistoryLocation: %v", err)
	}
	if got != resolved || identifier != "fixture-claude-uuid" {
		t.Fatalf("resolved manifest/id = %p/%q, want %p/fixture-claude-uuid", got, identifier, resolved)
	}
}
