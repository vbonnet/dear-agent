package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Prompt displays interactive prompt and returns user's choice
func Prompt(question string, options []string) (int, error) {
	fmt.Println(question)
	for i, opt := range options {
		fmt.Printf("  %d) %s\n", i+1, opt)
	}
	fmt.Print("\nChoice: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return -1, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	var choice int
	if _, err := fmt.Sscanf(input, "%d", &choice); err != nil {
		return -1, fmt.Errorf("invalid input: %s", input)
	}

	if choice < 1 || choice > len(options) {
		return -1, fmt.Errorf("choice out of range: %d", choice)
	}

	return choice - 1, nil
}

// Confirm displays yes/no confirmation prompt
func Confirm(question string) (bool, error) {
	fmt.Printf("%s (y/n): ", question)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes", nil
}
