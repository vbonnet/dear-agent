package api

import (
	"context"
	"net/http"
)

// Caller identifies the human (or service) who issued a request. The
// LoginName field is what gets stamped onto audit_events.actor for
// any state change the request triggers; Display is a human-friendly
// name suitable for log lines.
type Caller struct {
	LoginName string
	Display   string
}

// Identifier maps a request to a Caller. The production implementation
// in cmd/dear-agent-api uses tsnet.Server.WhoIs; tests use a stub that
// returns a fixed Caller regardless of the request.
//
// Identify must NOT block on the network — handlers call it on every
// request and a slow Identify becomes a slow API.
type Identifier interface {
	Identify(ctx context.Context, r *http.Request) (Caller, error)
}

// IdentifierFunc adapts a plain function to the Identifier interface.
type IdentifierFunc func(ctx context.Context, r *http.Request) (Caller, error)

// Identify calls f.
func (f IdentifierFunc) Identify(ctx context.Context, r *http.Request) (Caller, error) {
	return f(ctx, r)
}

// AnonymousIdentifier is the loopback fallback. It tags every request
// with the same synthetic caller. Only safe on a private listener
// (e.g. 127.0.0.1) — see docs/adrs/ADR-013-tailscale-api.md.
func AnonymousIdentifier(loginName string) Identifier {
	if loginName == "" {
		loginName = "loopback"
	}
	c := Caller{LoginName: loginName, Display: loginName}
	return IdentifierFunc(func(ctx context.Context, r *http.Request) (Caller, error) {
		return c, nil
	})
}
