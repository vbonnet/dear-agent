//go:build !darwin && !linux

package messages

import (
	"errors"
	"testing"
)

func TestMessageQueueStorageFailsClosedOnUnsupportedPlatform(t *testing.T) {
	storage, err := prepareMessageQueueStorage("/unsupported-home")
	if storage != nil {
		t.Fatal("prepareMessageQueueStorage() returned a capability on an unsupported platform")
	}
	if !errors.Is(err, ErrUnsafeQueueStorage) {
		t.Fatalf("prepareMessageQueueStorage() error = %v, want ErrUnsafeQueueStorage", err)
	}
}
