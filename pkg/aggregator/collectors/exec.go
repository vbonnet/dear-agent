package collectors

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Exec is the function signature collectors call to invoke external
// tools. Tests provide a stub; production wires DefaultExec, which
// just shells out via exec.CommandContext and captures stdout.
//
// stderr is not surfaced because every collector currently treats it
// as advisory. A collector that needs to inspect stderr should use
// exec.Cmd directly rather than this helper.
type Exec func(ctx context.Context, dir string, name string, args ...string) ([]byte, error)

// DefaultExec runs the named command in dir and returns its stdout.
// On non-zero exit, stderr is appended to the returned error so
// operators see what went wrong without a debugger.
func DefaultExec(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("collectors: %s %v: exit %d: %s",
				name, args, ee.ExitCode(), string(ee.Stderr))
		}
		return out, fmt.Errorf("collectors: %s %v: %w", name, args, err)
	}
	return out, nil
}

// LookPath wraps exec.LookPath so tests can replace it. Production
// just delegates.
var LookPath = exec.LookPath
