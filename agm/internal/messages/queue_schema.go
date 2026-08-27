package messages

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
)

const (
	queueTableName     = "message_queue"
	queueNextTableName = "message_queue_next"
)

const currentQueueTableTemplate = `CREATE TABLE "{{table}}" (
	"id" INTEGER PRIMARY KEY AUTOINCREMENT,
	"message_id" TEXT UNIQUE NOT NULL,
	"from_session" TEXT NOT NULL,
	"to_session" TEXT NOT NULL,
	"message" TEXT NOT NULL,
	"priority" TEXT NOT NULL DEFAULT 'MEDIUM',
	"queued_at" TIMESTAMP NOT NULL,
	"attempt_count" INTEGER NOT NULL DEFAULT 0,
	"last_attempt" TIMESTAMP,
	"status" TEXT NOT NULL DEFAULT 'queued',
	"created_at" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	"ack_required" INTEGER NOT NULL DEFAULT 1,
	"ack_received" INTEGER NOT NULL DEFAULT 0,
	"ack_timeout" TIMESTAMP,
	CONSTRAINT "message_queue_priority_domain" CHECK ("priority" IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW')),
	CONSTRAINT "message_queue_state_domain" CHECK ("status" IN ('queued', 'delivered', 'failed'))
)`

const legacyQueueTableSQL = `CREATE TABLE message_queue (
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
	)`

type queueIndexDefinition struct {
	name       string
	legacySQL  string
	currentSQL string
	columns    []string
}

var queueIndexDefinitions = []queueIndexDefinition{
	{
		name:       "idx_to_session_status",
		legacySQL:  `CREATE INDEX idx_to_session_status ON message_queue(to_session, status)`,
		currentSQL: `CREATE INDEX "idx_to_session_status" ON "message_queue" ("to_session", "status")`,
		columns:    []string{"to_session", "status"},
	},
	{
		name:       "idx_status",
		legacySQL:  `CREATE INDEX idx_status ON message_queue(status)`,
		currentSQL: `CREATE INDEX "idx_status" ON "message_queue" ("status")`,
		columns:    []string{"status"},
	},
	{
		name:       "idx_priority",
		legacySQL:  `CREATE INDEX idx_priority ON message_queue(priority)`,
		currentSQL: `CREATE INDEX "idx_priority" ON "message_queue" ("priority")`,
		columns:    []string{"priority"},
	},
	{
		name:       "idx_queued_at",
		legacySQL:  `CREATE INDEX idx_queued_at ON message_queue(queued_at)`,
		currentSQL: `CREATE INDEX "idx_queued_at" ON "message_queue" ("queued_at")`,
		columns:    []string{"queued_at"},
	},
	{
		name:       "idx_ack_required",
		legacySQL:  `CREATE INDEX idx_ack_required ON message_queue(ack_required, ack_received)`,
		currentSQL: `CREATE INDEX "idx_ack_required" ON "message_queue" ("ack_required", "ack_received")`,
		columns:    []string{"ack_required", "ack_received"},
	},
}

var queueStatisticsTableSQL = map[string]string{
	"sqlite_stat1": "CREATE TABLE sqlite_stat1(tbl,idx,stat)",
	"sqlite_stat4": "CREATE TABLE sqlite_stat4(tbl,idx,neq,nlt,ndlt,sample)",
}

var legacyQueueAckOrders = [][]string{
	nil,
	{"A"}, {"B"}, {"C"},
	{"A", "B"}, {"A", "C"}, {"B", "A"}, {"B", "C"}, {"C", "A"}, {"C", "B"},
	{"A", "B", "C"}, {"A", "C", "B"}, {"B", "A", "C"},
	{"B", "C", "A"}, {"C", "A", "B"}, {"C", "B", "A"},
}

type queueSchemaObject struct {
	typeName string
	name     string
	table    string
	rootPage int64
	sql      sql.NullString
}

type queueColumnInfo struct {
	cid          int
	name         string
	typeName     string
	notNull      int
	defaultValue sql.NullString
	primaryKey   int
	hidden       int
}

type queueIndexListInfo struct {
	unique  int
	origin  string
	partial int
}

type queueIndexColumnInfo struct {
	sequence int
	cid      int
	name     sql.NullString
	desc     int
	collate  sql.NullString
	key      int
}

type queueSequence struct {
	present bool
	value   int64
}

var errQueueDatabaseOperation = errors.New("database operation failed")

