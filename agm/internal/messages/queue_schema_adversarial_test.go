package messages

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type adversarialQueueSnapshot struct {
	schemaVersion int
	objects       []string
	rows          []string
	sequences     []string
}

func TestMessageQueueToleratesSQLiteAnalyzeObjects(t *testing.T) {
	tests := []struct {
		name   string
		create func(*testing.T, string)
	}{
		{
			name: "current",
			create: func(t *testing.T, dbPath string) {
				queue, err := openMessageQueueAtPath(dbPath)
				require.NoError(t, err)
				_, err = queue.db.Exec(`
					INSERT INTO message_queue
						(message_id, from_session, to_session, message, priority, queued_at, status)
					VALUES
						('analyzed-current', 'source', 'target', 'analyzed body',
						 'MEDIUM', CURRENT_TIMESTAMP, 'queued')
				`)
				require.NoError(t, err)
				require.NoError(t, queue.Close())
			},
		},
		{
			name: "legacy",
			create: func(t *testing.T, dbPath string) {
				createHistoricalQueue(t, dbPath, []string{"A", "B", "C"})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			test.create(t, dbPath)
			statObjects := analyzeQueueDatabase(t, dbPath)

			queue, err := openMessageQueueAtPath(dbPath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, queue.Close()) })

			assertSQLiteStatObjectsPresent(t, queue.db, statObjects)
			var rowCount int
			require.NoError(t, queue.db.QueryRow(`SELECT COUNT(*) FROM message_queue`).Scan(&rowCount))
			assert.Positive(t, rowCount)
		})
	}
}

func TestMessageQueueRejectsInvalidCompleteSequenceInventoryWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		mutation   string
		privateTag string
	}{
		{
			name:     "duplicate target row",
			mutation: `INSERT INTO sqlite_sequence(name, seq) VALUES ('message_queue', 99)`,
		},
		{
			name:     "reserved replacement row",
			mutation: `INSERT INTO sqlite_sequence(name, seq) VALUES ('message_queue_next', 99)`,
		},
		{
			name:       "orphan row",
			mutation:   `INSERT INTO sqlite_sequence(name, seq) VALUES ('private-orphan-sequence-canary', 99)`,
			privateTag: "private-orphan-sequence-canary",
		},
		{
			name: "other rows",
			mutation: `
				INSERT INTO sqlite_sequence(name, seq) VALUES ('private-other-sequence-one', 1);
				INSERT INTO sqlite_sequence(name, seq) VALUES ('private-other-sequence-two', 2)
			`,
			privateTag: "private-other-sequence",
		},
		{
			name:       "noninteger target",
			mutation:   `UPDATE sqlite_sequence SET seq = 'private-sequence-value-canary' WHERE name = 'message_queue'`,
			privateTag: "private-sequence-value-canary",
		},
		{
			name:     "null target",
			mutation: `UPDATE sqlite_sequence SET seq = NULL WHERE name = 'message_queue'`,
		},
		{
			name:     "real target",
			mutation: `UPDATE sqlite_sequence SET seq = 42.5 WHERE name = 'message_queue'`,
		},
		{
			name:     "negative target",
			mutation: `UPDATE sqlite_sequence SET seq = -1 WHERE name = 'message_queue'`,
		},
		{
			name:     "missing target for populated queue",
			mutation: `DELETE FROM sqlite_sequence WHERE name = 'message_queue'`,
		},
		{
			name:     "target below maximum row ID",
			mutation: `UPDATE sqlite_sequence SET seq = 6 WHERE name = 'message_queue'`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			createHistoricalQueue(t, dbPath, []string{"A", "B", "C"})

			db := openRawQueueDB(t, dbPath)
			_, err := db.Exec(test.mutation)
			require.NoError(t, err)
			require.NoError(t, db.Close())

			before := snapshotAdversarialQueue(t, dbPath)
			queue, openErr := openMessageQueueAtPath(dbPath)
			if queue != nil {
				require.NoError(t, queue.Close())
			}
			assert.Error(t, openErr)
			if openErr != nil {
				assertErrorOmits(
					t,
					openErr,
					test.privateTag,
					"preserved-row",
					"preserved body",
				)
			}

			after := snapshotAdversarialQueue(t, dbPath)
			assert.Equal(t, before, after, "rejected sequence metadata must roll back table, rows, and complete sequence inventory")
		})
	}
}

