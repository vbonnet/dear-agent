// Package sandbox provides isolation for wayfinder projects
package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Manager handles sandbox lifecycle operations
type Manager struct {
	baseDir string
	git     *GitWorktreeManager
}

// Sandbox represents an isolated wayfinder project environment
type Sandbox struct {
	ID            string    `json:"sandbox_id"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	LastUsedAt    time.Time `json:"last_used_at"`
	WorktreePath  string    `json:"worktree_path,omitempty"`
	GitRepository string    `json:"git_repository,omitempty"`
}

// NewManager creates a new sandbox manager
func NewManager(baseDir string) *Manager {
	if baseDir == "" {
		// Try workspace detection first (uses WORKSPACE env var or auto-detects)
		workspaceDir := GetWorkspaceProjectsDir("")
		baseDir = filepath.Join(workspaceDir, "sandboxes")
	}
	return &Manager{
		baseDir: baseDir,
		git:     NewGitWorktreeManager(),
	}
}

// CreateSandbox creates a new isolated sandbox with atomic rollback
func (m *Manager) CreateSandbox(name string) (*Sandbox, error) {
	// Generate unique ID
	id := uuid.New().String()

	// Prepare sandbox
	sandbox := &Sandbox{
		ID:         id,
		Name:       name,
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
	}

	sandboxPath := filepath.Join(m.baseDir, id)

	// Cleanup tracking for atomic operations
	var cleanupFuncs []func() error
	defer func() {
		// Only run cleanup on error
		if len(cleanupFuncs) > 0 {
			for i := len(cleanupFuncs) - 1; i >= 0; i-- {
				cleanupFuncs[i]()
			}
		}
	}()

	// Step 1: Create sandbox directory
	if err := os.MkdirAll(sandboxPath, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create sandbox directory: %w", err)
	}
	cleanupFuncs = append(cleanupFuncs, func() error {
		return os.RemoveAll(sandboxPath)
	})

	// Step 2: Create subdirectories
	for _, subdir := range []string{"sessions", "costs", "temp"} {
		if err := os.MkdirAll(filepath.Join(sandboxPath, subdir), 0755); err != nil {
			return nil, fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Step 3: Create git worktree (if in git repository)
	cwd, err := os.Getwd()
	if err == nil && m.git.IsGitRepository(cwd) {
		repoRoot, err := m.git.GetRepositoryRoot(cwd)
		if err == nil {
			worktreePath, err := m.git.CreateWorktree(id, repoRoot, "")
			if err != nil {
				return nil, fmt.Errorf("failed to create git worktree: %w", err)
			}
			sandbox.WorktreePath = worktreePath
			sandbox.GitRepository = repoRoot
			cleanupFuncs = append(cleanupFuncs, func() error {
				return m.git.RemoveWorktree(id, repoRoot)
			})
		}
	}

	// Step 4: Write metadata file
	metadataPath := filepath.Join(sandboxPath, ".wayfinder-project")
	if err := m.writeMetadata(metadataPath, sandbox); err != nil {
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	// Success: clear cleanup functions
	cleanupFuncs = nil

	return sandbox, nil
}

// GetActiveSandbox detects sandbox from current working directory
func (m *Manager) GetActiveSandbox() (*Sandbox, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Walk upward from CWD looking for .wayfinder-project marker
	dir := cwd
	for {
		metadataPath := filepath.Join(dir, ".wayfinder-project")
		if _, err := os.Stat(metadataPath); err == nil {
			// Found marker file, parse metadata
			return m.readMetadata(metadataPath)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	// No sandbox detected
	return nil, nil
}

// ListSandboxes returns all sandboxes
func (m *Manager) ListSandboxes() ([]*Sandbox, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Sandbox{}, nil
		}
		return nil, err
	}

	var sandboxes []*Sandbox
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadataPath := filepath.Join(m.baseDir, entry.Name(), ".wayfinder-project")
		sandbox, err := m.readMetadata(metadataPath)
		if err != nil {
			// Skip invalid sandbox
			continue
		}

		sandboxes = append(sandboxes, sandbox)
	}

	return sandboxes, nil
}

// CleanupSandbox removes sandbox and associated resources
func (m *Manager) CleanupSandbox(nameOrID string) error {
	// Find sandbox by name or ID
	sandboxes, err := m.ListSandboxes()
	if err != nil {
		return err
	}

	var target *Sandbox
	for _, s := range sandboxes {
		if s.ID == nameOrID || s.Name == nameOrID {
			target = s
			break
		}
	}

	if target == nil {
		return fmt.Errorf("sandbox not found: %s", nameOrID)
	}

	// Remove git worktree if exists
	if target.WorktreePath != "" && target.GitRepository != "" {
		if err := m.git.RemoveWorktree(target.ID, target.GitRepository); err != nil {
			// Log warning but continue with directory removal
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree: %v\n", err)
		}
	}

	// Remove sandbox directory
	sandboxPath := filepath.Join(m.baseDir, target.ID)
	return os.RemoveAll(sandboxPath)
}

// writeMetadata writes sandbox metadata to file
func (m *Manager) writeMetadata(path string, sandbox *Sandbox) error {
	data, err := json.MarshalIndent(sandbox, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// readMetadata reads sandbox metadata from file
func (m *Manager) readMetadata(path string) (*Sandbox, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sandbox Sandbox
	if err := json.Unmarshal(data, &sandbox); err != nil {
		return nil, err
	}

	return &sandbox, nil
}
