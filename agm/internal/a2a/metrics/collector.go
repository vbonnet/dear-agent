// Package metrics provides metrics collection and tracking for A2A channels.
package metrics

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// SchemaVersion is the metrics-file schema version; TokenBudgetLimit caps per-channel tokens.
const (
	SchemaVersion    = "1.0"
	TokenBudgetLimit = 2000
)

// Metrics represents the complete metrics structure for a channel
type Metrics struct {
	SchemaVersion     string              `json:"schema_version"`
	ChannelID         string              `json:"channel_id"`
	Created           string              `json:"created"`
	Updated           string              `json:"updated"`
	TokenUsage        TokenUsage          `json:"token_usage"`
	Consensus         ConsensusMetrics    `json:"consensus"`
	ResponseTimes     ResponseTimeMetrics `json:"response_times"`
	Participants      ParticipantMetrics  `json:"participants"`
	StatusTransitions []StatusTransition  `json:"status_transitions"`
}

// TokenUsage tracks token consumption across messages
type TokenUsage struct {
	TotalTokens      int             `json:"total_tokens"`
	MessageCount     int             `json:"message_count"`
	AverageTokens    float64         `json:"average_tokens"`
	MinTokens        *int            `json:"min_tokens"`
	MaxTokens        *int            `json:"max_tokens"`
	Messages         []MessageMetric `json:"messages"`
	BudgetViolations int             `json:"budget_violations"`
}

// MessageMetric records individual message statistics
type MessageMetric struct {
	MessageNum int    `json:"message_num"`
	Tokens     int    `json:"tokens"`
	Timestamp  string `json:"timestamp"`
	AgentID    string `json:"agent_id"`
}

// ConsensusMetrics tracks consensus achievement
type ConsensusMetrics struct {
	Status                 *string   `json:"status"`
	TimeToConsensusMinutes *float64  `json:"time_to_consensus_minutes"`
	MessagesToConsensus    *int      `json:"messages_to_consensus"`
	ReviewScores           []float64 `json:"review_scores"`
	AverageScore           *float64  `json:"average_score"`
	ConsensusTimestamp     *string   `json:"consensus_timestamp"`
}

// ResponseTimeMetrics tracks message response intervals
type ResponseTimeMetrics struct {
	AverageResponseMinutes *float64           `json:"average_response_minutes"`
	MinResponseMinutes     *float64           `json:"min_response_minutes"`
	MaxResponseMinutes     *float64           `json:"max_response_minutes"`
	ResponseIntervals      []ResponseInterval `json:"response_intervals"`
}

// ResponseInterval represents time between consecutive messages
type ResponseInterval struct {
	FromMsg int     `json:"from_msg"`
	ToMsg   int     `json:"to_msg"`
	Minutes float64 `json:"minutes"`
}

// ParticipantMetrics tracks participant statistics
type ParticipantMetrics struct {
	Total  int                `json:"total"`
	Agents []AgentParticipant `json:"agents"`
}

// AgentParticipant represents per-agent statistics
type AgentParticipant struct {
	AgentID       string  `json:"agent_id"`
	MessageCount  int     `json:"message_count"`
	TotalTokens   int     `json:"total_tokens"`
	AverageTokens float64 `json:"avg_tokens"`
}

// StatusTransition records a status change event
type StatusTransition struct {
	From      *string `json:"from"`
	To        string  `json:"to"`
	Timestamp string  `json:"timestamp"`
}

// Collector manages metrics collection for A2A channels
type Collector struct {
	channelsDir string
	activeDir   string
}

// NewCollector creates a new metrics collector
func NewCollector(channelsDir string) (*Collector, error) {
	if channelsDir == "" {
		return nil, fmt.Errorf("channels directory cannot be empty")
	}
	if filepath.Base(channelsDir) == "active" {
		channelsDir = filepath.Dir(channelsDir)
	}
	activeDir := filepath.Join(channelsDir, "active")
	return &Collector{
		channelsDir: channelsDir,
		activeDir:   activeDir,
	}, nil
}

