package retrospective

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// PromptUserForContext prompts user for rewind reason and learnings via stdin/stdout
//
// Magnitude-based prompting:
// - Magnitude 0: Skip prompting (no-op rewind)
// - Magnitude 1+: Prompt for reason (required) and learnings (optional)
//
// Handles non-interactive environments (CI/CD) by detecting terminal.
func PromptUserForContext(magnitude int, flags RewindFlags) (*UserProvidedContext, error) {
	// Skip if magnitude 0 (no-op rewind)
	if magnitude == 0 {
		return nil, nil
	}

	// Skip if --no-prompt flag set
	if flags.NoPrompt {
		return &UserProvidedContext{
			Reason:    flags.Reason,
			Learnings: flags.Learnings,
		}, nil
	}

	// Use pre-provided values from flags if available
	if flags.Reason != "" {
		return &UserProvidedContext{
			Reason:    flags.Reason,
			Learnings: flags.Learnings,
		}, nil
	}

	// Check if stdin is a terminal (non-interactive environments like CI/CD)
	if !isTerminal() {
		fmt.Fprintln(os.Stderr, "Warning: Non-interactive environment detected, skipping prompts")
		fmt.Fprintln(os.Stderr, "Hint: Use --reason and --learnings flags to provide context in CI/CD")
		return &UserProvidedContext{}, nil
	}

	// Prompt user for reason (required for magnitude 1+)
	reason, err := promptRequired("Why did you rewind? (reason): ")
	if err != nil {
		return nil, fmt.Errorf("failed to get reason: %w", err)
	}

	// Prompt user for learnings (optional)
	learnings, err := promptOptional("What did you learn? (learnings, optional): ")
	if err != nil {
		return nil, fmt.Errorf("failed to get learnings: %w", err)
	}

	return &UserProvidedContext{
		Reason:    reason,
		Learnings: learnings,
	}, nil
}

// promptRequired prompts for required input (retries if empty)
func promptRequired(prompt string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Fprint(os.Stderr, prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input != "" {
			return input, nil
		}

		fmt.Fprintln(os.Stderr, "Error: This field is required. Please provide a value.")
	}
}

// promptOptional prompts for optional input (accepts empty)
func promptOptional(prompt string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprint(os.Stderr, prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	return strings.TrimSpace(input), nil
}

// isTerminal checks if stdin is a terminal (not a pipe or file)
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}
