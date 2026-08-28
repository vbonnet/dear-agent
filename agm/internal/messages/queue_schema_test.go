package messages

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const historicalQueueSchema = `
	CREATE TABLE IF NOT EXISTS message_queue (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message_id TEXT UNIQUE NOT NULL,
		from_session TEXT NOT NULL,
		to_session TEXT NOT NULL,
		message TEXT NOT NULL,
		priority TEXT NOT NULL DEFAULT 'MEDIUM',
		queued_at TIMESTAMP NOT NULL,
		attempt_count INTEGER NOT NULL DEFAULT 0,
		last_attempt TIMESTAMP,
		status TEXT NOT NULL DEFAULT 'queued',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_to_session_status ON message_queue(to_session, status);
	CREATE INDEX IF NOT EXISTS idx_status ON message_queue(status);
	CREATE INDEX IF NOT EXISTS idx_priority ON message_queue(priority);
	CREATE INDEX IF NOT EXISTS idx_queued_at ON message_queue(queued_at);
`

var historicalAckColumnDDL = map[string]string{
	"A": `ALTER TABLE message_queue ADD COLUMN ack_required INTEGER NOT NULL DEFAULT 1`,
	"B": `ALTER TABLE message_queue ADD COLUMN ack_received INTEGER NOT NULL DEFAULT 0`,
	"C": `ALTER TABLE message_queue ADD COLUMN ack_timeout TIMESTAMP`,
}

var historicalAckOrders = [][]string{
	nil,
	{"A"}, {"B"}, {"C"},
	{"A", "B"}, {"A", "C"}, {"B", "A"}, {"B", "C"}, {"C", "A"}, {"C", "B"},
	{"A", "B", "C"}, {"A", "C", "B"}, {"B", "A", "C"},
	{"B", "C", "A"}, {"C", "A", "B"}, {"C", "B", "A"},
}

// These byte digests were captured from sqlite_schema after executing the
// repository's original DDL through both historical driver eras: mattn
// go-sqlite3 v1.14.37 at 8b55ccf63 and modernc SQLite v1.54.0 at 7dc8ef8e0.
// Both drivers produced the same stored SQL for all 16 reachable orderings.
var historicalQueueTableSQLSHA256 = map[string]string{
	"none": "18a2276d343dabc7b44ce8315a3d8bdf12c7cdc5d2f12b0d495940c2546c586d",
	"A":    "f31b6633ddba316fcdcb592924bd04edc90193a9887b66df35e98ce3c3b15693",
	"B":    "d153de4fba507373e46b23ecad6c88d085e13abe1bffb4f0cf0368da73f083cc",
	"C":    "502e8d6cb8f89d6464b2b174f6d7c9707a92264c0879d182988fe29eeca82417",
	"AB":   "f08837194f4f074574cfe079c631aebd1eb63ac049e710331096e0a48ccf825f",
	"AC":   "dafd0d02d1ae94fd2078a7d131aad18ddb7d823c20756552d0ba61c43db71cb0",
	"BA":   "15aee6b2913ca609e30c923e02b9059889d0462901d900b7d320b9715905931a",
	"BC":   "e4d10b7f58944f31c9a1dd52ede508531ee7c3405e9325f6c249600b81fadc83",
	"CA":   "f3e1056389dc43f1a04c67ded1d48e853fdc06f1f84e7c1f28d9cbc7434f3c19",
	"CB":   "9eb3c41df9839e51c258a948a960f0035a99aae272877591892efc841bbb284d",
	"ABC":  "f9b6892c5a62117e0447448adced5b1a0ad34f9075aa86e659dc7e8f3951899e",
	"ACB":  "82dc60239e8a8b551a129b75fb6db3462e29c51dbce8525ef4404818710ed8f2",
	"BAC":  "a892b8a6a8e8770514b80e02813691fc8ccb3283103705a0b75a2648919f6eac",
	"BCA":  "ecc14ec3dd95166d3a7894554f320202f8ad5505daff02a46a3bdbae754b8ec1",
	"CAB":  "93cec52d3ae3d01c807773841937060183abb7b01d6c7686cef9da2404e05efd",
	"CBA":  "335b2cdd8b06db09ca283d56cc508593220808173423644e1177707dfb153a80",
}

