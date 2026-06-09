// Package a2a is the agent-to-agent message bus.
//
// This file is a *stub* — the real implementation is being built in the
// a2a-spike worktree. Synchub depends on the interface declared here so the
// two packages can land in either order. When the real package arrives,
// the in-memory bus in inmem.go should move to the new package's testing
// subpackage (or stay here under a build tag); the interface should not
// change.
package a2a

import (
	"context"
	"errors"
)

// Message is the wire unit of the bus. Body is opaque to the bus; topics
// own their own encoding (JSON, protobuf, etc.).
type Message struct {
	From  string
	To    string // "" for broadcast on Topic
	Topic string
	Body  []byte
}

// Bus is what synchub depends on. The real package will satisfy this
// interface; until then, NewInMemoryBus() returns an implementation good
// enough for tests and single-process operation.
//
// Subscribe returns a channel that the caller owns the receive end of.
// The bus closes the channel when ctx is canceled or Close is called.
type Bus interface {
	Publish(ctx context.Context, m Message) error
	Subscribe(ctx context.Context, topic string) (<-chan Message, error)
	Close() error
}

// ErrBusClosed is returned by Publish/Subscribe after Close.
var ErrBusClosed = errors.New("a2a: bus closed")
