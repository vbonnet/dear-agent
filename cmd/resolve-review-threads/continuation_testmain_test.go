package main

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Production continuation issuance fails closed on non-Unix systems until
	// owner-private key ACLs and directory durability have runtime proof. The
	// test binary opts into the lower-level platform seams so portable parsing,
	// reconciliation, and cross-compilation coverage can still execute there.
	testPlatformSupport := true
	continuationReceiptPlatformTestOverride = &testPlatformSupport
	stateRoot, err := os.MkdirTemp("", "resolve-review-threads-test-state-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create isolated continuation state: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_STATE_HOME", stateRoot); err != nil {
		fmt.Fprintf(os.Stderr, "isolate continuation state: %v\n", err)
		_ = os.RemoveAll(stateRoot)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(stateRoot)
	os.Exit(code)
}
