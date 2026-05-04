// Package api implements the JSON HTTP control surface for dear-agent.
// It is the wire layer for the workflow- and audit-CLI surfaces — the
// same writes, served over HTTP instead of argv.
//
// The package is transport-agnostic: it depends on net/http but not on
// tsnet. The cmd/dear-agent-api binary wires this handler onto a
// tailnet listener and adapts tsnet's WhoIs into an Identifier.
//
// See docs/adrs/ADR-013-tailscale-api.md for the design.
package api
