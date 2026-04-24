// Package tokentracking provides Claude API token usage tracking for engram CLI.
//
// Integrates with core/internal/telemetry to automatically track token consumption
// across API calls and display session summaries.
//
// # Basic Usage
//
//	import (
//		"github.com/vbonnet/dear-agent/internal/telemetry"
//		"github.com/vbonnet/dear-agent/engram/internal/tokentracking"
//	)
//
//	collector, _ := telemetry.NewCollector(true, "~/.engram/telemetry.jsonl")
//	tracker := tokentracking.NewTokenTracker()
//	tracker.Initialize(collector)
//
//	// API responses are automatically tracked via telemetry events
//
//	tracker.DisplaySummary(os.Stderr)
//	tracker.Close()
//
// # Migration Notice
//
// This package was migrated from github.com/vbonnet/cli-token-tracking/tokentracking
// on 2026-01-06 as part of P4 Multi-Tokenizer Support implementation.
//
// Breaking changes from standalone version:
//   - TokenTracker.Initialize() now requires *telemetry.Collector parameter
//   - Event type replaced with *telemetry.Event (pointer)
//   - Level type replaced with telemetry.Level
//
// For migration guide, see: ~/src/cli-token-tracking/README.md
package tokentracking
