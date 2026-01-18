package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/gemini"
)

var rootCmd = &cobra.Command{
	Use:   "gemini",
	Short: "Gemini CLI - Interact with Google Gemini API",
	Long: `gemini is a command-line interface for Google's Gemini API.

Environment Variables:
  GOOGLE_API_KEY    Required. Your Gemini API key.
                    Get one at: https://makersuite.google.com/app/apikey

V1 Limitations:
  - No session persistence between invocations
  - Each 'gemini send' is independent (no conversation context)
  - V2 will add session management and history

Examples:
  gemini create my-session
  gemini send my-session "What is the capital of France?"
  gemini history my-session
`,
}

var createCmd = &cobra.Command{
	Use:   "create <session-name>",
	Short: "Create new Gemini chat session",
	Long: `Create a new Gemini chat session.

This initializes a connection to the Gemini API and validates your API key.

V1 Note: Session is not persisted. Use the session name with 'gemini send'
for this session.`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

var sendCmd = &cobra.Command{
	Use:   "send <session-name> <message>",
	Short: "Send message to Gemini and print response",
	Long: `Send a message to the Gemini API and print the assistant's response.

V1 Note: Each invocation is independent. Conversation history is not
maintained between CLI invocations. V2 will add session persistence.`,
	Args: cobra.ExactArgs(2),
	RunE: runSend,
}

var historyCmd = &cobra.Command{
	Use:   "history <session-name>",
	Short: "Show conversation history (V1 limitation)",
	Long: `Display conversation history for a session.

V1 Note: This command shows a limitation message. History retrieval
will be implemented in V2 with session persistence.`,
	Args: cobra.ExactArgs(1),
	RunE: runHistory,
}

func runCreate(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Load API key
	apiKey, err := loadAPIKey()
	if err != nil {
		return err
	}

	// Create client
	ctx := context.Background()
	client, err := gemini.NewClient(apiKey)
	if err != nil {
		return err
	}
	defer client.Close()

	// Create session
	_, err = client.CreateSession(ctx, "gemini-pro")
	if err != nil {
		return err
	}

	// Success output
	fmt.Printf("✓ Created Gemini session: %s\n", sessionName)
	fmt.Println("  Model: gemini-pro")
	fmt.Printf("  Ready to send messages with: gemini send %s \"your message\"\n", sessionName)

	return nil
}

func runSend(cmd *cobra.Command, args []string) error {
	_ = args[0] // sessionName unused in V1 (no persistence)
	message := args[1]

	if message == "" {
		return &gemini.UserError{Message: "message cannot be empty"}
	}

	// Load API key
	apiKey, err := loadAPIKey()
	if err != nil {
		return err
	}

	// Create client
	ctx := context.Background()
	client, err := gemini.NewClient(apiKey)
	if err != nil {
		return err
	}
	defer client.Close()

	// Create session (V1: new session each time, no history)
	session, err := client.CreateSession(ctx, "gemini-pro")
	if err != nil {
		return err
	}

	// Send message
	response, err := client.SendMessage(ctx, session, message)
	if err != nil {
		return err
	}

	// Output response
	fmt.Printf("Assistant: %s\n", response)

	return nil
}

func runHistory(cmd *cobra.Command, args []string) error {
	_ = args[0] // sessionName unused in V1 (no history persistence)

	// V1 limitation message
	fmt.Println("ℹ History not available (V1 limitation)")
	fmt.Println()
	fmt.Println("Each 'gemini send' invocation is independent in V1.")
	fmt.Println("Conversation history is not persisted between CLI invocations.")
	fmt.Println()
	fmt.Println("V2 will add session persistence and history retrieval.")

	return nil
}

func loadAPIKey() (string, error) {
	apiKey := os.Getenv("GOOGLE_API_KEY")

	if apiKey == "" {
		return "", &gemini.UserError{
			Message: "GOOGLE_API_KEY environment variable not set",
			Usage:   "Get API key at: https://makersuite.google.com/app/apikey",
		}
	}

	return apiKey, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		// Cobra prints error to stderr
		// Determine exit code based on error type
		var userErr *gemini.UserError
		var apiErr *gemini.APIError

		if errors.As(err, &userErr) {
			os.Exit(1) // User error
		} else if errors.As(err, &apiErr) {
			os.Exit(2) // API error
		} else {
			os.Exit(1) // Default to user error
		}
	}
}

func init() {
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(historyCmd)
}
