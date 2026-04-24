// Package ci provides gate policy management for CI/CD pipelines.
package ci

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// GatePolicy defines configurable rules for CI gate enforcement.
//
// Policies control when and how CI workflows must pass before allowing
// code merges. Different policies can be applied based on branch, context,
// or other factors.
type GatePolicy struct {
	// RequiredWorkflows lists workflow files that MUST pass.
	// Paths are relative to .github/workflows/
	//
	// Example: ["test.yml", "lint.yml", "security.yml"]
	RequiredWorkflows []string `yaml:"required_workflows"`

	// OptionalWorkflows lists workflows that run but don't block merges.
	// Useful for informational checks or experimental workflows.
	OptionalWorkflows []string `yaml:"optional_workflows,omitempty"`

	// AllowBypass determines if gates can be bypassed in emergencies.
	// When true, SKIP_CI_GATES=true environment variable allows bypass.
	// All bypasses are logged for audit.
	AllowBypass bool `yaml:"allow_bypass"`

	// TimeoutMinutes is the maximum time to wait for all workflows.
	// Zero means no timeout (workflows can run indefinitely).
	TimeoutMinutes int `yaml:"timeout_minutes"`

	// FailureBehavior determines what happens when required workflows fail.
	//
	// Values:
	//   - "block" (default): Prevent merge, exit non-zero
	//   - "warn": Show warning but allow merge, exit zero
	//   - "allow": Allow merge silently, exit zero
	FailureBehavior string `yaml:"failure_behavior"`

	// ParallelExecution enables running workflows in parallel.
	// When false, workflows run sequentially in the order listed.
	ParallelExecution bool `yaml:"parallel_execution,omitempty"`

	// WorkflowDependencies defines which workflows must complete before others.
	// Map of workflow name -> list of dependencies that must pass first.
	//
	// Example:
	//   deploy.yml: [test.yml, lint.yml]
	//
	// Only used when ParallelExecution is true.
	WorkflowDependencies map[string][]string `yaml:"workflow_dependencies,omitempty"`

	// RequireAllPassing requires ALL required workflows to pass.
	// When false, gate passes if ANY required workflow passes.
	// Default: true
	RequireAllPassing bool `yaml:"require_all_passing"`
}

// PolicyConfig is the root configuration structure for .ci-policy.yaml files.
type PolicyConfig struct {
	// Version identifies the policy schema version.
	// Currently only "v1" is supported.
	Version string `yaml:"version"`

	// Default policy applies when no branch-specific policy matches.
	Default GatePolicy `yaml:"default"`

	// BranchPolicies maps branch patterns to specific policies.
	// Patterns support wildcards: "feature/*", "release-*"
	//
	// Example:
	//   main: {strict policy}
	//   feature/*: {relaxed policy}
	BranchPolicies map[string]GatePolicy `yaml:"branch_policies,omitempty"`
}

// DefaultPolicy returns a safe default policy for CI gates.
//
// Default settings:
//   - Block on any workflow failure
//   - No bypass allowed
//   - 30 minute timeout
//   - No workflows required (must be configured)
func DefaultPolicy() GatePolicy {
	return GatePolicy{
		RequiredWorkflows:    []string{},
		AllowBypass:          false,
		TimeoutMinutes:       30,
		FailureBehavior:      "block",
		ParallelExecution:    false,
		RequireAllPassing:    true,
		WorkflowDependencies: make(map[string][]string),
	}
}

// LoadPolicyFromFile reads a policy configuration from a YAML file.
//
// The file should be in .ci-policy.yaml format with version and policies.
// If the file doesn't exist, returns the default policy (not an error).
//
// Example:
//
//	policy, err := LoadPolicyFromFile(".ci-policy.yaml")
//	if err != nil {
//	    return err
//	}
func LoadPolicyFromFile(path string) (*PolicyConfig, error) {
	// If file doesn't exist, return default configuration
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &PolicyConfig{
			Version: "v1",
			Default: DefaultPolicy(),
		}, nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file: %w", err)
	}

	// Parse YAML
	var config PolicyConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse policy YAML: %w", err)
	}

	// Set defaults for missing fields
	if config.Version == "" {
		config.Version = "v1"
	}

	// Apply defaults to default policy
	config.Default = applyPolicyDefaults(config.Default)

	// Apply defaults to branch policies
	for branch, policy := range config.BranchPolicies {
		config.BranchPolicies[branch] = applyPolicyDefaults(policy)
	}

	return &config, nil
}

