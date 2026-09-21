package main

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestPickerSessions_DuplicateNameStatusesStayWithID(t *testing.T) {
	updatedAt := time.Now().Add(-time.Hour)
	manifests := []*manifest.Manifest{
		{
			SessionID: "live-id", Name: "shared", UpdatedAt: updatedAt,
			Tmux: manifest.Tmux{SessionName: "live-tmux"},
		},
		{
			SessionID: "stopped-id", Name: "shared", UpdatedAt: updatedAt,
			Tmux: manifest.Tmux{SessionName: "stopped-tmux"},
		},
		{
			SessionID: "archived-id", Name: "shared", UpdatedAt: updatedAt,
			Lifecycle: manifest.LifecycleArchived,
		},
	}
	tmux := session.NewMockTmux()
	tmux.Sessions["live-tmux"] = true
	rows := pickerSessions(manifests, tmux)

	wantStatuses := []string{"active", "stopped", "archived"}
	for i, row := range rows {
		if row.Manifest != manifests[i] {
			t.Errorf("row %d lost its exact manifest", i)
		}
		if row.SessionID != manifests[i].SessionID || row.Status != wantStatuses[i] {
			t.Errorf("row %d: ID/status = %q/%q, want %q/%q", i,
				row.SessionID, row.Status, manifests[i].SessionID, wantStatuses[i])
		}
		if row.UpdatedAt != updatedAt {
			t.Errorf("row %d lost its update time", i)
		}
	}
}
