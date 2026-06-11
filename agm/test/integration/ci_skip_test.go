package integration

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() || os.Getenv("SKIP_E2E") != "" {
		fmt.Println("Skipping: requires infrastructure not available in CI")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
