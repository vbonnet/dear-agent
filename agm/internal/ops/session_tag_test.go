package ops

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func makeTagTestSession(id, name string, tags []string) *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     id,
		Name:          name,
		Harness:       "claude-code",
		Context:       manifest.Context{Tags: tags},
		Tmux:          manifest.Tmux{SessionName: name},
	}
}

func newTagOpCtx(t *testing.T) (*OpContext, *dolt.MockAdapter) {
	t.Helper()
	storage := dolt.NewMockAdapter()
	tmux := session.NewMockTmux()
	return &OpContext{Storage: storage, Tmux: tmux}, storage
}

func TestTagSession_AddTag(t *testing.T) {
	ctx, storage := newTagOpCtx(t)
	m := makeTagTestSession("sid-1", "worker-1", []string{"role:worker"})
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := TagSession(ctx, &TagSessionRequest{
		Identifier: "worker-1",
		Add:        "cap:claude-code",
	})
	if err != nil {
		t.Fatalf("TagSession failed: %v", err)
	}
	if res.Action != "added" {
		t.Errorf("action = %q, want %q", res.Action, "added")
	}
	if res.Tag != "cap:claude-code" {
		t.Errorf("tag = %q, want %q", res.Tag, "cap:claude-code")
	}

	// Verify persisted
	got, err := storage.GetSession("sid-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	found := false
	for _, tg := range got.Context.Tags {
		if tg == "cap:claude-code" {
			found = true
		}
	}
	if !found {
		t.Errorf("tag 'cap:claude-code' not persisted: %v", got.Context.Tags)
	}
}

func TestTagSession_AddDuplicate(t *testing.T) {
	ctx, storage := newTagOpCtx(t)
	m := makeTagTestSession("sid-2", "worker-2", []string{"role:worker"})
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := TagSession(ctx, &TagSessionRequest{
		Identifier: "worker-2",
		Add:        "role:worker", // already present
	})
	if err != nil {
		t.Fatalf("TagSession failed on duplicate: %v", err)
	}
	if res.Action != "noop" {
		t.Errorf("action = %q, want %q", res.Action, "noop")
	}

	// Verify only one copy
	got, _ := storage.GetSession("sid-2")
	count := 0
	for _, tg := range got.Context.Tags {
		if tg == "role:worker" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 copy of tag, got %d", count)
	}
}

func TestTagSession_RemoveTag(t *testing.T) {
	ctx, storage := newTagOpCtx(t)
	m := makeTagTestSession("sid-3", "worker-3", []string{"role:worker", "cap:web-search"})
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := TagSession(ctx, &TagSessionRequest{
		Identifier: "worker-3",
		Remove:     "cap:web-search",
	})
	if err != nil {
		t.Fatalf("TagSession failed: %v", err)
	}
	if res.Action != "removed" {
		t.Errorf("action = %q, want %q", res.Action, "removed")
	}

	got, _ := storage.GetSession("sid-3")
	for _, tg := range got.Context.Tags {
		if tg == "cap:web-search" {
			t.Error("removed tag still present")
		}
	}
	if len(got.Context.Tags) != 1 || got.Context.Tags[0] != "role:worker" {
		t.Errorf("tags = %v, want [role:worker]", got.Context.Tags)
	}
}

func TestTagSession_RemoveNonExistent(t *testing.T) {
	ctx, storage := newTagOpCtx(t)
	m := makeTagTestSession("sid-4", "worker-4", []string{"role:worker"})
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := TagSession(ctx, &TagSessionRequest{
		Identifier: "worker-4",
		Remove:     "cap:nonexistent",
	})
	if err == nil {
		t.Error("expected error removing non-existent tag")
	}
}

func TestTagSession_NoTagSpecified(t *testing.T) {
	ctx, storage := newTagOpCtx(t)
	m := makeTagTestSession("sid-5", "worker-5", nil)
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := TagSession(ctx, &TagSessionRequest{Identifier: "worker-5"})
	if err == nil {
		t.Error("expected error when no tag specified")
	}
}

func TestTagSession_BothAddAndRemove(t *testing.T) {
	ctx, storage := newTagOpCtx(t)
	m := makeTagTestSession("sid-6", "worker-6", []string{"role:worker"})
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := TagSession(ctx, &TagSessionRequest{
		Identifier: "worker-6",
		Add:        "cap:web-search",
		Remove:     "role:worker",
	})
	if err == nil {
		t.Error("expected error when both add and remove are set")
	}
}

func TestTagSession_SessionNotFound(t *testing.T) {
	ctx, _ := newTagOpCtx(t)

	_, err := TagSession(ctx, &TagSessionRequest{
		Identifier: "nonexistent-session",
		Add:        "role:worker",
	})
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}
