package dolt

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkspaceAdapter(t *testing.T) {
	tests := []struct {
		name      string
		configFn  func(t *testing.T) *DoltConfig
		wantErr   bool
	}{
		{
			name:     "nil config",
			configFn: func(t *testing.T) *DoltConfig { return nil },
			wantErr:  true,
		},
		{
			name: "valid config",
			configFn: func(t *testing.T) *DoltConfig {
				return &DoltConfig{
					WorkspaceName: "oss",
					DoltDir:       filepath.Join(t.TempDir(), "oss", ".dolt"),
					Port:          3307,
					Host:          "127.0.0.1",
					DatabaseName:  "workspace",
				}
			},
			wantErr: false,
		},
		{
			name: "empty workspace name",
			configFn: func(t *testing.T) *DoltConfig {
				return &DoltConfig{
					WorkspaceName: "",
					DoltDir:       filepath.Join(t.TempDir(), "oss", ".dolt"),
					Port:          3307,
				}
			},
			wantErr: true,
		},
		{
			name: "relative dolt dir",
			configFn: func(t *testing.T) *DoltConfig {
				return &DoltConfig{
					WorkspaceName: "oss",
					DoltDir:       ".dolt",
					Port:          3307,
				}
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			configFn: func(t *testing.T) *DoltConfig {
				return &DoltConfig{
					WorkspaceName: "oss",
					DoltDir:       filepath.Join(t.TempDir(), "oss", ".dolt"),
					Port:          0,
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.configFn(t)
			adapter, err := NewWorkspaceAdapter(config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, adapter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter)
				assert.Equal(t, config.WorkspaceName, adapter.Config().WorkspaceName)
			}
		})
	}
}

func TestGetDefaultConfig(t *testing.T) {
	tests := []struct {
		name          string
		workspaceName string
		expectedPort  int
	}{
		{
			name:          "oss workspace",
			workspaceName: "oss",
			expectedPort:  3307,
		},
		{
			name:          "acme workspace",
			workspaceName: "acme",
			expectedPort:  3308,
		},
		{
			name:          "other workspace",
			workspaceName: "custom",
			expectedPort:  3309, // Should be 3309 + hash
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := GetDefaultConfig(tt.workspaceName, filepath.Join(t.TempDir(), tt.workspaceName))
			assert.Equal(t, tt.workspaceName, config.WorkspaceName)
			assert.Equal(t, "127.0.0.1", config.Host)
			assert.Equal(t, "workspace", config.DatabaseName)

			if tt.workspaceName == "oss" {
				assert.Equal(t, 3307, config.Port)
			} else if tt.workspaceName == "acme" {
				assert.Equal(t, 3308, config.Port)
			} else {
				assert.GreaterOrEqual(t, config.Port, 3309)
				assert.LessOrEqual(t, config.Port, 3409)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &DoltConfig{
			WorkspaceName: "oss",
			DoltDir:       filepath.Join(t.TempDir(), "oss", ".dolt"),
			Port:          3307,
		}
		err := validateConfig(config)
		assert.NoError(t, err)
	})

	t.Run("sets defaults", func(t *testing.T) {
		config := &DoltConfig{
			WorkspaceName: "oss",
			DoltDir:       filepath.Join(t.TempDir(), "oss", ".dolt"),
			Port:          3307,
			Host:          "", // Should default to 127.0.0.1
			DatabaseName:  "", // Should default to workspace
		}
		err := validateConfig(config)
		assert.NoError(t, err)
		assert.Equal(t, "127.0.0.1", config.Host)
		assert.Equal(t, "workspace", config.DatabaseName)
	})
}

func TestIsDoltInstalled(t *testing.T) {
	// This test will pass if Dolt is installed, fail otherwise
	// In CI, we might need to skip this test
	t.Run("check dolt installation", func(t *testing.T) {
		installed := IsDoltInstalled()
		// Just verify it returns a boolean without panicking
		t.Logf("Dolt installed: %v", installed)
	})
}

func TestHashWorkspaceName(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"oss", hashWorkspaceName("oss")},
		{"acme", hashWorkspaceName("acme")},
		{"custom", hashWorkspaceName("custom")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashWorkspaceName(tt.name)
			assert.GreaterOrEqual(t, hash, 0)
			// Hash should be deterministic
			assert.Equal(t, hash, hashWorkspaceName(tt.name))
		})
	}
}

// MockAdapter tests (without real database)
func TestAdapter_ExecuteInTransaction_Mock(t *testing.T) {
	// This tests the transaction wrapper logic
	// Note: Without a real database, we can't fully test this
	config := &DoltConfig{
		WorkspaceName: "test",
		DoltDir:       filepath.Join(t.TempDir(), "test", ".dolt"),
		Port:          3307,
	}

	adapter, err := NewWorkspaceAdapter(config)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	ctx := context.Background()

	// Try to execute transaction without connection (should fail)
	err = adapter.ExecuteInTransaction(ctx, func(tx *sql.Tx) error {
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestAdapter_ConfigRetrieval(t *testing.T) {
	config := &DoltConfig{
		WorkspaceName: "oss",
		DoltDir:       filepath.Join(t.TempDir(), "oss", ".dolt"),
		Port:          3307,
		Host:          "127.0.0.1",
		DatabaseName:  "workspace",
	}

	adapter, err := NewWorkspaceAdapter(config)
	require.NoError(t, err)

	retrievedConfig := adapter.Config()
	assert.Equal(t, config.WorkspaceName, retrievedConfig.WorkspaceName)
	assert.Equal(t, config.DoltDir, retrievedConfig.DoltDir)
	assert.Equal(t, config.Port, retrievedConfig.Port)
}

func TestAdapter_CloseWithoutConnect(t *testing.T) {
	config := &DoltConfig{
		WorkspaceName: "test",
		DoltDir:       filepath.Join(t.TempDir(), "test", ".dolt"),
		Port:          3307,
	}

	adapter, err := NewWorkspaceAdapter(config)
	require.NoError(t, err)

	// Close without connecting should not error
	err = adapter.Close()
	assert.NoError(t, err)
}

// Benchmark tests
func BenchmarkHashWorkspaceName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = hashWorkspaceName("benchmark-workspace")
	}
}

func BenchmarkGetDefaultConfig(b *testing.B) {
	tmpDir := b.TempDir()
	for i := 0; i < b.N; i++ {
		_ = GetDefaultConfig("benchmark", filepath.Join(tmpDir, "benchmark"))
	}
}

// Test timeout scenarios
func TestAdapter_Connect_Timeout(t *testing.T) {
	config := &DoltConfig{
		WorkspaceName: "test",
		DoltDir:       filepath.Join(t.TempDir(), "test", ".dolt"),
		Port:          9999, // Non-existent port
		Host:          "127.0.0.1",
		DatabaseName:  "workspace",
		AutoStart:     false, // Don't try to start
	}

	adapter, err := NewWorkspaceAdapter(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = adapter.Connect(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}
