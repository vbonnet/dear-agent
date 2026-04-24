// Package eventbus implements a unified event bus for inter-component communication.
//
// The event bus supports four channels with different delivery guarantees:
//   - telemetry: fire-and-forget, best-effort delivery
//   - notification: durable, persisted to JSONL before sink dispatch
//   - audit: durable, persisted to JSONL before sink dispatch
//   - heartbeat: fire-and-forget, best-effort delivery
//
// Events are routed by type prefix (e.g., "telemetry.*", "audit.session.end").
// Subscribers can use wildcard patterns for flexible matching.
//
// Sinks receive dispatched events and can write to files, logs, or external systems.
package eventbus