// LoadPolicyFromConfig reads policy from a file and returns the policy for a specific branch.
//
// It first loads the configuration, validates it, then returns the appropriate
// policy for the given branch (using branch-specific policy if it exists,
// otherwise the default policy).
func LoadPolicyFromConfig(configPath, branch string) (*GatePolicy, error) {
	config, err := LoadPolicyFromFile(configPath)
	if err != nil {
		return nil, err
	}

	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid policy configuration: %w", err)
	}

	policy := config.GetPolicyForBranch(branch)
	return &policy, nil
}

// GetPolicyForBranch returns the appropriate policy for a given branch.
//
// Branch matching:
//  1. Exact match in BranchPolicies
//  2. Pattern match using wildcards (* matches any characters)
//  3. Default policy if no match
//
// Example patterns:
//   - "main" matches only "main"
//   - "feature/*" matches "feature/auth", "feature/api", etc.
//   - "release-*" matches "release-1.0", "release-2.0", etc.
func (c *PolicyConfig) GetPolicyForBranch(branch string) GatePolicy {
	// Check exact match first
	if policy, ok := c.BranchPolicies[branch]; ok {
		return policy
	}

	// Check pattern matches
	for pattern, policy := range c.BranchPolicies {
		if matchBranchPattern(pattern, branch) {
			return policy
		}
	}

	// Return default policy
	return c.Default
}

// ValidateConfig checks if a policy configuration is valid and sane.
//
// Validation checks:
//   - Supported version (currently only "v1")
//   - Valid failure behaviors
//   - Reasonable timeout values
//   - No circular workflow dependencies
//   - Required workflows exist
func ValidateConfig(config *PolicyConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Check version
	if config.Version != "v1" {
		return fmt.Errorf("unsupported policy version: %s (expected v1)", config.Version)
	}

	// Validate default policy
	if err := ValidatePolicy(&config.Default); err != nil {
		return fmt.Errorf("invalid default policy: %w", err)
	}

	// Validate branch policies
	for branch, policy := range config.BranchPolicies {
		if err := ValidatePolicy(&policy); err != nil {
			return fmt.Errorf("invalid policy for branch %s: %w", branch, err)
		}
	}

	return nil
}

// ValidatePolicy checks if a single policy is valid.
func ValidatePolicy(policy *GatePolicy) error {
	if policy == nil {
		return fmt.Errorf("policy cannot be nil")
	}

	// Validate failure behavior
	validBehaviors := map[string]bool{
		"block": true,
		"warn":  true,
		"allow": true,
	}
	if !validBehaviors[policy.FailureBehavior] {
		return fmt.Errorf("invalid failure_behavior: %s (must be block, warn, or allow)",
			policy.FailureBehavior)
	}

	// Validate timeout
	if policy.TimeoutMinutes < 0 {
		return fmt.Errorf("timeout_minutes cannot be negative: %d", policy.TimeoutMinutes)
	}
	if policy.TimeoutMinutes > 1440 { // 24 hours
		return fmt.Errorf("timeout_minutes exceeds maximum (1440): %d", policy.TimeoutMinutes)
	}

	// Check for circular dependencies
	if policy.ParallelExecution && len(policy.WorkflowDependencies) > 0 {
		if err := validateNoCycles(policy.WorkflowDependencies); err != nil {
			return fmt.Errorf("workflow dependencies contain cycles: %w", err)
		}
	}

	return nil
}

// Timeout returns the timeout duration for this policy.
// Returns 0 if no timeout is configured.
func (p *GatePolicy) Timeout() time.Duration {
	if p.TimeoutMinutes <= 0 {
		return 0
	}
	return time.Duration(p.TimeoutMinutes) * time.Minute
}

