//go:build !darwin && !linux

package messages

func prepareMessageQueueStorage(string) (*messageQueueStorage, error) {
	return nil, unsafeQueueStorageError("storage platform", "is unsupported")
}

func prepareMessageQueueStorageAtPath(string) (*messageQueueStorage, error) {
	return nil, unsafeQueueStorageError("storage platform", "is unsupported")
}

func (s *messageQueueStorage) prepareForSQLite() error {
	return unsafeQueueStorageError("storage platform", "is unsupported")
}

func (s *messageQueueStorage) verifyAfterSQLite() error {
	return unsafeQueueStorageError("storage platform", "is unsupported")
}

func (s *messageQueueStorage) Close() error {
	return nil
}
