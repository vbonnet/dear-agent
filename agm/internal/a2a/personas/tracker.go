package personas

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Issue represents a code review issue found by a persona
type Issue struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Tracker tracks persona reviews and emits telemetry events
type Tracker struct {
	sessionID    string
	phase        string
	startTime    time.Time
	inputTokens  int
	outputTokens int
}

// NewTracker creates a new persona review tracker
func NewTracker(sessionID, phase string) *Tracker {
	return &Tracker{
		sessionID: sessionID,
		phase:     phase,
		startTime: time.Now(),
	}
}

// RecordTokens records token usage for this review
func (t *Tracker) RecordTokens(inputTokens, outputTokens int) {
	t.inputTokens += inputTokens
	t.outputTokens += outputTokens
}

// OnReviewComplete emits telemetry event when persona review completes
func (t *Tracker) OnReviewComplete(persona *Persona, issues []Issue, telemetryEnabled bool) error {
	if !telemetryEnabled {
		return nil
	}
	durationMs := int(time.Since(t.startTime).Milliseconds())
	issueCounts := t.countBySeverity(issues)
	event := map[string]any{
		"id":        generateUUID(),
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"type":      "persona.review.completed",
		"data": map[string]any{
			"sessionId": t.sessionID,
			"phase":     t.phase,
			"persona": map[string]any{
				"name":     persona.Name,
				"version":  persona.Version,
				"maturity": persona.Maturity,
				"tier":     persona.Tier,
			},
			"effectiveness": map[string]any{
				"issuesCaught":   issueCounts,
				"falsePositives": 0,
				"durationMs":     durationMs,
				"tokenCost": map[string]any{
					"input":  t.inputTokens,
					"output": t.outputTokens,
				},
			},
		},
	}
	return t.emitEvent(event)
}

func (t *Tracker) countBySeverity(issues []Issue) map[string]int {
	counts := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}
	for _, issue := range issues {
		if _, exists := counts[issue.Severity]; exists {
			counts[issue.Severity]++
		}
	}
	return counts
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (t *Tracker) emitEvent(event map[string]any) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}
	telemetryDir := filepath.Join(home, ".engram", "telemetry")
	err = os.MkdirAll(telemetryDir, 0o700)
	if err != nil {
		return fmt.Errorf("create telemetry directory: %w", err)
	}
	now := time.Now().UTC()
	filename := fmt.Sprintf("%d-%02d.jsonl", now.Year(), now.Month())
	telemetryFile := filepath.Join(telemetryDir, filename)
	file, err := os.OpenFile(telemetryFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open telemetry file: %w", err)
	}
	defer file.Close()
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = file.Write(append(eventJSON, '\n'))
	if err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
