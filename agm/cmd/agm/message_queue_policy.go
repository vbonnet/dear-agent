package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

// handleQueueConstructionError keeps a filesystem trust refusal out of the
// availability fallback path while preserving ordinary direct-delivery
// behavior for non-security queue failures.
func handleQueueConstructionError(constructionErr error, fallback func() error) error {
	if errors.Is(constructionErr, messages.ErrUnsafeQueueStorage) {
		return constructionErr
	}
	return fallback()
}

func directQueueConstructionFallback(
	ctx context.Context,
	recipientSession string,
	senderName string,
	messageID string,
	formattedMessage string,
) func() error {
	return func() error {
		fallbackAdapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("initialize direct-delivery fallback storage: %w", err)
		}
		if fallbackAdapter != nil {
			defer func() { _ = fallbackAdapter.Close() }()
		}
		return sendDirectly(ctx, recipientSession, senderName, messageID, formattedMessage, "", fallbackAdapter)
	}
}
