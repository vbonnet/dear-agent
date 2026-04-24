package detectors

import (
	"context"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	detector := NewBashCommandPatternDetector()

	err := registry.Register(detector)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify detector was registered
	retrieved, exists := registry.Get("bash_command_pattern")
	if !exists {
		t.Fatal("Detector not found after registration")
	}
	if retrieved.Name() != detector.Name() {
		t.Errorf("Retrieved detector name = %q, want %q", retrieved.Name(), detector.Name())
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	registry := NewRegistry()
	detector := NewBashCommandPatternDetector()

	// First registration should succeed
	err := registry.Register(detector)
	if err != nil {
		t.Fatalf("First Register() error = %v", err)
	}

	// Second registration should fail
	err = registry.Register(detector)
	if err == nil {
		t.Fatal("Expected error for duplicate registration, got nil")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	detector := NewBashCommandPatternDetector()
	registry.Register(detector) // nolint:errcheck

	tests := []struct {
		name         string
		detectorName string
		wantExists   bool
	}{
		{
			name:         "existing detector",
			detectorName: "bash_command_pattern",
			wantExists:   true,
		},
		{
			name:         "non-existent detector",
			detectorName: "nonexistent",
			wantExists:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, exists := registry.Get(tt.detectorName)
			if exists != tt.wantExists {
				t.Errorf("Get(%q) exists = %v, want %v", tt.detectorName, exists, tt.wantExists)
			}
		})
	}
}

func TestRegistry_DetectorsForInstructionType(t *testing.T) {
	registry := NewRegistry()
	detector := NewBashCommandPatternDetector()
	registry.Register(detector) // nolint:errcheck

	tests := []struct {
		name            string
		instructionType string
		wantCount       int
	}{
		{
			name:            "tool_usage type",
			instructionType: "tool_usage",
			wantCount:       1,
		},
		{
			name:            "non-matching type",
			instructionType: "phase_scope",
			wantCount:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detectors := registry.DetectorsForInstructionType(tt.instructionType)
			if len(detectors) != tt.wantCount {
				t.Errorf("DetectorsForInstructionType(%q) count = %d, want %d",
					tt.instructionType, len(detectors), tt.wantCount)
			}
		})
	}
}

func TestRegistry_RunAll(t *testing.T) {
	registry := NewRegistry()
	detector := NewBashCommandPatternDetector()
	registry.Register(detector) // nolint:errcheck

	input := DetectorInput{
		Content: "cd /path && ls",
		Metadata: map[string]string{
			"agent": "claude-code",
		},
	}

	violations, err := registry.RunAll(context.Background(), input)
	if err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}

	if len(violations) == 0 {
		t.Fatal("Expected violations from RunAll(), got none")
	}
}
