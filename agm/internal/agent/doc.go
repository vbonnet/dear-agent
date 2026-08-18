// Package agent provides concrete harness adapters, harness identity metadata,
// model routing, and shared adapter data types for AGM.
//
// # Architecture
//
// Concrete adapters expose harness-specific lifecycle mechanisms. Shared
// cross-surface lifecycle ordering belongs to internal/ops, whose consumers
// define capability-sized interfaces for the mechanisms they need.
//
//	    metadata discovery
//	           │
//	           ▼
//	   Harness interface
//	name/version/capabilities
//	           ▲
//	           │
//	concrete harness adapters
//	           │
//	           ▼
//	consumer-owned ops interfaces
//
// # Usage
//
// Implementations live in sibling *_adapter.go files. The canonical active and
// deprecated harness sets are in harnesses.go. GetHarness returns only the
// descriptive Harness contract; callers needing behavior construct the
// concrete adapter or use an operation-specific consumer interface.
//
// Example:
//
//	harness, err := GetHarness("codex-cli")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	log.Printf("%s %s", harness.Name(), harness.Version())
package agent
