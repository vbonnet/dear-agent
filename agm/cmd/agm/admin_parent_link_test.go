package main

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type recordingSessionParentLinker struct {
	sessionID        string
	observedRevision string
	parentSessionID  string
	inheritedName    *string
	err              error
}

func (l *recordingSessionParentLinker) LinkSessionParent(_ context.Context, sessionID, observedRevision, parentSessionID string, inheritedName *string) error {
	l.sessionID = sessionID
	l.observedRevision = observedRevision
	l.parentSessionID = parentSessionID
	l.inheritedName = inheritedName
	return l.err
}

func TestPersistSessionParentLinkUsesObservedIdentityRevision(t *testing.T) {
	child := &manifest.Manifest{
		SessionID: "child-session",
		Tmux: manifest.Tmux{
			SessionRevision: "observed-identity-revision",
		},
	}
	inheritedName := "planning-session-exec"
	linker := &recordingSessionParentLinker{}

	if err := persistSessionParentLink(t.Context(), linker, child, "parent-session", &inheritedName); err != nil {
		t.Fatalf("persistSessionParentLink() error: %v", err)
	}
	if linker.sessionID != child.SessionID || linker.observedRevision != child.Tmux.SessionRevision || linker.parentSessionID != "parent-session" {
		t.Fatalf("link request = (session=%q revision=%q parent=%q), want (%q, %q, %q)", linker.sessionID, linker.observedRevision, linker.parentSessionID, child.SessionID, child.Tmux.SessionRevision, "parent-session")
	}
	if linker.inheritedName == nil || *linker.inheritedName != inheritedName {
		t.Fatalf("inherited name = %#v, want %q", linker.inheritedName, inheritedName)
	}

	conflict := errors.New("session identity changed concurrently")
	linker.err = conflict
	if err := persistSessionParentLink(t.Context(), linker, child, "parent-session", nil); !errors.Is(err, conflict) {
		t.Fatalf("persistSessionParentLink() conflict = %v, want %v", err, conflict)
	}
}
