package integration

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain skips all tests in this package when -short flag is used OR when
// the CI environment variable is set. These tests require the wayfinder-session
// binary to be installed in $PATH, which is not the case for the default CI run.
//
// To run locally: go install ./wayfinder/cmd/wayfinder-session && go test ./...
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() || os.Getenv("CI") != "" {
		fmt.Println("Skipping: requires wayfinder-session binary in $PATH (set CI= to run)")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
