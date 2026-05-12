package integration

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// TestMain skips all tests in this package when the wayfinder-session binary
// is not on $PATH, or when -short is set. The skip used to be gated by the
// CI environment variable, but that produced two surprising behaviours: tests
// would run locally when the binary was absent (and fail at the first runCmd),
// and they would be silently skipped on machines where CI=true was set for
// unrelated reasons. Probing $PATH is the property the tests actually need.
//
// To run locally: go install ./wayfinder/cmd/wayfinder-session && go test ./...
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("Skipping: -short was set")
		os.Exit(0)
	}
	if _, err := exec.LookPath("wayfinder-session"); err != nil {
		fmt.Println("Skipping: wayfinder-session not in $PATH; install with `go install ./wayfinder/cmd/wayfinder-session`")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
