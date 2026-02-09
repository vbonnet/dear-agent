# GitHub Actions Workflows

This directory contains CI/CD workflows for automated testing and releases.

## Workflows

### 1. Tests (`tests.yml`)

**Trigger**: Every push to `main` and all pull requests

**Jobs**:
- **unit-tests**: Run unit tests with race detector
- **integration-tests**: Run integration tests with tmux
- **e2e-tests**: Run end-to-end testscript tests
- **lint**: Run golangci-lint for code quality
- **test-summary**: Aggregate test results

**Features**:
- Coverage reporting to Codecov
- Parallel job execution
- Cached Go modules for faster builds
- Test result aggregation

**Requirements**:
- Go 1.23
- tmux (installed automatically)

---

### 2. Contract Tests (`contract-tests.yml`)

**Trigger**:
- Push to `main` branch
- Weekly schedule (Sundays at midnight UTC)
- Manual workflow dispatch

**Jobs**:
- **contract-tests**: Run contract tests with real AI APIs

**Features**:
- Quota-limited execution (max 20 API calls)
- Graceful degradation on missing API keys
- Separate Claude and Gemini test runs
- Weekly scheduled execution to detect API changes

**Requirements**:
- Go 1.23
- tmux
- `ANTHROPIC_API_KEY` secret (for Claude tests)
- `GOOGLE_API_KEY` secret (for Gemini tests)

**Notes**:
- Tests may skip or fail due to quota limits
- Only runs on main branch for security (API keys)
- Manual trigger available via Actions tab

---

### 3. Release (`release.yml`)

**Trigger**: Push tags matching `v*.*.*` (e.g., `v1.0.0`)

**Jobs**:
- **release**: Build multi-platform binaries and create GitHub release

**Artifacts**:
- `agm-linux-amd64`
- `agm-linux-arm64`
- `agm-darwin-amd64`
- `agm-darwin-arm64`
- `agm-windows-amd64.exe`
- `checksums.txt` (SHA256 checksums)

**Features**:
- Multi-platform builds (Linux, macOS, Windows)
- Automatic release notes generation
- Checksum verification for downloads
- Runs tests before building

**Creating a Release**:
```bash
git tag v1.0.0
git push origin v1.0.0
```

---

## Secrets Configuration

Configure these secrets in GitHub repository settings:

| Secret | Purpose | Required For |
|--------|---------|--------------|
| `ANTHROPIC_API_KEY` | Claude API access | Contract tests (optional) |
| `GOOGLE_API_KEY` | Gemini API access | Contract tests (optional) |
| `CODECOV_TOKEN` | Coverage reporting | Coverage uploads (optional) |

**Note**: Missing secrets cause graceful skips, not failures.

---

## Local Testing

### Run Tests Locally (Same as CI)

```bash
# Unit tests
go test -v -race ./test/unit/...

# Integration tests
go test -v -tags=integration ./test/integration/...

# E2E tests
go test -v ./test/e2e/...

# Contract tests (requires API keys)
ANTHROPIC_API_KEY=sk-... go test -v -tags=contract ./test/contract -run TestClaudeAPI
GOOGLE_API_KEY=... go test -v -tags=contract ./test/contract -run TestGeminiAPI
```

### Run Lint Locally

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run --timeout=5m
```

---

## Workflow Status Badges

Add to README.md:

```markdown
![Tests](https://github.com/vbonnet/ai-tools/workflows/Tests/badge.svg)
![Contract Tests](https://github.com/vbonnet/ai-tools/workflows/Contract%20Tests/badge.svg)
[![codecov](https://codecov.io/gh/vbonnet/ai-tools/branch/main/graph/badge.svg)](https://codecov.io/gh/vbonnet/ai-tools)
```

---

## Debugging Failed Workflows

### Unit/Integration Test Failures

1. Check test logs in GitHub Actions
2. Reproduce locally: `go test -v ./test/...`
3. Run with race detector: `go test -race ./test/...`

### Contract Test Failures

Common causes:
- **API quota exhausted**: Expected, tests skip gracefully
- **API keys missing**: Add secrets in repository settings
- **API changes**: Update contract tests to match new API behavior

Contract test failures are **not blocking** for PRs.

### Lint Failures

1. Run locally: `golangci-lint run`
2. Fix issues: `golangci-lint run --fix`
3. Commit changes

---

## Performance Metrics

Typical workflow execution times:

| Workflow | Duration | Frequency |
|----------|----------|-----------|
| Unit Tests | ~5s | Every push/PR |
| Integration Tests | ~40s | Every push/PR |
| E2E Tests | ~10s | Every push/PR |
| Contract Tests | ~30s | Weekly + main branch |
| Release | ~2m | On version tags |

---

## Maintenance

### Updating Go Version

Update `go-version` in all workflows:

```yaml
- name: Set up Go
  uses: actions/setup-go@v5
  with:
    go-version: '1.24'  # Update version
```

### Updating Actions

Use Dependabot to keep actions updated (see `.github/dependabot.yml`).

### Adding New Test Jobs

1. Add job to `tests.yml`
2. Configure dependencies (tmux, etc.)
3. Upload coverage if applicable
4. Add to test-summary dependencies

---

## References

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Go Actions Setup](https://github.com/actions/setup-go)
- [Codecov Action](https://github.com/codecov/codecov-action)
- [golangci-lint Action](https://github.com/golangci/golangci-lint-action)
