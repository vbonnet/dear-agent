//go:build integration
// +build integration

// Package integration contains current production-boundary tests for AGM.
//
// Integration tests own mutable dependencies instead of using host AGM state:
//   - portable adapter parity without credentials or services
//   - real tmux sessions on unique socket paths
//   - source-built AGM and fake harness executables
//   - private filesystem and SQLite state
//
// Run integration tests:
//
//	go test -tags=integration ./agm/test/integration/...
//	go test -tags=integration -race ./agm/test/integration/...
//
// Real lifecycle tests use helpers.NewIsolatedEnvironment and automatic
// cleanup registered through testing.TB.Cleanup.
package integration
