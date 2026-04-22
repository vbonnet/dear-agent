package interrupt

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateType(t *testing.T) {
	tests := []struct {
		input string
		want  Type
		err   bool
	}{
		{"stop", TypeStop, false},
		{"steer", TypeSteer, false},
		{"kill", TypeKill, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		got, err := ValidateType(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ValidateType(%q) error = %v, wantErr %v", tt.input, err, tt.err)
		}
		if got != tt.want {
			t.Errorf("ValidateType(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	session := "test-session"

	flag := &Flag{
		Type:     TypeStop,
		Reason:   "budget exceeded",
		IssuedBy: "orchestrator",
		IssuedAt: time.Now().UTC().Truncate(time.Second),
		Context:  map[string]string{"cost": "42.50"},
	}

	// Write
	if err := Write(dir, session, flag); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Verify file exists
	path := FlagPath(dir, session)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("flag file not created: %v", err)
	}

	// Verify no temp file left behind
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file was not cleaned up")
	}

	// Read
	got, err := Read(dir, session)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got == nil {
		t.Fatal("Read() returned nil")
	}

	if got.Type != flag.Type {
		t.Errorf("Type = %v, want %v", got.Type, flag.Type)
	}
	if got.Reason != flag.Reason {
		t.Errorf("Reason = %v, want %v", got.Reason, flag.Reason)
	}
	if got.IssuedBy != flag.IssuedBy {
		t.Errorf("IssuedBy = %v, want %v", got.IssuedBy, flag.IssuedBy)
	}
	if got.Context["cost"] != "42.50" {
		t.Errorf("Context[cost] = %v, want 42.50", got.Context["cost"])
	}
}

func TestReadNonExistent(t *testing.T) {
	dir := t.TempDir()
	got, err := Read(dir, "nonexistent")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got != nil {
		t.Error("Read() should return nil for nonexistent flag")
	}
}

func TestConsume(t *testing.T) {
	dir := t.TempDir()
	session := "consume-test"

	flag := &Flag{
		Type:     TypeSteer,
		Reason:   "change direction",
		IssuedBy: "user",
		IssuedAt: time.Now().UTC(),
	}

	if err := Write(dir, session, flag); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Consume should return the flag and delete it
	got, err := Consume(dir, session)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if got == nil {
		t.Fatal("Consume() returned nil")
	}
	if got.Type != TypeSteer {
		t.Errorf("Consumed Type = %v, want steer", got.Type)
	}

	// Second consume should return nil (file deleted)
	got2, err := Consume(dir, session)
	if err != nil {
		t.Fatalf("second Consume() error = %v", err)
	}
	if got2 != nil {
		t.Error("second Consume() should return nil")
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	session := "clear-test"

	flag := &Flag{
		Type:     TypeKill,
		Reason:   "emergency",
		IssuedBy: "user",
		IssuedAt: time.Now().UTC(),
	}

	if err := Write(dir, session, flag); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if err := Clear(dir, session); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	got, err := Read(dir, session)
	if err != nil {
		t.Fatalf("Read() after Clear() error = %v", err)
	}
	if got != nil {
		t.Error("Read() after Clear() should return nil")
	}
}

func TestClearNonExistent(t *testing.T) {
	dir := t.TempDir()
	// Should not error on nonexistent file
	if err := Clear(dir, "nope"); err != nil {
		t.Errorf("Clear() on nonexistent file should not error: %v", err)
	}
}

func TestClearStale(t *testing.T) {
	dir := t.TempDir()

	// Write two flags
	for _, name := range []string{"old-session", "new-session"} {
		flag := &Flag{
			Type:     TypeStop,
			Reason:   "test",
			IssuedBy: "test",
			IssuedAt: time.Now().UTC(),
		}
		if err := Write(dir, name, flag); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}

	// Make the old one have an old mtime
	oldPath := FlagPath(dir, "old-session")
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	// Clear stale (older than 1 hour)
	cleared, err := ClearStale(dir, 1*time.Hour)
	if err != nil {
		t.Fatalf("ClearStale() error = %v", err)
	}
	if cleared != 1 {
		t.Errorf("ClearStale() cleared %d, want 1", cleared)
	}

	// Old should be gone
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old flag should have been removed")
	}

	// New should still exist
	newPath := FlagPath(dir, "new-session")
	if _, err := os.Stat(newPath); err != nil {
		t.Error("new flag should still exist")
	}
}

func TestClearAll(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"a", "b", "c"} {
		flag := &Flag{Type: TypeStop, Reason: "test", IssuedBy: "test", IssuedAt: time.Now().UTC()}
		if err := Write(dir, name, flag); err != nil {
			t.Fatalf("Write(%s) error = %v", name, err)
		}
	}

	if err := ClearAll(dir); err != nil {
		t.Fatalf("ClearAll() error = %v", err)
	}

	entries, _ := os.ReadDir(dir)
	jsonFiles := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsonFiles++
		}
	}
	if jsonFiles != 0 {
		t.Errorf("ClearAll() left %d json files", jsonFiles)
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	session := "atomic-test"

	// Write initial flag
	flag1 := &Flag{Type: TypeStop, Reason: "first", IssuedBy: "a", IssuedAt: time.Now().UTC()}
	if err := Write(dir, session, flag1); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Overwrite with new flag
	flag2 := &Flag{Type: TypeKill, Reason: "second", IssuedBy: "b", IssuedAt: time.Now().UTC()}
	if err := Write(dir, session, flag2); err != nil {
		t.Fatalf("Write() overwrite error = %v", err)
	}

	// Should read the second flag
	got, err := Read(dir, session)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.Type != TypeKill {
		t.Errorf("Type = %v, want kill (overwrite should succeed)", got.Type)
	}
	if got.Reason != "second" {
		t.Errorf("Reason = %v, want 'second'", got.Reason)
	}
}
