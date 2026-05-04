package procguard

import (
	"strings"
	"testing"
)

func TestValidateSpawn_DepthLimit(t *testing.T) {
	limits := DefaultLimits()

	tests := []struct {
		name        string
		depth       int
		expectError bool
		errContains string
	}{
		{"depth 0 allowed", 0, false, ""},
		{"depth 4 allowed", 4, false, ""},
		{"depth at max rejected", MaxSessionDepth, true, "depth"},
		{"depth beyond max rejected", MaxSessionDepth + 3, true, "depth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpawn(limits, tt.depth, 0, 0)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for depth %d, got nil", tt.depth)
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errContains, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for depth %d: %v", tt.depth, err)
			}
		})
	}
}

func TestValidateSpawn_ChildrenLimit(t *testing.T) {
	limits := DefaultLimits()

	tests := []struct {
		name        string
		children    int
		expectError bool
	}{
		{"0 children allowed", 0, false},
		{"9 children allowed", 9, false},
		{"10 children rejected", MaxChildrenPerSession, true},
		{"20 children rejected", 20, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpawn(limits, 0, tt.children, 0)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for %d children, got nil", tt.children)
				}
				if !strings.Contains(err.Error(), "children") {
					t.Errorf("expected error to mention children, got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for %d children: %v", tt.children, err)
			}
		})
	}
}

func TestValidateSpawn_TotalActiveLimit(t *testing.T) {
	limits := DefaultLimits()

	tests := []struct {
		name        string
		total       int
		expectError bool
	}{
		{"0 active allowed", 0, false},
		{"49 active allowed", 49, false},
		{"50 active rejected", MaxTotalActiveSessions, true},
		{"100 active rejected", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpawn(limits, 0, 0, tt.total)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for %d active, got nil", tt.total)
				}
				if !strings.Contains(err.Error(), "active sessions") {
					t.Errorf("expected error to mention active sessions, got: %v", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error for %d active: %v", tt.total, err)
			}
		})
	}
}

func TestValidateSpawn_CustomLimits(t *testing.T) {
	limits := SpawnLimits{
		MaxDepth:       2,
		MaxChildren:    3,
		MaxTotalActive: 5,
		NprocLimit:     128,
	}

	t.Run("custom depth limit", func(t *testing.T) {
		if err := ValidateSpawn(limits, 1, 0, 0); err != nil {
			t.Fatalf("depth 1 should be allowed with max 2: %v", err)
		}
		if err := ValidateSpawn(limits, 2, 0, 0); err == nil {
			t.Fatal("depth 2 should be rejected with max 2")
		}
	})

	t.Run("custom children limit", func(t *testing.T) {
		if err := ValidateSpawn(limits, 0, 2, 0); err != nil {
			t.Fatalf("2 children should be allowed with max 3: %v", err)
		}
		if err := ValidateSpawn(limits, 0, 3, 0); err == nil {
			t.Fatal("3 children should be rejected with max 3")
		}
	})

	t.Run("custom total active limit", func(t *testing.T) {
		if err := ValidateSpawn(limits, 0, 0, 4); err != nil {
			t.Fatalf("4 active should be allowed with max 5: %v", err)
		}
		if err := ValidateSpawn(limits, 0, 0, 5); err == nil {
			t.Fatal("5 active should be rejected with max 5")
		}
	})
}

func TestValidateSpawn_PriorityOrder(t *testing.T) {
	limits := SpawnLimits{
		MaxDepth:       2,
		MaxChildren:    3,
		MaxTotalActive: 5,
	}

	// All limits exceeded — depth should be reported first
	err := ValidateSpawn(limits, 10, 10, 100)
	if err == nil {
		t.Fatal("expected error when all limits exceeded")
	}
	if !strings.Contains(err.Error(), "depth") {
		t.Errorf("expected depth violation first, got: %v", err)
	}
}

func TestActiveCounter(t *testing.T) {
	ResetActiveCount()

	if got := ActiveCount(); got != 0 {
		t.Fatalf("expected 0 after reset, got %d", got)
	}

	IncrementActive()
	IncrementActive()
	IncrementActive()

	if got := ActiveCount(); got != 3 {
		t.Fatalf("expected 3 after 3 increments, got %d", got)
	}

	DecrementActive()

	if got := ActiveCount(); got != 2 {
		t.Fatalf("expected 2 after decrement, got %d", got)
	}

	ResetActiveCount()

	if got := ActiveCount(); got != 0 {
		t.Fatalf("expected 0 after final reset, got %d", got)
	}
}

func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()

	if limits.MaxDepth != MaxSessionDepth {
		t.Errorf("expected MaxDepth=%d, got %d", MaxSessionDepth, limits.MaxDepth)
	}
	if limits.MaxChildren != MaxChildrenPerSession {
		t.Errorf("expected MaxChildren=%d, got %d", MaxChildrenPerSession, limits.MaxChildren)
	}
	if limits.MaxTotalActive != MaxTotalActiveSessions {
		t.Errorf("expected MaxTotalActive=%d, got %d", MaxTotalActiveSessions, limits.MaxTotalActive)
	}
	if limits.NprocLimit != DefaultNprocLimit {
		t.Errorf("expected NprocLimit=%d, got %d", DefaultNprocLimit, limits.NprocLimit)
	}
}

func TestProcessGroupAttr(t *testing.T) {
	attr := ProcessGroupAttr()
	if attr == nil {
		t.Fatal("ProcessGroupAttr returned nil")
	}
	if !attr.Setpgid {
		t.Error("expected Setpgid to be true")
	}
}
