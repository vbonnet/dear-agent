package main

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/codexcontrol"
)

func TestCodexThreadManifest(t *testing.T) {
	thread := codexcontrol.Thread{
		ID:        "thr_123456789",
		Name:      "Codex Remote Work",
		CWD:       "/tmp/project",
		Path:      "/Users/me/.codex/sessions/rollout.jsonl",
		CreatedAt: 1783010000,
		UpdatedAt: 1783010500,
	}

	m := codexThreadManifest(thread, codexThreadAGMName(thread), "oss")
	if m.Harness != "codex-cli" {
		t.Fatalf("Harness = %q, want codex-cli", m.Harness)
	}
	if m.Codex == nil || m.Codex.SessionID != thread.ID {
		t.Fatalf("Codex metadata = %#v, want session id %q", m.Codex, thread.ID)
	}
	if m.WorkingDirectory != thread.CWD || m.Context.Project != thread.CWD {
		t.Fatalf("cwd not preserved: working=%q project=%q", m.WorkingDirectory, m.Context.Project)
	}
	if m.Tmux.SessionName != "Codex-Remote-Work" {
		t.Fatalf("tmux name = %q, want sanitized name", m.Tmux.SessionName)
	}
	if len(m.Context.Tags) != 1 || m.Context.Tags[0] != "source:codex-reconcile" {
		t.Fatalf("tags = %#v, want source:codex-reconcile", m.Context.Tags)
	}
}
