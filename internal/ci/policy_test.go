package ci_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/ci"
)

// TestDefaultPolicy verifies the default policy settings.
func TestDefaultPolicy(t *testing.T) {
	policy := ci.DefaultPolicy()

	assert.Empty(t, policy.RequiredWorkflows)
	assert.False(t, policy.AllowBypass)
	assert.Equal(t, 30, policy.TimeoutMinutes)
	assert.Equal(t, "block", policy.FailureBehavior)
	assert.False(t, policy.ParallelExecution)
	assert.True(t, policy.RequireAllPassing)
	assert.NotNil(t, policy.WorkflowDependencies)
}

// TestGatePolicy_Timeout verifies timeout duration calculation.
func TestGatePolicy_Timeout(t *testing.T) {
	tests := []struct {
		name     string
		minutes  int
		expected time.Duration
	}{
		{
			name:     "zero timeout",
			minutes:  0,
			expected: 0,
		},
		{
			name:     "30 minutes",
			minutes:  30,
			expected: 30 * time.Minute,
		},
		{
			name:     "2 hours",
			minutes:  120,
			expected: 120 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ci.GatePolicy{TimeoutMinutes: tt.minutes}
			assert.Equal(t, tt.expected, policy.Timeout())
		})
	}
}

// TestGatePolicy_ShouldBlock verifies blocking behavior.
func TestGatePolicy_ShouldBlock(t *testing.T) {
	tests := []struct {
		name            string
		failureBehavior string
		expectedBlock   bool
	}{
		{"block mode", "block", true},
		{"warn mode", "warn", false},
		{"allow mode", "allow", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ci.GatePolicy{FailureBehavior: tt.failureBehavior}
			assert.Equal(t, tt.expectedBlock, policy.ShouldBlock())
		})
	}
}

// TestGatePolicy_ShouldWarn verifies warning behavior.
func TestGatePolicy_ShouldWarn(t *testing.T) {
	tests := []struct {
		name            string
		failureBehavior string
		expectedWarn    bool
	}{
		{"block mode", "block", false},
		{"warn mode", "warn", true},
		{"allow mode", "allow", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ci.GatePolicy{FailureBehavior: tt.failureBehavior}
			assert.Equal(t, tt.expectedWarn, policy.ShouldWarn())
		})
	}
}

// TestGatePolicy_AllWorkflows verifies workflow aggregation.
func TestGatePolicy_AllWorkflows(t *testing.T) {
	policy := ci.GatePolicy{
		RequiredWorkflows: []string{"test.yml", "lint.yml"},
		OptionalWorkflows: []string{"coverage.yml", "docs.yml"},
	}

	all := policy.AllWorkflows()
	expected := []string{"test.yml", "lint.yml", "coverage.yml", "docs.yml"}
	assert.Equal(t, expected, all)
}

// TestGatePolicy_IsRequired verifies workflow requirement checking.
func TestGatePolicy_IsRequired(t *testing.T) {
	policy := ci.GatePolicy{
		RequiredWorkflows: []string{"test.yml", "lint.yml"},
		OptionalWorkflows: []string{"coverage.yml"},
	}

	assert.True(t, policy.IsRequired("test.yml"))
	assert.True(t, policy.IsRequired("lint.yml"))
	assert.False(t, policy.IsRequired("coverage.yml"))
	assert.False(t, policy.IsRequired("unknown.yml"))
}

// TestLoadPolicyFromFile_Missing verifies default policy when file doesn't exist.
func TestLoadPolicyFromFile_Missing(t *testing.T) {
	config, err := ci.LoadPolicyFromFile("/nonexistent/path/.ci-policy.yaml")
	require.NoError(t, err)
	require.NotNil(t, config)

	assert.Equal(t, "v1", config.Version)
	assert.Equal(t, ci.DefaultPolicy(), config.Default)
}

// TestLoadPolicyFromFile_Valid verifies parsing valid YAML.
func TestLoadPolicyFromFile_Valid(t *testing.T) {
	// Create temporary policy file
	tempDir := t.TempDir()
	policyFile := filepath.Join(tempDir, ".ci-policy.yaml")

	policyContent := `
version: v1
default:
  required_workflows:
    - test.yml
    - lint.yml
  allow_bypass: true
  timeout_minutes: 45
  failure_behavior: warn
  parallel_execution: true
  require_all_passing: false
branch_policies:
  main:
    required_workflows:
      - test.yml
      - lint.yml
      - security.yml
    allow_bypass: false
    timeout_minutes: 60
    failure_behavior: block
`

	err := os.WriteFile(policyFile, []byte(policyContent), 0644)
	require.NoError(t, err)

	// Load policy
	config, err := ci.LoadPolicyFromFile(policyFile)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify default policy
	assert.Equal(t, "v1", config.Version)
	assert.Equal(t, []string{"test.yml", "lint.yml"}, config.Default.RequiredWorkflows)
	assert.True(t, config.Default.AllowBypass)
	assert.Equal(t, 45, config.Default.TimeoutMinutes)
	assert.Equal(t, "warn", config.Default.FailureBehavior)
	assert.True(t, config.Default.ParallelExecution)
	assert.False(t, config.Default.RequireAllPassing)

	// Verify branch policy
	mainPolicy, exists := config.BranchPolicies["main"]
	require.True(t, exists)
	assert.Equal(t, []string{"test.yml", "lint.yml", "security.yml"}, mainPolicy.RequiredWorkflows)
	assert.False(t, mainPolicy.AllowBypass)
	assert.Equal(t, 60, mainPolicy.TimeoutMinutes)
	assert.Equal(t, "block", mainPolicy.FailureBehavior)
}