type queueSchemaConflict struct {
	message string
}

func (e *queueSchemaConflict) Error() string {
	return e.message
}

func queueSchemaConflictf(format string, args ...any) error {
	return &queueSchemaConflict{message: fmt.Sprintf(format, args...)}
}

// openMessageQueueDB owns every SQLite precondition behind the queue's public
// constructor: URI escaping, per-connection pragmas, immediate initialization,
// schema classification, migration, and close-on-error.
func openMessageQueueDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", messageQueueDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open message queue database: %w", errQueueDatabaseOperation)
	}

	if err := initializeMessageQueueSchema(ctx, db); err != nil {
		initializationErr := sanitizeQueueInitializationError(err)
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(
				initializationErr,
				fmt.Errorf("close rejected message queue database: %w", errQueueDatabaseOperation),
			)
		}
		return nil, initializationErr
	}

	return db, nil
}

func sanitizeQueueInitializationError(err error) error {
	if conflict, ok := errors.AsType[*queueSchemaConflict](err); ok {
		return fmt.Errorf("initialize message queue schema: %w", conflict)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("initialize message queue schema: %w", context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("initialize message queue schema: %w", context.DeadlineExceeded)
	}
	return fmt.Errorf("initialize message queue schema: %w", errQueueDatabaseOperation)
}

func messageQueueDSN(dbPath string) string {
	uriPath := filepath.ToSlash(dbPath)
	if filepath.VolumeName(dbPath) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}

	databaseURL := &url.URL{Scheme: "file", Path: uriPath}
	query := databaseURL.Query()
	query.Set("_busy_timeout", "5000")
	query.Set("_journal_mode", "WAL")
	query.Set("_txlock", "immediate")
	query.Add("_pragma", "ignore_check_constraints(OFF)")
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}

func initializeMessageQueueSchema(ctx context.Context, db *sql.DB) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin immediate schema transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = verifyQueueConnection(ctx, tx); err != nil {
		return err
	}
	objects, err := readQueueSchemaObjects(ctx, tx)
	if err != nil {
		return err
	}
	if err = reconcileQueueSchema(ctx, tx, objects); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit queue schema transaction: %w", err)
	}
	return nil
}

func reconcileQueueSchema(
	ctx context.Context,
	tx *sql.Tx,
	objects map[string]queueSchemaObject,
) error {
	tableObject, tableExists := objects[queueTableName]
	if !tableExists {
		return createFreshQueueSchema(ctx, tx, objects)
	}

	if err := validateQueueSchemaObjectSet(objects); err != nil {
		return err
	}
	if !tableObject.sql.Valid {
		return queueSchemaConflictf("incompatible queue database: queue table has no stored definition")
	}
	if tableObject.sql.String == currentQueueTableSQL(queueTableName) {
		return reconcileCurrentQueueSchema(ctx, tx)
	}

	ackOrder, legacy := classifyLegacyQueueTableSQL(tableObject.sql.String)
	if !legacy {
		return queueSchemaConflictf(
			"incompatible queue database: queue table is outside the owned current and historical fingerprints",
		)
	}
	return migrateOwnedLegacyQueueSchema(ctx, tx, objects, ackOrder)
}

func createFreshQueueSchema(
	ctx context.Context,
	tx *sql.Tx,
	objects map[string]queueSchemaObject,
) error {
	if len(objects) != 0 {
		return queueSchemaConflictf(
			"incompatible queue database: %d schema object(s) exist without the owned queue table",
			len(objects),
		)
	}
	if err := createCurrentQueueSchema(ctx, tx); err != nil {
		return err
	}
	if err := validateCurrentQueueSchema(ctx, tx, true); err != nil {
		return fmt.Errorf("validate created queue schema: %w", err)
	}
	_, err := readQueueSequence(ctx, tx)
	return err
}

func reconcileCurrentQueueSchema(ctx context.Context, tx *sql.Tx) error {
	if _, err := readQueueSequence(ctx, tx); err != nil {
		return err
	}
	return ensureCurrentQueueIndexes(ctx, tx)
}

func migrateOwnedLegacyQueueSchema(
	ctx context.Context,
	tx *sql.Tx,
	objects map[string]queueSchemaObject,
	ackOrder []string,
) error {
	if err := validateLegacyQueueSchema(ctx, tx, objects, ackOrder); err != nil {
		return err
	}
	if err := migrateLegacyQueueSchema(ctx, tx, ackOrder); err != nil {
		return err
	}
	if err := validateCurrentQueueSchema(ctx, tx, true); err != nil {
		return fmt.Errorf("validate migrated queue schema: %w", err)
	}
	return nil
}

