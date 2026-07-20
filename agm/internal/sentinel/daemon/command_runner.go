package daemon

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

const nestedCommandTimeout = 30 * time.Second

type combinedOutputRunner func(name string, args ...string) ([]byte, error)

// newSocketCommandRunner returns a bounded subprocess runner that makes an
// explicit sentinel socket authoritative for nested AGM commands as well as
// direct tmux operations.
func newSocketCommandRunner(socketPath string) combinedOutputRunner {
	return func(name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), nestedCommandTimeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, name, args...)
		cmd.SysProcAttr = procguard.ProcessGroupAttr()
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		cmd.WaitDelay = time.Second
		if socketPath != "" {
			cmd.Env = environmentWithOverride(os.Environ(), "AGM_TMUX_SOCKET", socketPath)
		}
		return cmd.CombinedOutput()
	}
}

func environmentWithOverride(environ []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}
