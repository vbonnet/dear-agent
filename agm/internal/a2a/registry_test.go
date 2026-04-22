package a2a

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestNewRegistry_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	cardsDir := filepath.Join(dir, "nested", "cards")

	reg, err := NewRegistry(cardsDir)
	require.NoError(t, err)
	require.NotNil(t, reg)

	info, err := os.Stat(cardsDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRegistry_UpdateAndGetCard(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "cards"))
	require.NoError(t, err)

	m := &manifest.Manifest{
		Name:    "test-session",
		Harness: "claude-code",
		Context: manifest.Context{
			Purpose: "Test purpose",
			Tags:    []string{"go", "testing"},
		},
	}

	// Update card
	err = reg.UpdateCard(m)
	require.NoError(t, err)

	// Get card
	card, err := reg.GetCard("test-session")
	require.NoError(t, err)
	assert.Equal(t, "test-session", card.Name)
	assert.Equal(t, "Test purpose", card.Description)
}

func TestRegistry_UpdateCard_EmptyName(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "cards"))
	require.NoError(t, err)

	m := &manifest.Manifest{Harness: "claude-code"}
	err = reg.UpdateCard(m)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no name")
}

func TestRegistry_GetCard_NotFound(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "cards"))
	require.NoError(t, err)

	_, err = reg.GetCard("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no card")
}

func TestRegistry_RemoveCard(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "cards"))
	require.NoError(t, err)

	m := &manifest.Manifest{
		Name:    "to-remove",
		Harness: "claude-code",
	}

	// Create then remove
	require.NoError(t, reg.UpdateCard(m))
	_, err = reg.GetCard("to-remove")
	require.NoError(t, err)

	require.NoError(t, reg.RemoveCard("to-remove"))
	_, err = reg.GetCard("to-remove")
	assert.Error(t, err)
}

func TestRegistry_RemoveCard_Idempotent(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "cards"))
	require.NoError(t, err)

	// Removing nonexistent card should not error
	err = reg.RemoveCard("does-not-exist")
	assert.NoError(t, err)
}

func TestRegistry_ListCards(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "cards"))
	require.NoError(t, err)

	// Empty registry
	cards, err := reg.ListCards()
	require.NoError(t, err)
	assert.Empty(t, cards)

	// Add some cards
	for _, name := range []string{"session-a", "session-b", "session-c"} {
		require.NoError(t, reg.UpdateCard(&manifest.Manifest{
			Name:    name,
			Harness: "claude-code",
		}))
	}

	cards, err = reg.ListCards()
	require.NoError(t, err)
	assert.Len(t, cards, 3)

	names := make(map[string]bool)
	for _, c := range cards {
		names[c.Name] = true
	}
	assert.True(t, names["session-a"])
	assert.True(t, names["session-b"])
	assert.True(t, names["session-c"])
}

func TestRegistry_SyncFromManifests(t *testing.T) {
	reg, err := NewRegistry(filepath.Join(t.TempDir(), "cards"))
	require.NoError(t, err)

	// Start with some existing cards
	for _, name := range []string{"keep", "remove-me", "archive-me"} {
		require.NoError(t, reg.UpdateCard(&manifest.Manifest{
			Name:    name,
			Harness: "claude-code",
		}))
	}

	// Sync with new manifest list
	manifests := []*manifest.Manifest{
		{Name: "keep", Harness: "claude-code"},
		{Name: "new-session", Harness: "gemini-cli"},
		{Name: "archive-me", Harness: "claude-code", Lifecycle: manifest.LifecycleArchived},
	}

	err = reg.SyncFromManifests(manifests)
	require.NoError(t, err)

	cards, err := reg.ListCards()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, c := range cards {
		names[c.Name] = true
	}

	assert.True(t, names["keep"], "keep should remain")
	assert.True(t, names["new-session"], "new-session should be added")
	assert.False(t, names["remove-me"], "remove-me should be removed (orphan)")
	assert.False(t, names["archive-me"], "archive-me should be removed (archived)")
}

func TestRegistry_CardFileContents(t *testing.T) {
	cardsDir := filepath.Join(t.TempDir(), "cards")
	reg, err := NewRegistry(cardsDir)
	require.NoError(t, err)

	m := &manifest.Manifest{
		Name:    "file-test",
		Harness: "claude-code",
		Context: manifest.Context{Purpose: "Check file format"},
	}
	require.NoError(t, reg.UpdateCard(m))

	// Read raw file and verify it's valid JSON
	data, err := os.ReadFile(filepath.Join(cardsDir, "file-test.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"name": "file-test"`)
	assert.Contains(t, string(data), `"description": "Check file format"`)
}
