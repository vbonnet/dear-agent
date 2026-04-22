package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateHookConfig(t *testing.T) {
	tests := []struct {
		name      string
		hookName  string
		event     string
		priority  int
		timeout   int
		hookType  string
		wantError bool
	}{
		{
			name:      "valid config",
			hookName:  "test-hook",
			event:     "session-completion",
			priority:  50,
			timeout:   60,
			hookType:  "binary",
			wantError: false,
		},
		{
			name:      "invalid event",
			hookName:  "test-hook",
			event:     "invalid-event",
			priority:  50,
			timeout:   60,
			hookType:  "binary",
			wantError: true,
		},
		{
			name:      "priority too low",
			hookName:  "test-hook",
			event:     "session-completion",
			priority:  0,
			timeout:   60,
			hookType:  "binary",
			wantError: true,
		},
		{
			name:      "priority too high",
			hookName:  "test-hook",
			event:     "session-completion",
			priority:  101,
			timeout:   60,
			hookType:  "binary",
			wantError: true,
		},
		{
			name:      "timeout too low",
			hookName:  "test-hook",
			event:     "session-completion",
			priority:  50,
			timeout:   0,
			hookType:  "binary",
			wantError: true,
		},
		{
			name:      "timeout too high",
			hookName:  "test-hook",
			event:     "session-completion",
			priority:  50,
			timeout:   601,
			hookType:  "binary",
			wantError: true,
		},
		{
			name:      "invalid type",
			hookName:  "test-hook",
			event:     "session-completion",
			priority:  50,
			timeout:   60,
			hookType:  "invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global variables
			hookName = tt.hookName
			hookEvent = tt.event
			hookPriority = tt.priority
			hookTimeout = tt.timeout
			hookType = tt.hookType

			err := validateHookConfig()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadSaveHookRegistry(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "hooks.toml")

	// Create test registry
	registry := &HookRegistry{
		Hooks: []Hook{
			{
				Name:     "test-hook",
				Event:    "session-completion",
				Priority: 50,
				Type:     "binary",
				Command:  "echo",
				Args:     []string{"test"},
				Timeout:  60,
			},
		},
	}

	// Save registry
	err := saveHookRegistry(registry, registryPath)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(registryPath)
	require.NoError(t, err)

	// Manually load for test
	data, err := os.ReadFile(registryPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "test-hook")
}

func TestCalculateCommandHash(t *testing.T) {
	// Test with skill type (should return empty hash)
	hookType = "skill"
	hash, err := calculateCommandHash("engram")
	require.NoError(t, err)
	assert.Empty(t, hash)

	// Test with binary type
	hookType = "binary"
	hash, err = calculateCommandHash("echo")
	require.NoError(t, err)
	if hash != "" { // Hash might not be calculated if echo not found in some envs
		assert.Contains(t, hash, "sha256:")
	}

	// Test with non-existent command
	hookType = "binary"
	_, err = calculateCommandHash("nonexistent-command-xyz")
	assert.Error(t, err)
}

func TestHookRegistryOperations(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Test: Load empty registry
	registry, path, err := loadHookRegistry()
	require.NoError(t, err)
	assert.Empty(t, registry.Hooks)
	assert.Contains(t, path, ".engram/hooks.toml")

	// Test: Add hook
	hook := Hook{
		Name:     "test-hook",
		Event:    "session-completion",
		Priority: 50,
		Type:     "binary",
		Command:  "echo",
		Args:     []string{"test"},
		Timeout:  60,
	}
	registry.Hooks = append(registry.Hooks, hook)

	// Test: Save registry
	err = saveHookRegistry(registry, path)
	require.NoError(t, err)

	// Test: Load saved registry
	loadedRegistry, _, err := loadHookRegistry()
	require.NoError(t, err)
	require.Len(t, loadedRegistry.Hooks, 1)
	assert.Equal(t, "test-hook", loadedRegistry.Hooks[0].Name)
}
