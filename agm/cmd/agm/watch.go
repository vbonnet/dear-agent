package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

// WatchEvent represents a detected event from any source.
type WatchEvent struct {
	Type      string    // "directive", "message", "heartbeat"
	Source    string    // source description (e.g., file path, queue)
	Detail    string    // human-readable detail
	Timestamp time.Time // when the event was detected
}

// EventCallback is invoked when a WatchEvent is detected.
type EventCallback func(event WatchEvent)

// Watcher monitors multiple event sources and dispatches callbacks.
type Watcher struct {
	directiveDir  string
	pollInterval  time.Duration
	callback      EventCallback
	queueFactory  func() (QueuePoller, error) // factory for testability
	fsWatcher     *fsnotify.Watcher           // nil = use real fsnotify
	mu            sync.Mutex
	events        []WatchEvent // recorded events (for testing)
}

// QueuePoller abstracts message queue polling for testability.
type QueuePoller interface {
	GetStats() (map[string]int, error)
	Close() error
}

// WatcherOption configures a Watcher.
type WatcherOption func(*Watcher)

// WithQueueFactory overrides the default message queue factory.
func WithQueueFactory(f func() (QueuePoller, error)) WatcherOption {
	return func(w *Watcher) { w.queueFactory = f }
}

// WithFSWatcher injects an fsnotify.Watcher (for testing).
func WithFSWatcher(fsw *fsnotify.Watcher) WatcherOption {
	return func(w *Watcher) { w.fsWatcher = fsw }
}

// NewWatcher creates a Watcher with the given options.
func NewWatcher(directiveDir string, pollInterval time.Duration, callback EventCallback, opts ...WatcherOption) *Watcher {
	w := &Watcher{
		directiveDir: directiveDir,
		pollInterval: pollInterval,
		callback:     callback,
		queueFactory: defaultQueueFactory,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Events returns recorded events (thread-safe).
func (w *Watcher) Events() []WatchEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]WatchEvent, len(w.events))
	copy(out, w.events)
	return out
}

func (w *Watcher) recordAndCallback(ev WatchEvent) {
	w.mu.Lock()
	w.events = append(w.events, ev)
	w.mu.Unlock()
	if w.callback != nil {
		w.callback(ev)
	}
}

// Run starts all watchers and blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	fsw := w.fsWatcher
	if fsw == nil {
		var err error
		fsw, err = fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("failed to create filesystem watcher: %w", err)
		}
	}
	defer fsw.Close()

	if err := os.MkdirAll(w.directiveDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directive directory %s: %w", w.directiveDir, err)
	}
	if err := fsw.Add(w.directiveDir); err != nil {
		return fmt.Errorf("failed to watch %s: %w", w.directiveDir, err)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); w.watchDirectives(ctx, fsw) }()
	go func() { defer wg.Done(); w.watchQueue(ctx) }()
	go func() { defer wg.Done(); w.watchHeartbeat(ctx) }()

	<-ctx.Done()
	wg.Wait()
	return nil
}

// watchDirectives processes fsnotify events for the directive directory.
func (w *Watcher) watchDirectives(ctx context.Context, fsw *fsnotify.Watcher) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}
			w.recordAndCallback(WatchEvent{
				Type:      "directive",
				Source:    event.Name,
				Detail:    fmt.Sprintf("file %s: %s", event.Op, filepath.Base(event.Name)),
				Timestamp: time.Now(),
			})
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "fsnotify error: %v\n", err)
		}
	}
}

// watchQueue polls the message queue for new messages.
func (w *Watcher) watchQueue(ctx context.Context) {
	var lastQueued int
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queued, err := w.pollQueue()
			if err != nil {
				continue
			}
			if queued > lastQueued {
				w.recordAndCallback(WatchEvent{
					Type:      "message",
					Source:    "message-queue",
					Detail:    fmt.Sprintf("%d new message(s) queued", queued-lastQueued),
					Timestamp: time.Now(),
				})
			}
			lastQueued = queued
		}
	}
}

// watchHeartbeat emits periodic heartbeat events.
func (w *Watcher) watchHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.recordAndCallback(WatchEvent{
				Type:      "heartbeat",
				Source:    "timer",
				Detail:    "periodic heartbeat",
				Timestamp: time.Now(),
			})
		}
	}
}

func (w *Watcher) pollQueue() (int, error) {
	q, err := w.queueFactory()
	if err != nil {
		return 0, err
	}
	defer q.Close()
	stats, err := q.GetStats()
	if err != nil {
		return 0, err
	}
	return stats[messages.StatusQueued], nil
}

func defaultQueueFactory() (QueuePoller, error) {
	q, err := messages.NewMessageQueue()
	if err != nil {
		return nil, err
	}
	return q, nil
}

// --- CLI command ---

var watchDirectiveDir string
var watchHeartbeatInterval time.Duration

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch for events and trigger actions",
	Long: `Watch for filesystem and message queue events, printing each event
as it occurs. Falls back to a periodic heartbeat if no events arrive.

Event sources:
  - Directive files created/modified in the watch directory
  - New messages queued in the AGM message queue

Examples:
  agm watch
  agm watch --dir /tmp/agm-directives
  agm watch --heartbeat 2m`,
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().StringVar(&watchDirectiveDir, "dir", "/tmp/agm-directives", "Directory to watch for directive files")
	watchCmd.Flags().DurationVar(&watchHeartbeatInterval, "heartbeat", 5*time.Minute, "Heartbeat interval when no events occur")
}

func runWatch(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down watcher...")
		cancel()
	}()

	fmt.Printf("Watching for events...\n")
	fmt.Printf("  Directives: %s\n", watchDirectiveDir)
	fmt.Printf("  Heartbeat:  %s\n", watchHeartbeatInterval)
	fmt.Println()

	callback := func(ev WatchEvent) {
		fmt.Printf("[%s] %-10s  src=%s  %s\n",
			ev.Timestamp.Format("15:04:05"),
			ev.Type,
			ev.Source,
			ev.Detail,
		)
	}

	w := NewWatcher(watchDirectiveDir, watchHeartbeatInterval, callback)
	return w.Run(ctx)
}
