package auth

import (
	"context"
	"time"
)

// contextWithTimeout is a thin seam so keychain invocations stay cancellable
// without threading a context through every call site.
func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
