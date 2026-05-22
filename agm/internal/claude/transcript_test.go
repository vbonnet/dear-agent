package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTranscript(t *testing.T, home, slug, uuid string, lines []string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, uuid+".jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

func TestFindTranscriptCwd(t *testing.T) {
	home := t.TempDir()
	uuid := "61163f27-a35e-4370-be49-f8456401a872"

	// Leading summary/meta lines have no cwd; a later message line does.
	writeTranscript(t, home, "-Users-vbonnet", uuid, []string{
		`{"type":"summary","leafUuid":"abc","sessionId":"` + uuid + `"}`,
		`{"type":"user","sessionId":"` + uuid + `","cwd":"/Users/vbonnet"}`,
	})

	got, err := FindTranscriptCwd(home, uuid)
	if err != nil {
		t.Fatalf("FindTranscriptCwd error: %v", err)
	}
	if got != "/Users/vbonnet" {
		t.Errorf("cwd = %q, want %q", got, "/Users/vbonnet")
	}
}

func TestFindTranscriptCwd_NotFound(t *testing.T) {
	home := t.TempDir()
	if _, err := FindTranscriptCwd(home, "does-not-exist"); err == nil {
		t.Fatal("expected error for missing transcript, got nil")
	}
}

func TestFindTranscriptCwd_EmptyUUID(t *testing.T) {
	if _, err := FindTranscriptCwd(t.TempDir(), ""); err == nil {
		t.Fatal("expected error for empty UUID, got nil")
	}
}

func TestFindTranscriptCwd_NoCwdEntry(t *testing.T) {
	home := t.TempDir()
	uuid := "11111111-2222-3333-4444-555555555555"
	writeTranscript(t, home, "-Users-vbonnet", uuid, []string{
		`{"type":"summary","leafUuid":"abc"}`,
		`{"type":"meta","permissionMode":"default"}`,
	})
	if _, err := FindTranscriptCwd(home, uuid); err == nil {
		t.Fatal("expected error when no cwd entry present, got nil")
	}
}
