# AGM Multi-Workspace Testing Suite

Quick start guide for running workspace isolation tests.

## Quick Start

```bash
# 1. Start test environment
cd main/agm
./scripts/test-workspace-isolation.sh

# 2. Create sample data (optional)
go run scripts/create-test-data.go

# 3. Run benchmarks (optional)
RUN_BENCHMARKS=true ./scripts/test-workspace-isolation.sh
```

## Files in This Suite

| File | Purpose | LOC |
|------|---------|-----|
| `workspace_isolation_test.go` | Core test suite | 651 |
| `scripts/test-workspace-isolation.sh` | Automated test runner | 257 |
| `scripts/create-test-data.go` | Test data generator | 324 |
| `WORKSPACE_ISOLATION_TEST_REPORT.md` | Detailed test report | ~2,500 words |
| `TASK-3.4-COMPLETION-REPORT.md` | Task completion summary | ~3,000 words |

## Test Coverage

### Core Tests (8)
1. ✓ Workspace name validation
2. ✓ Session creation with overlapping IDs
3. ✓ Session retrieval isolation
4. ✓ Message isolation
5. ✓ List sessions isolation
6. ✓ Tool call isolation
7. ✓ Update operation isolation
8. ✓ Delete operation isolation

### Benchmarks (4)
- GetSession
- ListSessions
- CreateMessage
- GetSessionMessages

### Edge Cases (3)
- Non-existent session
- Empty session ID
- Complex filters

## Prerequisites

### 1. Dolt Installation

```bash
# macOS
brew install dolt

# Linux
curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash

# Verify
dolt version
```

### 2. Workspace Initialization

```bash
# OSS workspace
cd ~/projects/myworkspace
dolt init

# Acme Corp workspace (if exists)
cd ~/src/ws/acme
dolt init
```

### 3. Environment Variables

```bash
export DOLT_TEST_INTEGRATION=1  # Enable integration tests
export WORKSPACE=oss             # Current workspace
export DOLT_PORT=3307            # Dolt server port
```

## Running Tests

### Automated (Recommended)

```bash
./scripts/test-workspace-isolation.sh
```

This script:
- Checks Dolt installation
- Validates workspace directories
- Starts Dolt servers automatically
- Runs all tests
- Cleans up on exit

### Manual

#### Step 1: Start Dolt Servers

```bash
# Terminal 1 - OSS workspace
cd ~/projects/myworkspace
dolt sql-server --port 3307 --host 127.0.0.1 --user root &

# Terminal 2 - Acme Corp workspace (if exists)
cd ~/src/ws/acme
dolt sql-server --port 3308 --host 127.0.0.1 --user root &
```

#### Step 2: Run Tests

```bash
# All tests
DOLT_TEST_INTEGRATION=1 go test -v ./internal/dolt

# Specific test
DOLT_TEST_INTEGRATION=1 go test -v ./internal/dolt -run TestWorkspaceIsolation

# Benchmarks
DOLT_TEST_INTEGRATION=1 go test -v ./internal/dolt -bench=. -benchtime=10s
```

## Expected Results

### Success Output

```
=== RUN   TestWorkspaceIsolation
=== RUN   TestWorkspaceIsolation/WorkspaceNames
=== RUN   TestWorkspaceIsolation/CreateSessions
=== RUN   TestWorkspaceIsolation/SessionIsolation
=== RUN   TestWorkspaceIsolation/MessageIsolation
=== RUN   TestWorkspaceIsolation/ListSessionsIsolation
    workspace_isolation_test.go:XXX: OSS workspace has 3 sessions
    workspace_isolation_test.go:XXX: Acme Corp workspace has 3 sessions
=== RUN   TestWorkspaceIsolation/ToolCallIsolation
=== RUN   TestWorkspaceIsolation/UpdateIsolation
=== RUN   TestWorkspaceIsolation/DeleteIsolation
--- PASS: TestWorkspaceIsolation (X.XXs)
    --- PASS: TestWorkspaceIsolation/WorkspaceNames (0.00s)
    --- PASS: TestWorkspaceIsolation/CreateSessions (0.01s)
    --- PASS: TestWorkspaceIsolation/SessionIsolation (0.01s)
    --- PASS: TestWorkspaceIsolation/MessageIsolation (0.02s)
    --- PASS: TestWorkspaceIsolation/ListSessionsIsolation (0.01s)
    --- PASS: TestWorkspaceIsolation/ToolCallIsolation (0.02s)
    --- PASS: TestWorkspaceIsolation/UpdateIsolation (0.01s)
    --- PASS: TestWorkspaceIsolation/DeleteIsolation (0.01s)
PASS
```

### Performance Targets

