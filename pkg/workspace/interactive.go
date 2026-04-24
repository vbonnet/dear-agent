package workspace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Prompter handles interactive workspace selection.
type Prompter interface {
	PromptWorkspace(workspaces []Workspace) (*Workspace, error)
	PromptConfirm(message string) (bool, error)
}

// prompter implements TTY-based prompting.
type prompter struct {
	stdin  io.Reader
	stdout io.Writer
	isTTY  bool
}

// NewPrompter creates a prompter for interactive selection.
func NewPrompter() Prompter {
	return &prompter{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		isTTY:  isTTY(os.Stdin.Fd()),
	}
}

// NewPrompterWithIO creates a prompter with custom IO (for testing).
func NewPrompterWithIO(stdin io.Reader, stdout io.Writer, isTTY bool) Prompter {
	return &prompter{
		stdin:  stdin,
		stdout: stdout,
		isTTY:  isTTY,
	}
}

// PromptWorkspace prompts user to select workspace interactively.
func (p *prompter) PromptWorkspace(workspaces []Workspace) (*Workspace, error) {
	// Check if running in TTY
	if !p.isTTY {
		return nil, fmt.Errorf("no workspace detected (non-interactive mode)")
	}

	// Filter enabled workspaces
	enabled := make([]Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		if ws.Enabled {
			enabled = append(enabled, ws)
		}
	}

	if len(enabled) == 0 {
		return nil, ErrNoEnabledWorkspaces
	}

	// Display prompt
	fmt.Fprintln(p.stdout, "No workspace detected. Please select:")
	fmt.Fprintln(p.stdout)

	for i, ws := range enabled {
		fmt.Fprintf(p.stdout, "  %d) %-12s (%s)\n", i+1, ws.Name, ws.Root)
	}
	fmt.Fprintln(p.stdout)

	// Read selection
	reader := bufio.NewReader(p.stdin)
	for {
		fmt.Fprintf(p.stdout, "Select workspace [1-%d]: ", len(enabled))

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}

		// Parse selection
		input = strings.TrimSpace(input)
		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > len(enabled) {
			fmt.Fprintln(p.stdout, "Invalid selection. Please try again.")
			continue
		}

		// Return selected workspace
		selected := enabled[idx-1]
		fmt.Fprintln(p.stdout)
		fmt.Fprintf(p.stdout, "Selected workspace: %s\n", selected.Name)
		fmt.Fprintln(p.stdout)
		fmt.Fprintf(p.stdout, "Tip: Set default with: {tool} config set-default %s\n", selected.Name)
		fmt.Fprintf(p.stdout, "     Or use flag: {tool} --workspace=%s\n", selected.Name)

		return &selected, nil
	}
}

// PromptConfirm prompts user for yes/no confirmation.
func (p *prompter) PromptConfirm(message string) (bool, error) {
	if !p.isTTY {
		return false, fmt.Errorf("cannot prompt in non-interactive mode")
	}

	reader := bufio.NewReader(p.stdin)
	fmt.Fprintf(p.stdout, "%s [y/N]: ", message)

	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes", nil
}

// isTTY checks if file descriptor is a terminal.
func isTTY(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}
