# Dolt Storage Setup Instructions

## Dependencies

Add the MySQL driver to go.mod:

```bash
cd main/agm
go get github.com/go-sql-driver/mysql
```

This is required for Dolt connectivity (Dolt uses MySQL wire protocol).

## Build Migration Tool

```bash
cd main/agm
go build -o bin/agm-migrate-dolt ./cmd/agm-migrate-dolt
```

## Run Tests

### Unit Tests (no Dolt server required)

```bash
go test -v ./internal/dolt -run "TestNew|TestDefaultConfig|TestBuildDSN"
```

### Integration Tests (requires Dolt server)

1. Start Dolt server:
   ```bash
   cd ~/projects/myworkspace
   dolt init  # If not already initialized
   dolt sql-server --port 3307 &
   ```

2. Run tests:
   ```bash
   export DOLT_TEST_INTEGRATION=1
   export WORKSPACE=test
   export DOLT_PORT=3307
   go test -v ./internal/dolt
   ```

## Verify Implementation

```bash
# Check all files exist
ls -la internal/dolt/
# Expected: adapter.go, migrations.go, sessions.go, messages.go, tool_calls.go, adapter_test.go, README.md

ls -la internal/dolt/migrations/
# Expected: 001_initial_schema.sql through 005_add_performance_indexes.sql

ls -la cmd/agm-migrate-dolt/
# Expected: main.go

# Run golangci-lint
golangci-lint run ./internal/dolt/...

# Run tests
go test ./internal/dolt/...
```

## Next Steps

1. Add MySQL driver dependency: `go get github.com/go-sql-driver/mysql`
2. Run unit tests to verify basic functionality
3. Set up Dolt server for integration testing
4. Run migration script on sample data
5. Integrate with AGM CLI commands
