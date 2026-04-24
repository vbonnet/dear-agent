// Package unit contains unit tests for AGM components.
//
// Unit tests verify individual functions and methods in isolation:
//   - No external dependencies (no real tmux, no real files)
//   - Fast execution (<1 second total for all unit tests)
//   - High coverage (>80% for core packages)
//
// Run unit tests:
//
//	go test ./test/unit/...
//	go test -cover ./test/unit/...
//	go test -race ./test/unit/...
//
// Unit tests use standard Go testing patterns:
//   - t.TempDir() for temporary files
//   - t.Setenv() for environment variable isolation
//   - Mocking/stubbing where needed (no real I/O)
package unit