// TestLoadPolicyFromFile_InvalidYAML verifies error on malformed YAML.
func TestLoadPolicyFromFile_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	policyFile := filepath.Join(tempDir, ".ci-policy.yaml")

	invalidContent := `
invalid yaml content
  - missing colon
    bad indent
`

	err := os.WriteFile(policyFile, []byte(invalidContent), 0644)
	require.NoError(t, err)

	_, err = ci.LoadPolicyFromFile(policyFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse policy YAML")
}

// TestPolicyConfig_GetPolicyForBranch verifies branch matching.
func TestPolicyConfig_GetPolicyForBranch(t *testing.T) {
	config := ci.PolicyConfig{
		Version: "v1",
		Default: ci.GatePolicy{
			FailureBehavior: "block",
			TimeoutMinutes:  30,
		},
		BranchPolicies: map[string]ci.GatePolicy{
			"main": {
				FailureBehavior: "block",
				TimeoutMinutes:  60,
			},
			"feature/*": {
				FailureBehavior: "warn",
				TimeoutMinutes:  20,
			},
			"release/*": {
				FailureBehavior: "block",
				TimeoutMinutes:  90,
			},
		},
	}

	tests := []struct {
		branch          string
		expectedTimeout int
	}{
		{"main", 60},
		{"feature/auth", 20},
		{"feature/api", 20},
		{"release/1.0", 90},
		{"release/2.0", 90},
		{"develop", 30}, // default
		{"unknown", 30}, // default
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			policy := config.GetPolicyForBranch(tt.branch)
			assert.Equal(t, tt.expectedTimeout, policy.TimeoutMinutes)
		})
	}
}

// TestValidateConfig_ValidVersion verifies version validation.
func TestValidateConfig_ValidVersion(t *testing.T) {
	config := &ci.PolicyConfig{
		Version: "v1",
		Default: ci.DefaultPolicy(),
	}

	err := ci.ValidateConfig(config)
	assert.NoError(t, err)
}

// TestValidateConfig_InvalidVersion verifies version validation error.
func TestValidateConfig_InvalidVersion(t *testing.T) {
	config := &ci.PolicyConfig{
		Version: "v2",
		Default: ci.DefaultPolicy(),
	}

	err := ci.ValidateConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported policy version")
}

