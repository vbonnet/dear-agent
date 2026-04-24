package main

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

var answerCmd = &cobra.Command{
	Use:   "answer <session> <response>",
	Short: "Answer a pending question from an AI session",
	Long: `Answer a pending question from an AI session.

Reads the most recent pending question for the session, marks it as answered,
and delivers the response to the session via 'agm send msg'.

The answer is:
  1. Written to the question file (status changed to "answered")
  2. Sent to the AI session as a message via 'agm send msg'

Examples:
  # Answer a question
  agm answer my-session "Use the v3 API, it has better error handling"

  # View pending questions first
  ls ~/.agm/questions/

See Also:
  agm ask - Send a question to the human operator`,
	Args: cobra.ExactArgs(2),
	RunE: runAnswer,
}

func init() {
	rootCmd.AddCommand(answerCmd)
}

func runAnswer(_ *cobra.Command, args []string) error {
	sessionName := args[0]
	response := args[1]

	// Find the most recent pending question for this session
	questionPath, q, err := FindPendingQuestion(sessionName)
	if err != nil {
		return fmt.Errorf("failed to find pending question: %w", err)
	}

	// Mark the question as answered in the file
	if err := MarkQuestionAnswered(questionPath, response); err != nil {
		return fmt.Errorf("failed to mark question as answered: %w", err)
	}

	// Format the answer message with context about the original question
	answerMsg := fmt.Sprintf("[Answer to your question: %q]\n\n%s", q.Text, response)

	// Deliver the answer via agm send msg
	cmd := exec.Command("agm", "send", "msg", sessionName,
		"--sender", "human",
		"--prompt", answerMsg,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Still mark as answered since the file was updated
		fmt.Printf("! Question marked as answered but delivery failed: %v\n", err)
		fmt.Printf("  Output: %s\n", string(output))
		fmt.Printf("  You can manually send: agm send msg %s --sender human --prompt %q\n", sessionName, answerMsg)
		return fmt.Errorf("failed to deliver answer: %w", err)
	}

	fmt.Printf("+ Answered question for session '%s'\n", sessionName)
	fmt.Printf("  Question: %s\n", q.Text)
	fmt.Printf("  File: %s\n", questionPath)

	return nil
}
