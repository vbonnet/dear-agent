package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LocalBus is an in-process event bus implementation.
//
// It dispatches events concurrently to matching subscribers (best-effort)
// and persists durable channel events to JSONL before sink dispatch.
type LocalBus struct {
	mu        sync.RWMutex
	handlers  map[string][]handlerEntry
	sinks     []sinkEntry
	responses map[string][]*Response

	durableDir             string // directory for durable JSONL files
	durableMu              sync.Mutex
	durableFiles           map[Channel]*os.File
	maxSubscribersPerTopic int
	maxTotalSubscribers    int
	totalSubscribers       int
	maxEventSize           int
	logger                 *slog.Logger
}

type handlerEntry struct {
	id         string
	subscriber string
	handler    Handler
}

type sinkEntry struct {
	sink   Sink
	filter *Filter
}

// Option configures a LocalBus.
type Option func(*LocalBus)

// WithMaxSubscribersPerTopic sets the per-topic subscriber limit.
func WithMaxSubscribersPerTopic(n int) Option {
	return func(b *LocalBus) { b.maxSubscribersPerTopic = n }
}

// WithMaxTotalSubscribers sets the global subscriber limit.
func WithMaxTotalSubscribers(n int) Option {
	return func(b *LocalBus) { b.maxTotalSubscribers = n }
}

// WithMaxEventSize sets the maximum event data size in bytes.
func WithMaxEventSize(n int) Option {
	return func(b *LocalBus) { b.maxEventSize = n }
}

// WithLogger sets a structured logger for the bus.
func WithLogger(logger *slog.Logger) Option {
	return func(b *LocalBus) { b.logger = logger }
}

// WithDurableDir sets the directory for durable channel JSONL files.
// Defaults to ~/.agm/events/.
func WithDurableDir(dir string) Option {
	return func(b *LocalBus) { b.durableDir = dir }
}

// NewLocalBus creates a new in-process event bus.
func NewLocalBus(opts ...Option) *LocalBus {
	homeDir, _ := os.UserHomeDir()
	b := &LocalBus{
		handlers:               make(map[string][]handlerEntry),
		sinks:                  nil,
		responses:              make(map[string][]*Response),
		durableDir:             filepath.Join(homeDir, ".agm", "events"),
		durableFiles:           make(map[Channel]*os.File),
		maxSubscribersPerTopic: 1000,
		maxTotalSubscribers:    10000,
		maxEventSize:           1024 * 1024, // 1MB
		logger:                 slog.Default(),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Emit publishes an event to all matching subscribers and sinks.
//
// For durable channels (notification, audit), the event is persisted to JSONL
// before dispatching to subscribers and sinks. For best-effort channels
// (telemetry, heartbeat), events are dispatched directly.
//
// Subscribers are invoked concurrently. Panics in handlers are recovered.
func (b *LocalBus) Emit(ctx context.Context, event *Event) error {
	if err := b.validateEventSize(event); err != nil {
		return err
	}

	// Durable channels: persist before dispatch
	if event.Channel.IsDurable() {
		if err := b.persistEvent(event); err != nil {
			b.logger.Error("failed to persist durable event",
				"type", event.Type, "channel", event.Channel, "error", err)
			// Continue with dispatch even if persistence fails
		}
	}

	// Dispatch to subscribers
	handlers := b.matchSubscribers(event.Type)

	// Cleanup responses after completion
	defer func() {
		b.mu.Lock()
		delete(b.responses, event.ID)
		b.mu.Unlock()
	}()

	var wg sync.WaitGroup
	done := make(chan struct{})

	for _, entry := range handlers {
		wg.Add(1)
		go func(e handlerEntry) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("handler panic recovered",
						"event_id", event.ID,
						"subscriber", e.subscriber,
						"panic", fmt.Sprintf("%v", r))
				}
			}()

			select {
			case <-ctx.Done():
				return
			default:
			}

			response, err := e.handler(ctx, event)
			if err != nil {
				return
			}
			if response != nil {
				b.storeResponse(event.ID, response)
			}
		}(entry)
	}

	// Dispatch to sinks (concurrent, best-effort)
	for _, se := range b.sinks {
		if se.filter != nil && !se.filter.Matches(event) {
			continue
		}
		wg.Add(1)
		go func(s Sink) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("sink panic recovered",
						"sink", s.Name(),
						"event_id", event.ID,
						"panic", fmt.Sprintf("%v", r))
				}
			}()
			if err := s.HandleEvent(ctx, event); err != nil {
				b.logger.Warn("sink error",
					"sink", s.Name(),
					"event_id", event.ID,
					"error", err)
			}
		}(se.sink)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
		return ctx.Err()
	}
}

// Subscribe registers a handler for events matching the given pattern.
// Supports wildcard patterns: "*" (all), "telemetry.*" (prefix with dot),
// "audit*" (prefix without dot), or exact match.
// Returns subscriber ID, or empty string if limit reached.
func (b *LocalBus) Subscribe(pattern string, subscriber string, handler Handler) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.maxTotalSubscribers > 0 && b.totalSubscribers >= b.maxTotalSubscribers {
		b.logger.Warn("global subscriber limit reached",
			"pattern", pattern, "subscriber", subscriber,
			"limit", b.maxTotalSubscribers)
		return ""
	}

	if b.maxSubscribersPerTopic > 0 && len(b.handlers[pattern]) >= b.maxSubscribersPerTopic {
		b.logger.Warn("per-topic subscriber limit reached",
			"pattern", pattern, "subscriber", subscriber,
			"limit", b.maxSubscribersPerTopic)
		return ""
	}

	subscriberID := uuid.New().String()
	b.handlers[pattern] = append(b.handlers[pattern], handlerEntry{
		id:         subscriberID,
		subscriber: subscriber,
		handler:    handler,
	})
	b.totalSubscribers++

	return subscriberID
}

