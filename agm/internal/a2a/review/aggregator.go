// Package review provides review score aggregation and consensus calculation for A2A channels.
package review

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultThreshold    = 7.0
	DefaultMinReviews   = 3
	EscalationThreshold = 4.0
)

// Status represents the consensus status of a channel
type Status string

const (
	StatusConsensusReached Status = "consensus-reached"
	StatusBlocked          Status = "blocked"
	StatusEscalate         Status = "escalate"
	StatusAwaitingReview   Status = "awaiting-review"
)

// ReviewData contains review statistics for a channel
type ReviewData struct {
	ReviewCount int     `json:"review_count"`
	ReviewMean  float64 `json:"review_mean"`
	Status      Status  `json:"status"`
	Threshold   float64 `json:"threshold,omitempty"`
	Message     string  `json:"message,omitempty"`
	Timestamp   string  `json:"timestamp,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// Aggregator aggregates review scores and determines consensus status
type Aggregator struct {
	channelsDir string
	activeDir   string
}

// NewAggregator creates a new review aggregator
func NewAggregator(channelsDir string) (*Aggregator, error) {
	if channelsDir == "" {
		return nil, fmt.Errorf("channels directory cannot be empty")
	}
	if filepath.Base(channelsDir) == "active" {
		channelsDir = filepath.Dir(channelsDir)
	}
	activeDir := filepath.Join(channelsDir, "active")
	return &Aggregator{
		channelsDir: channelsDir,
		activeDir:   activeDir,
	}, nil
}

// ExtractScores extracts all review scores from channel content
func (a *Aggregator) ExtractScores(content string) []int {
	var scores []int
	pattern := regexp.MustCompile(`\*\*Review-Score\*\*:\s*(\d+)(?:/10)?`)
	matches := pattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			score, err := strconv.Atoi(match[1])
			if err == nil && score >= 1 && score <= 10 {
				scores = append(scores, score)
			}
		}
	}
	return scores
}

// CalculateMean calculates the mean review score
func (a *Aggregator) CalculateMean(scores []int) float64 {
	if len(scores) == 0 {
		return 0.0
	}
	sum := 0
	for _, score := range scores {
		sum += score
	}
	return float64(sum) / float64(len(scores))
}

// DetermineStatus applies decision logic to determine status from mean score
func (a *Aggregator) DetermineStatus(meanScore, threshold, escalationThreshold float64) Status {
	if meanScore >= threshold {
		return StatusConsensusReached
	} else if meanScore >= escalationThreshold {
		return StatusBlocked
	}
	return StatusEscalate
}

// ExtractMetadata extracts metadata from channel YAML frontmatter
func (a *Aggregator) ExtractMetadata(content string) map[string]string {
	metadata := make(map[string]string)
	pattern := regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---`)
	match := pattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return metadata
	}
	headerText := match[1]
	linePattern := regexp.MustCompile(`\*\*(.+?)\*\*:\s*(.+)`)
	lines := strings.Split(headerText, "\n")
	for _, line := range lines {
		lineMatch := linePattern.FindStringSubmatch(line)
		if len(lineMatch) > 2 {
			key := strings.TrimSpace(lineMatch[1])
			value := strings.TrimSpace(lineMatch[2])
			metadata[key] = value
		}
	}
	return metadata
}

// UpdateMetadata updates channel metadata with review statistics
func (a *Aggregator) UpdateMetadata(channelFile string, reviewData ReviewData) error {
	content, err := os.ReadFile(channelFile)
	if err != nil {
		return fmt.Errorf("read channel file: %w", err)
	}
	metadata := a.ExtractMetadata(string(content))
	metadata["Review-Count"] = strconv.Itoa(reviewData.ReviewCount)
	if reviewData.ReviewMean > 0 {
		metadata["Review-Mean"] = fmt.Sprintf("%.1f", reviewData.ReviewMean)
	} else {
		metadata["Review-Mean"] = "null"
	}
	metadata["Status"] = string(reviewData.Status)
	if reviewData.Status == StatusConsensusReached {
		if _, exists := metadata["Consensus-Timestamp"]; !exists {
			metadata["Consensus-Timestamp"] = reviewData.Timestamp
		}
	}
	var headerLines []string
	for key, value := range metadata {
		headerLines = append(headerLines, fmt.Sprintf("**%s**: %s", key, value))
	}
	newHeader := "---\n" + strings.Join(headerLines, "\n") + "\n---"
	pattern := regexp.MustCompile(`(?s)^---\s*\n.*?\n---`)
	updatedContent := pattern.ReplaceAllString(string(content), newHeader)
	err = os.WriteFile(channelFile, []byte(updatedContent), 0o600)
	if err != nil {
		return fmt.Errorf("write channel file: %w", err)
	}
	return nil
}

// CheckConsensus checks if consensus has been reached for a channel
func (a *Aggregator) CheckConsensus(channelID string, threshold float64, minReviews int) (bool, ReviewData) {
	channelFile := filepath.Join(a.activeDir, channelID+".md")
	if _, err := os.Stat(channelFile); os.IsNotExist(err) {
		return false, ReviewData{
			Error: fmt.Sprintf("Channel not found: %s", channelID),
		}
	}
	content, err := os.ReadFile(channelFile)
	if err != nil {
		return false, ReviewData{
			Error: fmt.Sprintf("Failed to read channel: %v", err),
		}
	}
	scores := a.ExtractScores(string(content))
	if len(scores) < minReviews {
		meanScore := a.CalculateMean(scores)
		return false, ReviewData{
			ReviewCount: len(scores),
			ReviewMean:  meanScore,
			Status:      StatusAwaitingReview,
			Message:     fmt.Sprintf("Need %d more review(s)", minReviews-len(scores)),
		}
	}
	meanScore := a.CalculateMean(scores)
	status := a.DetermineStatus(meanScore, threshold, EscalationThreshold)
	reviewData := ReviewData{
		ReviewCount: len(scores),
		ReviewMean:  meanScore,
		Status:      status,
		Threshold:   threshold,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
	return status == StatusConsensusReached, reviewData
}
