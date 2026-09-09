//go:build unix

package main

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadContinuationReceiptKeyRejectsFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, continuationReceiptKeyFile)
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("create key FIFO: %v", err)
	}
	if _, err := readContinuationReceiptKey(path); err == nil {
		t.Fatal("continuation key FIFO was accepted")
	}
}
