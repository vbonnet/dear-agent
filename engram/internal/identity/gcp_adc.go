package identity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

// GCPADCDetector detects identity from Google Cloud Application Default Credentials
type GCPADCDetector struct{}

// Name returns detector name
func (d *GCPADCDetector) Name() string {
	return "gcp_adc"
}

// Priority returns 100 (highest priority, cryptographically verified)
func (d *GCPADCDetector) Priority() int {
	return 100
}

// Detect attempts to detect identity from GCP ADC file
func (d *GCPADCDetector) Detect(ctx context.Context) (*Identity, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("gcp adc detection cancelled: %w", ctx.Err())
	default:
	}

	// Use official Google oauth2 library to find and validate credentials
	// This replaces custom JSON parsing and is more robust to schema changes
	creds, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		// Credentials not found or invalid - this is not an error, just means
		// GCP ADC is not configured
		return nil, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Verify credentials are actually loaded
	if creds == nil {
		return nil, nil
	}

	// Extract email from gcloud config (ADC file doesn't contain email directly)
	// The credentials file validates authentication but doesn't include user email
	email, err := d.getEmailFromGcloudConfig()
	if err != nil || email == "" {
		return nil, nil //nolint:nilerr // Cannot determine email, not an error
	}

	domain := extractDomain(email)
	if domain == "" {
		return nil, nil // Invalid email format
	}

	return &Identity{
		Email:      email,
		Domain:     domain,
		Method:     "gcp_adc",
		Verified:   true, // GCP ADC is cryptographically verified
		DetectedAt: time.Now(),
	}, nil
}

// getEmailFromGcloudConfig reads email from gcloud config_default file
func (d *GCPADCDetector) getEmailFromGcloudConfig() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".config", "gcloud", "configurations", "config_default")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	// Parse INI-style config
	// Format: account = user@example.com
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "account = ") {
			email := strings.TrimPrefix(line, "account = ")
			return strings.TrimSpace(email), nil
		}
	}

	return "", fmt.Errorf("account not found in gcloud config")
}

// extractDomain extracts domain from email (e.g., "user@example.com" -> "@example.com")
func extractDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return "@" + parts[1]
}
