package simple

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

// SimpleFileProvider implements consolidation.Provider using JSON files.
//
// Memories are stored as JSON files in a directory hierarchy based on namespace.
// File organization: {storagePath}/{namespace}/{memory-id}.json
//
// This is a reference implementation demonstrating the Provider interface.
// Not optimized for production use (no caching, simple file I/O).
type SimpleFileProvider struct {
	storagePath string // Root directory for all storage
}

func init() {
	// Register provider so it can be loaded by consolidation.Load()
	consolidation.Register("simple", NewProvider)
}

// NewProvider creates a SimpleFileProvider from configuration.
//
// Required config options:
//   - storage_path: Root directory for memory storage
//
// Example:
//
//	config := consolidation.Config{
//	    ProviderType: "simple",
//	    Options: map[string]interface{}{
//	        "storage_path": "/path/to/storage",
//	    },
//	}
//	provider, err := simple.NewProvider(config)
func NewProvider(config consolidation.Config) (consolidation.Provider, error) {
	storagePath, ok := config.Options["storage_path"].(string)
	if !ok || storagePath == "" {
		return nil, fmt.Errorf("%w: missing or invalid storage_path", consolidation.ErrInvalidConfig)
	}

	return &SimpleFileProvider{
		storagePath: storagePath,
	}, nil
}

// Initialize prepares the provider (validates storage path).
func (p *SimpleFileProvider) Initialize(ctx context.Context, config consolidation.Config) error {
	// Storage path will be created on first write
	return nil
}

// Close cleanly shuts down the provider (no-op for file provider).
func (p *SimpleFileProvider) Close(ctx context.Context) error {
	return nil
}

// HealthCheck verifies provider is operational (storage path accessible).
func (p *SimpleFileProvider) HealthCheck(ctx context.Context) error {
	// TODO: Check storage path is writable
	return nil
}
