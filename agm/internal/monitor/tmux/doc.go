// Package tmux provides utilities for capturing and monitoring tmux pane content.
//
// This package is part of the AGM session monitoring infrastructure and provides
// low-level primitives for reading tmux session state.
//
// Key features:
//   - Capture visible pane content from tmux sessions
//   - Capture full scrollback history
//   - Query session information (windows, created time, attached status)
//   - Robust error handling for edge cases (session not found, tmux not running, etc.)
//
// Example usage:
//
//	// Capture visible content from a session
//	content, err := tmux.CapturePaneContent("my-session")
//	if err != nil {
//	    if errors.Is(err, tmux.ErrSessionNotFound) {
//	        log.Printf("Session not found")
//	    }
//	    return err
//	}
//	fmt.Println(content)
//
//	// Get session information
//	info, err := tmux.GetSessionInfo("my-session")
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Session %s has %d windows\n", info.Name, info.Windows)
//
// This package handles large content (>100KB) correctly and provides comprehensive
// error types for different failure modes.
package tmux
