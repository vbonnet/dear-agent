//go:build integration

package lifecycle

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("SKIP_E2E") != "" {
		fmt.Println("Skipping: requires infrastructure not available in CI")
		os.Exit(0)
	}
	for key, value := range map[string]string{
		"ENGRAM_TEST_MODE":      "1",
		"ENGRAM_TEST_WORKSPACE": "test",
		"WORKSPACE":             "test",
		"DOLT_DATABASE":         "agm_test",
	} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "configure lifecycle test isolation: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
