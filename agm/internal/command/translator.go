// Package command provides command translation between AGM generic commands
// and agent-specific implementations.
//
// # Overview
//
// AGM (Agent Manager) needs to execute generic commands (rename session, set directory, etc.)
// across different AI agents (Claude, Gemini, GPT). Each agent has a different execution model:
//
//   - Claude: Slash commands via tmux (e.g., "/rename new-name")
//   - Gemini: API calls (e.g., UpdateConversationTitle)
//   - GPT: API calls (different endpoints)
//
// The CommandTranslator interface provides a uniform abstraction for these operations.
//
// # Usage
//
// Create a translator with dependency injection:
//
//	client := gemini.NewClient(apiKey)  // Gemini API client
//	translator := command.NewGeminiTranslator(client)
//
// Execute commands with context for timeout control:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	err := translator.RenameSession(ctx, sessionID, "new-name")
//	if errors.Is(err, command.ErrNotSupported) {
//	    // Command not supported - graceful degradation
//	} else if errors.Is(err, command.ErrAPIFailure) {
//	    // API call failed - log and retry or fail
//	} else if err != nil {
//	    // Other error (context timeout, etc.)
//	}
//
// # Error Handling
//
// The package uses sentinel errors for common cases:
//
//   - ErrNotSupported: Command not available for this agent
//   - ErrAPIFailure: API call failed (wraps original error)
//
// Use errors.Is() to check for these errors, and errors.Unwrap() to get details.
//
// # Testing
//
// Use MockGeminiClient for testing without real API calls:
//
//	mock := &command.MockGeminiClient{
//	    UpdateTitleFunc: func(ctx, id, title) error {
//	        return errors.New("simulated error")
//	    },
//	}
//	translator := command.NewGeminiTranslator(mock)
//	err := translator.RenameSession(ctx, "id", "name")  // Returns ErrAPIFailure
package command

import (
	"context"
	"errors"
)

// CommandTranslator translates generic AGM commands to agent-specific actions.
//
// Implementations must be safe for concurrent use by multiple goroutines.
// All methods accept context.Context for timeout and cancellation control.
//
// Example usage:
//
//	translator := command.NewGeminiTranslator(client)
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	err := translator.RenameSession(ctx, sessionID, "new-name")
//	if errors.Is(err, command.ErrNotSupported) {
//	    // Command not supported by this agent
//	} else if err != nil {
//	    // Other error (API failure, timeout, etc.)
//	}
type CommandTranslator interface {
	// RenameSession renames the agent session/conversation.
	//
	// Parameters:
	//   - ctx: Context for timeout and cancellation control
	//   - sessionID: Agent-specific session identifier
	//   - newName: New name for the session
	//
	// Returns:
	//   - nil on success
	//   - ErrNotSupported if agent doesn't support renaming
	//   - ErrAPIFailure (wrapped) if API call fails
	//   - context.DeadlineExceeded if context times out
	//   - context.Canceled if context is cancelled
	RenameSession(ctx context.Context, sessionID string, newName string) error

	// SetDirectory sets the working directory context for the session.
	//
	// Parameters:
	//   - ctx: Context for timeout and cancellation control
	//   - sessionID: Agent-specific session identifier
	//   - path: Absolute path to working directory
	//
	// Returns:
	//   - nil on success
	//   - ErrNotSupported if agent doesn't support directory context
	//   - ErrAPIFailure (wrapped) if API call fails
	//   - context.DeadlineExceeded if context times out
	//   - context.Canceled if context is cancelled
	SetDirectory(ctx context.Context, sessionID string, path string) error

	// RunHook executes a session initialization hook.
	//
	// Parameters:
	//   - ctx: Context for timeout and cancellation control
	//   - sessionID: Agent-specific session identifier
	//   - hook: Hook command to execute (e.g., "/agm:agm-assoc")
	//
	// Returns:
	//   - nil on success
	//   - ErrNotSupported if agent doesn't support hooks
	//   - ErrAPIFailure (wrapped) if execution fails
	//   - context.DeadlineExceeded if context times out
	//   - context.Canceled if context is cancelled
	RunHook(ctx context.Context, sessionID string, hook string) error
}

// Sentinel errors for command translation.
//
// Use errors.Is() to check for these errors:
//
//	if errors.Is(err, command.ErrNotSupported) {
//	    // Handle unsupported command
//	}
var (
	// ErrNotSupported indicates the command is not supported by this agent.
	//
	// Example: Gemini doesn't support RunHook (no terminal access).
	// Callers should handle gracefully (skip operation, log warning, etc.).
	ErrNotSupported = errors.New("command not supported by this agent")

	// ErrAPIFailure indicates the agent API call failed.
	//
	// Wraps underlying error: fmt.Errorf("%w: %v", ErrAPIFailure, originalErr)
	// Use errors.Unwrap() or %+v formatting to get original error details.
	ErrAPIFailure = errors.New("agent API call failed")
)
