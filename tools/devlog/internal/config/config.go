// Package config provides devlog workspace configuration management.
//
// It supports loading, merging, and validating YAML-based configurations
// with comprehensive security protections against common attack vectors
// including path traversal, YAML bomb DoS attacks, and malicious URLs.
//
// The package implements a dual-configuration system:
//   - config.yaml: Team-wide configuration (committed to git)
//   - config.local.yaml: Local overrides (git-ignored)
//
// Configurations are merged additively, with local config extending
// (not replacing) the base configuration.
package config

import (
	stderrors "errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	// Using yaml.v3 v3.0.1+ which contains fix for CVE-2022-28948
	// DO NOT downgrade below v3.0.1 - previous versions vulnerable to DoS
	"gopkg.in/yaml.v3"

	"github.com/vbonnet/dear-agent/tools/devlog/internal/errors"
)

// Config represents the devlog workspace configuration
type Config struct {
	Name        string `yaml:"name" validate:"required"`
	Description string `yaml:"description,omitempty"`
	Owner       string `yaml:"owner,omitempty"`
	Repos       []Repo `yaml:"repos" validate:"required,min=1,dive"`
}

// Repo represents a git repository in the workspace
type Repo struct {
	Name      string     `yaml:"name" validate:"required"`
	URL       string     `yaml:"url" validate:"required"`
	Type      RepoType   `yaml:"type,omitempty" validate:"omitempty,oneof=bare standard"` // "bare" or "standard"
	Worktrees []Worktree `yaml:"worktrees,omitempty" validate:"dive"`
}

// RepoType defines repository type
type RepoType string

const (
	// RepoTypeBare indicates a bare git repository with worktrees
	RepoTypeBare RepoType = "bare"
	// RepoTypeStandard indicates a standard git repository with a working directory
	RepoTypeStandard RepoType = "standard"
)

// Worktree represents a git worktree within a bare repo
type Worktree struct {
	Name      string `yaml:"name" validate:"required"`
	Branch    string `yaml:"branch" validate:"required"`
	Protected bool   `yaml:"protected,omitempty"`
}

const (
	// MaxConfigSize is the maximum allowed size for config files (1MB)
	// This prevents YAML bomb attacks and excessive memory usage
	MaxConfigSize = 1 * 1024 * 1024 // 1MB
)

// Load reads and parses a config file from the given path
func Load(path string) (*Config, error) {
	// Check file size before reading
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.WrapPath("load config", path, errors.ErrConfigNotFound)
		}
		return nil, errors.WrapPath("stat config", path, err)
	}

	if info.Size() > MaxConfigSize {
		return nil, errors.WrapPath("load config", path,
			fmt.Errorf("%w: file size %d bytes exceeds maximum %d bytes",
				errors.ErrConfigInvalid, info.Size(), MaxConfigSize))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WrapPath("read config", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, errors.WrapPath("parse config", path,
			fmt.Errorf("%w: %w", errors.ErrConfigInvalid, err))
	}

	return &cfg, nil
}

// Validate performs semantic validation on the config using go-playground/validator
func (c *Config) Validate() error {
	validate := validator.New()

	// Run struct validation
	if err := validate.Struct(c); err != nil {
		var validationErrors validator.ValidationErrors
		if stderrors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				field := e.StructField()
				switch field {
				case "Name":
					return fmt.Errorf("%w: name is required", errors.ErrConfigInvalid)
				case "Repos":
					if e.Tag() == "required" || e.Tag() == "min" {
						return fmt.Errorf("%w: at least one repo is required", errors.ErrConfigInvalid)
					}
				}
			}
		}
		return fmt.Errorf("%w: %w", errors.ErrConfigInvalid, err)
	}

	// Custom validation: Duplicate repo detection
	seenRepos := make(map[string]bool)
	for _, repo := range c.Repos {
		if seenRepos[repo.Name] {
			return fmt.Errorf("%w: duplicate repo name: %s", errors.ErrConfigInvalid, repo.Name)
		}
		seenRepos[repo.Name] = true

		// Additional custom validation for repos
		if err := repo.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// Validate performs validation on a Repo
func (r *Repo) Validate() error {
	validate := validator.New()

	// Run struct validation
	if err := validate.Struct(r); err != nil {
		var validationErrors validator.ValidationErrors
		if stderrors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				field := e.StructField()
				switch field {
				case "Name":
					return fmt.Errorf("%w: repo name is required", errors.ErrConfigInvalid)
				case "URL":
					return fmt.Errorf("%w: repo URL is required for %s", errors.ErrConfigInvalid, r.Name)
				case "Type":
					return fmt.Errorf("%w: invalid repo type for %s: %s (must be 'bare' or 'standard')",
						errors.ErrConfigInvalid, r.Name, r.Type)
				}
			}
		}
		return fmt.Errorf("%w: %w", errors.ErrConfigInvalid, err)
	}

	// Git URL validation (custom validation not in validator)
	if err := validateGitURL(r.URL); err != nil {
		return fmt.Errorf("%w: invalid git URL for repo %s: %w",
			errors.ErrConfigInvalid, r.Name, err)
	}

	// Duplicate worktree detection (custom validation)
	seenWorktrees := make(map[string]bool)
	for _, wt := range r.Worktrees {
		if seenWorktrees[wt.Name] {
			return fmt.Errorf("%w: duplicate worktree name in repo %s: %s",
				errors.ErrConfigInvalid, r.Name, wt.Name)
		}
		seenWorktrees[wt.Name] = true

		// Validate each worktree
		if err := wt.Validate(); err != nil {
			return fmt.Errorf("%w: in repo %s: %w", errors.ErrConfigInvalid, r.Name, err)
		}
	}

	return nil
}

