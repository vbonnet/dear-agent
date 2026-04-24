// Package helpers provides shared test utilities for AGM testing.
//
// This package contains reusable test helpers for:
//   - Tmux session management (SetupTestServer, CapturePane)
//   - Golden file comparison (CompareGolden with corruption detection)
//   - API quota management (for contract tests)
//   - CLI execution with environment isolation (RunCLI)
//
// All helpers follow Go testing best practices:
//   - Use t.Helper() to mark helper functions
//   - Use t.Cleanup() for automatic resource cleanup
//   - Use t.TempDir() for isolated temporary directories
//   - Use t.Setenv() for environment variable isolation
//
// Example usage:
//
//	func TestAGMSession(t *testing.T) {
//	    server := helpers.SetupTestServer(t)
//	    session := helpers.CreateSession(t, server, "test-session")
//	    output := helpers.CapturePane(t, server, session.Panes[0].ID)
//	    helpers.CompareGolden(t, "testdata/golden/session-output.golden", output)
//	}
package helpers
