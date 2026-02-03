package research_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/research"
)

// Example demonstrates basic usage of the research package
func Example() {
	// Set API key (normally done via environment variable)
	os.Setenv("GEMINI_API_KEY", "your-api-key-here")

	// Create client
	client, err := research.NewClient("")
	if err != nil {
		log.Fatal(err)
	}

	// Define research topics
	topics := []string{
		"machine learning",
		"neural networks",
		"deep learning",
	}

	// Run research with default configuration
	ctx := context.Background()
	report, err := client.RunResearch(ctx, topics, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Research completed: %d characters\n", len(report))
	// Output will vary based on actual API response
}

// Example_customConfig demonstrates using custom polling configuration
func Example_customConfig() {
	os.Setenv("GEMINI_API_KEY", "your-api-key-here")

	client, err := research.NewClient("")
	if err != nil {
		log.Fatal(err)
	}

	// Custom polling configuration
	config := &research.PollConfig{
		IntervalSeconds: 5,   // Poll every 5 seconds
		TimeoutMinutes:  120, // 2 hour timeout
	}

	topics := []string{"quantum computing", "cryptography"}
	ctx := context.Background()

	report, err := client.RunResearch(ctx, topics, config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Research report: %s\n", report[:100]+"...")
}

// Example_errorHandling demonstrates error handling with rich error types
func Example_errorHandling() {
	// Intentionally missing API key to demonstrate error handling
	os.Unsetenv("GEMINI_API_KEY")

	_, err := research.NewClient("")
	if err != nil {
		var researchErr *research.ResearchError
		if errors.As(err, &researchErr) {
			fmt.Printf("Operation: %s\n", researchErr.Operation)
			fmt.Printf("Error type: API key not set\n")
			fmt.Printf("Has troubleshooting tips: %v\n", len(researchErr.Troubleshooting) > 0)
			// Output:
			// Operation: authenticate with Deep Research API
			// Error type: API key not set
			// Has troubleshooting tips: true
		}
	}
}

// Example_separateSteps demonstrates starting and polling separately
func Example_separateSteps() {
	os.Setenv("GEMINI_API_KEY", "your-api-key-here")

	client, err := research.NewClient("")
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	topics := []string{"artificial intelligence"}

	// Step 1: Start research
	interactionID, err := client.StartResearch(ctx, topics)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Started research: %s\n", interactionID)

	// Step 2: Poll for completion (can be done later or in separate process)
	config := &research.PollConfig{
		IntervalSeconds: 10,
		TimeoutMinutes:  60,
	}

	report, err := client.PollStatus(ctx, interactionID, config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Report length: %d\n", len(report))
}