// Validate performs validation on a Worktree
func (w *Worktree) Validate() error {
	validate := validator.New()

	// Run struct validation
	if err := validate.Struct(w); err != nil {
		var validationErrors validator.ValidationErrors
		if stderrors.As(err, &validationErrors) {
			for _, e := range validationErrors {
				field := e.StructField()
				switch field {
				case "Name":
					return fmt.Errorf("%w: worktree name is required", errors.ErrConfigInvalid)
				case "Branch":
					return fmt.Errorf("%w: worktree branch is required for %s", errors.ErrConfigInvalid, w.Name)
				}
			}
		}
		return fmt.Errorf("%w: %w", errors.ErrConfigInvalid, err)
	}

	// Path traversal prevention (comprehensive - custom validation)
	if strings.ContainsAny(w.Name, "/\\") {
		return fmt.Errorf("%w: worktree name contains path separators: %s", errors.ErrConfigInvalid, w.Name)
	}
	if strings.Contains(w.Name, "..") {
		return fmt.Errorf("%w: worktree name contains path traversal: %s", errors.ErrConfigInvalid, w.Name)
	}
	if strings.Contains(w.Name, "\x00") {
		return fmt.Errorf("%w: worktree name contains null byte", errors.ErrConfigInvalid)
	}
	if filepath.IsAbs(w.Name) {
		return fmt.Errorf("%w: worktree name cannot be absolute path: %s", errors.ErrConfigInvalid, w.Name)
	}
	// Ensure it's a clean filename (no directory components)
	if w.Name != filepath.Base(filepath.Clean(w.Name)) {
		return fmt.Errorf("%w: invalid worktree name: %s", errors.ErrConfigInvalid, w.Name)
	}

	return nil
}

const (
	// MaxGitURLLength prevents excessively long URLs
	MaxGitURLLength = 2000
)

var (
	// SSH git URL pattern: git@hostname:path
	// Stricter pattern to prevent malformed hostnames:
	// - Hostname must start/end with alphanumeric
	// - No consecutive dots, no leading/trailing dots or dashes
	// - Labels between dots can contain hyphens (but not at start/end)
	// - Path must start with alphanumeric
	sshGitURLPattern = regexp.MustCompile(`^git@[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*:[a-zA-Z0-9][a-zA-Z0-9./_-]*$`)
)

// validateGitURL performs comprehensive validation of git repository URLs
func validateGitURL(gitURL string) error {
	// Length check
	if len(gitURL) == 0 {
		return fmt.Errorf("%w: URL cannot be empty", errors.ErrConfigInvalid)
	}
	if len(gitURL) > MaxGitURLLength {
		return fmt.Errorf("%w: URL exceeds maximum length of %d characters", errors.ErrConfigInvalid, MaxGitURLLength)
	}

	// HTTPS URLs
	if strings.HasPrefix(gitURL, "https://") {
		parsedURL, err := url.Parse(gitURL)
		if err != nil {
			return fmt.Errorf("%w: invalid HTTPS URL: %w", errors.ErrConfigInvalid, err)
		}
		if parsedURL.Host == "" {
			return fmt.Errorf("%w: HTTPS URL missing hostname", errors.ErrConfigInvalid)
		}
		return nil
	}

	// SSH URLs (git@...)
	if strings.HasPrefix(gitURL, "git@") {
		if !sshGitURLPattern.MatchString(gitURL) {
			return fmt.Errorf("%w: invalid SSH URL format (expected git@hostname:path)", errors.ErrConfigInvalid)
		}
		return nil
	}

	// Reject all other URL schemes
	return fmt.Errorf("%w: only https:// and git@ URLs are allowed (got: %s)", errors.ErrConfigInvalid, gitURL)
}
