package monitoring

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// FileWatcher monitors file system changes using fsnotify
type FileWatcher struct {
	agentID  string
	workDir  string
	watcher  *fsnotify.Watcher
	eventBus *eventbus.LocalBus
	filters  []FileFilter
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(agentID, workDir string, bus *eventbus.LocalBus) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWatcher{
		agentID:  agentID,
		workDir:  workDir,
		watcher:  watcher,
		eventBus: bus,
		filters: []FileFilter{
			IsGitFile,
			IsTempFile,
			IsIDEFile,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	return fw, nil
}

// Start begins watching the work directory
func (fw *FileWatcher) Start() error {
	// Add work directory to watcher
	if err := fw.watcher.Add(fw.workDir); err != nil {
		// Check for inotify limit error
		if strings.Contains(err.Error(), "no space left on device") {
			return fmt.Errorf("inotify watch limit reached (increase with: sysctl fs.inotify.max_user_watches=524288): %w", err)
		}
		return fmt.Errorf("failed to watch directory %s: %w", fw.workDir, err)
	}

	// Start event processing goroutine
	go fw.watchLoop()

	return nil
}

// Stop stops the file watcher
func (fw *FileWatcher) Stop() error {
	fw.cancel()
	return fw.watcher.Close()
}

// AddFilter adds a file filter function
func (fw *FileWatcher) AddFilter(filter FileFilter) {
	fw.filters = append(fw.filters, filter)
}

// watchLoop processes fsnotify events
func (fw *FileWatcher) watchLoop() {
	for {
		select {
		case <-fw.ctx.Done():
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// Log error but continue watching
			slog.Error("file watcher error", "error", err)
		}
	}
}

// handleEvent processes a single fsnotify event
func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	// Filter out unwanted files
	if fw.shouldFilter(event.Name) {
		return
	}

	// Map fsnotify operations to our event types
	var eventType string
	switch event.Op {
	case fsnotify.Create:
		eventType = EventFileCreated
	case fsnotify.Write:
		eventType = EventFileModified
	case fsnotify.Remove:
		eventType = EventFileDeleted
	case fsnotify.Rename:
		// Treat rename as delete (file moved away)
		eventType = EventFileDeleted
	case fsnotify.Chmod:
		// Ignore chmod events
		return
	default:
		return
	}

	// Publish event to EventBus
	fw.eventBus.Publish(context.Background(), &eventbus.Event{
		Type:      eventType,
		Source:    "file-watcher",
		Data: map[string]interface{}{
			"agent_id":  fw.agentID,
			"path":      event.Name,
			"operation": event.Op.String(),
			"timestamp": time.Now().Format(time.RFC3339),
		},
	})
}

// shouldFilter checks if a file should be filtered out
func (fw *FileWatcher) shouldFilter(path string) bool {
	for _, filter := range fw.filters {
		if filter(path) {
			return true
		}
	}
	return false
}

// IsGitFile filters out .git directory files
func IsGitFile(path string) bool {
	return strings.Contains(path, "/.git/") || strings.HasSuffix(path, "/.git")
}

// IsTempFile filters out temporary files
func IsTempFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "~") ||
		strings.HasSuffix(base, ".swp") ||
		strings.HasSuffix(base, ".tmp") ||
		strings.HasPrefix(base, ".#") ||
		strings.HasPrefix(base, "#") && strings.HasSuffix(base, "#")
}

// IsIDEFile filters out IDE configuration files
func IsIDEFile(path string) bool {
	return strings.Contains(path, "/.vscode/") ||
		strings.Contains(path, "/.idea/") ||
		strings.Contains(path, "/__pycache__/") ||
		strings.Contains(path, "/node_modules/")
}
