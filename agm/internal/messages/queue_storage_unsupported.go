//go:build !darwin && !linux

package messages

func prepareMessageQueueStorage(string) (*messageQueueStorage, error) {
	return nil, unsafeQueueStorageError("storage platform", "is unsupported")
}

func (s *messageQueueStorage) Close() error {
	return nil
}
