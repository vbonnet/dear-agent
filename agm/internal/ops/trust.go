package ops

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// TrustEventType classifies the type of trust event recorded.
type TrustEventType string

const (
	TrustEventSuccess            TrustEventType = "success"
	TrustEventFalseCompletion    TrustEventType = "false_completion"
	TrustEventStall              TrustEventType = "stall"
	TrustEventErrorLoop          TrustEventType = "error_loop"
	TrustEventPermissionChurn    TrustEventType = "permission_churn"
	TrustEventQualityGateFailure TrustEventType = "quality_gate_failure"
	// TrustEventGCArchived is recorded when a session is collected by the GC
	// pass rather than archived manually. Score impact is zero — it is an
	// informational audit marker, not a quality judgement.
	TrustEventGCArchived TrustEventType = "gc_archived"
)

// ValidTrustEventTypes returns all recognized trust event types.
func ValidTrustEventTypes() []TrustEventType {
	return []TrustEventType{
		TrustEventSuccess,
		TrustEventFalseCompletion,
		TrustEventStall,
		TrustEventErrorLoop,
		TrustEventPermissionChurn,
		TrustEventQualityGateFailure,
		TrustEventGCArchived,
	}
}

// IsValidTrustEventType reports whether t is a recognized trust event type.
func IsValidTrustEventType(t string) bool {
	for _, valid := range ValidTrustEventTypes() {
		if string(valid) == t {
			return true
		}
	}
	return false
}

// trustEventDeltas returns the score delta map from SLO contracts.
func trustEventDeltas() map[TrustEventType]int {
	slo := contracts.Load()
	result := make(map[TrustEventType]int, len(slo.TrustProtocol.EventDeltas))
	for k, v := range slo.TrustProtocol.EventDeltas {
		result[TrustEventType(k)] = v
	}
	return result
}

// RecordTrustEventForSession appends a trust event for sessionName without
// requiring an OpContext. It is the low-level helper used by archive and GC
// to record events automatically.
func RecordTrustEventForSession(sessionName string, eventType TrustEventType, detail string) error {
	event := TrustEvent{
		Timestamp:   time.Now(),
		EventType:   eventType,
		Detail:      detail,
		SessionName: sessionName,
	}
	return appendTrustEvent(sessionName, event)
}

func trustBaseScore() int { return contracts.Load().TrustProtocol.BaseScore }
func trustMinScore() int  { return contracts.Load().TrustProtocol.MinScore }
func trustMaxScore() int  { return contracts.Load().TrustProtocol.MaxScore }

// TrustEvent represents a single trust event recorded for a session.
type TrustEvent struct {
	Timestamp   time.Time      `json:"timestamp"`
	EventType   TrustEventType `json:"event_type"`
	Detail      string         `json:"detail,omitempty"`
	SessionName string         `json:"session_name"`
}

// TrustRecordRequest is the input for recording a trust event.
type TrustRecordRequest struct {
	SessionName string `json:"session_name"`
	EventType   string `json:"event_type"`
	Detail      string `json:"detail,omitempty"`
}

// TrustRecordResult is the output after recording a trust event.
type TrustRecordResult struct {
	Event TrustEvent `json:"event"`
}

// TrustScoreRequest is the input for computing a trust score.
type TrustScoreRequest struct {
	SessionName string `json:"session_name"`
}

// TrustScoreBreakdown shows how many events of each type contributed.
type TrustScoreBreakdown struct {
	EventType TrustEventType `json:"event_type"`
	Count     int            `json:"count"`
	Delta     int            `json:"delta"`
}

// TrustScoreResult is the output of a trust score computation.
type TrustScoreResult struct {
	SessionName string                `json:"session_name"`
	Score       int                   `json:"score"`
	Breakdown   []TrustScoreBreakdown `json:"breakdown"`
	TotalEvents int                   `json:"total_events"`
}

// TrustHistoryRequest is the input for listing trust events.
type TrustHistoryRequest struct {
	SessionName string `json:"session_name"`
}

// TrustHistoryResult is the output of a trust history query.
type TrustHistoryResult struct {
	SessionName string       `json:"session_name"`
	Events      []TrustEvent `json:"events"`
	Total       int          `json:"total"`
}

// TrustLeaderboardEntry represents one session in the leaderboard.
type TrustLeaderboardEntry struct {
	SessionName string `json:"session_name"`
	Score       int    `json:"score"`
	TotalEvents int    `json:"total_events"`
}

// TrustLeaderboardResult is the output of the leaderboard query.
type TrustLeaderboardResult struct {
	Entries []TrustLeaderboardEntry `json:"entries"`
}

// trustDir returns the directory where trust JSONL files are stored.
func trustDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agm", "trust")
}

// trustFilePath returns the JSONL file path for a given session.
func trustFilePath(sessionName string) string {
	return filepath.Join(trustDir(), sessionName+".jsonl")
}

