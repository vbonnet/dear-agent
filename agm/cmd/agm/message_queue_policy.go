package main

import (
	"context"
	"errors"

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
		fallbackAdapter, _ := getStorage()
		return sendDirectly(ctx, recipientSession, senderName, messageID, formattedMessage, "", fallbackAdapter)
	}
}
