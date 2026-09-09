//go:build unix

package main

import (
	"fmt"
	"os"
)

func syncContinuationReceiptKeyDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open continuation receipt key directory for sync: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("sync continuation receipt key directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close continuation receipt key directory: %w", err)
	}
	return nil
}
