# CI/CD Gate Configuration Guide

This guide explains how to configure and use the enhanced CI/CD gate system for the AGM sandbox.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration File](#configuration-file)
- [Policy Options](#policy-options)
- [Branch-Specific Policies](#branch-specific-policies)
- [Workflow Dependencies](#workflow-dependencies)
- [Emergency Bypass](#emergency-bypass)
- [Testing and Debugging](#testing-and-debugging)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

## Overview

The CI gate system enforces quality checks before allowing code merges. It runs GitHub Actions workflows locally using `nektos/act` and validates that all required checks pass before completing a merge.

### Key Features

- **Configurable Policies**: Define different requirements per branch
- **Multi-Workflow Support**: Run multiple workflows in sequence or parallel
- **Dependency Management**: Control workflow execution order
- **Emergency Bypass**: Allow bypasses in critical situations (with audit logging)
- **Flexible Failure Handling**: Block, warn, or allow on failures
- **Detailed Reporting**: Clear feedback on what passed/failed

## Quick Start

### 1. Create Policy Configuration

Create `.ci-policy.yaml` in your repository root:

```yaml
version: v1

default:
  required_workflows:
    - test.yml
    - lint.yml
  timeout_minutes: 30
  failure_behavior: block
  allow_bypass: false
```

### 2. Install the Hook

The pre-merge-commit hook is automatically installed when you set up the AGM sandbox. To manually install:

```bash
# Copy hook to .git/hooks/
cp cmd/agm-hooks/pre-merge-commit/pre-merge-commit .git/hooks/

# Make executable
chmod +x .git/hooks/pre-merge-commit
```

### 3. Test the Configuration

Run a dry-run to verify your configuration:

```bash
.git/hooks/pre-merge-commit --dry-run -v
```

### 4. Perform a Merge

The hook runs automatically during merges:

```bash
git checkout main
git merge feature/my-feature
# Hook runs CI checks automatically
```

## Configuration File

### File Location

The system searches for `.ci-policy.yaml` in:
1. Current directory
2. Parent directories (up to repository root)
3. If not found, uses default policy

### File Structure

```yaml
version: v1          # Policy schema version (required)

default:             # Default policy for all branches
  required_workflows: []
  optional_workflows: []
  allow_bypass: false
  timeout_minutes: 30
  failure_behavior: block
  parallel_execution: false
  require_all_passing: true
  workflow_dependencies: {}

branch_policies:     # Branch-specific overrides
  main:
    # ... policy for main branch
  feature/*:
    # ... policy for feature branches
```

## Policy Options

### `required_workflows` (array of strings)

List of workflow files that **must pass** before allowing merge.

```yaml
required_workflows:
  - test.yml
  - lint.yml
  - security.yml
```

**Notes:**
- Paths are relative to `.github/workflows/`
- Absolute paths are supported: `/path/to/workflow.yml`
- Empty list = no requirements (merge always allowed)

### `optional_workflows` (array of strings)

Workflows that run but don't block merges on failure. Useful for:
- Informational checks (coverage reports)
- Experimental workflows
- Non-critical validations

```yaml
optional_workflows:
  - coverage.yml
  - performance-benchmark.yml
```

### `allow_bypass` (boolean)

Enable/disable emergency bypass mechanism.

```yaml
allow_bypass: true   # Allow bypass via SKIP_CI_GATES=true
allow_bypass: false  # Never allow bypass (recommended for main)
```

**Security Note:** All bypass events are logged in `.git/ci-gate-bypass.log`

### `timeout_minutes` (integer)

Maximum time to wait for all workflows to complete.

```yaml
timeout_minutes: 30   # 30 minutes
timeout_minutes: 120  # 2 hours
timeout_minutes: 0    # No timeout
```

**Valid Range:** 0-1440 (0 = no limit, max = 24 hours)

### `failure_behavior` (string)

What to do when required workflows fail.

```yaml
failure_behavior: block  # Prevent merge (recommended)
failure_behavior: warn   # Show warning but allow merge
failure_behavior: allow  # Allow merge silently
```

**Options:**
- `block`: Exit non-zero, abort merge, show errors
- `warn`: Exit zero, allow merge, show warnings
- `allow`: Exit zero, allow merge, minimal output

### `parallel_execution` (boolean)

Run workflows concurrently or sequentially.

```yaml
parallel_execution: true   # Run all workflows at once (faster)
parallel_execution: false  # Run one at a time (safer, stops on first failure)
```

**Trade-offs:**
- **Parallel**: Faster, but uses more resources
- **Sequential**: Slower, but fails fast and saves resources

### `require_all_passing` (boolean)

Require all or any required workflows to pass.

```yaml
require_all_passing: true   # ALL must pass (recommended)
require_all_passing: false  # ANY can pass
```

### `workflow_dependencies` (map)

Define execution order for workflows (only with `parallel_execution: true`).

```yaml
workflow_dependencies:
  deploy.yml:
    - test.yml
    - lint.yml
  e2e.yml:
    - deploy.yml
```

**Behavior:**
- `deploy.yml` waits for `test.yml` and `lint.yml` to pass
- `e2e.yml` waits for `deploy.yml` to pass
- If a dependency fails, dependent workflows are skipped
- Circular dependencies are detected and rejected

## Branch-Specific Policies

Override the default policy for specific branches or patterns.

### Exact Branch Match

```yaml
branch_policies:
  main:
    required_workflows:
      - test.yml
      - lint.yml
      - security.yml
    failure_behavior: block
    allow_bypass: false
```

### Wildcard Patterns

**Prefix Match:**
```yaml
branch_policies:
  feature/*:
    required_workflows:
      - test.yml
    failure_behavior: warn
    allow_bypass: true
```

Matches: `feature/auth`, `feature/api`, `feature/payments`

**Suffix Match:**
```yaml
branch_policies:
  */stable:
    required_workflows:
      - test.yml
      - security.yml
    failure_behavior: block
```

Matches: `v1/stable`, `v2/stable`, `production/stable`

### Matching Precedence

1. Exact match (e.g., `main`)
2. Pattern match (e.g., `feature/*`)
3. Default policy

## Workflow Dependencies

### Simple Dependencies

```yaml
parallel_execution: true
workflow_dependencies:
  security.yml:
    - test.yml
```

**Execution Order:**
1. `test.yml` runs first
2. `security.yml` runs after `test.yml` passes

### Complex Dependencies

```yaml
parallel_execution: true
required_workflows:
  - test.yml
  - lint.yml
  - security.yml
  - deploy.yml
  - e2e.yml

workflow_dependencies:
  security.yml:
    - test.yml
    - lint.yml
  deploy.yml:
    - test.yml
    - lint.yml
    - security.yml
  e2e.yml:
    - deploy.yml
```

**Execution Plan:**
1. **Phase 1** (parallel): `test.yml`, `lint.yml`
2. **Phase 2** (after Phase 1): `security.yml`
3. **Phase 3** (after Phase 2): `deploy.yml`
4. **Phase 4** (after Phase 3): `e2e.yml`

### Dependency Validation

The system validates dependencies on load:
- ✅ No circular dependencies
- ✅ All dependencies reference existing workflows
- ❌ Rejects invalid configurations

## Emergency Bypass

### When to Use

Use bypass **only** in critical situations:
- Production outage requiring immediate fix
- CI infrastructure failure
- Critical security patch

**Do NOT use for:**
- Skipping broken tests (fix the tests!)
- Rushing features to meet deadlines
- Avoiding code review

### How to Bypass

1. **Ensure bypass is allowed** in `.ci-policy.yaml`:
   ```yaml
   allow_bypass: true
   ```

2. **Set environment variable**:
   ```bash
   SKIP_CI_GATES=true git merge feature/hotfix
   ```

3. **Verify bypass was logged**:
   ```bash
   cat .git/ci-gate-bypass.log
   ```

### Bypass Audit Log

All bypass events are logged with:
- Timestamp (ISO 8601)
- User who performed bypass
- Branch being merged
- Target branch

**Example log entry:**
```
[2026-03-20T14:30:00Z] User alice bypassed CI gates for merge to main
```

**Location:** `.git/ci-gate-bypass.log`

## Testing and Debugging

### Dry Run Mode

Test your configuration without running workflows:

```bash
.git/hooks/pre-merge-commit --dry-run
```

**Output:**
```
🧪 Dry-run mode - would execute workflows:
  - test.yml
  - lint.yml
  - security.yml
```

### Verbose Mode

Show detailed execution information:

```bash
.git/hooks/pre-merge-commit --verbose
```

**Output includes:**
- Policy file location
- Loaded configuration
- Workflow execution progress
- Detailed results

### Combine Flags

```bash
.git/hooks/pre-merge-commit --dry-run --verbose
```

### Manual Execution

Run the hook manually (outside of merge):

```bash
# Navigate to repository root
cd /path/to/repo

# Run hook
.git/hooks/pre-merge-commit -v
```

## Troubleshooting

### Common Issues

#### "CI executor binary not found: act"

**Problem:** `nektos/act` is not installed.

**Solution:**
```bash
# Install act
# macOS
brew install act

# Linux
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash

# Verify installation
act --version
```

#### "Docker daemon not available"

**Problem:** Docker is not running or not installed.

**Solution:**
```bash
# Start Docker
sudo systemctl start docker  # Linux
open -a Docker              # macOS

# Verify
docker version
```

#### "workflow file not found"

**Problem:** Workflow path is incorrect.

**Solution:**
- Verify workflow exists: `ls .github/workflows/`
- Check path in `.ci-policy.yaml`
- Use relative paths: `test.yml` not `.github/workflows/test.yml`

#### "circular dependency detected"

**Problem:** Workflow dependencies form a cycle.

**Solution:**
```yaml
# ❌ Bad: circular dependency
workflow_dependencies:
  a.yml: [b.yml]
  b.yml: [c.yml]
  c.yml: [a.yml]  # cycle!

# ✅ Good: linear dependency
workflow_dependencies:
  c.yml: [b.yml]
  b.yml: [a.yml]
```

#### "timeout_minutes exceeds maximum"

**Problem:** Timeout is set too high.

**Solution:**
```yaml
# Maximum is 1440 (24 hours)
timeout_minutes: 60  # Use reasonable value
```

### Getting Help

1. **Check configuration**: `--dry-run -v`
2. **Review logs**: Look for error messages
3. **Validate YAML**: Use online YAML validator
4. **Check workflow files**: Ensure workflows are valid
5. **Test workflows directly**: `act -W .github/workflows/test.yml`

## Best Practices

### General Recommendations

1. **Start Simple**
   ```yaml
   # Begin with minimal configuration
   default:
     required_workflows:
       - test.yml
     failure_behavior: block
   ```

2. **Use Strict Policies for Main**
   ```yaml
   branch_policies:
     main:
       required_workflows:
         - test.yml
         - lint.yml
         - security.yml
       allow_bypass: false
       failure_behavior: block
   ```

3. **Relax for Feature Branches**
   ```yaml
   branch_policies:
     feature/*:
       required_workflows:
         - test.yml
       failure_behavior: warn
       allow_bypass: true
   ```

4. **Set Reasonable Timeouts**
   ```yaml
   # Consider workflow duration + buffer
   timeout_minutes: 30  # Most projects
   timeout_minutes: 60  # Complex test suites
   timeout_minutes: 120 # Integration tests
   ```

5. **Use Dependencies Wisely**
   ```yaml
   # Only add dependencies when order matters
   workflow_dependencies:
     e2e.yml:
       - test.yml  # E2E needs unit tests to pass first
   ```

### Security Best Practices

1. **Disable Bypass for Protected Branches**
   ```yaml
   branch_policies:
     main:
       allow_bypass: false
     production:
       allow_bypass: false
   ```

2. **Monitor Bypass Log**
   ```bash
   # Review regularly
   cat .git/ci-gate-bypass.log

   # Alert on bypass events
   if [ -s .git/ci-gate-bypass.log ]; then
     echo "⚠️  CI gate bypasses detected - review required"
   fi
   ```

3. **Require All Checks for Releases**
   ```yaml
   branch_policies:
     release/*:
       required_workflows:
         - test.yml
         - lint.yml
         - security.yml
         - integration.yml
       require_all_passing: true
       failure_behavior: block
   ```

### Performance Optimization

1. **Use Parallel Execution**
   ```yaml
   parallel_execution: true
   ```
   Reduces total execution time when workflows are independent.

2. **Optimize Workflow Order**
   ```yaml
   # Fast workflows first (sequential mode)
   required_workflows:
     - lint.yml      # Fast (seconds)
     - test.yml      # Medium (minutes)
     - integration.yml  # Slow (minutes)
   ```

3. **Use Optional for Non-Critical**
   ```yaml
   optional_workflows:
     - coverage.yml      # Informational only
     - benchmark.yml     # Nice to have
   ```

## Examples

### Example 1: Simple Configuration

**Use Case:** Small project, basic checks

```yaml
version: v1

default:
  required_workflows:
    - test.yml
  timeout_minutes: 15
  failure_behavior: block
  allow_bypass: false
```

### Example 2: Multi-Branch Strategy

**Use Case:** Different requirements for different branches

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

  release/*:
    required_workflows:
      - test.yml
      - lint.yml
      - security.yml
      - e2e.yml
    timeout_minutes: 120
    failure_behavior: block
    allow_bypass: false
```

### Example 3: Complex Dependencies

**Use Case:** Workflows with specific execution order

```yaml
version: v1

default:
  required_workflows:
    - test.yml
    - lint.yml
    - security.yml
    - build.yml
    - e2e.yml
  parallel_execution: true
  timeout_minutes: 60
  failure_behavior: block
  workflow_dependencies:
    # Security scans after basic checks
    security.yml:
      - test.yml
      - lint.yml
    # Build after all checks pass
    build.yml:
      - test.yml
      - lint.yml
      - security.yml
    # E2E tests after build
    e2e.yml:
      - build.yml
```

### Example 4: Gradual Rollout

**Use Case:** Testing new checks without blocking merges

```yaml
version: v1

default:
  required_workflows:
    - test.yml
  optional_workflows:
    - new-experimental-check.yml  # Run but don't block
  failure_behavior: block

# After validation, move to required:
# default:
#   required_workflows:
#     - test.yml
#     - new-experimental-check.yml
```

## Command Reference

### Hook Commands

```bash
# Show help
.git/hooks/pre-merge-commit --help

# Dry run (show what would execute)
.git/hooks/pre-merge-commit --dry-run

# Verbose output
.git/hooks/pre-merge-commit --verbose

# Combine flags
.git/hooks/pre-merge-commit --dry-run --verbose
```

### Environment Variables

```bash
# Bypass CI gates (requires allow_bypass: true)
SKIP_CI_GATES=true git merge feature/branch

# Other values that enable bypass
SKIP_CI_GATES=1 git merge feature/branch
SKIP_CI_GATES=yes git merge feature/branch
```

### Related Commands

```bash
# Test workflow manually with act
act pull_request -W .github/workflows/test.yml

# Validate workflow syntax
act -l

# Check Docker status
docker version

# View bypass log
cat .git/ci-gate-bypass.log

# Clear bypass log (use with caution)
rm .git/ci-gate-bypass.log
```

## Migration Guide

### From Basic Hook to Enhanced System

If you're upgrading from the basic pre-merge-commit hook:

1. **Create configuration file**:
   ```bash
   cp .ci-policy.yaml.example .ci-policy.yaml
   ```

2. **Configure your workflows**:
   ```yaml
   version: v1
   default:
     required_workflows:
       - test.yml  # Your existing workflow
   ```

3. **Update hook** (if needed):
   ```bash
   go install ./cmd/agm-hooks/pre-merge-commit
   ```

4. **Test configuration**:
   ```bash
   .git/hooks/pre-merge-commit --dry-run -v
   ```

5. **Perform test merge**:
   ```bash
   git checkout -b test-ci-gates
   git checkout main
   git merge test-ci-gates --no-commit
   # Verify hook runs correctly
   git merge --abort
   ```

## Reference

### Configuration Schema

Complete YAML schema for `.ci-policy.yaml`:

```yaml
version: string  # "v1" (required)

default: Policy  # Default policy (required)

branch_policies: map[string]Policy  # Branch overrides (optional)

# Policy structure:
Policy:
  required_workflows: []string
  optional_workflows: []string
  allow_bypass: boolean
  timeout_minutes: integer
  failure_behavior: "block" | "warn" | "allow"
  parallel_execution: boolean
  require_all_passing: boolean
  workflow_dependencies: map[string][]string
```

### Exit Codes

- `0`: Success (all required workflows passed)
- `1`: Failure (required workflow failed or infrastructure error)

### File Locations

- **Policy file**: `.ci-policy.yaml` (repository root)
- **Bypass log**: `.git/ci-gate-bypass.log`
- **Hook**: `.git/hooks/pre-merge-commit`

---

**Related Documentation:**
- [AGM Sandbox Overview](./README.md)
- [Workflow Development Guide](./WORKFLOWS.md)
- [Troubleshooting Guide](./TROUBLESHOOTING.md)

**Version:** 1.0.0
**Last Updated:** 2026-03-20