var historicalQueueIndexSQLSHA256 = map[string]string{
	"idx_to_session_status": "be73292ff1e976dc0926f429ce2f3193b9b25a86a704ecb48dd57853ceca343f",
	"idx_status":            "4c687719d0fba10b39f7862197a90b508cd95eafe5ef65c2dd17bffc6f50d648",
	"idx_priority":          "e630e7ee81a1f1a23cd5604c871333bb307f90c6d5b49e9027bea721ff4f5a1f",
	"idx_queued_at":         "7d10fe30db3d4275b9412f32c4dc865de30ec9e37b147642ca17457652254528",
	"idx_ack_required":      "f324fa1ee5101abb5f33e31a1b69b8da429c9d93e58abf7ed7f257ff229f88b5",
}

func openMessageQueueAtPath(dbPath string) (*MessageQueue, error) {
	db, err := openMessageQueueDatabaseAtPath(context.Background(), dbPath)
	if err != nil {
		return nil, err
	}
	return &MessageQueue{db: db}, nil
}

func openMessageQueueDatabaseAtPath(ctx context.Context, dbPath string) (*sql.DB, error) {
	storage, err := prepareMessageQueueStorageAtPath(dbPath)
	if err != nil {
		return nil, err
	}
	return openMessageQueueDB(ctx, storage)
}

func TestHistoricalQueueFingerprintsMatchDriverEraGoldens(t *testing.T) {
	require.Len(t, legacyQueueAckOrders, len(historicalQueueTableSQLSHA256))
	for _, order := range legacyQueueAckOrders {
		label := strings.Join(order, "")
		if label == "" {
			label = "none"
		}
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(historicalQueueTableSQL(order))))
		assert.Equal(t, historicalQueueTableSQLSHA256[label], digest, label)
	}

	for _, definition := range queueIndexDefinitions {
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(definition.legacySQL)))
		assert.Equal(t, historicalQueueIndexSQLSHA256[definition.name], digest, definition.name)
	}
}

func TestMessageQueueFreshSchemaEnforcesDomains(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "queue ?# vocabulary.db")
	queue, err := openMessageQueueAtPath(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, queue.Close()) })

	_, err = os.Stat(dbPath)
	require.NoError(t, err, "the escaped URI must open the intended path")

	var journalMode string
	var busyTimeout, ignoreChecks int
	require.NoError(t, queue.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode))
	require.NoError(t, queue.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout))
	require.NoError(t, queue.db.QueryRow(`PRAGMA ignore_check_constraints`).Scan(&ignoreChecks))
	assert.Equal(t, "wal", strings.ToLower(journalMode))
	assert.Equal(t, 5000, busyTimeout)
	assert.Zero(t, ignoreChecks)

	for priorityIndex, priority := range []Priority{
		PriorityCritical,
		PriorityHigh,
		PriorityMedium,
		PriorityLow,
	} {
		for stateIndex, state := range []QueueState{
			QueueStateQueued,
			QueueStateDelivered,
			QueueStateFailed,
		} {
			_, err := queue.db.Exec(`
				INSERT INTO message_queue
					(message_id, from_session, to_session, message, priority, queued_at, status)
				VALUES (?, 'source', 'target', 'valid body', ?, CURRENT_TIMESTAMP, ?)
			`, fmt.Sprintf("valid-%d-%d", priorityIndex, stateIndex), priority, state)
			require.NoError(t, err)
		}
	}

	_, err = queue.db.Exec(`
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES ('invalid-priority', 'source', 'target', 'private body', 'IMMEDIATE', CURRENT_TIMESTAMP, 'queued')
	`)
	require.Error(t, err)

	_, err = queue.db.Exec(`
		INSERT INTO message_queue
			(message_id, from_session, to_session, message, priority, queued_at, status)
		VALUES ('invalid-state', 'source', 'target', 'private body', 'MEDIUM', CURRENT_TIMESTAMP, 'waiting')
	`)
	require.Error(t, err)

	_, err = queue.db.Exec(`UPDATE message_queue SET priority = 'IMMEDIATE' WHERE message_id = 'valid-0-0'`)
	require.Error(t, err)
	_, err = queue.db.Exec(`UPDATE message_queue SET status = 'waiting' WHERE message_id = 'valid-0-0'`)
	require.Error(t, err)
}