func verifyQueueConnection(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `PRAGMA ignore_check_constraints = OFF`); err != nil {
		return fmt.Errorf("enable queue CHECK enforcement: %w", err)
	}

	var ignoreChecks, busyTimeout int
	var journalMode string
	if err := tx.QueryRowContext(ctx, `PRAGMA ignore_check_constraints`).Scan(&ignoreChecks); err != nil {
		return fmt.Errorf("read queue CHECK enforcement: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		return fmt.Errorf("read queue busy timeout: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		return fmt.Errorf("read queue journal mode: %w", err)
	}

	if ignoreChecks != 0 {
		return queueSchemaConflictf("queue CHECK enforcement is disabled")
	}
	if busyTimeout != 5000 {
		return queueSchemaConflictf("queue busy timeout is %d ms, expected 5000 ms", busyTimeout)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return queueSchemaConflictf("queue journal mode is not WAL")
	}
	return nil
}

func createCurrentQueueSchema(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, currentQueueTableSQL(queueTableName)); err != nil {
		return fmt.Errorf("create constrained queue table: %w", err)
	}
	for _, definition := range queueIndexDefinitions {
		if _, err := tx.ExecContext(ctx, definition.currentSQL); err != nil {
			return fmt.Errorf("create queue index %q: %w", definition.name, err)
		}
	}
	return nil
}

func ensureCurrentQueueIndexes(ctx context.Context, tx *sql.Tx) error {
	objects, err := readQueueSchemaObjects(ctx, tx)
	if err != nil {
		return err
	}
	missing, err := validateCurrentQueueSchemaWithObjects(ctx, tx, objects, false)
	if err != nil {
		return err
	}
	for _, definition := range missing {
		if _, err := tx.ExecContext(ctx, definition.currentSQL); err != nil {
			return fmt.Errorf("restore queue index %q: %w", definition.name, err)
		}
	}
	return validateCurrentQueueSchema(ctx, tx, true)
}

func validateCurrentQueueSchema(ctx context.Context, tx *sql.Tx, requireAllIndexes bool) error {
	objects, err := readQueueSchemaObjects(ctx, tx)
	if err != nil {
		return err
	}
	_, err = validateCurrentQueueSchemaWithObjects(ctx, tx, objects, requireAllIndexes)
	return err
}