// TrustRecord records a trust event for a session.
func TrustRecord(_ *OpContext, req *TrustRecordRequest) (*TrustRecordResult, error) {
	if req.SessionName == "" {
		return nil, ErrInvalidInput("session_name", "session name is required")
	}
	if !IsValidTrustEventType(req.EventType) {
		valid := make([]string, len(ValidTrustEventTypes()))
		for i, t := range ValidTrustEventTypes() {
			valid[i] = string(t)
		}
		return nil, ErrInvalidInput("event_type",
			fmt.Sprintf("invalid event type %q; valid types: %v", req.EventType, valid))
	}

	event := TrustEvent{
		Timestamp:   time.Now(),
		EventType:   TrustEventType(req.EventType),
		Detail:      req.Detail,
		SessionName: req.SessionName,
	}

	if err := appendTrustEvent(req.SessionName, event); err != nil {
		return nil, ErrStorageError("trust-record", err)
	}

	// Record negative trust events to error memory for learning loop
	switch TrustEventType(req.EventType) {
	case TrustEventFalseCompletion:
		recordErrorMemory(
			"false completion: "+req.Detail,
			ErrMemCatFalseCompletion,
			"",
			"Review session logs; may indicate early termination or permission blocks",
			SourceAGMTrust,
			req.SessionName,
		)
	case TrustEventStall:
		recordErrorMemory(
			"stall: "+req.Detail,
			ErrMemCatStall,
			"",
			"Check for blocking errors or permission prompts",
			SourceAGMTrust,
			req.SessionName,
		)
	case TrustEventErrorLoop:
		recordErrorMemory(
			"error loop: "+req.Detail,
			ErrMemCatErrorLoop,
			"",
			"Send diagnostic to orchestrator or manually review error",
			SourceAGMTrust,
			req.SessionName,
		)
	case TrustEventQualityGateFailure:
		recordErrorMemory(
			"quality gate failure: "+req.Detail,
			ErrMemCatQualityGate,
			"",
			"Review gate output and fix failing checks",
			SourceAGMTrust,
			req.SessionName,
		)
	}

	return &TrustRecordResult{Event: event}, nil
}

// TrustScore computes the trust score for a session.
func TrustScore(_ *OpContext, req *TrustScoreRequest) (*TrustScoreResult, error) {
	if req.SessionName == "" {
		return nil, ErrInvalidInput("session_name", "session name is required")
	}

	events, err := readTrustEvents(req.SessionName)
	if err != nil {
		return nil, ErrStorageError("trust-score", err)
	}

	return computeScore(req.SessionName, events), nil
}

// TrustHistory returns the trust event history for a session.
func TrustHistory(_ *OpContext, req *TrustHistoryRequest) (*TrustHistoryResult, error) {
	if req.SessionName == "" {
		return nil, ErrInvalidInput("session_name", "session name is required")
	}

	events, err := readTrustEvents(req.SessionName)
	if err != nil {
		return nil, ErrStorageError("trust-history", err)
	}

	return &TrustHistoryResult{
		SessionName: req.SessionName,
		Events:      events,
		Total:       len(events),
	}, nil
}

// TrustLeaderboard returns all sessions ranked by trust score.
func TrustLeaderboard(_ *OpContext) (*TrustLeaderboardResult, error) {
	dir := trustDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &TrustLeaderboardResult{}, nil
		}
		return nil, ErrStorageError("trust-leaderboard", err)
	}

	var leaderboard []TrustLeaderboardEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".jsonl" {
			continue
		}
		sessionName := name[:len(name)-len(".jsonl")]
		events, err := readTrustEvents(sessionName)
		if err != nil {
			continue
		}
		result := computeScore(sessionName, events)
		leaderboard = append(leaderboard, TrustLeaderboardEntry{
			SessionName: sessionName,
			Score:       result.Score,
			TotalEvents: result.TotalEvents,
		})
	}

	sort.Slice(leaderboard, func(i, j int) bool {
		return leaderboard[i].Score > leaderboard[j].Score
	})

	return &TrustLeaderboardResult{Entries: leaderboard}, nil
}

// computeScore calculates the trust score from a list of events.
func computeScore(sessionName string, events []TrustEvent) *TrustScoreResult {
	counts := make(map[TrustEventType]int)
	for _, e := range events {
		counts[e.EventType]++
	}

	score := trustBaseScore()
	var breakdown []TrustScoreBreakdown
	for _, et := range ValidTrustEventTypes() {
		count := counts[et]
		if count == 0 {
			continue
		}
		delta := trustEventDeltas()[et] * count
		score += delta
		breakdown = append(breakdown, TrustScoreBreakdown{
			EventType: et,
			Count:     count,
			Delta:     delta,
		})
	}

	if score < trustMinScore() {
		score = trustMinScore()
	}
	if score > trustMaxScore() {
		score = trustMaxScore()
	}

	return &TrustScoreResult{
		SessionName: sessionName,
		Score:       score,
		Breakdown:   breakdown,
		TotalEvents: len(events),
	}
}

// appendTrustEvent appends a trust event to the session's JSONL file.
func appendTrustEvent(sessionName string, event TrustEvent) error {
	path := trustFilePath(sessionName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create trust directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open trust log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal trust event: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write trust event: %w", err)
	}

	return nil
}

// readTrustEvents reads all trust events for a session from its JSONL file.
func readTrustEvents(sessionName string) ([]TrustEvent, error) {
	path := trustFilePath(sessionName)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open trust log: %w", err)
	}
	defer f.Close()

	var events []TrustEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event TrustEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // skip malformed lines
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("read trust log: %w", err)
	}

	return events, nil
}
