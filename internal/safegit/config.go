package safegit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoConfig is the per-repo configuration loaded from .safe-merge.yml at the
// repo root. Agents configure which bots must review and which CI checks are
// known-flaky (eligible for a single automatic rerun).
type RepoConfig struct {
	// ExpectedReviewers lists GitHub logins that must have posted a review on
	// the PR's current head SHA before merging. Defaults to [ReviewBot] when
	// the file is absent or this field is empty.
	ExpectedReviewers []string `yaml:"expected_reviewers"`

	// FlakyChecks lists CI check names known to fail intermittently. A single
	// automatic rerun is triggered when ONLY checks in this list are failing;
	// a second consecutive failure is treated as real and blocks the merge.
	FlakyChecks []string `yaml:"flaky_checks"`
}

// contentsAPIResponse is the minimal shape of the GitHub Contents API response.
type contentsAPIResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// loadRepoConfig fetches and parses .safe-merge.yml from the GitHub repo using
// the gh CLI. If the file does not exist or cannot be fetched, it returns the
// default config (no flaky checks, standard reviewer list). A parse error is
// surfaced so misconfigured files don't silently fall back to insecure defaults.
func loadRepoConfig(repo string) (RepoConfig, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return defaultRepoConfig(), nil
	}
	owner, name := parts[0], parts[1]

	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/%s/contents/.safe-merge.yml", owner, name),
	)
	out, err := cmd.Output()
	if err != nil {
		// .safe-merge.yml is optional: absent (404) or inaccessible → use defaults.
		return defaultRepoConfig(), nil //nolint:nilerr // intentional: missing config is not an error condition
	}

	var resp contentsAPIResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return defaultRepoConfig(), fmt.Errorf("parsing Contents API response: %w", err)
	}
	if resp.Encoding != "base64" {
		return defaultRepoConfig(), fmt.Errorf(".safe-merge.yml: unexpected encoding %q", resp.Encoding)
	}

	// The GitHub API encodes content as base64 with embedded newlines.
	encoded := strings.ReplaceAll(resp.Content, "\n", "")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return defaultRepoConfig(), fmt.Errorf("decoding .safe-merge.yml: %w", err)
	}

	var cfg RepoConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return defaultRepoConfig(), fmt.Errorf("parsing .safe-merge.yml: %w", err)
	}
	if len(cfg.ExpectedReviewers) == 0 {
		cfg.ExpectedReviewers = []string{ReviewBot}
	}
	return cfg, nil
}

// defaultRepoConfig returns the built-in defaults: Gemini bot reviewer required,
// no known-flaky checks.
func defaultRepoConfig() RepoConfig {
	return RepoConfig{
		ExpectedReviewers: []string{ReviewBot},
	}
}
