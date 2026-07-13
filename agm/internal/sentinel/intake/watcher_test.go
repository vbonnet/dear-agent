package intake

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntakeWatcher_DetectsNewItems(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")
	require.NoError(t, os.WriteFile(queuePath, nil, 0600))

	received := make(chan *WorkItem, 1)
	w := NewIntakeWatcher(IntakeWatcherConfig{
		FilePath:     queuePath,
		PollInterval: 10 * time.Millisecond,
		Handler: func(item *WorkItem) {
			received <- item
		},
	})

	go func() { _ = w.Start() }()
	defer w.Stop()

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for sequence := 1; ; sequence++ {
		select {
		case item := <-received:
			assert.Contains(t, item.ID, "intake-watcher-")
			return
		case <-ticker.C:
			item := validTestItem()
			item.ID = fmt.Sprintf("intake-watcher-%03d", sequence)
			data, err := json.Marshal(item)
			require.NoError(t, err)
			file, err := os.OpenFile(queuePath, os.O_APPEND|os.O_WRONLY, 0600)
			require.NoError(t, err)
			_, writeErr := file.Write(append(data, '\n'))
			require.NoError(t, file.Close())
			require.NoError(t, writeErr)
		case <-deadline.C:
			t.Fatal("timed out waiting for intake watcher callback")
		}
	}
}

func TestIntakeWatcher_SkipsExistingOnStartup(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	data, _ := json.Marshal(item)
	require.NoError(t, os.WriteFile(queuePath, append(data, '\n'), 0600))

	var mu sync.Mutex
	var received []*WorkItem
	w := NewIntakeWatcher(IntakeWatcherConfig{
		FilePath:     queuePath,
		PollInterval: 10 * time.Millisecond,
		Handler: func(item *WorkItem) {
			mu.Lock()
			received = append(received, item)
			mu.Unlock()
		},
	})

	go func() { _ = w.Start() }()
	time.Sleep(50 * time.Millisecond)
	w.Stop()
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, received)
	assert.Equal(t, 1, w.KnownCount())
}

func TestIntakeWatcher_NoFileNoPanic(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "nonexistent.jsonl")

	w := NewIntakeWatcher(IntakeWatcherConfig{
		FilePath:     queuePath,
		PollInterval: 10 * time.Millisecond,
	})

	go func() { _ = w.Start() }()
	time.Sleep(30 * time.Millisecond)
	w.Stop()
	time.Sleep(20 * time.Millisecond)
}

func TestIntakeWatcher_NoEmitOnUnchangedFile(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	data, _ := json.Marshal(item)
	require.NoError(t, os.WriteFile(queuePath, append(data, '\n'), 0600))

	var mu sync.Mutex
	callCount := 0
	w := NewIntakeWatcher(IntakeWatcherConfig{
		FilePath:     queuePath,
		PollInterval: 10 * time.Millisecond,
		Handler: func(item *WorkItem) {
			mu.Lock()
			callCount++
			mu.Unlock()
		},
	})

	go func() { _ = w.Start() }()
	time.Sleep(50 * time.Millisecond)
	w.Stop()
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, callCount, "unchanged file should not emit items")
}

func TestIntakeWatcher_StartAlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	w := NewIntakeWatcher(IntakeWatcherConfig{
		FilePath:     filepath.Join(tmpDir, "queue.jsonl"),
		PollInterval: 10 * time.Millisecond,
	})

	go func() { _ = w.Start() }()
	time.Sleep(20 * time.Millisecond)

	err := w.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	w.Stop()
	time.Sleep(20 * time.Millisecond)
}
