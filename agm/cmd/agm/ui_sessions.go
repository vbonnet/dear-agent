package main

import (
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

func uiSessionsFromManifests(manifests []*manifest.Manifest, tmux session.TmuxInterface) []*ui.Session {
	statuses := session.ComputeStatusBatchByID(manifests, tmux)
	uiSessions := make([]*ui.Session, len(manifests))
	for i, m := range manifests {
		uiSessions[i] = &ui.Session{
			Manifest:  m,
			Status:    statuses[m.SessionID],
			UpdatedAt: m.UpdatedAt,
		}
	}
	return uiSessions
}
