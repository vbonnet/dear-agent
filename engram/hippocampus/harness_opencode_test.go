package hippocampus

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCodeAdapterDiscoverAndRead(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	dbPath := filepath.Join(root, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE session (id text PRIMARY KEY, directory text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL)`,
		`CREATE TABLE message (id text PRIMARY KEY, session_id text NOT NULL, time_created integer NOT NULL, data text NOT NULL)`,
		`CREATE TABLE part (id text PRIMARY KEY, message_id text NOT NULL, session_id text NOT NULL, time_created integer NOT NULL, data text NOT NULL)`,
		fmt.Sprintf(`INSERT INTO session VALUES ('ses_1', %q, 1000, 2000)`, project),
		`INSERT INTO message VALUES ('msg_1', 'ses_1', 1100, '{"role":"user"}')`,
		`INSERT INTO message VALUES ('msg_2', 'ses_1', 1200, '{"role":"assistant"}')`,
		`INSERT INTO part VALUES ('part_1', 'msg_1', 'ses_1', 1100, '{"type":"text","text":"question"}')`,
		`INSERT INTO part VALUES ('part_noise', 'msg_2', 'ses_1', 1190, '{"type":"step-start"}')`,
		`INSERT INTO part VALUES ('part_2', 'msg_2', 'ses_1', 1200, '{"type":"text","text":"answer"}')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	adapter := NewOpenCodeAdapter(root)
	sessions, err := adapter.DiscoverSessions(context.Background(), project, time.UnixMilli(500))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != "ses_1" {
		t.Fatalf("sessions = %#v", sessions)
	}
	got, err := adapter.ReadTranscript(context.Background(), sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	if got != "user: question\nassistant: answer" {
		t.Fatalf("transcript = %q", got)
	}
}

func TestOpenCodeAdapterMissingDatabase(t *testing.T) {
	got, err := NewOpenCodeAdapter(t.TempDir()).DiscoverSessions(context.Background(), "", time.Time{})
	if err != nil || len(got) != 0 {
		t.Fatalf("sessions=%v err=%v", got, err)
	}
}