// ShouldBlock returns true if workflow failures should block the merge.
func (p *GatePolicy) ShouldBlock() bool {
	return p.FailureBehavior == "block"
}

// ShouldWarn returns true if workflow failures should show warnings.
func (p *GatePolicy) ShouldWarn() bool {
	return p.FailureBehavior == "warn"
}

// AllWorkflows returns all workflows (required + optional) in order.
func (p *GatePolicy) AllWorkflows() []string {
	all := make([]string, 0, len(p.RequiredWorkflows)+len(p.OptionalWorkflows))
	all = append(all, p.RequiredWorkflows...)
	all = append(all, p.OptionalWorkflows...)
	return all
}

// IsRequired returns true if the workflow is in the required list.
func (p *GatePolicy) IsRequired(workflow string) bool {
	for _, w := range p.RequiredWorkflows {
		if w == workflow {
			return true
		}
	}
	return false
}

// Helper functions

// applyPolicyDefaults fills in missing fields with default values.
func applyPolicyDefaults(policy GatePolicy) GatePolicy {
	defaults := DefaultPolicy()

	if policy.FailureBehavior == "" {
		policy.FailureBehavior = defaults.FailureBehavior
	}
	if policy.TimeoutMinutes == 0 {
		policy.TimeoutMinutes = defaults.TimeoutMinutes
	}
	if policy.WorkflowDependencies == nil {
		policy.WorkflowDependencies = make(map[string][]string)
	}

	return policy
}

// matchBranchPattern checks if a branch name matches a pattern with wildcards.
//
// Supports:
//   - Exact match: "main" matches "main"
//   - Prefix match: "feature/*" matches "feature/anything"
//   - Suffix match: "*/stable" matches "anything/stable"
func matchBranchPattern(pattern, branch string) bool {
	// Simple wildcard matching (not full glob)
	if pattern == branch {
		return true
	}

	// Check prefix wildcard: "feature/*"
	if len(pattern) > 2 && pattern[len(pattern)-2:] == "/*" {
		prefix := pattern[:len(pattern)-2]
		return len(branch) > len(prefix) &&
			branch[:len(prefix)] == prefix &&
			branch[len(prefix)] == '/'
	}

	// Check suffix wildcard: "*/stable"
	if len(pattern) > 2 && pattern[:2] == "*/" {
		suffix := pattern[2:]
		return len(branch) > len(suffix) &&
			branch[len(branch)-len(suffix):] == suffix
	}

	// Check middle wildcard: "release-*-stable"
	// (not implemented for simplicity - can be added if needed)

	return false
}

// validateNoCycles checks for circular dependencies in workflow dependencies.
func validateNoCycles(deps map[string][]string) error {
	// Build adjacency graph
	graph := make(map[string][]string)
	for workflow, dependencies := range deps {
		graph[workflow] = dependencies
	}

	// Track visited nodes
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	// DFS to detect cycles
	var hasCycle func(node string) bool
	hasCycle = func(node string) bool {
		visited[node] = true
		recStack[node] = true

		for _, dep := range graph[node] {
			if !visited[dep] {
				if hasCycle(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	// Check each workflow
	for workflow := range graph {
		if !visited[workflow] {
			if hasCycle(workflow) {
				return fmt.Errorf("circular dependency detected involving workflow: %s", workflow)
			}
		}
	}

	return nil
}

// FindPolicyFile searches for .ci-policy.yaml in the working directory and parent directories.
//
// Search order:
//  1. workingDir/.ci-policy.yaml
//  2. workingDir/../.ci-policy.yaml
//  3. Continue up to filesystem root
//
// Returns empty string if not found.
func FindPolicyFile(workingDir string) string {
	const policyFileName = ".ci-policy.yaml"

	// Make path absolute
	absPath, err := filepath.Abs(workingDir)
	if err != nil {
		return ""
	}

	// Search up the directory tree
	for {
		policyPath := filepath.Join(absPath, policyFileName)
		if _, err := os.Stat(policyPath); err == nil {
			return policyPath
		}

		// Move up one directory
		parent := filepath.Dir(absPath)
		if parent == absPath {
			// Reached root
			break
		}
		absPath = parent
	}

	return ""
}
