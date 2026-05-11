package identity

import (
	"flag"
	"os"
	"testing"
)

// TestMain sets ENGRAM_USER_EMAIL so the env-var detector succeeds on CI
// runners that lack a configured git identity or GCP ADC.
func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("ENGRAM_USER_EMAIL") == "" {
		os.Setenv("ENGRAM_USER_EMAIL", "ci-tests@dear-agent.local")
	}
	os.Exit(m.Run())
}
