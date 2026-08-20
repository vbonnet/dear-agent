// Command spec-contract-hook-status performs a read-only content and trust
// audit of both operator-installed SPEC terminal-hook helper identities.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vbonnet/dear-agent/internal/hookparity"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// reportError writes msg to stderr. If that write itself fails, it falls
// back to the process's real stderr (in case the caller's writer wraps a
// broken pipe or similar) rather than silently discarding the failure.
func reportError(stderr io.Writer, err error) {
	if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
		fmt.Fprintf(os.Stderr, "%s (also failed to write to the configured stderr: %v)\n", err, writeErr)
	}
}

func run(args []string, output, stderr io.Writer) int {
	return runWithPolicy(args, output, stderr, hookparity.ProductionHelperTrustPolicy())
}

func runWithPolicy(args []string, output, stderr io.Writer, policy hookparity.HelperTrustPolicy) int {
	flags := flag.NewFlagSet("spec-contract-hook-status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	artifact := flags.String("artifact", "bin/spec-contract-hook", "built helper artifact to compare")
	deployed := flags.String("deployed", "/usr/local/libexec/dear-agent-spec-contract-hook", "stable deployed helper path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return 2
	}
	status, err := hookparity.InspectHelperDeployment(*artifact, *deployed, policy)
	if err != nil {
		reportError(stderr, fmt.Errorf("spec-contract-hook-status: %w", err))
		return 2
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(status); err != nil {
		reportError(stderr, fmt.Errorf("spec-contract-hook-status: encode result: %w", err))
		return 2
	}
	if status.Status != hookparity.HelperCurrent {
		return 1
	}
	return 0
}