func TestMessageQueueMigratesReachableLegacyColumnOrders(t *testing.T) {
	for _, order := range historicalAckOrders {
		name := strings.Join(order, "")
		if name == "" {
			name = "none"
		}

		t.Run(name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			createHistoricalQueue(t, dbPath, order)

			queue, err := openMessageQueueAtPath(dbPath)
			require.NoError(t, err)

			var beforeReopenVersion int
			require.NoError(t, queue.db.QueryRow(`PRAGMA schema_version`).Scan(&beforeReopenVersion))

			var (
				id                                      int64
				messageID, fromSession, toSession, body string
				priority, queuedAt, lastAttempt, state  string
				createdAt                               string
				attemptCount, ackRequired, ackReceived  int
				ackTimeout                              sql.NullString
			)
			require.NoError(t, queue.db.QueryRow(`
				SELECT id, message_id, from_session, to_session, message,
				       priority, CAST(queued_at AS TEXT), attempt_count,
				       CAST(last_attempt AS TEXT), status, CAST(created_at AS TEXT),
				       ack_required, ack_received, CAST(ack_timeout AS TEXT)
				FROM message_queue WHERE message_id = 'preserved-row'
			`).Scan(
				&id,
				&messageID,
				&fromSession,
				&toSession,
				&body,
				&priority,
				&queuedAt,
				&attemptCount,
				&lastAttempt,
				&state,
				&createdAt,
				&ackRequired,
				&ackReceived,
				&ackTimeout,
			))
			assert.Equal(t, int64(7), id)
			assert.Equal(t, "preserved-row", messageID)
			assert.Equal(t, "source", fromSession)
			assert.Equal(t, "target", toSession)
			assert.Equal(t, "preserved body", body)
			assert.Equal(t, "HIGH", priority)
			assert.Equal(t, "2026-08-24 12:30:00", queuedAt)
			assert.Equal(t, 2, attemptCount)
			assert.Equal(t, "2026-08-24 12:31:00", lastAttempt)
			assert.Equal(t, "delivered", state)
			assert.Equal(t, "2026-08-24 12:29:00", createdAt)

			present := historicalAckPresence(order)
			if present["A"] {
				assert.Zero(t, ackRequired)
			} else {
				assert.Equal(t, 1, ackRequired)
			}
			if present["B"] {
				assert.Equal(t, 1, ackReceived)
			} else {
				assert.Zero(t, ackReceived)
			}
			if present["C"] {
				assert.Equal(t, sql.NullString{String: "2026-08-24 12:34:56", Valid: true}, ackTimeout)
			} else {
				assert.False(t, ackTimeout.Valid)
			}

			var sequence int64
			require.NoError(t, queue.db.QueryRow(`
				SELECT seq FROM sqlite_sequence WHERE name = 'message_queue'
			`).Scan(&sequence))
			assert.Equal(t, int64(42), sequence)

			var tableSQL string
			require.NoError(t, queue.db.QueryRow(`
				SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'message_queue'
			`).Scan(&tableSQL))
			assert.Equal(t, currentQueueTableSQL(queueTableName), tableSQL)
			require.NoError(t, queue.Close())

			reopened, err := openMessageQueueAtPath(dbPath)
			require.NoError(t, err)
			defer reopened.Close()
			var afterReopenVersion int
			require.NoError(t, reopened.db.QueryRow(`PRAGMA schema_version`).Scan(&afterReopenVersion))
			assert.Equal(t, beforeReopenVersion, afterReopenVersion, "current schema reopen must be DDL-idempotent")
		})
	}
}

