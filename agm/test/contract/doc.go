//go:build contract
// +build contract

// Package contract contains portable registry contracts and optional live
// provider probes for AGM harnesses.
//
// The active-harness registry contract always runs without credentials. Live
// Claude, OpenCode, and deprecated Gemini compatibility probes skip only when
// their own prerequisite is unavailable. Codex lifecycle behavior is covered
// by the integration-tagged isolated source-binary test because Codex is a CLI
// harness, not an OpenAI API adapter.
//
// Run contract tests:
//
//	go test -tags=contract ./agm/test/contract/...
package contract
