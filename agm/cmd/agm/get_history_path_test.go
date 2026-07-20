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

func TestResolveNamedHistoryLocation_PropagatesManifestResolutionFailure(t *testing.T) {
	wantErr := errors.New("fixture storage unavailable")
	store := &fakeHistoryPathSessionStore{resolveErr: wantErr}

	_, _, err := resolveNamedHistoryLocation("missing", store)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