func TestMessageQueuePreservesValidLegacySequenceStates(t *testing.T) {
	tests := []struct {
		name            string
		mutation        string
		expectedPresent bool
		expectedValue   int64
		expectedNextID  int64
	}{
		{
			name:            "never used without metadata",
			mutation:        `DELETE FROM message_queue; DELETE FROM sqlite_sequence WHERE name = 'message_queue'`,
			expectedPresent: false,
			expectedNextID:  1,
		},
		{
			name: "empty with explicit zero",
			mutation: `
				DELETE FROM message_queue;
				UPDATE sqlite_sequence SET seq = 0 WHERE name = 'message_queue'
			`,
			expectedPresent: true,
			expectedValue:   0,
			expectedNextID:  1,
		},
		{
			name:            "populated at maximum row ID",
			mutation:        `UPDATE sqlite_sequence SET seq = 7 WHERE name = 'message_queue'`,
			expectedPresent: true,
			expectedValue:   7,
			expectedNextID:  8,
		},
		{
			name:            "deleted high water mark",
			expectedPresent: true,
			expectedValue:   42,
			expectedNextID:  43,
		},
		{
			name:            "valid value above maximum row ID",
			mutation:        `UPDATE sqlite_sequence SET seq = 99 WHERE name = 'message_queue'`,
			expectedPresent: true,
			expectedValue:   99,
			expectedNextID:  100,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			createHistoricalQueue(t, dbPath, []string{"A", "B", "C"})
			if test.mutation != "" {
				db := openRawQueueDB(t, dbPath)
				require.NoError(t, execSQLBatch(db, test.mutation))
				require.NoError(t, db.Close())
			}

			queue, err := openMessageQueueAtPath(dbPath)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, queue.Close()) })

			var count int
			var value sql.NullInt64
			require.NoError(t, queue.db.QueryRow(`
				SELECT COUNT(*), MAX(seq)
				FROM sqlite_sequence WHERE name = 'message_queue'
			`).Scan(&count, &value))
			if test.expectedPresent {
				assert.Equal(t, 1, count)
				assert.Equal(t, sql.NullInt64{Int64: test.expectedValue, Valid: true}, value)
			} else {
				assert.Zero(t, count)
				assert.False(t, value.Valid)
			}

			result, err := queue.db.Exec(`
				INSERT INTO message_queue
					(message_id, from_session, to_session, message, priority, queued_at, status)
				VALUES (?, 'source', 'target', 'sequence body', 'MEDIUM', CURRENT_TIMESTAMP, 'queued')
			`, "sequence-"+strings.ReplaceAll(test.name, " ", "-"))
			require.NoError(t, err)
			nextID, err := result.LastInsertId()
			require.NoError(t, err)
			assert.Equal(t, test.expectedNextID, nextID)
		})
	}
}

func TestMessageQueueConfiguresEveryPhysicalConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "message_queue.db")
	queue, err := openMessageQueueAtPath(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, queue.Close()) })
	queue.db.SetMaxOpenConns(2)

	ctx := context.Background()
	first, err := queue.db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, first.Close()) })
	second, err := queue.db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })

	for _, connection := range []*sql.Conn{first, second} {
		var journalMode string
		var busyTimeout, ignoreChecks int
		require.NoError(t, connection.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode))
		require.NoError(t, connection.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout))
		require.NoError(t, connection.QueryRowContext(ctx, `PRAGMA ignore_check_constraints`).Scan(&ignoreChecks))
		assert.Equal(t, "wal", strings.ToLower(journalMode))
		assert.Equal(t, 5000, busyTimeout)
		assert.Zero(t, ignoreChecks)
	}
}

