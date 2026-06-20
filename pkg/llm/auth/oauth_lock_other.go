//go:build !unix

package auth

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

// withCredentialsLock is unsupported off Unix: the cross-process refresh lock
// relies on POSIX flock. The supervisor mesh runs on macOS/Linux only, so this
// stub keeps the package buildable elsewhere while refusing to refresh without
// the serialization guarantee that prevents token-family poisoning.
func withCredentialsLock(_ context.Context, _ string, _ time.Duration, _ func() error) error {
	return fmt.Errorf("credentials file locking is not supported on %s", runtime.GOOS)
}
