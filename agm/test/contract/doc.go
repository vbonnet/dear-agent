//go:build contract
// +build contract

// Package contract contains contract tests for AGM with real AI CLIs.
//
// Contract tests verify end-to-end workflows with real APIs:
//   - Real Claude CLI integration (requires ANTHROPIC_API_KEY)
//   - Real Gemini CLI integration (requires GOOGLE_API_KEY)
//   - Quota-limited execution (max 20 API calls per run)
//   - Slower execution (<30 seconds total)
//
// Run contract tests:
//
//	ANTHROPIC_API_KEY=sk-... go test -tags=contract ./test/contract/...
//
// Contract tests use:
//   - helpers.GetAPIQuota() for rate limiting
//   - Graceful skip on API key missing or rate limit
//   - Snapshot testing for JSON responses
//
// Contract tests run only on main branch in CI (not every PR).
package contract
