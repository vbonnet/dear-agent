package intake

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// IntakeWatcher polls queue.jsonl for new work items.
type IntakeWatcher struct {
	filePath     string
	pollInterval time.Duration
	logger       *slog.Logger

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}

	lastModTime time.Time
	lastSize    int64
	knownIDs    map[string]bool

	handler      func(*WorkItem)
	statFunc     func(string) (os.FileInfo, error)
	readFileFunc func(string) ([]byte, error)
}

// IntakeWatcherConfig configures the watcher.
type IntakeWatcherConfig struct {
	FilePath     string
	PollInterval time.Duration
	Logger       *slog.Logger
	Handler      func(*WorkItem)
}

// NewIntakeWatcher creates a watcher with the given configuration.
func NewIntakeWatcher(cfg IntakeWatcherConfig) *IntakeWatcher {
	if cfg.FilePath == "" {
		home, _ := os.UserHomeDir()
		cfg.FilePath = filepath.Join(home, ".agm", "intake", "queue.jsonl")
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Handler == nil {
		cfg.Handler = func(*WorkItem) {}
	}
	return &IntakeWatcher{
		filePath:     cfg.FilePath,
		pollInterval: cfg.PollInterval,
		logger:       cfg.Logger,
		handler:      cfg.Handler,
		stopCh:       make(chan struct{}),
		knownIDs:     make(map[string]bool),
		statFunc:     os.Stat,
		readFileFunc: os.ReadFile,
	}
}

// Start begins polling. Blocks until Stop is called.
func (w *IntakeWatcher) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("watcher is already running")
	}
	w.running = true
	w.mu.Unlock()

	w.logger.Info("IntakeWatcher started", "file", w.filePath, "interval", w.pollInterval)
	w.initialScan()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.poll()
		case <-w.stopCh:
			w.logger.Info("IntakeWatcher stopped")
			w.mu.Lock()
			w.running = false
			w.mu.Unlock()
			return nil
		}
	}
}

// Stop halts the watcher.
func (w *IntakeWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		close(w.stopCh)
	}
}

// initialScan reads the file and populates knownIDs without calling handler.
func (w *IntakeWatcher) initialScan() {
	info, err := w.statFunc(w.filePath)
	if err != nil {
		return
	}
	w.lastModTime = info.ModTime()
	w.lastSize = info.Size()

	data, err := w.readFileFunc(w.filePath)
	if err != nil {
		return
	}
	items, err := ParseWorkItems(data)
	if err != nil {
		w.logger.Warn("Failed to parse queue.jsonl during initial scan", "error", err)
		return
	}
	for _, item := range items {
		w.knownIDs[item.ID] = true
	}
	w.logger.Info("Initial scan complete", "known_items", len(w.knownIDs))
}

// poll checks file mtime/size and reads new items if changed.
func (w *IntakeWatcher) poll() {
	info, err := w.statFunc(w.filePath)
	if err != nil {
		return
	}

	if info.ModTime().Equal(w.lastModTime) && info.Size() == w.lastSize {
		return
	}
	w.lastModTime = info.ModTime()
	w.lastSize = info.Size()

	data, err := w.readFileFunc(w.filePath)
	if err != nil {
		w.logger.Warn("Failed to read queue.jsonl", "error", err)
		return
	}

	items, err := ParseWorkItems(data)
	if err != nil {
		w.logger.Warn("Failed to parse queue.jsonl", "error", err)
		return
	}

	for _, item := range items {
		if !w.knownIDs[item.ID] {
			w.knownIDs[item.ID] = true
			w.logger.Info("New work item detected", "id", item.ID, "title", item.Title)
			w.handler(item)
		}
	}
}

// KnownCount returns the number of tracked item IDs.
func (w *IntakeWatcher) KnownCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.knownIDs)
}
