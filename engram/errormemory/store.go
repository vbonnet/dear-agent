package errormemory

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store manages the error-memory.jsonl database.
type Store struct {
	path string
}

// NewStore creates a Store. path is the JSONL file path (expanded from ~ if needed).
func NewStore(path string) *Store {
	return &Store{path: expandPath(path)}
}

// Path returns the resolved file path.
func (s *Store) Path() string {
	return s.path
}

// expandPath replaces a leading ~ with the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// lockPath returns the lock file path for the JSONL file.
func (s *Store) lockPath() string {
	return s.path + ".lock"
}

// acquireLock creates a .lock file using O_CREATE|O_EXCL for exclusive access.
// Retries up to 3 times with 100ms delay between attempts.
func (s *Store) acquireLock() (*os.File, error) {
	for i := 0; i < 3; i++ {
		f, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			return f, nil
		}
		if i < 2 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil, fmt.Errorf("failed to acquire lock after 3 attempts: %s", s.lockPath())
}

// releaseLock removes the lock file.
func (s *Store) releaseLock() {
	os.Remove(s.lockPath())
}

// Load reads all records from the JSONL file.
// Returns an empty slice and no error if the file does not exist.
func (s *Store) Load() ([]ErrorRecord, error) {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ErrorRecord{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var records []ErrorRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec ErrorRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // skip malformed lines
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// Save writes all records to the JSONL file atomically via temp file + rename.
func (s *Store) Save(records []ErrorRecord) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "errormemory-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	w := bufio.NewWriter(tmp)
	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("marshaling record: %w", err)
		}
		w.Write(data)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("flushing: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// Upsert adds or updates a record. If pattern+error_category match an existing
// record, it increments count, updates last_seen and ttl_expiry, and appends
// session_id (keeping the last 5). Returns the updated record.
func (s *Store) Upsert(rec ErrorRecord) (ErrorRecord, error) {
	lock, err := s.acquireLock()
	if err != nil {
		return ErrorRecord{}, err
	}
	lock.Close()
	defer s.releaseLock()

	records, err := s.Load()
	if err != nil {
		return ErrorRecord{}, err
	}

	id := recordID(rec.Pattern, rec.ErrorCategory)
	rec.ID = id

	found := false
	for i, existing := range records {
		if existing.ID == id {
			records[i].Count += rec.Count
			if rec.LastSeen.After(records[i].LastSeen) {
				records[i].LastSeen = rec.LastSeen
			}
			records[i].TTLExpiry = rec.LastSeen.Add(DefaultTTL)
			if rec.CommandSample != "" {
				records[i].CommandSample = rec.CommandSample
			}
			if rec.Remediation != "" {
				records[i].Remediation = rec.Remediation
			}
			// Append session IDs, keeping last 5
			for _, sid := range rec.SessionIDs {
				if sid != "" {
					records[i].SessionIDs = append(records[i].SessionIDs, sid)
				}
			}
			if len(records[i].SessionIDs) > 5 {
				records[i].SessionIDs = records[i].SessionIDs[len(records[i].SessionIDs)-5:]
			}
			rec = records[i]
			found = true
			break
		}
	}

	if !found {
		if rec.TTLExpiry.IsZero() {
			rec.TTLExpiry = rec.LastSeen.Add(DefaultTTL)
		}
		if rec.Count == 0 {
			rec.Count = 1
		}
		records = append(records, rec)
	}

	// Enforce MaxRecords limit: evict oldest records by LastSeen
	records = enforceMaxRecords(records, MaxRecords)

	if err := s.Save(records); err != nil {
		return ErrorRecord{}, err
	}
	return rec, nil
}

// enforceMaxRecords trims records to maxRecords by evicting the oldest by LastSeen.
func enforceMaxRecords(records []ErrorRecord, maxRecords int) []ErrorRecord {
	if len(records) <= maxRecords {
		return records
	}
	// Sort by LastSeen descending (most recent first)
	sort.Slice(records, func(i, j int) bool {
		return records[i].LastSeen.After(records[j].LastSeen)
	})
	return records[:maxRecords]
}

// recordID generates a deterministic ID from pattern + error_category using SHA256.
func recordID(pattern, category string) string {
	h := sha256.New()
	h.Write([]byte(pattern + "|" + category))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
