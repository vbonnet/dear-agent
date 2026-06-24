package freshness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBinaryVCSRevision_NotABinary(t *testing.T) {
	// A non-binary file should cause "go version -m" to fail.
	tmp := t.TempDir()
	notBin := filepath.Join(tmp, "notabin")
	if err := os.WriteFile(notBin, []byte("not a Go binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ReadBinaryVCSRevision(notBin)
	if err == nil {
		t.Error("expected error for non-binary file")
	}
}

func TestReadBinaryVCSRevision_MissingBinary(t *testing.T) {
	_, err := ReadBinaryVCSRevision("/tmp/nonexistent-token-refresher-xyz")
	if err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestFindDearAgentRepoPath_EnvOverride(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEAR_AGENT_SOURCE_DIR", tmp)
	path, err := FindDearAgentRepoPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != tmp {
		t.Errorf("expected %s, got %s", tmp, path)
	}
}

func TestFindDearAgentRepoPath_InvalidEnv(t *testing.T) {
	t.Setenv("DEAR_AGENT_SOURCE_DIR", "/tmp/nonexistent-dear-agent-xyz-123")
	_, err := FindDearAgentRepoPath()
	if err == nil {
		t.Error("expected error for invalid DEAR_AGENT_SOURCE_DIR")
	}
}

func TestReadBinaryVCSRevision_RealBinary(t *testing.T) {
	// Use the actual installed token-refresher if present; skip otherwise.
	home, _ := os.UserHomeDir()
	bin := filepath.Join(home, "go", "bin", "token-refresher")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("token-refresher not installed — skipping live binary test")
	}
	rev, err := ReadBinaryVCSRevision(bin)
	if err != nil {
		t.Fatalf("unexpected error reading real binary: %v", err)
	}
	// rev may be "" (no vcs stamps) or a short sha — both are valid.
	t.Logf("token-refresher vcs.revision = %q", rev)
}
