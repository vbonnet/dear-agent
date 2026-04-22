package tmux

import (
	"context"
	"strings"
)

// Completion patterns that indicate /agm:agm-assoc skill has finished
var completionPatterns = []string{
	"Session association complete",
	"✓ Session associated successfully",
}

// WaitForSkillCompletion blocks until skill completion patterns are detected in output
// or the context timeout expires. Returns error on timeout, but caller should proceed
// anyway (non-fatal per requirements).
func WaitForSkillCompletion(ctx context.Context, outputChan <-chan string) error {
	for {
		select {
		case <-ctx.Done():
			// Timeout reached - return error but caller continues
			return ctx.Err()

		case line, ok := <-outputChan:
			if !ok {
				// Channel closed - treat as completion (AGM process ended)
				return nil
			}

			// Check if line matches any completion pattern
			for _, pattern := range completionPatterns {
				if strings.Contains(line, pattern) {
					return nil
				}
			}
			// Continue waiting for pattern match
		}
	}
}
