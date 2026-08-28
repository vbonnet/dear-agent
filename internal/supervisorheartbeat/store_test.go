package supervisorheartbeat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreWriteReadRoundTrip(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "supervisors")
	store := New(root)
	want := Record{
		ID:          "vroom-orchestrator",
		PrimaryFor:  "vroom-meta-orchestrator",
		TertiaryFor: "vroom-overseer",
		LastBeatUTC: time.Date(2026, time.August, 28, 19, 15, 16, 123_000_000, time.UTC),
		PID:         31415,
		TmuxSession: "vroom-orchestrator",
	}

	if err := store.Write(want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	dir := filepath.Join(root, want.ID)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat heartbeat directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o755 {
		t.Fatalf("heartbeat directory mode = %04o, want 0755", got)
	}

	path := filepath.Join(dir, "heartbeat.json")
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat heartbeat file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("heartbeat file mode = %04o, want 0600", got)
	}
	if matches, err := filepath.Glob(filepath.Join(dir, ".heartbeat.json.*.tmp")); err != nil || len(matches) != 0 {
		t.Fatalf("temporary siblings remain after rename: matches=%v err=%v", matches, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read heartbeat JSON: %v", err)
	}
	if !strings.Contains(string(data), "\n  \"id\"") {
		t.Fatalf("heartbeat JSON is not indented: %q", data)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal heartbeat JSON: %v", err)
	}
	for _, key := range []string{"id", "primary_for", "tertiary_for", "last_beat_utc", "pid", "tmux_session"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("heartbeat JSON missing %q: %s", key, data)
		}
	}
	if len(fields) != 6 {
		t.Errorf("heartbeat JSON fields = %d, want 6: %s", len(fields), data)
	}

	got, err := store.Read(want.ID)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertRecordEqual(t, got, want)
}

func TestStoreReadsExistingAGMRecordFixture(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	id := "vroom-orchestrator"
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := []byte(`{
  "id": "vroom-orchestrator",
  "primary_for": "vroom-overseer",
  "tertiary_for": "vroom-meta-orchestrator",
  "last_beat_utc": "2026-08-28T19:15:16.123Z",
  "pid": 31415,
  "tmux_session": "vroom-orchestrator"
}`)
	if err := os.WriteFile(filepath.Join(dir, heartbeatFilename), fixture, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := New(root).Read(id)
	if err != nil {
		t.Fatalf("Read() existing AGM fixture error = %v", err)
	}
	assertRecordEqual(t, got, Record{
		ID:          id,
		PrimaryFor:  "vroom-overseer",
		TertiaryFor: "vroom-meta-orchestrator",
		LastBeatUTC: time.Date(2026, time.August, 28, 19, 15, 16, 123_000_000, time.UTC),
		PID:         31415,
		TmuxSession: id,
	})
}

func TestStoreReadRejectsMismatchedRecordID(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "orchestrator", heartbeatFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"id":"overseer","last_beat_utc":"2026-08-28T19:15:16Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := New(root).Read("orchestrator")
	if err == nil || !strings.Contains(err.Error(), `heartbeat "orchestrator" contains record ID "overseer"`) {
		t.Fatalf("Read() = %+v, %v; want mismatched-ID error", got, err)
	}
	if got != nil {
		t.Fatalf("Read() mismatched record = %+v, want nil", got)
	}
}

func TestStoreReadMissingDoesNotCreateDirectories(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "absent", "supervisors")
	got, err := New(root).Read("never-heartbeated")
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("Read() = %+v, want nil", got)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read() created or changed missing root: %v", err)
	}
}

func TestStoreRejectsInvalidSupervisorIDs(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"", ".", "..", "../escape", "nested/name", `nested\name`, "/absolute"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			store := New(root)
			if got, err := store.Read(id); err == nil {
				t.Errorf("Read(%q) = %+v, nil; want invalid-ID error", id, got)
			}
			if err := store.Write(Record{ID: id, LastBeatUTC: time.Unix(1, 0).UTC()}); err == nil {
				t.Errorf("Write(ID=%q) error = nil, want invalid-ID error", id)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Errorf("invalid ID %q created state beneath root: %v", id, entries)
			}
		})
	}
}

