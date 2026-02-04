package types

import (
	"fmt"
)

// Flags represents CLI flags for the application
type Flags struct {
	// Per-stage prompt flags
	ExtractPrompt  string // --extract-prompt: Custom prompt for extraction stage
	AnalyzePrompt  string // --analyze-prompt: Custom prompt for topic analysis stage
	ResearchPrompt string // --research-prompt: Custom prompt for deep research stage

	// Other flags
	Type      string // --type: Content type override
	Mode      string // --mode: Analysis mode (general|competitive)
	OutputDir string // --output-dir: Output directory
	Timeout   int    // --timeout: Deep Research timeout in minutes
	Project   string // --project: GCP project ID
	Force     bool   // --force: Force refresh of existing research

	// Competitive mode discovery flags
	NoDiscovery    bool // --no-discovery: Skip URL discovery in competitive mode
	DiscoveryLimit int  // --discovery-limit: Max URLs to discover (default: 5)
}

// Validate validates flag values
func (f *Flags) Validate() error {
	// Validate type if specified
	if f.Type != "" {
		validTypes := map[string]bool{
			"video":       true,
			"article":     true,
			"arxiv":       true,
			"huggingface": true,
		}
		if !validTypes[f.Type] {
			return fmt.Errorf("invalid type: %s (must be: video, article, arxiv, huggingface)", f.Type)
		}
	}

	// Validate mode if specified
	if f.Mode != "" {
		validModes := map[string]bool{
			"general":     true,
			"competitive": true,
		}
		if !validModes[f.Mode] {
			return fmt.Errorf("invalid mode: %s (must be: general, competitive)", f.Mode)
		}
	}

	// Validate discovery limit if specified
	if f.DiscoveryLimit < 0 {
		return fmt.Errorf("invalid discovery-limit: %d (must be >= 0)", f.DiscoveryLimit)
	}
	if f.DiscoveryLimit > 20 {
		return fmt.Errorf("invalid discovery-limit: %d (must be <= 20 to avoid excessive API usage)", f.DiscoveryLimit)
	}

	return nil
}