func TestMessageQueueConcurrentConstructorsSerializeMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "message_queue.db")
	createHistoricalQueue(t, dbPath, []string{"B", "C", "A"})

	const constructorCount = 16
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, constructorCount)
	for range constructorCount {
		go func() {
			<-start
			db, err := openMessageQueueDatabaseAtPath(ctx, dbPath)
			if err == nil {
				err = db.Close()
			}
			results <- err
		}()
	}
	close(start)
	for range constructorCount {
		require.NoError(t, <-results)
	}

	queue, err := openMessageQueueAtPath(dbPath)
	require.NoError(t, err)
	var schemaVersion, rowCount int
	require.NoError(t, queue.db.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion))
	require.NoError(t, queue.db.QueryRow(`SELECT COUNT(*) FROM message_queue`).Scan(&rowCount))
	assert.Equal(t, 1, rowCount)
	require.NoError(t, queue.Close())

	reopened, err := openMessageQueueAtPath(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	var reopenedVersion int
	require.NoError(t, reopened.db.QueryRow(`PRAGMA schema_version`).Scan(&reopenedVersion))
	assert.Equal(t, schemaVersion, reopenedVersion)
}

func TestMessageQueueImmediateTransactionBusyTimeoutIsBoundedAndRetryable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "message_queue.db")
	db, err := openMessageQueueDatabaseAtPath(context.Background(), dbPath)
	require.NoError(t, err)

	// _txlock=immediate must acquire write intent at BeginTx, before any write.
	blockingTx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	const portableBusyWatchdog = 4 * queueBusyTimeout
	blockedContext, cancel := context.WithTimeout(context.Background(), portableBusyWatchdog+queueBusyTimeout)
	defer cancel()
	started := time.Now()
	blockedDB, blockedErr := openMessageQueueDatabaseAtPath(blockedContext, dbPath)
	if blockedDB != nil {
		require.NoError(t, blockedDB.Close())
	}
	require.Error(t, blockedErr)
	assert.ErrorIs(t, blockedErr, errQueueDatabaseOperation)
	elapsed := time.Since(started)
	assert.GreaterOrEqual(t, elapsed, queueBusyTimeout-time.Second,
		"busy handler should honor the configured wait budget")
	assert.Less(t, elapsed, portableBusyWatchdog,
		"retry scheduling plus an in-flight lazy connection setup and BEGIN must remain within the portable watchdog")

	require.NoError(t, blockingTx.Rollback())
	require.NoError(t, db.Close())

	retried, err := openMessageQueueDatabaseAtPath(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, retried.Close()) })

	var tableSQL string
	require.NoError(t, retried.QueryRow(`
		SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'message_queue'
	`).Scan(&tableSQL))
	assert.Equal(t, currentQueueTableSQL(queueTableName), tableSQL)
}

func TestQueueBusyRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	err := waitForQueueBusyRetry(ctx, time.Hour)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(started), time.Second)
}

