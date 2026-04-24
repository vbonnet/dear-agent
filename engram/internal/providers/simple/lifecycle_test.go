package simple

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

func TestSimpleFileProvider_Initialize(t *testing.T) {
	tests := []struct {
		name    string
		config  consolidation.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: consolidation.Config{
				ProviderType: "simple",
				Options: map[string]interface{}{
					"storage_path": t.TempDir(),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.config)
			if err != nil {
				t.Fatalf("NewProvider() error = %v", err)
			}

			ctx := context.Background()
			err = provider.Initialize(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSimpleFileProvider_Close(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Close should not error
	if err := provider.Close(ctx); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Multiple closes should be safe
	if err := provider.Close(ctx); err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

func TestSimpleFileProvider_HealthCheck(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if err := provider.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
}

func TestNewProvider_InvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  consolidation.Config
		wantErr bool
	}{
		{
			name: "missing storage_path",
			config: consolidation.Config{
				ProviderType: "simple",
				Options:      map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "empty storage_path",
			config: consolidation.Config{
				ProviderType: "simple",
				Options: map[string]interface{}{
					"storage_path": "",
				},
			},
			wantErr: true,
		},
		{
			name: "wrong type for storage_path",
			config: consolidation.Config{
				ProviderType: "simple",
				Options: map[string]interface{}{
					"storage_path": 123,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSimpleFileProvider_StoragePathCreation(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "memories")

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": storagePath,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Storage path doesn't exist yet (created on first write)
	if _, err := os.Stat(storagePath); err == nil {
		// Path exists, which is fine
	} else if !os.IsNotExist(err) {
		t.Errorf("Unexpected error checking storage path: %v", err)
	}

	// Store a memory to trigger directory creation
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "test-mem",
		Type:          consolidation.Episodic,
		Namespace:     []string{"test"},
		Content:       "Test",
	}

	if err := provider.StoreMemory(ctx, memory.Namespace, memory); err != nil {
		t.Fatalf("StoreMemory() error = %v", err)
	}

	// Now storage path should exist
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		t.Error("Storage path was not created on first write")
	}
}
