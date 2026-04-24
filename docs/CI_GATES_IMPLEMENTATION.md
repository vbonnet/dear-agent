# CI/CD Gates Implementation - Task 3.3

Implementation of enhanced CI/CD gate system with configurable policies, multi-workflow support, and bypass mechanisms.

## Overview

This implementation extends the basic pre-merge-commit hook with a sophisticated policy-based gate system that provides:

- **Configurable Gate Policies**: Define different CI requirements per branch
- **Multi-Workflow Support**: Execute multiple workflows sequentially or in parallel
- **Workflow Dependencies**: Control execution order with dependency graphs
- **Emergency Bypass**: Allow gates to be bypassed in critical situations (with audit)
- **Flexible Failure Modes**: Block, warn, or allow on workflow failures
- **Enhanced Reporting**: Clear, actionable feedback on failures

## File Structure

```
internal/ci/
├── policy.go               # Gate policy management
├── policy_test.go          # Policy tests (100% coverage)
├── act/
│   ├── executor.go         # Enhanced with multi-workflow support
│   └── multi_workflow_test.go  # Multi-workflow execution tests

cmd/agm-hooks/
└── pre-merge-commit/
    └── main.go            # Enhanced hook with policy enforcement

docs/
├── CI_GATES.md            # Comprehensive user guide
└── CI_GATES_IMPLEMENTATION.md  # This file

scripts/
└── test-ci-gates.sh       # Integration test script

.ci-policy.yaml            # Example configuration file
```

## Components

### 1. Policy Management (`internal/ci/policy.go`)

**Core Types:**
- `GatePolicy`: Defines gate behavior (workflows, timeout, bypass, etc.)
- `PolicyConfig`: Root configuration with version and branch policies
- `WorkflowDependencies`: Dependency graph for workflow execution order

**Key Functions:**
- `LoadPolicyFromFile()`: Parse YAML configuration
- `LoadPolicyFromConfig()`: Load and validate policy for a branch
- `ValidatePolicy()`: Ensure policy is valid (no cycles, valid timeouts, etc.)
- `GetPolicyForBranch()`: Match branch to appropriate policy
- `FindPolicyFile()`: Search directory tree for policy file

**Features:**
- Default policy when no config exists
- Branch-specific policies with wildcard patterns (`feature/*`, `*/stable`)
- Circular dependency detection
- Comprehensive validation

### 2. Multi-Workflow Execution (`internal/ci/act/executor.go`)

**New Types:**
- `WorkflowResult`: Outcome of single workflow execution
- `MultiWorkflowResult`: Aggregate results from multiple workflows

**New Methods:**
- `ExecuteWorkflows()`: Run multiple workflows (sequential or parallel)
- `ExecuteWorkflowsWithDependencies()`: Execute with dependency order
- `executeWorkflowsSequential()`: One at a time, stop on failure
- `executeWorkflowsParallel()`: All at once, wait for all

**Execution Strategies:**

**Sequential:**
```go
result, err := executor.ExecuteWorkflows(ctx, req, workflows, false)
```
- Runs workflows one at a time in order
- Stops on first failure (fast-fail)
- Lower resource usage
- Predictable execution order

**Parallel:**
```go
result, err := executor.ExecuteWorkflows(ctx, req, workflows, true)
```
- Runs all workflows concurrently
- Waits for all to complete
- Higher resource usage, faster total time
- No guaranteed order

**With Dependencies:**
```go
dependencies := map[string][]string{
    "e2e.yml": {"test.yml", "build.yml"},
}
result, err := executor.ExecuteWorkflowsWithDependencies(ctx, req, workflows, dependencies)
```
- Topologically sorts workflows
- Runs independent workflows in parallel (phases)
- Skips dependent workflows if dependencies fail
- Validates no circular dependencies

### 3. Enhanced Pre-Merge-Commit Hook (`cmd/agm-hooks/pre-merge-commit/main.go`)

**Features:**
- Policy-based enforcement
- Multiple workflow execution
- Bypass mechanism with audit logging
- Dry-run mode for testing
- Verbose mode for debugging
- Clear error messages with remediation steps

**Command-Line Interface:**
```bash
pre-merge-commit [options]

Options:
  --dry-run         Show what would execute without running
  -v, --verbose     Show detailed execution information
  -h, --help        Display help message
```

**Environment Variables:**
```bash
SKIP_CI_GATES=true   # Bypass gates (requires allow_bypass: true)
```

**Workflow:**
1. Check if merge commit
2. Load policy for target branch
3. Check for bypass
4. Execute required workflows
5. Execute optional workflows (informational)
6. Report results
7. Exit based on failure behavior

