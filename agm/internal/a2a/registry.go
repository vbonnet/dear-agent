package a2a

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Registry manages A2A Agent Cards on disk at ~/.agm/a2a/cards/.
type Registry struct {
	cardsDir string
}

// NewRegistry creates a Registry rooted at the given cards directory.
// The directory is created if it does not exist.
func NewRegistry(cardsDir string) (*Registry, error) {
	if err := os.MkdirAll(cardsDir, 0o700); err != nil {
		return nil, fmt.Errorf("a2a: create cards directory: %w", err)
	}
	return &Registry{cardsDir: cardsDir}, nil
}

// DefaultCardsDir returns the default cards directory (~/.agm/a2a/cards).
func DefaultCardsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("a2a: get home directory: %w", err)
	}
	return filepath.Join(home, ".agm", "a2a", "cards"), nil
}

// UpdateCard generates an Agent Card from the manifest and writes it to disk.
// The card file is named {session-name}.json.
func (r *Registry) UpdateCard(m *manifest.Manifest) error {
	if m.Name == "" {
		return fmt.Errorf("a2a: manifest has no name")
	}

	card := GenerateCard(m)
	data, err := CardJSON(card)
	if err != nil {
		return err
	}

	filename := cardFilename(m.Name)
	path := filepath.Join(r.cardsDir, filename)

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("a2a: write card %s: %w", path, err)
	}
	return nil
}

// RemoveCard deletes the Agent Card file for the given session name.
func (r *Registry) RemoveCard(sessionName string) error {
	filename := cardFilename(sessionName)
	path := filepath.Join(r.cardsDir, filename)

	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("a2a: remove card %s: %w", path, err)
	}
	return nil
}

// GetCard reads and returns the Agent Card for the given session name.
func (r *Registry) GetCard(sessionName string) (*a2a.AgentCard, error) {
	filename := cardFilename(sessionName)
	path := filepath.Join(r.cardsDir, filename)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("a2a: no card for session %q", sessionName)
		}
		return nil, fmt.Errorf("a2a: read card %s: %w", path, err)
	}

	var card a2a.AgentCard
	if err := json.Unmarshal(data, &card); err != nil {
		return nil, fmt.Errorf("a2a: parse card %s: %w", path, err)
	}
	return &card, nil
}

// ListCards returns all Agent Cards in the registry.
func (r *Registry) ListCards() ([]a2a.AgentCard, error) {
	entries, err := os.ReadDir(r.cardsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("a2a: list cards directory: %w", err)
	}

	var cards []a2a.AgentCard
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(r.cardsDir, entry.Name()))
		if err != nil {
			continue // skip unreadable files
		}

		var card a2a.AgentCard
		if err := json.Unmarshal(data, &card); err != nil {
			continue // skip malformed files
		}
		cards = append(cards, card)
	}

	return cards, nil
}

// SyncFromManifests updates the card registry to match the given manifests.
// Cards for sessions not in the manifest list are removed.
// Archived sessions have their cards removed.
func (r *Registry) SyncFromManifests(manifests []*manifest.Manifest) error {
	// Build set of active session names
	activeNames := make(map[string]bool)
	for _, m := range manifests {
		if m.Lifecycle == manifest.LifecycleArchived {
			// Remove cards for archived sessions
			if err := r.RemoveCard(m.Name); err != nil {
				return err
			}
			continue
		}
		activeNames[m.Name] = true
		if err := r.UpdateCard(m); err != nil {
			return err
		}
	}

	// Remove orphan cards (sessions that no longer exist)
	entries, err := os.ReadDir(r.cardsDir)
	if err != nil {
		return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		if !activeNames[name] {
			_ = os.Remove(filepath.Join(r.cardsDir, entry.Name()))
		}
	}

	return nil
}

// cardFilename returns the filename for a session's card.
func cardFilename(sessionName string) string {
	return sessionName + ".json"
}
