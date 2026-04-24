package metacontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Constitution represents the immutable constitution file "Root of Trust".
// The constitution can be either AGENTS.md (preferred) or CLAUDE.md (backward compatibility).
// Once loaded at service initialization, it cannot be modified at runtime.
// Any runtime edits to the constitution file will trigger validation errors.
type Constitution struct {
	Content  string // Raw constitution file content (AGENTS.md or CLAUDE.md)
	Hash     string // SHA-256 hash for integrity verification
	Path     string // Absolute path to the loaded constitution file
	LoadedAt int64  // Unix timestamp when loaded
}

// ConstitutionService manages the Constitution lifecycle.
type ConstitutionService struct {
	constitution *Constitution
	mu           sync.RWMutex
}

// NewConstitutionService creates a new ConstitutionService.
// It loads the constitution file (AGENTS.md or CLAUDE.md) from the working directory
// and computes its hash. Preference order: AGENTS.md > CLAUDE.md.
func NewConstitutionService(workingDir string) (*ConstitutionService, error) {
	constitution, err := loadConstitution(workingDir)
	if err != nil {
		return nil, err
	}

	return &ConstitutionService{
		constitution: constitution,
	}, nil
}

// GetConstitution returns the loaded Constitution (read-only).
func (cs *ConstitutionService) GetConstitution(ctx context.Context) (*Constitution, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.constitution == nil {
		return nil, fmt.Errorf("constitution not loaded")
	}

	return cs.constitution, nil
}

// ValidateIntegrity checks if the constitution file (AGENTS.md or CLAUDE.md) has been modified at runtime.
// Returns an error if the file has changed since initialization.
func (cs *ConstitutionService) ValidateIntegrity(ctx context.Context, workingDir string) error {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.constitution == nil {
		return fmt.Errorf("constitution not loaded")
	}

	// Read current file content using the same path that was loaded
	currentContent, err := os.ReadFile(cs.constitution.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("constitution file deleted at runtime (path: %s)", cs.constitution.Path)
		}
		return fmt.Errorf("failed to read constitution file for validation: %w", err)
	}

	// Compute current hash
	currentHash := computeHash(string(currentContent))

	// Compare with loaded hash
	if currentHash != cs.constitution.Hash {
		return fmt.Errorf("constitution file modified at runtime (path: %s, original hash: %s, current hash: %s). Runtime edits are forbidden. Use self-improvement PR workflow instead",
			cs.constitution.Path, cs.constitution.Hash[:8], currentHash[:8])
	}

	return nil
}

// loadConstitution reads AGENTS.md or CLAUDE.md and computes its hash.
// Preference order: AGENTS.md > CLAUDE.md
// This supports both the Agentic AI Foundation standard (AGENTS.md) and
// legacy Claude-specific projects (CLAUDE.md).
func loadConstitution(workingDir string) (*Constitution, error) {
	// Try AGENTS.md first (preferred - Agentic AI Foundation standard)
	agentsPath := filepath.Join(workingDir, "AGENTS.md")
	content, err := os.ReadFile(agentsPath)

	if err == nil {
		// AGENTS.md found and readable
		contentStr := string(content)
		hash := computeHash(contentStr)

		return &Constitution{
			Content:  contentStr,
			Hash:     hash,
			Path:     agentsPath,
			LoadedAt: getCurrentTimestamp(),
		}, nil
	}

	// If AGENTS.md doesn't exist, try CLAUDE.md (backward compatibility)
	if os.IsNotExist(err) {
		claudePath := filepath.Join(workingDir, "CLAUDE.md")
		content, err = os.ReadFile(claudePath)

		if err == nil {
			// CLAUDE.md found and readable
			contentStr := string(content)
			hash := computeHash(contentStr)

			return &Constitution{
				Content:  contentStr,
				Hash:     hash,
				Path:     claudePath,
				LoadedAt: getCurrentTimestamp(),
			}, nil
		}

		if os.IsNotExist(err) {
			return nil, fmt.Errorf("neither AGENTS.md nor CLAUDE.md found in %s (Constitution required)", workingDir)
		}

		return nil, fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	// AGENTS.md exists but couldn't be read (permissions, etc.)
	return nil, fmt.Errorf("failed to read AGENTS.md: %w", err)
}

// computeHash calculates SHA-256 hash of content.
func computeHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// getCurrentTimestamp returns current Unix timestamp.
func getCurrentTimestamp() int64 {
	return time.Now().Unix()
}