**Exit Codes:**
- `0`: Success or allowed failure
- `1`: Blocked failure or infrastructure error

### 4. Configuration File (`.ci-policy.yaml`)

**Structure:**
```yaml
version: v1

default:
  required_workflows: []
  optional_workflows: []
  allow_bypass: false
  timeout_minutes: 30
  failure_behavior: block
  parallel_execution: false
  require_all_passing: true
  workflow_dependencies: {}

branch_policies:
  main:
    # Override for main branch
  feature/*:
    # Override for feature branches
```

**Policy Options:**

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `required_workflows` | array | Must pass before merge | `[]` |
| `optional_workflows` | array | Run but don't block | `[]` |
| `allow_bypass` | bool | Enable emergency bypass | `false` |
| `timeout_minutes` | int | Max execution time | `30` |
| `failure_behavior` | string | `block`, `warn`, or `allow` | `block` |
| `parallel_execution` | bool | Run concurrently | `false` |
| `require_all_passing` | bool | All vs any must pass | `true` |
| `workflow_dependencies` | map | Execution order | `{}` |

## Testing

### Unit Tests

**Policy Tests (`internal/ci/policy_test.go`):**
- Default policy creation
- YAML parsing (valid and invalid)
- Branch pattern matching
- Policy validation (timeout, behavior, dependencies)
- Circular dependency detection
- File discovery

**Multi-Workflow Tests (`internal/ci/act/multi_workflow_test.go`):**
- Sequential execution
- Parallel execution
- Dependency-based execution
- Failure handling
- Skipped workflows

**Coverage:**
```bash
go test ./internal/ci/... -cover
```

### Integration Tests

**Test Script (`scripts/test-ci-gates.sh`):**
1. Validate policy file syntax
2. Run unit tests
3. Build hook binary
4. Test help output
5. Test dry-run mode

**Manual Testing:**
```bash
# Dry-run to test configuration
.git/hooks/pre-merge-commit --dry-run -v

# Test with verbose output
.git/hooks/pre-merge-commit -v

# Test bypass mechanism
SKIP_CI_GATES=true git merge feature/branch
```

## Usage Examples

### Example 1: Simple Configuration

**Use Case:** Small project, basic checks

```yaml
version: v1

default:
  required_workflows:
    - test.yml
  timeout_minutes: 15
  failure_behavior: block
```

### Example 2: Branch-Specific Policies

**Use Case:** Strict main, relaxed features

```yaml
version: v1

default:
  required_workflows:
    - test.yml
  failure_behavior: warn
  allow_bypass: true

branch_policies:
  main:
    required_workflows:
      - test.yml
      - lint.yml
      - security.yml
    failure_behavior: block
    allow_bypass: false
```

### Example 3: Workflow Dependencies

**Use Case:** E2E tests depend on build

```yaml
version: v1

default:
  required_workflows:
    - test.yml
    - build.yml
    - e2e.yml
  parallel_execution: true
  workflow_dependencies:
    build.yml:
      - test.yml
    e2e.yml:
      - build.yml
```

### Example 4: Emergency Bypass

**Scenario:** Production outage, need immediate fix

```yaml
# .ci-policy.yaml
default:
  required_workflows:
    - test.yml
  allow_bypass: true  # Enable bypass
```

```bash
# Perform bypass
SKIP_CI_GATES=true git merge hotfix/critical-fix

# Verify bypass was logged
cat .git/ci-gate-bypass.log
# Output: [2026-03-20T14:30:00Z] User alice bypassed CI gates for merge to main
```

## Design Decisions

### 1. Policy-Based Configuration

**Why:** Flexibility without code changes
- Different branches have different requirements
- Easy to adjust over time
- Self-documenting requirements

**Alternative Considered:** Hard-coded rules
**Rejected Because:** Not flexible enough for diverse workflows

### 2. Topological Sort for Dependencies

**Why:** Correct execution order with parallelism
- Maximizes concurrency while respecting dependencies
- Fails fast when dependencies fail
- Prevents wasted execution

**Implementation:** Kahn's algorithm for cycle detection

### 3. Bypass with Audit Logging

**Why:** Safety valve for emergencies
- Production outages need immediate fixes
- Logging ensures accountability
- Policy-level control prevents abuse

**Security:** Bypass events logged to `.git/ci-gate-bypass.log`

### 4. Three Failure Behaviors

**Why:** Different use cases need different responses