| Operation | Target | Acceptable Range |
|-----------|--------|------------------|
| GetSession | <10ms | 3-5ms |
| ListSessions | <10ms | 5-8ms |
| CreateMessage | <10ms | 2-4ms |
| GetSessionMessages | <10ms | 4-7ms |

## Security Validation

Tests include explicit security violation checks:

```go
// Example check
if ossRetrieved.Context.Project == acmeRetrieved.Context.Project {
    t.Error("SECURITY VIOLATION: OSS and Acme Corp sessions have same project path")
}
```

All tests must show:
- ✓ Zero cross-contamination
- ✓ Workspace filters applied
- ✓ Query results workspace-scoped

## Troubleshooting

### "Failed to connect to Dolt server"

```bash
# Check if server is running
ps aux | grep "dolt sql-server"

# Check logs
cat /tmp/dolt-oss.log
cat /tmp/dolt-acme.log

# Restart server
cd ~/projects/myworkspace
dolt sql-server --port 3307 --host 127.0.0.1 --user root
```

### "Port already in use"

```bash
# Find process using port
lsof -i :3307

# Kill process
kill -9 <PID>
```

### "Workspace directory not found"

```bash
# Create workspace
mkdir -p ~/projects/myworkspace
cd ~/projects/myworkspace
dolt init
```

## Creating Test Data

```bash
# Create sample sessions
go run scripts/create-test-data.go

# Clean up test data
go run scripts/create-test-data.go --clean

# Verbose output
go run scripts/create-test-data.go --verbose
```

## Verifying Isolation

### SQL Queries

```sql
-- Connect to OSS workspace
mysql -h 127.0.0.1 -P 3307 -u root

-- List OSS sessions
SELECT id, name, workspace FROM agm_sessions WHERE workspace = 'oss';

-- Verify no Acme Corp data
SELECT COUNT(*) FROM agm_sessions WHERE workspace = 'acme';
-- Expected: 0
```

```sql
-- Connect to Acme Corp workspace
mysql -h 127.0.0.1 -P 3308 -u root

-- List Acme Corp sessions
SELECT id, name, workspace FROM agm_sessions WHERE workspace = 'acme';

-- Verify no OSS data
SELECT COUNT(*) FROM agm_sessions WHERE workspace = 'oss';
-- Expected: 0
```

## CI/CD Integration

Example GitHub Actions workflow:

```yaml
name: Workspace Isolation Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      dolt-oss:
        image: dolthub/dolt-sql-server:latest
        ports:
          - 3307:3306
      dolt-acme:
        image: dolthub/dolt-sql-server:latest
        ports:
          - 3308:3306
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.24'
      - name: Run tests
        run: |
          export DOLT_TEST_INTEGRATION=1
          go test -v ./internal/dolt -run TestWorkspaceIsolation
```

## Performance Monitoring

```bash
# Run benchmarks and save results
DOLT_TEST_INTEGRATION=1 go test -v ./internal/dolt \
  -bench=BenchmarkWorkspaceQueries \
  -benchtime=10s \
  -benchmem | tee benchmark-results.txt

# Compare with baseline
benchstat baseline.txt benchmark-results.txt
```

## Documentation

- **Test Report**: `WORKSPACE_ISOLATION_TEST_REPORT.md` - Comprehensive test documentation
- **Completion Report**: `TASK-3.4-COMPLETION-REPORT.md` - Task deliverables and findings
- **Dolt Setup**: `SETUP.md` - Dolt installation and configuration
- **Storage README**: `README.md` - AGM Dolt storage implementation

## Support

For issues or questions:

1. Check logs: `/tmp/dolt-oss.log`, `/tmp/dolt-acme.log`
2. Review documentation: `WORKSPACE_ISOLATION_TEST_REPORT.md`
3. Run verbose tests: `go test -v ./internal/dolt`
4. Check Dolt status: `dolt status`, `dolt sql -q "SHOW TABLES"`

## Quick Reference

```bash
# Start servers
./scripts/test-workspace-isolation.sh

# Run all tests
DOLT_TEST_INTEGRATION=1 go test -v ./internal/dolt

# Run specific test
DOLT_TEST_INTEGRATION=1 go test -v ./internal/dolt -run TestWorkspaceIsolation

# Run benchmarks
DOLT_TEST_INTEGRATION=1 go test -v ./internal/dolt -bench=. -benchtime=10s

# Create test data
go run scripts/create-test-data.go

# Clean test data
go run scripts/create-test-data.go --clean

# Check Dolt connection
mysql -h 127.0.0.1 -P 3307 -u root -e "SELECT 1"
```

---

**Task**: 3.4 - AGM Multi-Workspace Testing (bead: oss-6xkh)
**Status**: COMPLETED
**Date**: 2026-02-19
