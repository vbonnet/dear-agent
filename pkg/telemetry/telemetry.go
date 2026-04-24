// Package telemetry provides a public API for telemetry event handling.
//
// This package re-exports types from internal/telemetry to enable external
// modules (like ai-tools) to implement EventListener without violating Go's
// internal package visibility rules.
//
// For P3 AGM Token Logger Plugin and future telemetry integrations.
package telemetry

import "github.com/vbonnet/dear-agent/internal/telemetry"

// EventListener is the public interface for handling telemetry events.
// External modules can implement this interface to receive event notifications.
type EventListener = telemetry.EventListener

// Event represents a telemetry event with metadata.
type Event = telemetry.Event

// Level represents telemetry event severity levels.
type Level = telemetry.Level

// Severity level constants
const (
	LevelInfo     = telemetry.LevelInfo
	LevelWarn     = telemetry.LevelWarn
	LevelError    = telemetry.LevelError
	LevelCritical = telemetry.LevelCritical
)