func validateCurrentQueueSchemaWithObjects(
	ctx context.Context,
	tx *sql.Tx,
	objects map[string]queueSchemaObject,
	requireAllIndexes bool,
) ([]queueIndexDefinition, error) {
	if err := validateQueueSchemaObjectSet(objects); err != nil {
		return nil, err
	}
	tableObject := objects[queueTableName]
	if !tableObject.sql.Valid || tableObject.sql.String != currentQueueTableSQL(queueTableName) {
		return nil, queueSchemaConflictf("current queue table fingerprint does not match the owned definition")
	}

	columns, err := readQueueColumns(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(columns, currentQueueColumns()) {
		return nil, queueSchemaConflictf("current queue column fingerprint does not match the owned definition")
	}

	missing, err := validateQueueIndexes(ctx, tx, objects, columns, true)
	if err != nil {
		return nil, err
	}
	if requireAllIndexes && len(missing) != 0 {
		return nil, queueSchemaConflictf("current queue schema is missing %d owned index(es)", len(missing))
	}
	return missing, nil
}

func validateLegacyQueueSchema(
	ctx context.Context,
	tx *sql.Tx,
	objects map[string]queueSchemaObject,
	ackOrder []string,
) error {
	columns, err := readQueueColumns(ctx, tx)
	if err != nil {
		return err
	}
	if !slices.Equal(columns, legacyQueueColumns(ackOrder)) {
		return queueSchemaConflictf("historical queue column fingerprint does not match its stored definition")
	}
	if _, err := validateQueueIndexes(ctx, tx, objects, columns, false); err != nil {
		return err
	}
	return nil
}

func migrateLegacyQueueSchema(ctx context.Context, tx *sql.Tx, ackOrder []string) error {
	if err := validateLegacyQueueValues(ctx, tx); err != nil {
		return err
	}
	sequence, err := readQueueSequence(ctx, tx)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, currentQueueTableSQL(queueNextTableName)); err != nil {
		return fmt.Errorf("create constrained replacement queue table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, legacyQueueCopySQL(ackOrder)); err != nil {
		return fmt.Errorf("copy legacy queue rows into constrained table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE "message_queue"`); err != nil {
		return fmt.Errorf("drop replaced legacy queue table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE "message_queue_next" RENAME TO "message_queue"`); err != nil {
		return fmt.Errorf("activate constrained queue table: %w", err)
	}
	if err := restoreQueueSequence(ctx, tx, sequence); err != nil {
		return err
	}
	for _, definition := range queueIndexDefinitions {
		if _, err := tx.ExecContext(ctx, definition.currentSQL); err != nil {
			return fmt.Errorf("recreate queue index %q: %w", definition.name, err)
		}
	}
	return nil
}

func validateLegacyQueueValues(ctx context.Context, tx *sql.Tx) error {
	const query = `
		SELECT
			COALESCE(SUM(CASE
				WHEN priority IS NULL OR priority NOT IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW') THEN 1
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN status IS NULL OR status NOT IN ('queued', 'delivered', 'failed') THEN 1
				ELSE 0
			END), 0)
		FROM message_queue
	`
	var invalidPriorities, invalidStates int64
	if err := tx.QueryRowContext(ctx, query).Scan(&invalidPriorities, &invalidStates); err != nil {
		return fmt.Errorf("inventory legacy queue domains: %w", err)
	}
	if invalidPriorities != 0 || invalidStates != 0 {
		return queueSchemaConflictf(
			"legacy queue contains undeclared domain values (priority count %d, state count %d)",
			invalidPriorities,
			invalidStates,
		)
	}
	return nil
}

func readQueueSequence(ctx context.Context, tx *sql.Tx) (queueSequence, error) {
	var totalRows, queueRows, nonIntegerRows, negativeRows int64
	if err := tx.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE
				WHEN typeof(name) = 'text' AND name = 'message_queue' THEN 1
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN typeof(name) = 'text' AND name = 'message_queue' AND typeof(seq) <> 'integer' THEN 1
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN typeof(name) = 'text' AND name = 'message_queue' AND
				     typeof(seq) = 'integer' AND seq < 0 THEN 1
				ELSE 0
			END), 0)
		FROM sqlite_sequence
	`).Scan(&totalRows, &queueRows, &nonIntegerRows, &negativeRows); err != nil {
		return queueSequence{}, fmt.Errorf("inventory queue autoincrement metadata: %w", err)
	}

	if foreignRows := totalRows - queueRows; foreignRows != 0 {
		return queueSequence{}, queueSchemaConflictf(
			"queue autoincrement metadata contains %d non-owned row(s)",
			foreignRows,
		)
	}
	if queueRows > 1 {
		return queueSequence{}, queueSchemaConflictf(
			"queue autoincrement metadata has %d rows, expected at most one",
			queueRows,
		)
	}
	if nonIntegerRows != 0 {
		return queueSequence{}, queueSchemaConflictf("queue autoincrement metadata is not an integer")
	}
	if negativeRows != 0 {
		return queueSequence{}, queueSchemaConflictf("queue autoincrement metadata is negative")
	}

	var maximumID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(id) FROM message_queue`).Scan(&maximumID); err != nil {
		return queueSequence{}, fmt.Errorf("read legacy queue maximum row ID: %w", err)
	}
	if queueRows == 0 {
		if maximumID.Valid {
			return queueSequence{}, queueSchemaConflictf("populated queue has no autoincrement metadata")
		}
		return queueSequence{}, nil
	}

	var value int64
	if err := tx.QueryRowContext(ctx, `
		SELECT seq FROM sqlite_sequence WHERE name = 'message_queue'
	`).Scan(&value); err != nil {
		return queueSequence{}, fmt.Errorf("read queue autoincrement value: %w", err)
	}
	if maximumID.Valid && value < maximumID.Int64 {
		return queueSequence{}, queueSchemaConflictf("queue autoincrement metadata is below the maximum row ID")
	}
	return queueSequence{present: true, value: value}, nil
}