func TestMessageQueueRejectsUnknownObjectsAndConflictingOwnedIndex(t *testing.T) {
	tests := []struct {
		name       string
		mutation   string
		privateTag string
	}{
		{
			name:       "unknown table",
			mutation:   `CREATE TABLE "private-table-canary" (value TEXT)`,
			privateTag: "private-table-canary",
		},
		{
			name:       "unknown view",
			mutation:   `CREATE VIEW "private-view-canary" AS SELECT message_id FROM message_queue`,
			privateTag: "private-view-canary",
		},
		{
			name: "unknown trigger",
			mutation: `
				CREATE TRIGGER "private-trigger-canary"
				AFTER INSERT ON message_queue BEGIN SELECT 1; END
			`,
			privateTag: "private-trigger-canary",
		},
		{
			name:       "unknown index",
			mutation:   `CREATE INDEX "private-index-canary" ON message_queue(status)`,
			privateTag: "private-index-canary",
		},
		{
			name: "conflicting owned index with unexpected collation",
			mutation: `
				DROP INDEX "idx_priority";
				CREATE INDEX "idx_priority" ON "message_queue" ("priority" COLLATE NOCASE)
			`,
			privateTag: "NOCASE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			createCurrentQueueWithPrivacyCanary(t, dbPath)

			db := openRawQueueDB(t, dbPath)
			_, err := db.Exec(test.mutation)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			before := snapshotAdversarialQueue(t, dbPath)

			queue, openErr := openMessageQueueAtPath(dbPath)
			if queue != nil {
				require.NoError(t, queue.Close())
			}
			require.Error(t, openErr)
			assertErrorOmits(
				t,
				openErr,
				test.privateTag,
				"private-message-id-canary",
				"private-message-body-canary",
				"private-sender-canary",
				"private-recipient-canary",
			)

			after := snapshotAdversarialQueue(t, dbPath)
			assert.Equal(t, before, after)
		})
	}
}

func TestMessageQueueRestoresMissingOwnedCurrentIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "message_queue.db")
	createCurrentQueueWithPrivacyCanary(t, dbPath)

	db := openRawQueueDB(t, dbPath)
	_, err := db.Exec(`DROP INDEX "idx_status"`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	queue, err := openMessageQueueAtPath(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, queue.Close()) })

	var indexSQL string
	require.NoError(t, queue.db.QueryRow(`
		SELECT sql FROM sqlite_schema WHERE type = 'index' AND name = 'idx_status'
	`).Scan(&indexSQL))
	assert.Equal(t, `CREATE INDEX "idx_status" ON "message_queue" ("status")`, indexSQL)

	var body string
	require.NoError(t, queue.db.QueryRow(`
		SELECT message FROM message_queue WHERE message_id = 'private-message-id-canary'
	`).Scan(&body))
	assert.Equal(t, "private-message-body-canary", body)
}

func TestMessageQueueRejectsWeakenedCurrentTableFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		oldFragment string
		newFragment string
	}{
		{
			name: "same named priority constraint with weaker expression",
			oldFragment: `CONSTRAINT "message_queue_priority_domain" ` +
				`CHECK ("priority" IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW'))`,
			newFragment: `CONSTRAINT "message_queue_priority_domain" CHECK (1)`,
		},
		{
			name:        "base priority column with nocase collation",
			oldFragment: `"priority" TEXT NOT NULL DEFAULT 'MEDIUM'`,
			newFragment: `"priority" TEXT COLLATE NOCASE NOT NULL DEFAULT 'MEDIUM'`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			createCurrentQueueWithPrivacyCanary(t, dbPath)
			rewriteStoredQueueTableSQL(t, dbPath, test.oldFragment, test.newFragment)
			before := snapshotAdversarialQueue(t, dbPath)

			queue, openErr := openMessageQueueAtPath(dbPath)
			if queue != nil {
				require.NoError(t, queue.Close())
			}
			require.Error(t, openErr)
			assert.Contains(t, openErr.Error(), "fingerprint")
			assertErrorOmits(
				t,
				openErr,
				"private-message-id-canary",
				"private-message-body-canary",
				"private-sender-canary",
				"private-recipient-canary",
			)
			assert.Equal(t, before, snapshotAdversarialQueue(t, dbPath))
		})
	}
}

