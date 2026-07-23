package agent

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
)

func resolvePrivateLaunchSubmission(harness string, prepared harnessexec.PreparedCommand, submissionErr error) error {
	uncertain, err := harnessexec.ResolveSubmission(submissionErr, prepared.Cancel)
	if uncertain {
		fmt.Fprintf(os.Stderr,
			"Warning: %s launch submission acknowledgement was lost; preserving the private handoff because the command may be queued: %v\n",
			harness, submissionErr)
	}
	return err
}