func restoreQueueSequence(ctx context.Context, tx *sql.Tx, sequence queueSequence) error {
	if !sequence.present {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sqlite_sequence WHERE name = 'message_queue'`); err != nil {
			return fmt.Errorf("remove absent queue autoincrement metadata: %w", err)
		}
		return validateRestoredQueueSequence(ctx, tx, sequence)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE sqlite_sequence SET seq = ? WHERE name = 'message_queue'
	`, sequence.value)
	if err != nil {
		return fmt.Errorf("restore queue autoincrement metadata: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read restored queue autoincrement row count: %w", err)
	}
	if updated == 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sqlite_sequence (name, seq) VALUES ('message_queue', ?)
		`, sequence.value); err != nil {
			return fmt.Errorf("insert queue autoincrement metadata: %w", err)
		}
	} else if updated != 1 {
		return queueSchemaConflictf("restoring queue autoincrement metadata changed %d rows", updated)
	}
	return validateRestoredQueueSequence(ctx, tx, sequence)
}

func validateRestoredQueueSequence(ctx context.Context, tx *sql.Tx, expected queueSequence) error {
	var count int64
	var value sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(seq) FROM sqlite_sequence WHERE name = 'message_queue'
	`).Scan(&count, &value); err != nil {
		return fmt.Errorf("verify restored queue autoincrement metadata: %w", err)
	}
	if !expected.present {
		if count != 0 {
			return queueSchemaConflictf("queue autoincrement metadata was created for a never-used queue")
		}
		return nil
	}
	if count != 1 || !value.Valid || value.Int64 != expected.value {
		return queueSchemaConflictf("queue autoincrement metadata was not restored exactly")
	}
	return nil
}

func legacyQueueCopySQL(ackOrder []string) string {
	present := make(map[string]bool, len(ackOrder))
	for _, key := range ackOrder {
		present[key] = true
	}

	ackRequired := "1"
	if present["A"] {
		ackRequired = `"ack_required"`
	}
	ackReceived := "0"
	if present["B"] {
		ackReceived = `"ack_received"`
	}
	ackTimeout := "NULL"
	if present["C"] {
		ackTimeout = `"ack_timeout"`
	}

	return fmt.Sprintf(`
		INSERT INTO "message_queue_next" (
			"id", "message_id", "from_session", "to_session", "message", "priority",
			"queued_at", "attempt_count", "last_attempt", "status", "created_at",
			"ack_required", "ack_received", "ack_timeout"
		)
		SELECT
			"id", "message_id", "from_session", "to_session", "message", "priority",
			"queued_at", "attempt_count", "last_attempt", "status", "created_at",
			%s, %s, %s
		FROM "message_queue"
	`, ackRequired, ackReceived, ackTimeout)
}