- **Block:** Production branches (main, release/*)
- **Warn:** Development branches (feature/*)
- **Allow:** Experimental workflows, gradual rollout

### 5. Workflow Path Resolution

**Why:** User convenience
- Relative paths: `test.yml` → `.github/workflows/test.yml`
- Absolute paths supported for custom locations
- Consistent with GitHub Actions conventions

## Migration Path

### From Basic Hook

1. **Install enhanced hook:**
   ```bash
   go install ./cmd/agm-hooks/pre-merge-commit
   cp $(go env GOPATH)/bin/pre-merge-commit .git/hooks/
   ```

2. **Create policy file:**
   ```bash
   cp .ci-policy.yaml.example .ci-policy.yaml
   ```

3. **Configure workflows:**
   ```yaml
   version: v1
   default:
     required_workflows:
       - test.yml  # Your existing workflow
   ```

4. **Test configuration:**
   ```bash
   .git/hooks/pre-merge-commit --dry-run -v
   ```

### Backward Compatibility

- No policy file = basic behavior (no workflows required)
- Empty required_workflows = always allow merge
- Default timeout = 30 minutes (reasonable for most workflows)

## Performance Considerations

### Sequential vs Parallel

**Sequential:**
- Time: Sum of all workflow durations
- Resources: One workflow at a time
- Best for: Limited CI resources, fast-fail desired

**Parallel:**
- Time: Max of all workflow durations
- Resources: All workflows simultaneously
- Best for: Fast feedback, abundant resources

**Example:**
```
Workflows: test.yml (5 min), lint.yml (2 min), security.yml (10 min)

Sequential: 5 + 2 + 10 = 17 minutes
Parallel:   max(5, 2, 10) = 10 minutes

Savings: 41% faster with parallel
```

### Optimization Tips

1. **Order workflows by duration (sequential):**
   ```yaml
   required_workflows:
     - lint.yml      # Fastest (seconds)
     - test.yml      # Medium (minutes)
     - e2e.yml       # Slowest (many minutes)
   ```

2. **Use parallel for independent workflows:**
   ```yaml
   parallel_execution: true
   required_workflows:
     - test.yml      # Can run together
     - lint.yml      # Can run together
   ```

3. **Use dependencies for conditional execution:**
   ```yaml
   workflow_dependencies:
     expensive.yml:
       - cheap.yml   # Only run expensive if cheap passes
   ```

## Troubleshooting

### Common Issues

**"circular dependency detected"**
- Check `workflow_dependencies` for cycles
- Use topological ordering visualization
- Solution: Remove circular references

**"timeout_minutes exceeds maximum"**
- Maximum is 1440 (24 hours)
- Solution: Reduce timeout or optimize workflows

**"workflow file not found"**
- Verify path in `.ci-policy.yaml`
- Check file exists: `ls .github/workflows/`
- Solution: Use correct relative path

**"Docker daemon not available"**
- nektos/act requires Docker
- Solution: Start Docker daemon

### Debug Mode

```bash
# Verbose output shows:
# - Policy file location
# - Loaded configuration
# - Workflow execution progress
# - Detailed results
.git/hooks/pre-merge-commit --verbose
```

### Validation

```bash
# Test configuration without executing
.git/hooks/pre-merge-commit --dry-run
```

## Future Enhancements

### Potential Improvements

1. **Remote Policy Files:**
   - Load policies from URLs
   - Share policies across repositories
   - Centralized policy management

2. **Policy Inheritance:**
   - Base policies with overrides
   - Reduce duplication
   - Easier maintenance

3. **Conditional Workflows:**
   - Run workflows based on file changes
   - Path-based filtering
   - More efficient execution

4. **Caching:**
   - Cache workflow results
   - Skip re-running unchanged workflows
   - Faster feedback

5. **Better Pattern Matching:**
   - Full glob support (`release-*`, `v[0-9]*`)
   - Regex patterns
   - More flexible branch matching

6. **Metrics and Reporting:**
   - Track bypass frequency
   - Workflow duration trends
   - Failure rate analysis

## References

### Related Documentation
- [CI Gates User Guide](./CI_GATES.md)
- [AGM Sandbox Overview](./README.md)
- [GitHub Actions Workflows](../.github/workflows/)

### External Resources
- [nektos/act](https://github.com/nektos/act) - Local GitHub Actions runner
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Git Hooks](https://git-scm.com/book/en/v2/Customizing-Git-Git-Hooks)

### Standards
- [YAML Specification](https://yaml.org/spec/)
- [Semantic Versioning](https://semver.org/)
- [Conventional Commits](https://www.conventionalcommits.org/)

---

**Implementation Date:** 2026-03-20
**Task Reference:** Task 3.3 - Enhanced CI/CD gates for Phase 3
**Bead:** scheduling-infrastructure-consolidation-73vu
