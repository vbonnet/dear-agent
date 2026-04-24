package integration

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain skips all tests in this package when -short flag is used.
// These tests require infrastructure (tmux, external services, etc.)
// that is not available in CI. Run without -short for local testing.
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("Skipping: requires infrastructure not available in CI")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
