package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// JSONLSink writes events to a JSONL file organized by channel.
// Each channel gets its own file: {dir}/{channel}.jsonl
type JSONLSink struct {
	mu    sync.Mutex
	dir   string
	files map[Channel]*os.File
}

// NewJSONLSink creates a sink that writes events to JSONL files in dir.
// The directory is created if it does not exist.
func NewJSONLSink(dir string) (*JSONLSink, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create sink dir: %w", err)
	}
	return &JSONLSink{
		dir:   dir,
		files: make(map[Channel]*os.File),
	}, nil
}

func (s *JSONLSink) Name() string { return "jsonl" }

// HandleEvent writes the event as a JSON line to the appropriate channel file.
func (s *JSONLSink) HandleEvent(_ context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, ok := s.files[event.Channel]
	if !ok {
		path := filepath.Join(s.dir, string(event.Channel)+".jsonl")
		var err error
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("open jsonl file: %w", err)
		}
		s.files[event.Channel] = f
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	_, err = f.Write(append(line, '\n'))
	return err
}

// Close closes all open file handles.
func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var firstErr error
	for ch, f := range s.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.files, ch)
	}
	return firstErr
}
