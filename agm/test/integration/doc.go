//go:build integration
// +build integration

// Package integration contains integration tests for AGM with real tmux.
//
// Integration tests verify component interactions with real dependencies:
//   - Real tmux sessions (isolated via unique socket paths)
//   - Real file I/O (isolated via t.TempDir())
//   - Medium execution time (<5 seconds total)
//
// Run integration tests:
//
//	go test -tags=integration ./test/integration/...
//	go test -tags=integration -race ./test/integration/...
//
// Integration tests use:
//   - helpers.SetupTestServer() for isolated tmux
//   - helpers.CompareGolden() for output verification
//   - Automatic cleanup via t.Cleanup()
package integration