// InitializeMetrics creates initial metrics.json for a new channel
func (c *Collector) InitializeMetrics(channelID string) error {
	metricsFile := filepath.Join(c.activeDir, channelID, "metrics.json")
	err := os.MkdirAll(filepath.Dir(metricsFile), 0o700)
	if err != nil {
		return fmt.Errorf("create metrics directory: %w", err)
	}
	if _, err := os.Stat(metricsFile); err == nil {
		return fmt.Errorf("metrics already initialized for %s", channelID)
	}
	now := time.Now().Format(time.RFC3339)
	initialMetrics := Metrics{
		SchemaVersion: SchemaVersion,
		ChannelID:     channelID,
		Created:       now,
		Updated:       now,
		TokenUsage: TokenUsage{
			Messages: []MessageMetric{},
		},
		Consensus: ConsensusMetrics{
			ReviewScores: []float64{},
		},
		ResponseTimes: ResponseTimeMetrics{
			ResponseIntervals: []ResponseInterval{},
		},
		Participants: ParticipantMetrics{
			Agents: []AgentParticipant{},
		},
		StatusTransitions: []StatusTransition{},
	}
	data, err := json.MarshalIndent(initialMetrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	return os.WriteFile(metricsFile, data, 0o600)
}

// RecordMessage records message posting metrics
func (c *Collector) RecordMessage(channelID string, messageNum, tokens int, agentID string, timestamp *time.Time) error {
	metricsFile := filepath.Join(c.activeDir, channelID, "metrics.json")
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		return fmt.Errorf("metrics not initialized for %s", channelID)
	}
	ts := time.Now()
	if timestamp != nil {
		ts = *timestamp
	}
	metrics, unlock, err := c.loadMetricsLocked(metricsFile)
	if err != nil {
		return err
	}
	defer unlock()

	metrics.TokenUsage.TotalTokens += tokens
	metrics.TokenUsage.MessageCount++
	metrics.TokenUsage.AverageTokens = float64(metrics.TokenUsage.TotalTokens) / float64(metrics.TokenUsage.MessageCount)
	if metrics.TokenUsage.MinTokens == nil {
		metrics.TokenUsage.MinTokens = &tokens
	} else {
		minVal := min(*metrics.TokenUsage.MinTokens, tokens)
		metrics.TokenUsage.MinTokens = &minVal
	}
	if metrics.TokenUsage.MaxTokens == nil {
		metrics.TokenUsage.MaxTokens = &tokens
	} else {
		maxVal := max(*metrics.TokenUsage.MaxTokens, tokens)
		metrics.TokenUsage.MaxTokens = &maxVal
	}
	if tokens > TokenBudgetLimit {
		metrics.TokenUsage.BudgetViolations++
	}
	metrics.TokenUsage.Messages = append(metrics.TokenUsage.Messages, MessageMetric{
		MessageNum: messageNum,
		Tokens:     tokens,
		Timestamp:  ts.Format(time.RFC3339),
		AgentID:    agentID,
	})
	c.updateParticipants(&metrics, agentID, tokens)
	if len(metrics.TokenUsage.Messages) >= 2 {
		c.updateResponseTimes(&metrics)
	}
	metrics.Updated = time.Now().Format(time.RFC3339)
	return c.saveMetricsLocked(metricsFile, metrics)
}

// RecordStatusChange records a status transition
func (c *Collector) RecordStatusChange(channelID string, fromStatus *string, toStatus string, timestamp *time.Time) error {
	metricsFile := filepath.Join(c.activeDir, channelID, "metrics.json")
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		return fmt.Errorf("metrics not initialized for %s", channelID)
	}
	ts := time.Now()
	if timestamp != nil {
		ts = *timestamp
	}
	metrics, unlock, err := c.loadMetricsLocked(metricsFile)
	if err != nil {
		return err
	}
	defer unlock()
	metrics.StatusTransitions = append(metrics.StatusTransitions, StatusTransition{
		From:      fromStatus,
		To:        toStatus,
		Timestamp: ts.Format(time.RFC3339),
	})
	if toStatus == "consensus-reached" {
		c.calculateConsensusMetrics(&metrics)
	}
	metrics.Updated = time.Now().Format(time.RFC3339)
	return c.saveMetricsLocked(metricsFile, metrics)
}

// AddReviewScore adds a review score for consensus tracking
func (c *Collector) AddReviewScore(channelID string, score float64) error {
	metricsFile := filepath.Join(c.activeDir, channelID, "metrics.json")
	if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
		return fmt.Errorf("metrics not initialized for %s", channelID)
	}
	metrics, unlock, err := c.loadMetricsLocked(metricsFile)
	if err != nil {
		return err
	}
	defer unlock()
	metrics.Consensus.ReviewScores = append(metrics.Consensus.ReviewScores, score)
	if len(metrics.Consensus.ReviewScores) > 0 {
		sum := 0.0
		for _, s := range metrics.Consensus.ReviewScores {
			sum += s
		}
		avg := sum / float64(len(metrics.Consensus.ReviewScores))
		metrics.Consensus.AverageScore = &avg
	}
	metrics.Updated = time.Now().Format(time.RFC3339)
	return c.saveMetricsLocked(metricsFile, metrics)
}

