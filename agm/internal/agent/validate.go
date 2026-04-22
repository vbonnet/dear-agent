package agent

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Known harness names
var knownHarnesses = []string{"claude-code", "gemini-cli", "codex-cli", "opencode-cli"}

// Harness-to-environment-variable mapping
var harnessEnvVars = map[string]string{
	"claude-code": "ANTHROPIC_API_KEY",
	"gemini-cli":  "GEMINI_API_KEY",
	"codex-cli":   "OPENAI_API_KEY",
}

// Harness-to-binary mapping for PATH-based availability checks
var harnessBinaries = map[string][]string{
	"claude-code": {"claude"},
	"gemini-cli":  {"gemini"},
	"codex-cli":   {"codex"},
}

// lookPath is a variable for testing
var lookPath = exec.LookPath

// Harness help URLs
var harnessHelpURLs = map[string]string{
	"claude-code": "https://console.anthropic.com/",
	"gemini-cli":  "https://ai.google.dev/",
	"codex-cli":   "https://platform.openai.com/api-keys",
}

// ValidateHarnessName checks if the harness name is valid
func ValidateHarnessName(name string) error {
	for _, known := range knownHarnesses {
		if name == known {
			return nil
		}
	}

	// Suggest closest match for typos
	suggestion := suggestHarness(name)
	if suggestion != "" {
		return fmt.Errorf("invalid harness '%s'. Must be one of: %v\n\nDid you mean '%s'?",
			name, knownHarnesses, suggestion)
	}

	return fmt.Errorf("invalid harness '%s'. Must be one of: %v", name, knownHarnesses)
}

// ValidateHarnessAvailability checks if the harness is available.
// A harness is available if its binary is on PATH (it manages its own auth),
// or if the appropriate API key / auth environment is configured.
func ValidateHarnessAvailability(name string) error {
	// Special case: OpenCode availability = server reachable (not API key based)
	if name == "opencode-cli" {
		return validateOpenCodeServerAvailable()
	}

	envVar, ok := harnessEnvVars[name]
	if !ok {
		return fmt.Errorf("unknown harness: %s", name)
	}

	// If the harness binary is on PATH, it's available — the binary handles its own auth
	if isHarnessBinaryOnPath(name) {
		return nil
	}

	// Skip API key check for Claude if Vertex AI is configured
	if name == "claude-code" && isVertexAIConfigured() {
		return nil
	}

	// Skip API key check for Claude if running inside Claude Code CLI
	if name == "claude-code" && isClaudeCodeCLI() {
		return nil
	}

	// Skip API key check for Codex if OAuth credentials exist (~/.codex/auth.json)
	if name == "codex-cli" && IsCodexOAuthConfigured() {
		return nil
	}

	if os.Getenv(envVar) == "" {
		return &HarnessUnavailableError{
			Harness: name,
			EnvVar:  envVar,
		}
	}

	return nil
}

// isHarnessBinaryOnPath checks if the harness's CLI binary is available on PATH
func isHarnessBinaryOnPath(name string) bool {
	binaries, ok := harnessBinaries[name]
	if !ok {
		return false
	}
	for _, bin := range binaries {
		if _, err := lookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// isVertexAIConfigured checks if Vertex AI environment is configured
func isVertexAIConfigured() bool {
	return os.Getenv("CLOUD_ML_REGION") != "" ||
		os.Getenv("GOOGLE_CLOUD_PROJECT") != "" ||
		os.Getenv("GCP_PROJECT") != "" ||
		os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" ||
		os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID") != "" ||
		os.Getenv("CLAUDE_CODE_USE_VERTEX") != ""
}

// isClaudeCodeCLI checks if running inside Claude Code CLI
func isClaudeCodeCLI() bool {
	return os.Getenv("CLAUDECODE") != ""
}

// IsCodexOAuthConfigured checks if Codex CLI has OAuth credentials
func IsCodexOAuthConfigured() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(home + "/.codex/auth.json")
	return err == nil
}

// validateOpenCodeServerAvailable checks if OpenCode server is running and accessible.
//
// Performs HTTP GET request to the health endpoint (http://localhost:4096/health).
// Returns error if server is unreachable or returns non-200 status.
//
// Timeout: 2 seconds (quick check, prevents CLI hanging)
func validateOpenCodeServerAvailable() error {
	serverURL := "http://localhost:4096"

	// Allow override via environment variable
	if envURL := os.Getenv("OPENCODE_SERVER_URL"); envURL != "" {
		serverURL = envURL
	}

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(serverURL + "/health")
	if err != nil {
		return fmt.Errorf("OpenCode server not running. Start with: opencode serve --port 4096")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenCode server health check failed: HTTP %d", resp.StatusCode)
	}

	return nil
}

// HarnessUnavailableError represents an error when a harness's API key is not set
type HarnessUnavailableError struct {
	Harness string
	EnvVar  string
}

// Error returns a formatted error message with helpful instructions
func (e *HarnessUnavailableError) Error() string {
	helpURL, ok := harnessHelpURLs[e.Harness]
	if !ok {
		helpURL = "https://www.anthropic.com/"
	}

	return fmt.Sprintf("Harness '%s' unavailable. %s environment variable not set.\n\n"+
		"To use %s, set your API key:\n  export %s=your-api-key\n\n"+
		"Get API key: %s",
		e.Harness, e.EnvVar, e.Harness, e.EnvVar, helpURL)
}

// suggestHarness suggests the closest matching harness name for typos
func suggestHarness(name string) string {
	// Simple prefix matching for common typos
	for _, known := range knownHarnesses {
		if strings.HasPrefix(known, name) {
			return known
		}
		// Also check if the input is a prefix match (e.g., "gem" -> "gemini-cli")
		if strings.HasPrefix(known, strings.ToLower(name)) {
			return known
		}
	}

	// Check for common typos (edit distance 1)
	for _, known := range knownHarnesses {
		if levenshteinDistance(strings.ToLower(name), known) == 1 {
			return known
		}
	}

	return ""
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			matrix[i][j] = min3(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

func min3(a, b, c int) int {
	return min(min(a, b), c)
}
