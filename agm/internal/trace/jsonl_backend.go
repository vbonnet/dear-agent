package trace

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// JSONLBackend writes TraceRecords as newline-delimited JSON to a file.
type JSONLBackend struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
}

// NewJSONLBackend opens (or creates) path for append-only JSONL writing.
func NewJSONLBackend(path string) (*JSONLBackend, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open jsonl file: %w", err)
	}
	return &JSONLBackend{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

func (b *JSONLBackend) Write(_ context.Context, rec *TraceRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal trace record: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, err := b.writer.Write(data); err != nil {
		return err
	}
	return b.writer.WriteByte('\n')
}

func (b *JSONLBackend) Flush(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.writer.Flush(); err != nil {
		return err
	}
	return b.file.Sync()
}

func (b *JSONLBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.writer.Flush(); err != nil {
		return err
	}
	return b.file.Close()
}

// compile-time check
var _ Backend = (*JSONLBackend)(nil)
