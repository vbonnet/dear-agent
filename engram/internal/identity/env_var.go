package identity

import (
	"context"
	"os"
	"time"
)

// EnvVarDetector detects identity from ENGRAM_USER_EMAIL environment variable
type EnvVarDetector struct{}

// Name returns detector name
func (d *EnvVarDetector) Name() string {
	return "env_var"
}

// Priority returns 10 (lowest priority, fully user-controlled)
func (d *EnvVarDetector) Priority() int {
	return 10
}

// Detect attempts to detect identity from environment variable
func (d *EnvVarDetector) Detect(ctx context.Context) (*Identity, error) {
	email := os.Getenv("ENGRAM_USER_EMAIL")
	if email == "" {
		return nil, nil // Not set, not an error
	}

	domain := extractDomain(email)
	if domain == "" {
		return nil, nil // Invalid email format
	}

	return &Identity{
		Email:      email,
		Domain:     domain,
		Method:     "env_var",
		Verified:   false, // Fully user-controlled, unverified
		DetectedAt: time.Now(),
	}, nil
}
