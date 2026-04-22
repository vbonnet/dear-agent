package beads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractMetadata(t *testing.T) {
	linker := &Linker{channelsDir: "/tmp", activeDir: "/tmp/active"}

	content := `# A2A Channel: test

---
**Created**: 2024-01-01
**Topic**: Test Topic
**Participants**: agent-1, agent-2
---

Some body content
`

	metadata := linker.ExtractMetadata(content)

	if metadata["Created"] != "2024-01-01" {
		t.Errorf("expected Created=2024-01-01, got %s", metadata["Created"])
	}
	if metadata["Topic"] != "Test Topic" {
		t.Errorf("expected Topic=Test Topic, got %s", metadata["Topic"])
	}
	if metadata["Participants"] != "agent-1, agent-2" {
		t.Errorf("expected Participants=agent-1, agent-2, got %s", metadata["Participants"])
	}
}

func TestExtractMetadataEmpty(t *testing.T) {
	linker := &Linker{channelsDir: "/tmp", activeDir: "/tmp/active"}

	metadata := linker.ExtractMetadata("no frontmatter here")
	if len(metadata) != 0 {
		t.Errorf("expected empty metadata, got %v", metadata)
	}
}

func TestExtractMetadataMinimalHeader(t *testing.T) {
	linker := &Linker{channelsDir: "/tmp", activeDir: "/tmp/active"}

	content := `---
**Key**: Value
---`

	metadata := linker.ExtractMetadata(content)
	if metadata["Key"] != "Value" {
		t.Errorf("expected Key=Value, got %s", metadata["Key"])
	}
}

func TestUpdateMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	channelFile := filepath.Join(tmpDir, "test-channel.md")

	originalContent := `# A2A Channel

---
**Created**: 2024-01-01
**Topic**: Original Topic
---

Body content here.
`
	if err := os.WriteFile(channelFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	linker := &Linker{channelsDir: tmpDir, activeDir: tmpDir}

	newMetadata := map[string]string{
		"Created": "2024-01-01",
		"Topic":   "Updated Topic",
		"Bead-ID": "bead-123",
	}

	if err := linker.UpdateMetadata(channelFile, newMetadata); err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	content, err := os.ReadFile(channelFile)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}

	contentStr := string(content)
	updated := linker.ExtractMetadata(contentStr)
	if updated["Bead-ID"] != "bead-123" {
		t.Errorf("expected Bead-ID=bead-123, got %s", updated["Bead-ID"])
	}
}

func TestGetLinkedBeadNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	activeDir := filepath.Join(tmpDir, "active")
	if err := os.MkdirAll(activeDir, 0755); err != nil {
		t.Fatalf("create active dir: %v", err)
	}

	linker := NewLinker(tmpDir)

	_, err := linker.GetLinkedBead("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent channel")
	}
}

func TestGetLinkedBeadNoLink(t *testing.T) {
	tmpDir := t.TempDir()
	activeDir := filepath.Join(tmpDir, "active")
	if err := os.MkdirAll(activeDir, 0755); err != nil {
		t.Fatalf("create active dir: %v", err)
	}

	channelFile := filepath.Join(activeDir, "test-channel.md")
	content := `---
**Created**: 2024-01-01
**Topic**: Test
---

Body
`
	if err := os.WriteFile(channelFile, []byte(content), 0644); err != nil {
		t.Fatalf("write channel: %v", err)
	}

	linker := NewLinker(tmpDir)

	beadID, err := linker.GetLinkedBead("test-channel")
	if err != nil {
		t.Fatalf("GetLinkedBead failed: %v", err)
	}
	if beadID != "" {
		t.Errorf("expected empty bead ID, got %s", beadID)
	}
}

func TestNewLinkerWithCustomDir(t *testing.T) {
	linker := NewLinker("/custom/channels")
	if linker.channelsDir != "/custom/channels" {
		t.Errorf("expected channelsDir=/custom/channels, got %s", linker.channelsDir)
	}
	if linker.activeDir != "/custom/channels/active" {
		t.Errorf("expected activeDir=/custom/channels/active, got %s", linker.activeDir)
	}
}

func TestValidateBeadExistsNonExistent(t *testing.T) {
	linker := &Linker{channelsDir: "/tmp", activeDir: "/tmp/active"}

	exists, path := linker.ValidateBeadExists("nonexistent-bead-12345")
	if exists {
		t.Errorf("expected bead to not exist, got path: %s", path)
	}
	if path != "" {
		t.Errorf("expected empty path, got %s", path)
	}
}