// Unsubscribe removes a handler by subscriber ID.
func (b *LocalBus) Unsubscribe(pattern string, subscriberID string) error {
	if subscriberID == "" {
		return fmt.Errorf("subscriberID cannot be empty")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	handlers, exists := b.handlers[pattern]
	if !exists {
		return fmt.Errorf("no subscribers for pattern %s", pattern)
	}

	for i, entry := range handlers {
		if entry.id == subscriberID {
			b.handlers[pattern] = append(handlers[:i], handlers[i+1:]...)
			b.totalSubscribers--
			return nil
		}
	}

	return fmt.Errorf("subscriber %s not found for pattern %s", subscriberID, pattern)
}

// AddSink registers a sink with an optional filter.
func (b *LocalBus) AddSink(sink Sink, filter *Filter) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sinks = append(b.sinks, sinkEntry{sink: sink, filter: filter})
}

// Close releases all resources including durable files and sinks.
func (b *LocalBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers = make(map[string][]handlerEntry)
	b.responses = make(map[string][]*Response)

	b.durableMu.Lock()
	for _, f := range b.durableFiles {
		f.Close()
	}
	b.durableFiles = make(map[Channel]*os.File)
	b.durableMu.Unlock()

	var firstErr error
	for _, se := range b.sinks {
		if err := se.sink.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	b.sinks = nil

	return firstErr
}

// matchSubscribers finds all subscribers matching the given event type.
func (b *LocalBus) matchSubscribers(eventType string) []handlerEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var matched []handlerEntry
	for pattern, handlers := range b.handlers {
		if matchesPattern(eventType, pattern) {
			matched = append(matched, handlers...)
		}
	}
	return matched
}

// matchesPattern checks if an event type matches a subscription pattern.
func matchesPattern(eventType, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := pattern[:len(pattern)-2]
		return strings.HasPrefix(eventType, prefix+".")
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(eventType, prefix)
	}
	return eventType == pattern
}

// validateEventSize checks if an event's data size is within limits.
func (b *LocalBus) validateEventSize(event *Event) error {
	if b.maxEventSize <= 0 {
		return nil
	}

	size := len(event.ID) + len(event.Type) + len(event.Source)
	if event.Data != nil {
		for k, v := range event.Data {
			size += len(k) + len(fmt.Sprintf("%v", v))
		}
	}

	if size > b.maxEventSize {
		b.logger.Warn("event too large",
			"type", event.Type, "source", event.Source,
			"size", size, "limit", b.maxEventSize)
		return fmt.Errorf("event size %d exceeds limit %d", size, b.maxEventSize)
	}
	return nil
}

// persistEvent writes a durable event to its channel's JSONL file.
func (b *LocalBus) persistEvent(event *Event) error {
	b.durableMu.Lock()
	defer b.durableMu.Unlock()

	f, ok := b.durableFiles[event.Channel]
	if !ok {
		if err := os.MkdirAll(b.durableDir, 0700); err != nil {
			return fmt.Errorf("create durable dir: %w", err)
		}
		path := filepath.Join(b.durableDir, string(event.Channel)+".jsonl")
		var err error
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("open durable file: %w", err)
		}
		b.durableFiles[event.Channel] = f
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write event: %w", err)
	}

	return f.Sync()
}

// storeResponse stores a response for an event.
func (b *LocalBus) storeResponse(eventID string, response *Response) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.responses[eventID] = append(b.responses[eventID], response)
}

// Publish is an alias for Emit, provided for backward compatibility.
func (b *LocalBus) Publish(ctx context.Context, event *Event) error {
	return b.Emit(ctx, event)
}

// PublishSync dispatches an event sequentially to all matching subscribers.
// Handlers execute in registration order.
func (b *LocalBus) PublishSync(ctx context.Context, event *Event) error {
	if err := b.validateEventSize(event); err != nil {
		return err
	}

	if event.Channel.IsDurable() {
		if err := b.persistEvent(event); err != nil {
			b.logger.Error("failed to persist durable event",
				"type", event.Type, "channel", event.Channel, "error", err)
		}
	}

	handlers := b.matchSubscribers(event.Type)

	for _, entry := range handlers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		func(e handlerEntry) {
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("handler panic recovered",
						"event_id", event.ID,
						"subscriber", e.subscriber,
						"panic", fmt.Sprintf("%v", r))
				}
			}()
			response, err := e.handler(ctx, event)
			if err != nil {
				return
			}
			if response != nil {
				b.storeResponse(event.ID, response)
			}
		}(entry)
	}

	// Dispatch to sinks sequentially
	for _, se := range b.sinks {
		if se.filter != nil && !se.filter.Matches(event) {
			continue
		}
		if err := se.sink.HandleEvent(ctx, event); err != nil {
			b.logger.Warn("sink error",
				"sink", se.sink.Name(),
				"event_id", event.ID,
				"error", err)
		}
	}

	return nil
}
