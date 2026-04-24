package config_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/config"
)

// ExampleLoad demonstrates how to load Wayfinder configuration
func ExampleLoad() {
	// Load config (returns default if file doesn't exist)
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Storage mode: %s\n", cfg.Storage.Mode)
	fmt.Printf("Workspace: %s\n", cfg.Storage.Workspace)
	fmt.Printf("Relative path: %s\n", cfg.Storage.RelativePath)
}

// ExampleConfig_GetStoragePath demonstrates storage path resolution
func ExampleConfig_GetStoragePath() {
	// Dotfile mode
	dotfileConfig := &config.Config{
		Storage: config.StorageConfig{
			Mode: config.ModeDotfile,
		},
	}
	dotfilePath, _ := dotfileConfig.GetStoragePath()
	fmt.Printf("Dotfile mode path: %s\n", filepath.Base(dotfilePath))

	// Centralized mode with explicit path
	centralizedConfig := &config.Config{
		Storage: config.StorageConfig{
			Mode:            config.ModeCentralized,
			CentralizedPath: "/workspace/wf",
		},
	}
	centralizedPath, _ := centralizedConfig.GetStoragePath()
	fmt.Printf("Centralized mode path: %s\n", centralizedPath)

	// Output:
	// Dotfile mode path: .wayfinder
	// Centralized mode path: /workspace/wf
}

// ExampleDefaultConfig shows the default configuration
func ExampleDefaultConfig() {
	cfg := config.DefaultConfig()

	fmt.Printf("Mode: %s\n", cfg.Storage.Mode)
	fmt.Printf("Workspace: %s\n", cfg.Storage.Workspace)
	fmt.Printf("Relative path: %s\n", cfg.Storage.RelativePath)
	fmt.Printf("Auto symlink: %t\n", cfg.Storage.AutoSymlink)

	// Output:
	// Mode: centralized
	// Workspace: engram-research
	// Relative path: wf
	// Auto symlink: true
}

// ExampleDetectWorkspace demonstrates workspace detection
func ExampleDetectWorkspace() {
	// Test mode example
	os.Setenv("ENGRAM_TEST_MODE", "1")
	os.Setenv("ENGRAM_TEST_WORKSPACE", "/tmp/test-workspace")
	defer func() {
		os.Unsetenv("ENGRAM_TEST_MODE")
		os.Unsetenv("ENGRAM_TEST_WORKSPACE")
	}()

	workspace, err := config.DetectWorkspace("any-name")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Detected workspace: %s\n", workspace)

	// Output:
	// Detected workspace: /tmp/test-workspace
}

// ExampleConfig_Validate shows configuration validation
func ExampleConfig_Validate() {
	// Valid config
	validConfig := &config.Config{
		Storage: config.StorageConfig{
			Mode:         config.ModeCentralized,
			Workspace:    "engram-research",
			RelativePath: "wf",
		},
	}
	if err := validConfig.Validate(); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
	} else {
		fmt.Println("Valid configuration")
	}

	// Invalid config (missing workspace)
	invalidConfig := &config.Config{
		Storage: config.StorageConfig{
			Mode:         config.ModeCentralized,
			RelativePath: "wf",
		},
	}
	if err := invalidConfig.Validate(); err != nil {
		fmt.Println("Invalid: workspace required for centralized mode")
	}

	// Output:
	// Valid configuration
	// Invalid: workspace required for centralized mode
}
