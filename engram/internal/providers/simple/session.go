package simple

import (
	"context"
	"errors"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

// ErrNotImplemented indicates session operations are not yet implemented
var ErrNotImplemented = errors.New("session operations not implemented in simple provider")

// Working Memory Operations (not implemented)

// GetWorkingContext returns error (not implemented).
func (p *SimpleFileProvider) GetWorkingContext(ctx context.Context, sessionID string) (*consolidation.WorkingContext, error) {
	return nil, ErrNotImplemented
}

// UpdateWorkingContext returns error (not implemented).
func (p *SimpleFileProvider) UpdateWorkingContext(ctx context.Context, sessionID string, updates consolidation.ContextUpdate) error {
	return ErrNotImplemented
}

// Session Memory Operations (not implemented)

// GetSessionHistory returns error (not implemented).
func (p *SimpleFileProvider) GetSessionHistory(ctx context.Context, sessionID string) (*consolidation.SessionHistory, error) {
	return nil, ErrNotImplemented
}

// AppendSessionEvent returns error (not implemented).
func (p *SimpleFileProvider) AppendSessionEvent(ctx context.Context, sessionID string, event consolidation.SessionEvent) error {
	return ErrNotImplemented
}

// PersistSession returns error (not implemented).
func (p *SimpleFileProvider) PersistSession(ctx context.Context, sessionID string) error {
	return ErrNotImplemented
}
