package messages

import (
	"database/sql"
	"errors"
	"fmt"
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
