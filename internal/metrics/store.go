package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultDir = ".agm/benchmarks"
const dataFile = "metrics.jsonl"

// Store persists and queries metric records via JSONL.
type Store struct {
	mu   sync.Mutex
	dir  string
	file string
}

// NewStore creates a store at the given directory.
// If dir is empty, defaults to ~/.agm/benchmarks/.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		dir = filepath.Join(home, defaultDir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create metrics dir: %w", err)
	}
	return &Store{
		dir:  dir,
		file: filepath.Join(dir, dataFile),
	}, nil
}

// Append writes a record to the JSONL file.
func (s *Store) Append(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	f, err := os.OpenFile(s.file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open metrics file: %w", err)
	}
	defer f.Close()

	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// Query reads records matching the filter.
func (s *Store) Query(filter QueryFilter) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open metrics file: %w", err)
	}
	defer f.Close()

	var results []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r Record
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue // skip malformed lines
		}
		if filter.Metric != "" && r.Metric != filter.Metric {
			continue
		}
		if !filter.Since.IsZero() && r.Timestamp.Before(filter.Since) {
			continue
		}
		if !filter.Until.IsZero() && r.Timestamp.After(filter.Until) {
			continue
		}
		results = append(results, r)
	}
	return results, scanner.Err()
}

// Summarize computes aggregate statistics for a metric.
func (s *Store) Summarize(filter QueryFilter) (*Summary, error) {
	records, err := s.Query(filter)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	sum := &Summary{
		Metric:   filter.Metric,
		Category: records[0].Category,
		Count:    len(records),
		Min:      math.MaxFloat64,
		Max:      -math.MaxFloat64,
	}

	var total float64
	for _, r := range records {
		total += r.Value
		if r.Value < sum.Min {
			sum.Min = r.Value
		}
		if r.Value > sum.Max {
			sum.Max = r.Value
		}
	}
	sum.Mean = total / float64(len(records))
	sum.Latest = records[len(records)-1].Value
	return sum, nil
}

// Dir returns the store directory path.
func (s *Store) Dir() string {
	return s.dir
}
