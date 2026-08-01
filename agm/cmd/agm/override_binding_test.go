package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/pkg/override"
)

func TestResolveOverrideApprovalCodexSourceBindsCommittedBytes(t *testing.T) {
	if _, err := resolveOverrideApprovalCodexSource(
		t.Context(), override.KindCodexHookTrust, "",
	); err == nil {
		t.Fatal("generic Codex hook-trust approval source was accepted")
	}
	if _, err := resolveOverrideApprovalCodexSource(
		t.Context(), override.KindAdmissionBrake, "/tmp/not-applicable",
	); err == nil {
		t.Fatal("Codex source was accepted for a non-hook override")
	}

	repo := gittest.NewRepo(t)
	manifest := filepath.Join(repo, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("{\"hooks\":{}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, repo, "add", ".codex/hooks.json")
	gittest.Run(t, repo, "commit", "-m", "reviewed hooks")

	source, err := resolveOverrideApprovalCodexSource(
		t.Context(), override.KindCodexHookTrust, repo,
	)
	if err != nil {
		t.Fatalf("resolve exact Codex hook source: %v", err)
	}
	if source.Repository == "" || len(source.Commit) != 40 || len(source.Digest) != 64 {
		t.Fatalf("resolved source = %#v, want canonical repository, commit, and digest", source)
	}
	subject, err := source.Subject()
	if err != nil {
		t.Fatalf("derive bound subject: %v", err)
	}

	if err := os.WriteFile(manifest, []byte("{\"hooks\":{\"PreToolUse\":[]}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterWorkingTreeEdit, err := resolveOverrideApprovalCodexSource(
		t.Context(), override.KindCodexHookTrust, repo,
	)
	if err != nil {
		t.Fatalf("resolve source after working-tree edit: %v", err)
	}
	afterSubject, err := afterWorkingTreeEdit.Subject()
	if err != nil {
		t.Fatal(err)
	}
	if afterSubject != subject {
		t.Fatalf("mutable working-tree edit changed approval subject: before=%q after=%q", subject, afterSubject)
	}
}
