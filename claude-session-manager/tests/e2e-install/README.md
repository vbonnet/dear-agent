# E2E Installation Tests

End-to-end installation tests for claude-session-manager (csm) binary across multiple Linux distributions.

## Overview

These tests verify that `csm` can be built from source and installed correctly on different Linux distributions. They run in isolated Docker containers to ensure reproducible results.

## Supported Distributions

- **Ubuntu 22.04** - Long-term support release
- **Debian 12** - Latest stable Debian release

## What is Tested

### Binary Verification Tests
1. **Command availability**: Verifies `csm` binary is in PATH
2. **Version check**: Ensures `csm version` command works correctly
3. **Installation location**: Confirms binary is installed to `~/go/bin/csm`

## Local Testing

### Prerequisites

- Docker installed and running
- Access to the repository root (includes both ai-tools and engram repos)

### Run All Tests

From the repository root (`/path/to/repos/`):

```bash
# Test Ubuntu 22.04
docker build \
  -f ai-tools/main/claude-session-manager/tests/e2e-install/Dockerfiles/ubuntu.Dockerfile \
  -t csm-test-ubuntu \
  .
docker run --rm csm-test-ubuntu

# Test Debian 12
docker build \
  -f ai-tools/main/claude-session-manager/tests/e2e-install/Dockerfiles/debian.Dockerfile \
  -t csm-test-debian \
  .
docker run --rm csm-test-debian
```

### Run Individual Test Suites

To run specific test scripts inside a running container:

```bash
# Start container with interactive shell
docker run --rm -it csm-test-ubuntu /bin/bash

# Run individual test scripts
/tmp/tests/verify-binary.sh
```

## CI Integration

Tests run automatically on:
- Push to `main` or `develop` branches
- Pull requests targeting `main` or `develop`

See `.github/workflows/e2e-install.yml` for workflow configuration.

### CI Workflow

The GitHub Actions workflow:
1. Checks out both `claude-session-manager` and `engram/core` repositories
2. Builds Docker images for each distribution
3. Runs E2E tests in parallel using matrix strategy
4. Reports results in GitHub Actions summary

## Test Structure

```
tests/e2e-install/
├── Dockerfiles/
│   ├── ubuntu.Dockerfile    # Ubuntu 22.04 test environment
│   └── debian.Dockerfile    # Debian 12 test environment
├── scripts/
│   ├── test-helpers.sh      # Shared test utility functions
│   ├── test-install.sh      # Main test orchestrator
│   └── verify-binary.sh     # Binary verification tests
└── README.md                # This file
```

## Troubleshooting

### Build Failures

**Error: `no required module provides package`**
- **Cause**: go.mod module name mismatch with import paths
- **Fix**: Ensure `go.mod` declares `module github.com/vbonnet/ai-tools/claude-session-manager`

**Error: `go.mod: no such file or directory`**
- **Cause**: Missing engram/core dependency in Docker context
- **Fix**: Build from repos root with both repos present

### Test Failures

**Error: `Command not found: csm`**
- **Cause**: Binary not built or not in PATH
- **Fix**: Check `go build` step in Dockerfile

**Error: `unknown flag: --version`**
- **Cause**: Using incorrect version flag
- **Fix**: Use `csm version` (not `csm --version`)

## Adding New Tests

To add new test cases:

1. **Add test function** to appropriate script in `scripts/`
2. **Update test orchestrator** in `test-install.sh` to call new test
3. **Test locally** on all supported distributions
4. **Update this README** with new test description

## Development Notes

### Dependencies

- **Go 1.24.0+**: Required for building csm
- **engram/core**: Internal dependency via go.mod replace directive
- Build context must include both repositories due to local replace directive

### Test Execution

Tests run as non-root user `testuser` to simulate realistic installation environment.

### Binary Location

CSM binary is installed to `~/go/bin/csm` following standard Go tooling conventions.
