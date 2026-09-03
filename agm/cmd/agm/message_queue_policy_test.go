package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

// The suppression is deliberately narrow: only an established trust violation
// may skip direct delivery. A storage outage that merely made the queue
// unavailable must still fall back, or a transient descriptor or space
// shortage would silently drop the message.
func TestHandleQueueConstructionErrorSuppressesOnlyTrustFailures(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantFallback bool
	}{
		{
			name:         "trust violation is returned without fallback",
			err:          fmt.Errorf("%w: main database has an unexpected owner", messages.ErrUnsafeQueueStorage),
			wantFallback: false,
		},
		{
			name:         "availability failure falls back",
			err:          fmt.Errorf("%w: main database: %w", messages.ErrQueueStorageUnavailable, errors.New("too many open files")),
			wantFallback: true,
		},
		{
			name:         "unclassified failure falls back",
			err:          errors.New("open message_queue.db: database is locked"),
			wantFallback: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			sentinel := errors.New("fallback ran")
			got := handleQueueConstructionError(tc.err, func() error {
				called = true
				return sentinel
			})

			if called != tc.wantFallback {
				t.Fatalf("fallback called = %v, want %v", called, tc.wantFallback)
			}
			if tc.wantFallback {
				if !errors.Is(got, sentinel) {
					t.Errorf("returned %v, want the fallback result", got)
				}
				return
			}
			if !errors.Is(got, messages.ErrUnsafeQueueStorage) {
				t.Errorf("returned %v, want the preserved trust identity", got)
			}
		})
	}
}
