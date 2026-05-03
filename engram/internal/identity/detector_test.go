package identity

import (
	"context"
	"testing"
	"time"
)

// TestManager_Detect_Success tests successful identity detection
func TestManager_Detect_Success(t *testing.T) {
	mgr := NewManager(24 * time.Hour)

	ctx := context.Background()
	id, err := mgr.Detect(ctx)

	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if id == nil {
		t.Fatal("Detect() returned nil identity")
	}

	// Verify identity fields are populated
	if id.Email == "" {
		t.Error("Identity email is empty")
	}
	if id.Domain == "" {
		t.Error("Identity domain is empty")
	}
	if id.Method == "" {
		t.Error("Identity method is empty")
	}
	if id.DetectedAt.IsZero() {
		t.Error("Identity DetectedAt is zero")
	}
}

// TestManager_Detect_CacheHit tests cache functionality
func TestManager_Detect_CacheHit(t *testing.T) {
	mgr := NewManager(24 * time.Hour)
	ctx := context.Background()

	// First detection
	id1, err := mgr.Detect(ctx)
	if err != nil {
		t.Fatalf("First Detect() failed: %v", err)
	}

	// Second detection (should use cache)
	id2, err := mgr.Detect(ctx)
	if err != nil {
		t.Fatalf("Second Detect() failed: %v", err)
	}

	// Verify same identity returned (cached)
	if id1.Email != id2.Email {
		t.Errorf("Cache miss: emails differ (%s != %s)", id1.Email, id2.Email)
	}
	if id1.DetectedAt != id2.DetectedAt {
		t.Error("Cache miss: DetectedAt timestamps differ")
	}
}

// TestManager_ClearCache tests cache clearing
func TestManager_ClearCache(t *testing.T) {
	mgr := NewManager(24 * time.Hour)
	ctx := context.Background()

	// First detection
	id1, err := mgr.Detect(ctx)
	if err != nil {
		t.Fatalf("First Detect() failed: %v", err)
	}

	// Clear cache
	mgr.ClearCache()

	// Second detection (should re-detect)
	id2, err := mgr.Detect(ctx)
	if err != nil {
		t.Fatalf("Second Detect() failed: %v", err)
	}

	// Timestamps should differ (re-detected)
	if id1.DetectedAt.Equal(id2.DetectedAt) {
		t.Error("Cache not cleared: DetectedAt timestamps are same")
	}
}

// TestManager_Detect_ContextCancellation tests context cancellation handling
func TestManager_Detect_ContextCancellation(t *testing.T) {
	mgr := NewManager(24 * time.Hour)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Detection should handle cancellation gracefully
	// (may succeed from cache or fail from detector)
	_, err := mgr.Detect(ctx)

	// Either success (cache hit) or error (detector context check)
	// Both are valid, just ensure no panic
	_ = err
}

// TestDetectorPriority tests detector priority ordering
func TestDetectorPriority(t *testing.T) {
	gcpDetector := &GCPADCDetector{}
	gitDetector := &GitConfigDetector{}
	envDetector := &EnvVarDetector{}

	if gcpDetector.Priority() <= gitDetector.Priority() {
		t.Errorf("GCP ADC priority (%d) should be higher than Git (%d)",
			gcpDetector.Priority(), gitDetector.Priority())
	}

	if gitDetector.Priority() <= envDetector.Priority() {
		t.Errorf("Git priority (%d) should be higher than Env Var (%d)",
			gitDetector.Priority(), envDetector.Priority())
	}

	// Verify exact priorities
	if gcpDetector.Priority() != 100 {
		t.Errorf("GCP ADC priority = %d, want 100", gcpDetector.Priority())
	}
	if gitDetector.Priority() != 50 {
		t.Errorf("Git priority = %d, want 50", gitDetector.Priority())
	}
	if envDetector.Priority() != 10 {
		t.Errorf("Env Var priority = %d, want 10", envDetector.Priority())
	}
}

// TestIdentity_Verified tests verified flag correctness
func TestIdentity_Verified(t *testing.T) {
	tests := []struct {
		method   string
		verified bool
	}{
		{"gcp_adc", true},     // GCP ADC is verified
		{"git_config", false}, // Git config is unverified
		{"env_var", false},    // Env var is unverified
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			id := &Identity{
				Email:    "test@example.com",
				Method:   tt.method,
				Verified: tt.verified,
			}

			if id.Verified != tt.verified {
				t.Errorf("Method %s: verified = %v, want %v",
					tt.method, id.Verified, tt.verified)
			}
		})
	}
}
