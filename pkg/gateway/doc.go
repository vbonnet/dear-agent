// Package gateway is dear-agent's transport-agnostic dispatch layer.
//
// A Gateway accepts Commands from one or more Adapters (CLI, HTTP, future
// chat platforms), routes them to handlers that wrap pkg/workflow, and
// returns Responses. It also broadcasts Events from handlers (or future
// runs.db tailers) to every subscribed adapter.
//
// See docs/adrs/ADR-017-gateway-platform-adapters.md for the design
// rationale and the rules an Adapter must follow.
//
// The package is in-process only. Multi-process topologies (gateway
// daemon + remote adapters) are explicitly out of scope.
package gateway
