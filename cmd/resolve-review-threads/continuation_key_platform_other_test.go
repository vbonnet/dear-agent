//go:build !unix

package main

import "testing"

func TestContinuationProductionPlatformDefaultIsUnsupported(t *testing.T) {
	if continuationReceiptPlatformDefaultSupported {
		t.Fatal("non-Unix production continuation support defaulted to enabled")
	}
	previousOverride := continuationReceiptPlatformTestOverride
	continuationReceiptPlatformTestOverride = nil
	t.Cleanup(func() { continuationReceiptPlatformTestOverride = previousOverride })
	if continuationReceiptPlatformIsSupported() {
		t.Fatal("non-Unix production continuation platform gate was bypassed")
	}
}