func TestMessageQueueMalformedSchemaErrorsArePrivacyBounded(t *testing.T) {
	tests := []struct {
		name       string
		mutation   string
		privateTag string
	}{
		{
			name: "private DDL canary",
			mutation: `
				ALTER TABLE message_queue
				ADD COLUMN "private-schema-column-canary" TEXT DEFAULT 'private-ddl-value-canary'
			`,
			privateTag: "private-ddl-value-canary",
		},
		{
			name: "unexpected column collation",
			mutation: `
				ALTER TABLE message_queue
				ADD COLUMN "private-collation-column-canary" TEXT COLLATE NOCASE
			`,
			privateTag: "NOCASE",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			createHistoricalQueue(t, dbPath, []string{"A", "B", "C"})

			db := openRawQueueDB(t, dbPath)
			_, err := db.Exec(test.mutation)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			before := snapshotAdversarialQueue(t, dbPath)

			queue, openErr := openMessageQueueAtPath(dbPath)
			if queue != nil {
				require.NoError(t, queue.Close())
			}
			require.Error(t, openErr)
			assertErrorOmits(
				t,
				openErr,
				test.privateTag,
				"private-schema-column-canary",
				"private-collation-column-canary",
				"preserved-row",
				"preserved body",
			)

			after := snapshotAdversarialQueue(t, dbPath)
			assert.Equal(t, before, after)
		})
	}
}

func TestMessageQueueMalformedSQLiteSchemaDriverErrorIsSanitized(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "message_queue.db")
	createCurrentQueueWithPrivacyCanary(t, dbPath)

	db := openRawQueueDB(t, dbPath)
	var schemaVersion int
	require.NoError(t, db.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion))
	require.NoError(t, execSQLBatch(db, `
		PRAGMA writable_schema = ON;
		INSERT INTO sqlite_schema(type, name, tbl_name, rootpage, sql)
		VALUES (
			'table',
			'private-driver-schema-canary',
			'private-driver-schema-canary',
			0,
			'CREATE TABLE "private-driver-schema-canary" ('
		);
		PRAGMA writable_schema = OFF;
	`))
	_, err := db.Exec(fmt.Sprintf(`PRAGMA schema_version = %d`, schemaVersion+1))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	queue, openErr := openMessageQueueAtPath(dbPath)
	if queue != nil {
		require.NoError(t, queue.Close())
	}
	require.Error(t, openErr)
	assert.Equal(t, "initialize message queue schema: database operation failed", openErr.Error())
	assertErrorOmits(
		t,
		openErr,
		"private-driver-schema-canary",
		"private-message-id-canary",
		"private-message-body-canary",
	)
}

func TestMessageQueueRejectsAliasedSchemaRootPagesWithoutMutation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "message_queue.db")
	queue, err := openMessageQueueAtPath(dbPath)
	require.NoError(t, err)
	require.NoError(t, queue.Close())

	db := openRawQueueDB(t, dbPath)
	var sequenceRootPage, schemaVersion int
	require.NoError(t, db.QueryRow(`
		SELECT rootpage FROM sqlite_schema
		WHERE type = 'table' AND name = 'sqlite_sequence'
	`).Scan(&sequenceRootPage))
	require.NoError(t, db.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion))
	require.NoError(t, execSQLBatch(db, `PRAGMA writable_schema = ON`))
	_, err = db.Exec(`
		UPDATE sqlite_schema SET rootpage = ?
		WHERE type = 'table' AND name = 'message_queue'
	`, sequenceRootPage)
	require.NoError(t, err)
	require.NoError(t, execSQLBatch(db, `PRAGMA writable_schema = OFF`))
	_, err = db.Exec(fmt.Sprintf(`PRAGMA schema_version = %d`, schemaVersion+1))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	before := snapshotAdversarialQueue(t, dbPath)
	rejected, openErr := openMessageQueueAtPath(dbPath)
	if rejected != nil {
		require.NoError(t, rejected.Close())
	}
	require.Error(t, openErr)
	assert.Contains(t, openErr.Error(), "root pages")
	assert.Equal(t, before, snapshotAdversarialQueue(t, dbPath))
}

