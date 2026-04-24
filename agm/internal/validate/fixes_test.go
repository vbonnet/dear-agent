package validate

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestFixStrategy_Structure(t *testing.T) {
	// Test that FixStrategy can be created with all fields
	strategy := &FixStrategy{
		RequiresBackup: true,
		Confirmation:   "Are you sure you want to apply this fix?",
		Apply: func(m *manifest.Manifest, i *Issue) error {
			return nil
		},
	}

	if !strategy.RequiresBackup {
		t.Error("Expected RequiresBackup=true")
	}
	if strategy.Confirmation == "" {
		t.Error("Expected Confirmation to be set")
	}
	if strategy.Apply == nil {
		t.Error("Expected Apply function to be set")
	}
}

func TestFixStrategy_NoConfirmation(t *testing.T) {
	// Test strategy that doesn't require confirmation (safe fix)
	strategy := &FixStrategy{
		RequiresBackup: false,
		Confirmation:   "", // Empty means no confirmation needed
		Apply: func(m *manifest.Manifest, i *Issue) error {
			return nil
		},
	}

	if strategy.RequiresBackup {
		t.Error("Safe fixes should not require backup")
	}
	if strategy.Confirmation != "" {
		t.Error("Safe fixes should not require confirmation")
	}
}

func TestFixStrategy_ApplyFunction(t *testing.T) {
	called := false
	strategy := &FixStrategy{
		RequiresBackup: false,
		Confirmation:   "",
		Apply: func(m *manifest.Manifest, i *Issue) error {
			called = true
			return nil
		},
	}

	// Call the Apply function
	err := strategy.Apply(&manifest.Manifest{}, &Issue{})
	if err != nil {
		t.Errorf("Apply function returned error: %v", err)
	}
	if !called {
		t.Error("Apply function was not called")
	}
}