func TestStoreReadReturnsMalformedAndUnreadableErrors(t *testing.T) {
	t.Parallel()

	t.Run("malformed JSON", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "orch", "heartbeat.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"id":`), 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := New(root).Read("orch")
		if err == nil {
			t.Fatalf("Read() = %+v, nil; want malformed JSON error", got)
		}
		if got != nil {
			t.Fatalf("Read() record = %+v, want nil on malformed JSON", got)
		}
	})

	t.Run("unreadable path", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "orch", "heartbeat.json")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}

		got, err := New(root).Read("orch")
		if err == nil {
			t.Fatalf("Read() = %+v, nil; want unreadable-path error", got)
		}
		if got != nil {
			t.Fatalf("Read() record = %+v, want nil on unreadable path", got)
		}
	})
}

func TestStoreWriteOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rec := Record{
		ID:          "custom-supervisor",
		LastBeatUTC: time.Date(2026, time.August, 28, 20, 0, 0, 0, time.UTC),
	}
	if err := New(root).Write(rec); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, rec.ID, "heartbeat.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("heartbeat JSON fields = %d, want only required fields: %s", len(fields), data)
	}
	for _, key := range []string{"id", "last_beat_utc"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("heartbeat JSON missing required field %q: %s", key, data)
		}
	}
	for _, key := range []string{"primary_for", "tertiary_for", "pid", "tmux_session"} {
		if _, ok := fields[key]; ok {
			t.Errorf("heartbeat JSON retained empty optional field %q: %s", key, data)
		}
	}
}

func TestStoreWriteReplacesExistingRecordWithPrivateFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := New(root)
	first := Record{ID: "orch", LastBeatUTC: time.Unix(1, 0).UTC(), PID: 1}
	if err := store.Write(first); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	path := filepath.Join(root, first.ID, "heartbeat.json")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	second := Record{ID: first.ID, LastBeatUTC: time.Unix(2, 0).UTC(), PID: 2}
	if err := store.Write(second); err != nil {
		t.Fatalf("replacement Write() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("replacement heartbeat mode = %04o, want 0600", got)
	}
	got, err := store.Read(first.ID)
	if err != nil {
		t.Fatalf("Read() replacement error = %v", err)
	}
	assertRecordEqual(t, got, second)
}

func TestStoreConcurrentWritesUseIndependentTemporaryFiles(t *testing.T) {
	t.Parallel()

	const (
		writers         = 32
		writesPerWriter = 8
	)
	root := t.TempDir()
	store := New(root)
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for writer := range writers {
		writer := writer + 1
		wg.Go(func() {
			<-start
			for write := range writesPerWriter {
				err := store.Write(Record{
					ID:          "shared-supervisor",
					LastBeatUTC: time.Unix(int64(writer*writesPerWriter+write), 0).UTC(),
					PID:         writer,
				})
				if err != nil {
					errs <- err
					return
				}
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Write() error = %v", err)
	}
	if t.Failed() {
		return
	}

	got, err := store.Read("shared-supervisor")
	if err != nil {
		t.Fatalf("Read() after concurrent writes: %v", err)
	}
	if got == nil || got.ID != "shared-supervisor" || got.PID < 1 || got.PID > writers {
		t.Fatalf("final concurrent record = %+v", got)
	}
	tmpPattern := filepath.Join(root, "shared-supervisor", ".heartbeat.json.*.tmp")
	if matches, err := filepath.Glob(tmpPattern); err != nil || len(matches) != 0 {
		t.Fatalf("concurrent Write() left temporary files: matches=%v err=%v", matches, err)
	}
}

func TestStoreWriteReturnsDirectoryCreationError(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(root, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := New(root).Write(Record{
		ID:          "orch",
		LastBeatUTC: time.Unix(1, 0).UTC(),
	})
	if err == nil {
		t.Fatal("Write() error = nil, want directory creation error")
	}
}

func assertRecordEqual(t *testing.T, got *Record, want Record) {
	t.Helper()
	if got == nil {
		t.Fatal("record = nil, want populated record")
	}
	if got.ID != want.ID ||
		got.PrimaryFor != want.PrimaryFor ||
		got.TertiaryFor != want.TertiaryFor ||
		got.PID != want.PID ||
		got.TmuxSession != want.TmuxSession ||
		!got.LastBeatUTC.Equal(want.LastBeatUTC) {
		t.Fatalf("record = %+v, want %+v", *got, want)
	}
}