func TestMessageQueueInvalidLegacyDomainsRollBack(t *testing.T) {
	for _, test := range []struct {
		name        string
		priority    string
		state       string
		domain      string
		secretValue string
	}{
		{name: "priority", priority: "IMMEDIATE", state: "queued", domain: "priority", secretValue: "IMMEDIATE"},
		{name: "state", priority: "MEDIUM", state: "waiting", domain: "state", secretValue: "waiting"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "message_queue.db")
			createHistoricalQueue(t, dbPath, []string{"A", "B", "C"})

			db := openRawQueueDB(t, dbPath)
			const messageID = "private-message-id"
			const body = "private message body"
			_, err := db.Exec(`
				INSERT INTO message_queue
					(message_id, from_session, to_session, message, priority, queued_at, status)
				VALUES (?, 'private-sender', 'private-recipient', ?, ?, CURRENT_TIMESTAMP, ?)
			`, messageID, body, test.priority, test.state)
			require.NoError(t, err)
			beforeSchema := queueTableSQL(t, db)
			require.NoError(t, db.Close())

			queue, err := openMessageQueueAtPath(dbPath)
			require.Error(t, err)
			assert.Nil(t, queue)
			assert.Contains(t, err.Error(), test.domain)
			assert.NotContains(t, err.Error(), test.secretValue)
			assert.NotContains(t, err.Error(), messageID)
			assert.NotContains(t, err.Error(), body)
			assert.NotContains(t, err.Error(), "private-sender")
			assert.NotContains(t, err.Error(), "private-recipient")

			db = openRawQueueDB(t, dbPath)
			defer db.Close()
			assert.Equal(t, beforeSchema, queueTableSQL(t, db))
			var gotPriority, gotState, gotBody string
			require.NoError(t, db.QueryRow(`
				SELECT priority, status, message FROM message_queue WHERE message_id = ?
			`, messageID).Scan(&gotPriority, &gotState, &gotBody))
			assert.Equal(t, test.priority, gotPriority)
			assert.Equal(t, test.state, gotState)
			assert.Equal(t, body, gotBody)
		})
	}
}

func createHistoricalQueue(t *testing.T, dbPath string, order []string) {
	t.Helper()

	db := openRawQueueDB(t, dbPath)
	require.NoError(t, execSQLBatch(db, historicalQueueSchema))
	for _, key := range order {
		_, ok := historicalAckColumnDDL[key]
		require.True(t, ok, "unknown historical acknowledgement column key %q", key)
		_, err := db.Exec(historicalAckColumnDDL[key])
		require.NoError(t, err)
	}

	_, err := db.Exec(`
		INSERT INTO message_queue
			(id, message_id, from_session, to_session, message, priority, queued_at,
			 attempt_count, last_attempt, status, created_at)
		VALUES
			(42, 'deleted-high-water', 'source', 'target', 'deleted body', 'LOW',
			 '2026-08-24 12:00:00', 0, NULL, 'queued', '2026-08-24 12:00:00')
	`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM message_queue WHERE id = 42`)
	require.NoError(t, err)

	columns := []string{
		"id", "message_id", "from_session", "to_session", "message", "priority",
		"queued_at", "attempt_count", "last_attempt", "status", "created_at",
	}
	values := []any{
		int64(7), "preserved-row", "source", "target", "preserved body", "HIGH",
		"2026-08-24 12:30:00", 2, "2026-08-24 12:31:00", "delivered", "2026-08-24 12:29:00",
	}
	present := historicalAckPresence(order)
	if present["A"] {
		columns = append(columns, "ack_required")
		values = append(values, 0)
	}
	if present["B"] {
		columns = append(columns, "ack_received")
		values = append(values, 1)
	}
	if present["C"] {
		columns = append(columns, "ack_timeout")
		values = append(values, "2026-08-24 12:34:56")
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	_, err = db.Exec(
		fmt.Sprintf("INSERT INTO message_queue (%s) VALUES (%s)", strings.Join(columns, ","), placeholders),
		values...,
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

func historicalAckPresence(order []string) map[string]bool {
	present := make(map[string]bool, len(order))
	for _, key := range order {
		present[key] = true
	}
	return present
}

func openRawQueueDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	return db
}

func execSQLBatch(db *sql.DB, sqlText string) error {
	_, err := db.Exec(sqlText)
	return err
}

func queueTableSQL(t *testing.T, db *sql.DB) string {
	t.Helper()
	var tableSQL string
	require.NoError(t, db.QueryRow(`
		SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'message_queue'
	`).Scan(&tableSQL))
	return tableSQL
}
