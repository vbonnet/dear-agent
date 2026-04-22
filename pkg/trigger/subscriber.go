package trigger

import (
	"context"
	"log"

	"github.com/vbonnet/dear-agent/pkg/engram"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// TriggerSubscriber subscribes to EventBus events and evaluates triggers.
type TriggerSubscriber struct {
	matcher     *TriggerMatcher
	state       *TriggerState
	injector    *FileInjector
	parser      *engram.Parser
	statePath   string // path to trigger-state.json
	projectRoot string // for FileInjector output
}

// NewTriggerSubscriber creates a new TriggerSubscriber.
func NewTriggerSubscriber(matcher *TriggerMatcher, statePath string, projectRoot string) *TriggerSubscriber {
	state, err := LoadTriggerState(statePath)
	if err != nil {
		log.Printf("trigger: failed to load state from %s, starting fresh: %v", statePath, err)
		state = NewTriggerState()
	}

	return &TriggerSubscriber{
		matcher:     matcher,
		state:       state,
		injector:    NewFileInjector(),
		parser:      engram.NewParser(),
		statePath:   statePath,
		projectRoot: projectRoot,
	}
}

// Start subscribes to relevant EventBus topics.
func (s *TriggerSubscriber) Start(bus *eventbus.LocalBus) {
	bus.Subscribe("wayfinder.*", "trigger-engine", s.handleEvent)
	bus.Subscribe("task.*", "trigger-engine", s.handleEvent)
	bus.Subscribe("trigger.evaluate", "trigger-engine", s.handleEvent)
}

// handleEvent processes an incoming event through the trigger pipeline.
func (s *TriggerSubscriber) handleEvent(ctx context.Context, event *eventbus.Event) (*eventbus.Response, error) {
	// 1. Convert eventbus.Event to TriggerEvent
	triggerEvent := TriggerEvent{
		Type: event.Type,
		Data: event.Data,
	}

	// Extract project/session IDs from event data if present
	if pid, ok := event.Data["project_id"].(string); ok {
		triggerEvent.ProjectID = pid
	}
	if sid, ok := event.Data["session_id"].(string); ok {
		triggerEvent.SessionID = sid
	}

	// 2. Call matcher.Match
	matches := s.matcher.Match(triggerEvent)
	if len(matches) == 0 {
		return nil, nil
	}

	// 3. For each match: check state, parse engram, collect results
	var collected []*engram.Engram
	var injectedPaths []string

	for _, match := range matches {
		// Check cooldown
		if !s.state.ShouldInject(match.EngramPath, match.Trigger.Cooldown) {
			continue
		}

		// Parse the engram file
		eg, err := s.parser.Parse(match.EngramPath)
		if err != nil {
			log.Printf("trigger: failed to parse engram %s: %v", match.EngramPath, err)
			continue
		}

		collected = append(collected, eg)
		injectedPaths = append(injectedPaths, match.EngramPath)
	}

	if len(collected) == 0 {
		return nil, nil
	}

	// 4. Call injector.Inject with collected engrams
	if err := s.injector.Inject(s.projectRoot, event.Type, collected); err != nil {
		log.Printf("trigger: injection failed: %v", err)
		return nil, nil
	}

	// 5. Record injections in state, save state
	for _, path := range injectedPaths {
		s.state.RecordInjection(path)
	}

	if err := s.state.Save(s.statePath); err != nil {
		log.Printf("trigger: failed to save state: %v", err)
	}

	// 6. Return nil response (fire-and-forget)
	return nil, nil
}
