package analytics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// Parser reads and parses telemetry events
type Parser struct {
	telemetryPath string
}

// NewParser creates a parser for the given telemetry file
func NewParser(telemetryPath string) *Parser {
	return &Parser{
		telemetryPath: telemetryPath,
	}
}

// ParseAll reads all Wayfinder events from telemetry file
// Returns events grouped by session ID
func (p *Parser) ParseAll() (map[string][]ParsedEvent, error) {
	file, err := os.Open(p.telemetryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("telemetry file not found: %s", p.telemetryPath)
		}
		return nil, fmt.Errorf("failed to open telemetry file: %w", err)
	}
	defer file.Close()

	eventsBySession := make(map[string][]ParsedEvent)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Skip empty lines
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		// Parse telemetry event
		var telEvent telemetry.Event
		if err := json.Unmarshal(line, &telEvent); err != nil {
			// Log warning and skip malformed line
			fmt.Fprintf(os.Stderr, "Warning: Skipping malformed JSON at line %d: %v\n", lineNum, err)
			continue
		}

		// Filter for Wayfinder events only
		if telEvent.Agent != "wayfinder" {
			continue
		}

		// Only process EventBus publish events (where Wayfinder session data lives)
		if telEvent.Type != telemetry.EventEventBusPublish {
			continue
		}

		// Parse Wayfinder-specific data from event.Data
		parsed, err := p.parseWayfinderEvent(&telEvent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Skipping invalid Wayfinder event at line %d: %v\n", lineNum, err)
			continue
		}

		// Group by session ID
		if parsed.SessionID != "" {
			eventsBySession[parsed.SessionID] = append(eventsBySession[parsed.SessionID], *parsed)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading telemetry file: %w", err)
	}

	return eventsBySession, nil
}

// ParseSession reads events for a specific session
func (p *Parser) ParseSession(sessionID string) ([]ParsedEvent, error) {
	allEvents, err := p.ParseAll()
	if err != nil {
		return nil, err
	}

	events, ok := allEvents[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return events, nil
}

// parseWayfinderEvent converts a telemetry event to ParsedEvent
func (p *Parser) parseWayfinderEvent(telEvent *telemetry.Event) (*ParsedEvent, error) {
	// Extract session_id from event.Data
	sessionID, ok := telEvent.Data["session_id"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("missing session_id in event data")
	}

	// Extract event_topic (e.g., "wayfinder.phase.started")
	eventTopic, ok := telEvent.Data["event_topic"].(string)
	if !ok || eventTopic == "" {
		return nil, fmt.Errorf("missing event_topic in event data")
	}

	// Extract phase (optional, only present in phase events)
	phase, _ := telEvent.Data["phase"].(string)

	return &ParsedEvent{
		Type:       telEvent.Type,
		Timestamp:  telEvent.Timestamp,
		SessionID:  sessionID,
		Phase:      phase,
		EventTopic: eventTopic,
		Data:       telEvent.Data,
	}, nil
}
