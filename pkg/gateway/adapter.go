package gateway

import "context"

// Adapter is a per-platform I/O loop. Implementations parse a
// transport-specific request format into a Command, call
// Gateway.Dispatch, and write the resulting Response back to the
// transport. Adapters that subscribe to events also forward those.
//
// One Adapter per transport instance. A process that wants to expose
// the gateway over both stdin and HTTP runs two adapters concurrently
// against the same Gateway.
type Adapter interface {
	// Name identifies the adapter in logs and metrics. Must be a short,
	// lowercase, dotless string ("cli", "http", "discord").
	Name() string

	// Run blocks until ctx is cancelled or the underlying transport
	// closes cleanly. The error returned describes why Run exited; a
	// nil error means a clean shutdown (typically: ctx.Err() received
	// while waiting for the next message).
	//
	// Run may call gw.Dispatch and gw.Subscribe freely. It must NOT
	// retain references to gw beyond its lifetime.
	Run(ctx context.Context, gw *Gateway) error
}