// TestValidateConfig_NilConfig verifies nil config error.
func TestValidateConfig_NilConfig(t *testing.T) {
	err := ci.ValidateConfig(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config cannot be nil")
}

// TestValidatePolicy_ValidBehaviors verifies valid failure behaviors.
func TestValidatePolicy_ValidBehaviors(t *testing.T) {
	behaviors := []string{"block", "warn", "allow"}

	for _, behavior := range behaviors {
		t.Run(behavior, func(t *testing.T) {
			policy := ci.DefaultPolicy()
			policy.FailureBehavior = behavior

			err := ci.ValidatePolicy(&policy)
			assert.NoError(t, err)
		})
	}
}

// TestValidatePolicy_InvalidBehavior verifies invalid behavior error.
func TestValidatePolicy_InvalidBehavior(t *testing.T) {
	policy := ci.DefaultPolicy()
	policy.FailureBehavior = "invalid"

	err := ci.ValidatePolicy(&policy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid failure_behavior")
}

// TestValidatePolicy_NegativeTimeout verifies negative timeout error.
func TestValidatePolicy_NegativeTimeout(t *testing.T) {
	policy := ci.DefaultPolicy()
	policy.TimeoutMinutes = -10

	err := ci.ValidatePolicy(&policy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout_minutes cannot be negative")
}

// TestValidatePolicy_ExcessiveTimeout verifies maximum timeout validation.
func TestValidatePolicy_ExcessiveTimeout(t *testing.T) {
	policy := ci.DefaultPolicy()
	policy.TimeoutMinutes = 2000 // > 24 hours

	err := ci.ValidatePolicy(&policy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout_minutes exceeds maximum")
}

// TestValidatePolicy_CircularDependencies verifies cycle detection.
func TestValidatePolicy_CircularDependencies(t *testing.T) {
	policy := ci.DefaultPolicy()
	policy.ParallelExecution = true
	policy.WorkflowDependencies = map[string][]string{
		"a.yml": {"b.yml"},
		"b.yml": {"c.yml"},
		"c.yml": {"a.yml"}, // circular!
	}

	err := ci.ValidatePolicy(&policy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

// TestValidatePolicy_ValidDependencies verifies valid dependency graph.
func TestValidatePolicy_ValidDependencies(t *testing.T) {
	policy := ci.DefaultPolicy()
	policy.ParallelExecution = true
	policy.WorkflowDependencies = map[string][]string{
		"deploy.yml": {"test.yml", "lint.yml"},
		"e2e.yml":    {"deploy.yml"},
	}

	err := ci.ValidatePolicy(&policy)
	assert.NoError(t, err)
}

// TestLoadPolicyFromConfig verifies complete loading and validation.
func TestLoadPolicyFromConfig(t *testing.T) {
	tempDir := t.TempDir()
	policyFile := filepath.Join(tempDir, ".ci-policy.yaml")

	policyContent := `
version: v1
default:
  required_workflows:
    - test.yml
  timeout_minutes: 30
  failure_behavior: block
branch_policies:
  main:
    required_workflows:
      - test.yml
      - security.yml
    timeout_minutes: 60
    failure_behavior: block
`

	err := os.WriteFile(policyFile, []byte(policyContent), 0644)
	require.NoError(t, err)

	// Load policy for main branch
	policy, err := ci.LoadPolicyFromConfig(policyFile, "main")
	require.NoError(t, err)
	require.NotNil(t, policy)

	assert.Equal(t, []string{"test.yml", "security.yml"}, policy.RequiredWorkflows)
	assert.Equal(t, 60, policy.TimeoutMinutes)
	assert.Equal(t, "block", policy.FailureBehavior)

	// Load policy for other branch (should get default)
	policy, err = ci.LoadPolicyFromConfig(policyFile, "develop")
	require.NoError(t, err)
	require.NotNil(t, policy)

	assert.Equal(t, []string{"test.yml"}, policy.RequiredWorkflows)
	assert.Equal(t, 30, policy.TimeoutMinutes)
}

// TestFindPolicyFile verifies policy file discovery.
func TestFindPolicyFile(t *testing.T) {
	// Create nested directory structure
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir", "nested")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// Create policy file in parent
	policyFile := filepath.Join(tempDir, ".ci-policy.yaml")
	err = os.WriteFile(policyFile, []byte("version: v1\n"), 0644)
	require.NoError(t, err)

	// Search from nested directory
	found := ci.FindPolicyFile(subDir)
	assert.Equal(t, policyFile, found)
}

// TestFindPolicyFile_NotFound verifies behavior when file doesn't exist.
func TestFindPolicyFile_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	found := ci.FindPolicyFile(tempDir)
	assert.Empty(t, found)
}

// TestBranchPatternMatching verifies wildcard pattern matching.
func TestBranchPatternMatching(t *testing.T) {
	config := ci.PolicyConfig{
		Version: "v1",
		Default: ci.GatePolicy{TimeoutMinutes: 1},
		BranchPolicies: map[string]ci.GatePolicy{
			"exact":     {TimeoutMinutes: 10},
			"prefix/*":  {TimeoutMinutes: 20},
			"*/suffix":  {TimeoutMinutes: 30},
			"feature/*": {TimeoutMinutes: 40},
		},
	}

	tests := []struct {
		branch          string
		expectedTimeout int
	}{
		{"exact", 10},
		{"prefix/anything", 20},
		{"prefix/nested/deep", 20},
		{"anything/suffix", 30},
		{"feature/auth", 40},
		{"feature/api/v2", 40},
		{"other", 1}, // default
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			policy := config.GetPolicyForBranch(tt.branch)
			assert.Equal(t, tt.expectedTimeout, policy.TimeoutMinutes,
				"Branch %s should match policy with timeout %d", tt.branch, tt.expectedTimeout)
		})
	}
}

// TestPolicyDefaults verifies default values are applied.
func TestPolicyDefaults(t *testing.T) {
	tempDir := t.TempDir()
	policyFile := filepath.Join(tempDir, ".ci-policy.yaml")

	// Minimal config without optional fields
	policyContent := `
version: v1
default:
  required_workflows:
    - test.yml
`

	err := os.WriteFile(policyFile, []byte(policyContent), 0644)
	require.NoError(t, err)

	config, err := ci.LoadPolicyFromFile(policyFile)
	require.NoError(t, err)

	// Verify defaults were applied
	assert.Equal(t, "block", config.Default.FailureBehavior)
	assert.Equal(t, 30, config.Default.TimeoutMinutes)
	assert.NotNil(t, config.Default.WorkflowDependencies)
}
