//go:build contract

package contract

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/test/helpers"
)

func runNewSessionCLI(t *testing.T, name, harness string) helpers.CLIResult {
	t.Helper()
	return helpers.RunCLI(t, "session", "new", name, "--detached", "--harness", harness)
}

func runSessionCLI(t *testing.T, args ...string) helpers.CLIResult {
	t.Helper()
	return helpers.RunCLI(t, append([]string{"session"}, args...)...)
}

func runMessageCLI(t *testing.T, session, prompt string) helpers.CLIResult {
	t.Helper()
	return helpers.RunCLI(t, "send", "msg", session, "--prompt", prompt)
}

func requireOpenCodeServer(t *testing.T) {
	t.Helper()
	serverURL := strings.TrimRight(os.Getenv("OPENCODE_SERVER_URL"), "/")
	if serverURL == "" {
		t.Skip("OPENCODE_SERVER_URL not set, skipping OpenCode contract test")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serverURL + "/health") //nolint:gosec // Explicit live-contract endpoint.
	if err != nil {
		t.Skipf("OpenCode server unavailable at %s: %v", serverURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("OpenCode server health check returned HTTP %d", resp.StatusCode)
	}
}