// GetMetrics loads metrics for a channel
func (c *Collector) GetMetrics(channelID string) (*Metrics, error) {
	metricsFile := filepath.Join(c.activeDir, channelID, "metrics.json")
	data, err := os.ReadFile(metricsFile)
	if err != nil {
		return nil, fmt.Errorf("read metrics file: %w", err)
	}
	var metrics Metrics
	err = json.Unmarshal(data, &metrics)
	if err != nil {
		return nil, fmt.Errorf("unmarshal metrics: %w", err)
	}
	return &metrics, nil
}

func (c *Collector) updateParticipants(metrics *Metrics, agentID string, tokens int) {
	var agentEntry *AgentParticipant
	for i := range metrics.Participants.Agents {
		if metrics.Participants.Agents[i].AgentID == agentID {
			agentEntry = &metrics.Participants.Agents[i]
			break
		}
	}
	if agentEntry == nil {
		metrics.Participants.Agents = append(metrics.Participants.Agents, AgentParticipant{
			AgentID: agentID,
		})
		agentEntry = &metrics.Participants.Agents[len(metrics.Participants.Agents)-1]
		metrics.Participants.Total++
	}
	agentEntry.MessageCount++
	agentEntry.TotalTokens += tokens
	agentEntry.AverageTokens = float64(agentEntry.TotalTokens) / float64(agentEntry.MessageCount)
}

func (c *Collector) updateResponseTimes(metrics *Metrics) {
	messages := metrics.TokenUsage.Messages
	var intervals []ResponseInterval
	for i := 1; i < len(messages); i++ {
		prevTime, _ := time.Parse(time.RFC3339, messages[i-1].Timestamp)
		currTime, _ := time.Parse(time.RFC3339, messages[i].Timestamp)
		delta := currTime.Sub(prevTime)
		minutes := delta.Minutes()
		intervals = append(intervals, ResponseInterval{
			FromMsg: messages[i-1].MessageNum,
			ToMsg:   messages[i].MessageNum,
			Minutes: roundFloat(minutes, 1),
		})
	}
	metrics.ResponseTimes.ResponseIntervals = intervals
	if len(intervals) > 0 {
		var sum float64
		minVal := math.Inf(1)
		maxVal := math.Inf(-1)
		for _, interval := range intervals {
			sum += interval.Minutes
			minVal = math.Min(minVal, interval.Minutes)
			maxVal = math.Max(maxVal, interval.Minutes)
		}
		avg := roundFloat(sum/float64(len(intervals)), 1)
		metrics.ResponseTimes.AverageResponseMinutes = &avg
		metrics.ResponseTimes.MinResponseMinutes = &minVal
		metrics.ResponseTimes.MaxResponseMinutes = &maxVal
	}
}

func (c *Collector) calculateConsensusMetrics(metrics *Metrics) {
	if len(metrics.StatusTransitions) == 0 || len(metrics.TokenUsage.Messages) == 0 {
		return
	}
	firstMessageTime, _ := time.Parse(time.RFC3339, metrics.TokenUsage.Messages[0].Timestamp)
	var consensusTime time.Time
	for _, transition := range metrics.StatusTransitions {
		if transition.To == "consensus-reached" {
			consensusTime, _ = time.Parse(time.RFC3339, transition.Timestamp)
			break
		}
	}
	if !consensusTime.IsZero() {
		delta := consensusTime.Sub(firstMessageTime)
		minutes := roundFloat(delta.Minutes(), 1)
		status := "consensus-reached"
		messageCount := len(metrics.TokenUsage.Messages)
		consensusTS := consensusTime.Format(time.RFC3339)
		metrics.Consensus.Status = &status
		metrics.Consensus.TimeToConsensusMinutes = &minutes
		metrics.Consensus.MessagesToConsensus = &messageCount
		metrics.Consensus.ConsensusTimestamp = &consensusTS
	}
}

func (c *Collector) loadMetricsLocked(metricsFile string) (Metrics, func(), error) {
	file, err := os.OpenFile(metricsFile, os.O_RDWR, 0o600)
	if err != nil {
		return Metrics{}, nil, fmt.Errorf("open metrics file: %w", err)
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err != nil {
		_ = file.Close()
		return Metrics{}, nil, fmt.Errorf("acquire lock: %w", err)
	}
	data, err := os.ReadFile(metricsFile)
	if err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return Metrics{}, nil, fmt.Errorf("read metrics file: %w", err)
	}
	var metrics Metrics
	err = json.Unmarshal(data, &metrics)
	if err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return Metrics{}, nil, fmt.Errorf("unmarshal metrics: %w", err)
	}
	unlock := func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}
	return metrics, unlock, nil
}

func (c *Collector) saveMetricsLocked(metricsFile string, metrics Metrics) error {
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	return os.WriteFile(metricsFile, data, 0o600)
}

func roundFloat(val float64, places int) float64 {
	shift := math.Pow(10, float64(places))
	return math.Round(val*shift) / shift
}