func TestMessageQueueRejectsShellQueueLineageWithoutMutation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "message_queue.db")
	db := openRawQueueDB(t, dbPath)
	require.NoError(t, execSQLBatch(db, `
		CREATE TABLE message_queue (
			message_id TEXT PRIMARY KEY,
			from_session TEXT NOT NULL,
			to_session TEXT NOT NULL,
			message TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'pending',
			attempt_count INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			delivered_at TIMESTAMP,
			ack_required INTEGER NOT NULL DEFAULT 1,
			ack_received INTEGER NOT NULL DEFAULT 0,
			ack_timeout TIMESTAMP
		);
		CREATE INDEX idx_pending ON message_queue(to_session, status, created_at)
			WHERE status = 'pending';
		PRAGMA user_version = 1;
	`))
	_, err := db.Exec(`
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, status, created_at)
		VALUES
			('private-shell-id-canary', 'private-shell-sender-canary',
			 'private-shell-recipient-canary', 'private-shell-body-canary',
			 1, 'pending', CURRENT_TIMESTAMP)
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	before := snapshotShellQueue(t, dbPath)

	queue, openErr := openMessageQueueAtPath(dbPath)
	if queue != nil {
		require.NoError(t, queue.Close())
	}
	require.Error(t, openErr)
	assertErrorOmits(
		t,
		openErr,
		"private-shell-id-canary",
		"private-shell-sender-canary",
		"private-shell-recipient-canary",
		"private-shell-body-canary",
	)
	assert.Equal(t, before, snapshotShellQueue(t, dbPath))
}

func TestNewMessageQueueEscapesReservedHomePath(t *testing.T) {
	baseDir := t.TempDir()
	homeDir := filepath.Join(baseDir, "private home %#? canary")
	require.NoError(t, os.MkdirAll(homeDir, 0o755))
	t.Setenv("HOME", homeDir)

	queue, err := NewMessageQueue()
	require.NoError(t, err)
	require.NoError(t, queue.Close())

	wantPath := filepath.Join(homeDir, ".config", "agm", "message_queue.db")
	_, err = os.Stat(wantPath)
	require.NoError(t, err)

	var queueDatabases []string
	require.NoError(t, filepath.Walk(baseDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() && info.Name() == "message_queue.db" {
			queueDatabases = append(queueDatabases, path)
		}
		return nil
	}))
	assert.Equal(t, []string{wantPath}, queueDatabases)
}

func createCurrentQueueWithPrivacyCanary(t *testing.T, dbPath string) {
	t.Helper()
	queue, err := openMessageQueueAtPath(dbPath)
	require.NoError(t, err)
	_, err = queue.db.Exec(`
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES
			('private-message-id-canary', 'private-sender-canary', 'private-recipient-canary',
			 'private-message-body-canary', 'MEDIUM', CURRENT_TIMESTAMP, 'queued')
	`)
	require.NoError(t, err)
	require.NoError(t, queue.Close())
}

func rewriteStoredQueueTableSQL(t *testing.T, dbPath, oldFragment, newFragment string) {
	t.Helper()
	db := openRawQueueDB(t, dbPath)
	var schemaVersion int
	require.NoError(t, db.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion))
	require.NoError(t, execSQLBatch(db, `PRAGMA writable_schema = ON`))
	result, err := db.Exec(`
		UPDATE sqlite_schema
		SET sql = replace(sql, ?, ?)
		WHERE type = 'table' AND name = 'message_queue' AND instr(sql, ?) > 0
	`, oldFragment, newFragment, oldFragment)
	require.NoError(t, err)
	changed, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), changed)
	require.NoError(t, execSQLBatch(db, `PRAGMA writable_schema = OFF`))
	_, err = db.Exec(fmt.Sprintf(`PRAGMA schema_version = %d`, schemaVersion+1))
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

func analyzeQueueDatabase(t *testing.T, dbPath string) []string {
	t.Helper()
	db := openRawQueueDB(t, dbPath)
	_, err := db.Exec(`ANALYZE`)
	require.NoError(t, err)

	rows, err := db.Query(`
		SELECT name FROM sqlite_schema
		WHERE type = 'table' AND name GLOB 'sqlite_stat*'
		ORDER BY name
	`)
	require.NoError(t, err)
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	require.Contains(t, names, "sqlite_stat1")
	require.NoError(t, rows.Close())
	require.NoError(t, db.Close())
	return names
}

func assertSQLiteStatObjectsPresent(t *testing.T, db *sql.DB, names []string) {
	t.Helper()
	for _, name := range names {
		var count int
		require.NoError(t, db.QueryRow(`
			SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?
		`, name).Scan(&count))
		assert.Equal(t, 1, count, "SQLite-owned analysis table %q should be tolerated", name)
	}
}

func snapshotAdversarialQueue(t *testing.T, dbPath string) adversarialQueueSnapshot {
	t.Helper()
	db := openRawQueueDB(t, dbPath)
	defer db.Close()

	var snapshot adversarialQueueSnapshot
	require.NoError(t, db.QueryRow(`PRAGMA schema_version`).Scan(&snapshot.schemaVersion))
	snapshot.objects = queryAdversarialSnapshotRows(t, db, `
		SELECT type, name, tbl_name, CAST(rootpage AS TEXT), COALESCE(sql, '<NULL>')
		FROM sqlite_schema
		ORDER BY type, name
	`, 5)
	snapshot.rows = queryAdversarialSnapshotRows(t, db, `
		SELECT
			quote(id), quote(message_id), quote(from_session), quote(to_session),
			quote(message), quote(priority), quote(queued_at), quote(attempt_count),
			quote(last_attempt), quote(status), quote(created_at),
			quote(ack_required), quote(ack_received), quote(ack_timeout)
		FROM message_queue
		ORDER BY id
	`, 14)
	snapshot.sequences = queryAdversarialSnapshotRows(t, db, `
		SELECT CAST(rowid AS TEXT), quote(name), typeof(seq), quote(seq)
		FROM sqlite_sequence
		ORDER BY rowid
	`, 4)
	return snapshot
}

func snapshotShellQueue(t *testing.T, dbPath string) []string {
	t.Helper()
	db := openRawQueueDB(t, dbPath)
	defer db.Close()

	rows := queryAdversarialSnapshotRows(t, db, `
		SELECT type, name, tbl_name, CAST(rootpage AS TEXT), COALESCE(sql, '<NULL>')
		FROM sqlite_schema
		ORDER BY type, name
	`, 5)
	rows = append(rows, queryAdversarialSnapshotRows(t, db, `
		SELECT
			quote(message_id), quote(from_session), quote(to_session), quote(message),
			quote(priority), quote(status), quote(attempt_count), quote(created_at),
			quote(delivered_at), quote(ack_required), quote(ack_received), quote(ack_timeout)
		FROM message_queue
		ORDER BY message_id
	`, 12)...)
	var userVersion int
	require.NoError(t, db.QueryRow(`PRAGMA user_version`).Scan(&userVersion))
	rows = append(rows, fmt.Sprintf("user_version=%d", userVersion))
	sort.Strings(rows)
	return rows
}

func queryAdversarialSnapshotRows(t *testing.T, db *sql.DB, query string, columnCount int) []string {
	t.Helper()
	rows, err := db.Query(query)
	require.NoError(t, err)
	defer rows.Close()

	var result []string
	for rows.Next() {
		values := make([]string, columnCount)
		destinations := make([]any, columnCount)
		for index := range values {
			destinations[index] = &values[index]
		}
		require.NoError(t, rows.Scan(destinations...))
		result = append(result, strings.Join(values, "\x1f"))
	}
	require.NoError(t, rows.Err())
	sort.Strings(result)
	return result
}

func assertErrorOmits(t *testing.T, err error, privateValues ...string) {
	t.Helper()
	errorText := strings.ToLower(err.Error())
	for _, value := range privateValues {
		if value == "" {
			continue
		}
		assert.NotContains(t, errorText, strings.ToLower(value))
	}
}
