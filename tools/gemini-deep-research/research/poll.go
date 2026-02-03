package research

import (
	"context"
	"fmt"
	"log"
	"time"
)

// PollConfig holds polling configuration
type PollConfig struct {
	IntervalSeconds int // Polling interval in seconds
	TimeoutMinutes  int // Maximum timeout in minutes
}

// DefaultPollConfig returns the default polling configuration
func DefaultPollConfig() *PollConfig {
	return &PollConfig{
		IntervalSeconds: 10,
		TimeoutMinutes:  60,
	}
}

// PollStatus polls the interaction status until completion or timeout
// Returns the research report on success
func (c *Client) PollStatus(ctx context.Context, interactionID string, config *PollConfig) (string, error) {
	if config == nil {
		config = DefaultPollConfig()
	}

	timeout := time.Duration(config.TimeoutMinutes) * time.Minute
	pollInterval := time.Duration(config.IntervalSeconds) * time.Second

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()
	log.Printf("Research started (ID: %s)", interactionID)
	log.Printf("Maximum wait: %d minutes", config.TimeoutMinutes)
	log.Println()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Poll immediately first
	status, err := c.GetInteractionStatus(ctx, interactionID)
	if err != nil {
		return "", err
	}

	// Check if already completed
	if status.Status == "completed" {
		return c.extractOutput(interactionID, status)
	}

	// Poll until completion or timeout
	for {
		select {
		case <-ctx.Done():
			// Timeout or cancellation
			if ctx.Err() == context.DeadlineExceeded {
				return "", NewTimeoutError(interactionID, config.TimeoutMinutes)
			}
			return "", ctx.Err()

		case <-ticker.C:
			// Poll for status
			status, err := c.GetInteractionStatus(ctx, interactionID)
			if err != nil {
				// Log error but continue polling (may be transient)
				log.Printf("WARNING: Polling error: %v", err)
				continue
			}

			// Log status
			elapsed := time.Since(startTime)
			c.logStatus(status.Status, elapsed)

			// Check completion
			switch status.Status {
			case "completed":
				log.Println()
				log.Println("Research complete!")
				return c.extractOutput(interactionID, status)

			case "failed":
				errMsg := status.Error
				if errMsg == "" {
					errMsg = "Unknown error"
				}
				return "", fmt.Errorf("research failed: %s", errMsg)
			}
		}
	}
}

// extractOutput extracts the research report from the interaction response
func (c *Client) extractOutput(interactionID string, resp *InteractionResponse) (string, error) {
	// Try outputs array first (new format)
	if len(resp.Outputs) > 0 && resp.Outputs[0].Text != "" {
		output := resp.Outputs[0].Text
		if len(output) < 100 {
			return "", NewEmptyOutputError(interactionID)
		}
		return output, nil
	}

	// No output found
	return "", NewEmptyOutputError(interactionID)
}

// logStatus logs the current status with elapsed time
func (c *Client) logStatus(status string, elapsed time.Duration) {
	// Format elapsed time
	minutes := int(elapsed.Minutes())
	seconds := int(elapsed.Seconds()) % 60

	switch status {
	case "in_progress", "processing":
		fmt.Printf("Research status: IN_PROGRESS (elapsed: %dm %ds)\n", minutes, seconds)
	case "pending":
		fmt.Printf("Research status: PENDING (elapsed: %dm %ds)\n", minutes, seconds)
	default:
		fmt.Printf("Research status: %s (elapsed: %dm %ds)\n", status, minutes, seconds)
	}
}

// RunResearch is a convenience function that starts research and polls for completion
// This combines StartResearch and PollStatus into a single operation
func (c *Client) RunResearch(ctx context.Context, topics []string, pollConfig *PollConfig) (string, error) {
	// Start research
	interactionID, err := c.StartResearch(ctx, topics)
	if err != nil {
		return "", fmt.Errorf("start research: %w", err)
	}

	log.Printf("Created interaction: %s", interactionID)

	// Poll for completion
	report, err := c.PollStatus(ctx, interactionID, pollConfig)
	if err != nil {
		return "", fmt.Errorf("poll status: %w", err)
	}

	return report, nil
}
