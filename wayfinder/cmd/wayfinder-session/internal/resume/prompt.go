package resume

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// showMenu displays interactive menu and returns user's choice
// Implements FR2 interactive menu display requirement
func showMenu(state DirectoryState, dir string, reader io.Reader) (MenuChoice, error) {
	// Display menu header
	fmt.Printf("\nResumable project directory detected:\n")
	fmt.Printf("  Path: %s\n", dir)

	// Show what files were found
	switch state {
	case StateW0Only:
		fmt.Printf("  Contains: W0 charter file\n")
	case StateStatusOnly:
		fmt.Printf("  Contains: WAYFINDER-STATUS.md\n")
	case StateBothW0AndStatus:
		fmt.Printf("  Contains: W0 charter + WAYFINDER-STATUS.md\n")
	case StateEmpty, StateNonResumable:
		fmt.Printf("  Contains: project files\n")
	}

	// Display options (per D4 FR2 menu format)
	fmt.Printf("\nWhat would you like to do?\n")
	fmt.Printf("  [R] Resume existing project\n")
	fmt.Printf("      - Keep existing files\n")
	fmt.Printf("      - Continue where left off\n")
	fmt.Printf("\n")
	fmt.Printf("  [N] Create with different name\n")
	fmt.Printf("      - Enter new project name\n")
	fmt.Printf("      - Choose new directory\n")
	fmt.Printf("\n")
	fmt.Printf("  [A] Abort\n")
	fmt.Printf("      - Exit without changes\n")
	fmt.Printf("\n")

	// Read user choice with retry loop (per FR2 input validation)
	bufReader := bufio.NewReader(reader)
	for {
		fmt.Print("Choice [R/N/A]: ")

		input, err := readUserInput(bufReader)
		if err != nil {
			return 0, fmt.Errorf("failed to read input: %w", err)
		}

		choice, valid := validateChoice(input)
		if valid {
			return choice, nil
		}

		// Invalid input - re-prompt per FR2 requirement
		fmt.Println("Invalid choice. Please enter R, N, or A:")
	}
}

// readUserInput reads a line from input and returns trimmed string
// Uses bufio.Reader pattern from S5 research findings
func readUserInput(reader *bufio.Reader) (string, error) {
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// TrimSpace handles both Unix \n and Windows \r\n (per S5 gotcha #1)
	return strings.TrimSpace(input), nil
}

// validateChoice validates user input and returns MenuChoice
// Accepts R/r/N/n/A/a (case-insensitive per NFR4 UX requirement)
// Returns (choice, true) if valid, (0, false) if invalid
func validateChoice(input string) (MenuChoice, bool) {
	// Case-insensitive matching (per D4 FR2 input validation)
	choice := strings.ToUpper(input)

	switch choice {
	case "R":
		return ChoiceResume, true
	case "N":
		return ChoiceNew, true
	case "A":
		return ChoiceAbort, true
	default:
		return 0, false
	}
}

// getStdin returns os.Stdin as io.Reader
// Extracted for testability (dependency injection)
func getStdin() io.Reader {
	return os.Stdin
}