func readQueueSchemaObjects(ctx context.Context, tx *sql.Tx) (map[string]queueSchemaObject, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT type, name, tbl_name, rootpage, sql
		FROM sqlite_schema
		ORDER BY type, name
	`)
	if err != nil {
		return nil, fmt.Errorf("read queue schema objects: %w", err)
	}
	defer rows.Close()

	objects := make(map[string]queueSchemaObject)
	for rows.Next() {
		var object queueSchemaObject
		if err := rows.Scan(
			&object.typeName,
			&object.name,
			&object.table,
			&object.rootPage,
			&object.sql,
		); err != nil {
			return nil, fmt.Errorf("scan queue schema object: %w", err)
		}
		objects[object.name] = object
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue schema objects: %w", err)
	}
	return objects, nil
}

func validateQueueSchemaObjectSet(objects map[string]queueSchemaObject) error {
	unsupported := 0
	for _, object := range objects {
		if !isSupportedQueueSchemaObject(object) {
			unsupported++
		}
	}
	if unsupported != 0 {
		return queueSchemaConflictf("incompatible queue database: %d unsupported schema object(s)", unsupported)
	}
	if err := validateQueueSchemaRootPages(objects); err != nil {
		return err
	}
	for _, required := range []string{queueTableName, "sqlite_sequence", "sqlite_autoindex_message_queue_1"} {
		if _, ok := objects[required]; !ok {
			return queueSchemaConflictf("incompatible queue database: required schema object %q is missing", required)
		}
	}
	return nil
}

func validateQueueSchemaRootPages(objects map[string]queueSchemaObject) error {
	seen := make(map[int64]struct{}, len(objects))
	for _, object := range objects {
		if object.rootPage <= 1 {
			return queueSchemaConflictf("owned queue schema has an invalid b-tree root page")
		}
		if _, duplicate := seen[object.rootPage]; duplicate {
			return queueSchemaConflictf("owned queue schema has aliased b-tree root pages")
		}
		seen[object.rootPage] = struct{}{}
	}
	return nil
}

func isSupportedQueueSchemaObject(object queueSchemaObject) bool {
	if expectedSQL, ok := queueStatisticsTableSQL[object.name]; ok {
		return isExactQueueTableObject(object, object.name, expectedSQL)
	}
	return isSupportedQueueCoreObject(object)
}

func isSupportedQueueCoreObject(object queueSchemaObject) bool {
	switch object.name {
	case queueTableName:
		return object.typeName == "table" && object.table == queueTableName
	case "sqlite_sequence":
		return isExactQueueTableObject(
			object,
			"sqlite_sequence",
			"CREATE TABLE sqlite_sequence(name,seq)",
		)
	case "sqlite_autoindex_message_queue_1":
		return object.typeName == "index" && object.table == queueTableName && !object.sql.Valid
	default:
		_, owned := findQueueIndexDefinition(object.name)
		return owned && object.typeName == "index" && object.table == queueTableName && object.sql.Valid
	}
}

func isExactQueueTableObject(object queueSchemaObject, name, expectedSQL string) bool {
	return object.typeName == "table" && object.table == name &&
		object.sql.Valid && object.sql.String == expectedSQL
}

func findQueueIndexDefinition(name string) (queueIndexDefinition, bool) {
	for _, definition := range queueIndexDefinitions {
		if definition.name == name {
			return definition, true
		}
	}
	return queueIndexDefinition{}, false
}

func readQueueColumns(ctx context.Context, tx *sql.Tx) ([]queueColumnInfo, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_xinfo("message_queue")`)
	if err != nil {
		return nil, fmt.Errorf("read queue column fingerprint: %w", err)
	}
	defer rows.Close()

	var columns []queueColumnInfo
	for rows.Next() {
		var column queueColumnInfo
		if err := rows.Scan(
			&column.cid,
			&column.name,
			&column.typeName,
			&column.notNull,
			&column.defaultValue,
			&column.primaryKey,
			&column.hidden,
		); err != nil {
			return nil, fmt.Errorf("scan queue column fingerprint: %w", err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue column fingerprint: %w", err)
	}
	return columns, nil
}

func validateQueueIndexes(
	ctx context.Context,
	tx *sql.Tx,
	objects map[string]queueSchemaObject,
	columns []queueColumnInfo,
	current bool,
) ([]queueIndexDefinition, error) {
	indexList, err := readQueueIndexList(ctx, tx)
	if err != nil {
		return nil, err
	}

	columnIDs := queueColumnIDs(columns)
	for name, info := range indexList {
		object, ok := objects[name]
		if !ok || object.typeName != "index" {
			return nil, queueSchemaConflictf("queue index %q is missing its stored schema object", name)
		}
		if err := validateQueueIndex(ctx, tx, name, info, object, columnIDs, current); err != nil {
			return nil, err
		}
	}

	if _, ok := indexList["sqlite_autoindex_message_queue_1"]; !ok {
		return nil, queueSchemaConflictf("queue message identity autoindex is missing")
	}
	if err := validateStoredQueueIndexInventory(objects, indexList); err != nil {
		return nil, err
	}
	return missingQueueIndexes(indexList, current), nil
}

func queueColumnIDs(columns []queueColumnInfo) map[string]int {
	columnIDs := make(map[string]int, len(columns))
	for _, column := range columns {
		columnIDs[column.name] = column.cid
	}
	return columnIDs
}

func validateQueueIndex(
	ctx context.Context,
	tx *sql.Tx,
	name string,
	info queueIndexListInfo,
	object queueSchemaObject,
	columnIDs map[string]int,
	current bool,
) error {
	if name == "sqlite_autoindex_message_queue_1" {
		return validateQueueIdentityAutoindex(ctx, tx, info, columnIDs)
	}
	definition, ok := findQueueIndexDefinition(name)
	if !ok {
		return queueSchemaConflictf("queue index inventory contains an unsupported index")
	}
	return validateOwnedQueueIndex(ctx, tx, info, object, definition, columnIDs, current)
}

func validateQueueIdentityAutoindex(
	ctx context.Context,
	tx *sql.Tx,
	info queueIndexListInfo,
	columnIDs map[string]int,
) error {
	if info != (queueIndexListInfo{unique: 1, origin: "u", partial: 0}) {
		return queueSchemaConflictf("queue message identity autoindex fingerprint does not match")
	}
	got, err := readQueueIndexColumns(ctx, tx, "sqlite_autoindex_message_queue_1")
	if err != nil {
		return err
	}
	want := expectedQueueIndexColumns(columnIDs, []string{"message_id"})
	if !slices.Equal(got, want) {
		return queueSchemaConflictf("queue message identity autoindex columns do not match")
	}
	return nil
}

func validateOwnedQueueIndex(
	ctx context.Context,
	tx *sql.Tx,
	info queueIndexListInfo,
	object queueSchemaObject,
	definition queueIndexDefinition,
	columnIDs map[string]int,
	current bool,
) error {
	if info != (queueIndexListInfo{unique: 0, origin: "c", partial: 0}) {
		return queueSchemaConflictf("owned queue index %q attributes do not match", definition.name)
	}
	expectedSQL := definition.legacySQL
	if current {
		expectedSQL = definition.currentSQL
	}
	if !object.sql.Valid || object.sql.String != expectedSQL {
		return queueSchemaConflictf(
			"owned queue index %q definition conflicts with the expected fingerprint",
			definition.name,
		)
	}
	for _, column := range definition.columns {
		if _, ok := columnIDs[column]; !ok {
			return queueSchemaConflictf(
				"owned queue index %q refers to a column outside this queue lineage",
				definition.name,
			)
		}
	}
	got, err := readQueueIndexColumns(ctx, tx, definition.name)
	if err != nil {
		return err
	}
	want := expectedQueueIndexColumns(columnIDs, definition.columns)
	if !slices.Equal(got, want) {
		return queueSchemaConflictf("owned queue index %q columns do not match", definition.name)
	}
	return nil
}

func validateStoredQueueIndexInventory(
	objects map[string]queueSchemaObject,
	indexList map[string]queueIndexListInfo,
) error {
	for name, object := range objects {
		if object.typeName == "index" {
			if _, ok := indexList[name]; !ok {
				return queueSchemaConflictf(
					"stored queue index %q is absent from the table index inventory",
					name,
				)
			}
		}
	}
	return nil
}

func missingQueueIndexes(
	indexList map[string]queueIndexListInfo,
	current bool,
) []queueIndexDefinition {
	if !current {
		return nil
	}
	missing := make([]queueIndexDefinition, 0, len(queueIndexDefinitions))
	for _, definition := range queueIndexDefinitions {
		if _, ok := indexList[definition.name]; !ok {
			missing = append(missing, definition)
		}
	}
	return missing
}

func readQueueIndexList(ctx context.Context, tx *sql.Tx) (map[string]queueIndexListInfo, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA index_list("message_queue")`)
	if err != nil {
		return nil, fmt.Errorf("read queue index inventory: %w", err)
	}
	defer rows.Close()

	indexes := make(map[string]queueIndexListInfo)
	for rows.Next() {
		var sequence int
		var name string
		var info queueIndexListInfo
		if err := rows.Scan(&sequence, &name, &info.unique, &info.origin, &info.partial); err != nil {
			return nil, fmt.Errorf("scan queue index inventory: %w", err)
		}
		indexes[name] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue index inventory: %w", err)
	}
	return indexes, nil
}

func readQueueIndexColumns(ctx context.Context, tx *sql.Tx, name string) ([]queueIndexColumnInfo, error) {
	allowed := name == "sqlite_autoindex_message_queue_1"
	if !allowed {
		for _, definition := range queueIndexDefinitions {
			if definition.name == name {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return nil, queueSchemaConflictf("refuse index fingerprint query for unsupported index")
	}

	query := fmt.Sprintf(`PRAGMA index_xinfo("%s")`, name)
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read queue index %q columns: %w", name, err)
	}
	defer rows.Close()

	var columns []queueIndexColumnInfo
	for rows.Next() {
		var column queueIndexColumnInfo
		if err := rows.Scan(
			&column.sequence,
			&column.cid,
			&column.name,
			&column.desc,
			&column.collate,
			&column.key,
		); err != nil {
			return nil, fmt.Errorf("scan queue index %q columns: %w", name, err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queue index %q columns: %w", name, err)
	}
	return columns, nil
}

func expectedQueueIndexColumns(columnIDs map[string]int, names []string) []queueIndexColumnInfo {
	columns := make([]queueIndexColumnInfo, 0, len(names)+1)
	for sequence, name := range names {
		columns = append(columns, queueIndexColumnInfo{
			sequence: sequence,
			cid:      columnIDs[name],
			name:     sql.NullString{String: name, Valid: true},
			collate:  sql.NullString{String: "BINARY", Valid: true},
			key:      1,
		})
	}
	columns = append(columns, queueIndexColumnInfo{
		sequence: len(names),
		cid:      -1,
		collate:  sql.NullString{String: "BINARY", Valid: true},
	})
	return columns
}

func currentQueueTableSQL(tableName string) string {
	if tableName != queueTableName && tableName != queueNextTableName {
		panic("unsupported queue table name")
	}
	return strings.Replace(currentQueueTableTemplate, "{{table}}", tableName, 1)
}

func classifyLegacyQueueTableSQL(tableSQL string) ([]string, bool) {
	for _, order := range legacyQueueAckOrders {
		if tableSQL == historicalQueueTableSQL(order) {
			return slices.Clone(order), true
		}
	}
	return nil, false
}

func historicalQueueTableSQL(ackOrder []string) string {
	if len(ackOrder) == 0 {
		return legacyQueueTableSQL
	}
	definitions := make([]string, 0, len(ackOrder))
	for _, key := range ackOrder {
		definition, ok := legacyAckColumnDefinition(key)
		if !ok {
			panic("unsupported historical acknowledgement column key")
		}
		definitions = append(definitions, definition)
	}
	prefix := strings.TrimSuffix(legacyQueueTableSQL, "\n\t)")
	return prefix + "\n\t, " + strings.Join(definitions, ", ") + ")"
}

func legacyAckColumnDefinition(key string) (string, bool) {
	switch key {
	case "A":
		return "ack_required INTEGER NOT NULL DEFAULT 1", true
	case "B":
		return "ack_received INTEGER NOT NULL DEFAULT 0", true
	case "C":
		return "ack_timeout TIMESTAMP", true
	default:
		return "", false
	}
}

func currentQueueColumns() []queueColumnInfo {
	columns := baseQueueColumns()
	columns = append(columns,
		queueColumnInfo{cid: 11, name: "ack_required", typeName: "INTEGER", notNull: 1, defaultValue: nullableString("1")},
		queueColumnInfo{cid: 12, name: "ack_received", typeName: "INTEGER", notNull: 1, defaultValue: nullableString("0")},
		queueColumnInfo{cid: 13, name: "ack_timeout", typeName: "TIMESTAMP"},
	)
	return columns
}

func legacyQueueColumns(ackOrder []string) []queueColumnInfo {
	columns := baseQueueColumns()
	for cidOffset, key := range ackOrder {
		cid := 11 + cidOffset
		switch key {
		case "A":
			columns = append(columns, queueColumnInfo{
				cid: cid, name: "ack_required", typeName: "INTEGER", notNull: 1, defaultValue: nullableString("1"),
			})
		case "B":
			columns = append(columns, queueColumnInfo{
				cid: cid, name: "ack_received", typeName: "INTEGER", notNull: 1, defaultValue: nullableString("0"),
			})
		case "C":
			columns = append(columns, queueColumnInfo{cid: cid, name: "ack_timeout", typeName: "TIMESTAMP"})
		default:
			panic("unsupported historical acknowledgement column key")
		}
	}
	return columns
}

func baseQueueColumns() []queueColumnInfo {
	return []queueColumnInfo{
		{cid: 0, name: "id", typeName: "INTEGER", primaryKey: 1},
		{cid: 1, name: "message_id", typeName: "TEXT", notNull: 1},
		{cid: 2, name: "from_session", typeName: "TEXT", notNull: 1},
		{cid: 3, name: "to_session", typeName: "TEXT", notNull: 1},
		{cid: 4, name: "message", typeName: "TEXT", notNull: 1},
		{cid: 5, name: "priority", typeName: "TEXT", notNull: 1, defaultValue: nullableString("'MEDIUM'")},
		{cid: 6, name: "queued_at", typeName: "TIMESTAMP", notNull: 1},
		{cid: 7, name: "attempt_count", typeName: "INTEGER", notNull: 1, defaultValue: nullableString("0")},
		{cid: 8, name: "last_attempt", typeName: "TIMESTAMP"},
		{cid: 9, name: "status", typeName: "TEXT", notNull: 1, defaultValue: nullableString("'queued'")},
		{cid: 10, name: "created_at", typeName: "TIMESTAMP", notNull: 1, defaultValue: nullableString("CURRENT_TIMESTAMP")},
	}
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}
