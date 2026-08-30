//go:build integration

package dolt_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

const (
	liveDoltGate       = "DOLT_TEST_INTEGRATION"
	corruptedChecksum  = "corrupted-by-live-dolt-test"
	serverStartTimeout = 5 * time.Second
	serverStopTimeout  = 5 * time.Second
)

var errOwnedDoltExited = errors.New("owned Dolt exited before readiness")

type ownedDoltProcess struct {
	cmd         *exec.Cmd
	done        chan struct{}
	address     string
	socketPath  string
	logPath     string
	waitErr     error
	logCloseErr error
}

type migrationRegistryRow struct {
	Component       string
	Version         int
	Name            string
	Checksum        string
	AppliedAt       time.Time
	AppliedBy       sql.NullString
	ExecutionTimeMS sql.NullInt64
	TablesCreated   string
}

type columnShape struct {
	dataType  string
	nullable  string
	charWidth sql.NullInt64
}

type indexColumn struct {
	nonUnique int
	sequence  int
	name      string
}

func TestLiveDoltMigrationContract(t *testing.T) {
	gate := os.Getenv(liveDoltGate)
	if gate == "" {
		t.Skip("set DOLT_TEST_INTEGRATION=1 to run the owned live-Dolt migration contract")
	}
	if gate != "1" {
		t.Fatalf("%s=%q, want exact opt-in value 1", liveDoltGate, gate)
	}

	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		t.Fatalf("resolve opted-in Dolt executable: %v", err)
	}

	// Keep the Unix socket below the macOS path-length limit while retaining
	// testing.T's checked recursive cleanup for every fixture artifact.
	t.Setenv("TMPDIR", "/tmp")
	root := t.TempDir()
	workspace := newOwnedWorkspace(t)
	t.Setenv("ENGRAM_TEST_MODE", "1")
	t.Setenv("ENGRAM_TEST_WORKSPACE", workspace)

	dataDir, childEnv := initializeOwnedDolt(t, doltPath, root, workspace)
	server, config := startOwnedDolt(t, doltPath, root, dataDir, childEnv, workspace)

	var active *dolt.Adapter
	t.Cleanup(func() {
		if active != nil {
			if closeErr := active.Close(); closeErr != nil {
				t.Errorf("close active Dolt adapter during cleanup: %v", closeErr)
			}
		}
	})

	active = openOwnedAdapter(t, &config)
	if err := active.ApplyMigrations(); err != nil {
		t.Fatalf("apply migrations to fresh owned Dolt: %v", err)
	}
	initialRegistry := assertMigrationRegistry(t, active.Conn())
	assertReservationSchema(t, active.Conn())
	assertReservationConstraints(t, active.Conn())
	closeOwnedAdapter(t, &active)

	active = openOwnedAdapter(t, &config)
	if err := active.ApplyMigrations(); err != nil {
		t.Fatalf("revalidate migrations through reopened adapter: %v", err)
	}
	reopenedRegistry := assertMigrationRegistry(t, active.Conn())
	if !reflect.DeepEqual(reopenedRegistry, initialRegistry) {
		t.Fatalf("reopened migration registry changed\ninitial:  %#v\nreopened: %#v", initialRegistry, reopenedRegistry)
	}
	closeOwnedAdapter(t, &active)

	active = openOwnedAdapter(t, &config)
	corruptMigrationChecksum(t, active.Conn(), corruptedChecksum)
	closeOwnedAdapter(t, &active)

	migration19 := requireMigration(t, 19)
	active = openOwnedAdapter(t, &config)
	err = active.ApplyMigrations()
	wantErr := fmt.Sprintf(
		"migration 19 checksum mismatch: checksum mismatch (stored: %s, expected: %s)",
		corruptedChecksum,
		migration19.Checksum,
	)
	if err == nil {
		t.Fatal("ApplyMigrations after checksum corruption succeeded, want rejection")
	}
	if err.Error() != wantErr {
		t.Fatalf("checksum rejection error = %q, want %q", err, wantErr)
	}
	assertStoredChecksum(t, active.Conn(), corruptedChecksum)
	closeOwnedAdapter(t, &active)

	t.Logf("validated 19 AGM migrations on owned Dolt at %s (server log %s)", server.address, server.logPath)
}

func newOwnedWorkspace(t *testing.T) string {
	t.Helper()
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("generate owned Dolt workspace identity: %v", err)
	}
	return "agm_live_" + hex.EncodeToString(suffix[:])
}

func initializeOwnedDolt(
	t *testing.T,
	doltPath string,
	root string,
	workspace string,
) (string, []string) {
	t.Helper()

	homeDir := filepath.Join(root, "home")
	tmpDir := filepath.Join(root, "tmp")
	dataDir := filepath.Join(root, "data")
	configDir := filepath.Join(root, "config")
	runtimeDir := filepath.Join(root, "runtime")
	logDir := filepath.Join(root, "logs")
	databaseDir := filepath.Join(dataDir, workspace)
	xdgConfigDir := filepath.Join(root, "xdg-config")
	xdgCacheDir := filepath.Join(root, "xdg-cache")
	xdgDataDir := filepath.Join(root, "xdg-data")

	for _, dir := range []string{
		homeDir,
		tmpDir,
		dataDir,
		configDir,
		runtimeDir,
		logDir,
		databaseDir,
		xdgConfigDir,
		xdgCacheDir,
		xdgDataDir,
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create owned Dolt directory %s: %v", dir, err)
		}
	}

	childEnv := ownedDoltEnvironment(root)
	runOwnedDoltCommand(
		t,
		doltPath,
		root,
		childEnv,
		"config",
		"--global",
		"--add",
		"metrics.disabled",
		"true",
	)
	runOwnedDoltCommand(
		t,
		doltPath,
		databaseDir,
		childEnv,
		"init",
		"--name",
		"agm-live-test",
		"--email",
		"agm-live-test@example.invalid",
		"--initial-branch",
		"main",
	)

	return dataDir, childEnv
}

func ownedDoltEnvironment(root string) []string {
	filtered := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if strings.HasPrefix(key, "DOLT_") {
			continue
		}
		switch key {
		case "HOME", "TMPDIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME":
			continue
		}
		filtered = append(filtered, entry)
	}

	return append(
		filtered,
		"HOME="+filepath.Join(root, "home"),
		"TMPDIR="+filepath.Join(root, "tmp"),
		"XDG_CONFIG_HOME="+filepath.Join(root, "xdg-config"),
		"XDG_CACHE_HOME="+filepath.Join(root, "xdg-cache"),
		"XDG_DATA_HOME="+filepath.Join(root, "xdg-data"),
		"DOLT_DISABLE_EVENT_FLUSH=1",
	)
}

func runOwnedDoltCommand(
	t *testing.T,
	doltPath string,
	dir string,
	env []string,
	args ...string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, doltPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("owned Dolt command %q timed out: %v\n%s", args, ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("owned Dolt command %q failed: %v\n%s", args, err, output)
	}
}

func startOwnedDolt(
	t *testing.T,
	doltPath string,
	root string,
	dataDir string,
	env []string,
	workspace string,
) (*ownedDoltProcess, dolt.Config) {
	t.Helper()

	var lastCollision string
	for attempt := 1; attempt <= 3; attempt++ {
		port := reserveLoopbackPort(t)
		socketPath := filepath.Join(root, "runtime", fmt.Sprintf("dolt-attempt-%d.sock", attempt))
		logPath := filepath.Join(root, "logs", fmt.Sprintf("dolt-attempt-%d.log", attempt))
		process, err := launchOwnedDolt(
			doltPath,
			root,
			dataDir,
			filepath.Join(root, "config"),
			env,
			port,
			socketPath,
			logPath,
		)
		if err != nil {
			t.Fatalf("start owned Dolt attempt %d: %v", attempt, err)
		}

		readyErr := waitForOwnedDolt(t.Context(), process, workspace, port)
		if readyErr == nil {
			config := dolt.Config{
				Workspace: workspace,
				Port:      strconv.Itoa(port),
				Host:      "127.0.0.1",
				Database:  workspace,
				User:      "root",
				Password:  "",
			}
			t.Cleanup(func() {
				if stopErr := process.stopAndWait(); stopErr != nil {
					t.Errorf(
						"owned Dolt cleanup failed: %v\nserver log %s:\n%s",
						stopErr,
						process.logPath,
						process.logContents(),
					)
				}
			})
			return process, config
		}

		logText := process.logContents()
		if errors.Is(readyErr, errOwnedDoltExited) &&
			strings.Contains(strings.ToLower(logText), "already in use") {
			if cleanupErr := process.verifyExitedAttemptArtifacts(); cleanupErr != nil {
				t.Fatalf("bind-collision attempt %d cleanup failed: %v", attempt, cleanupErr)
			}
			lastCollision = fmt.Sprintf("attempt %d on port %d: %s", attempt, port, strings.TrimSpace(logText))
			continue
		}

		cleanupErr := process.stopAndWait()
		t.Fatalf(
			"owned Dolt readiness failed: %v\ncleanup: %v\nserver log %s:\n%s",
			readyErr,
			cleanupErr,
			logPath,
			process.logContents(),
		)
	}

	t.Fatalf("owned Dolt could not bind after three diagnosed collisions; last: %s", lastCollision)
	return nil, dolt.Config{}
}

func reserveLoopbackPort(t *testing.T) int {
	t.Helper()

	for range 10 {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve owned loopback port: %v", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			t.Fatalf("release owned loopback port %d: %v", port, err)
		}
		if port != 3307 {
			return port
		}
	}

	t.Fatal("failed to reserve an owned loopback port other than 3307")
	return 0
}

func launchOwnedDolt(
	doltPath string,
	root string,
	dataDir string,
	configDir string,
	env []string,
	port int,
	socketPath string,
	logPath string,
) (*ownedDoltProcess, error) {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create server log: %w", err)
	}

	args := []string{
		"sql-server",
		"--host=127.0.0.1",
		fmt.Sprintf("--port=%d", port),
		"--data-dir=" + dataDir,
		"--doltcfg-dir=" + configDir,
		"--privilege-file=" + filepath.Join(configDir, "privileges.db"),
		"--branch-control-file=" + filepath.Join(configDir, "branch_control.db"),
		"--socket=" + socketPath,
		"--loglevel=info",
		"--logformat=text",
		"--event-scheduler=OFF",
		"--max-connections=16",
	}
	cmd := exec.Command(doltPath, args...)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(fmt.Errorf("start Dolt sql-server: %w", err), logFile.Close())
	}

	process := &ownedDoltProcess{
		cmd:        cmd,
		done:       make(chan struct{}),
		address:    net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		socketPath: socketPath,
		logPath:    logPath,
	}
	go func() {
		process.waitErr = cmd.Wait()
		process.logCloseErr = logFile.Close()
		close(process.done)
	}()
	return process, nil
}

func waitForOwnedDolt(
	ctx context.Context,
	process *ownedDoltProcess,
	workspace string,
	port int,
) (resultErr error) {
	driverConfig := mysql.NewConfig()
	driverConfig.User = "root"
	driverConfig.Net = "unix"
	driverConfig.Addr = process.socketPath
	driverConfig.DBName = workspace
	driverConfig.ParseTime = true

	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return fmt.Errorf("open readiness connection: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close readiness connection: %w", closeErr))
		}
	}()
	db.SetMaxOpenConns(1)

	overall, cancel := context.WithTimeout(ctx, serverStartTimeout)
	defer cancel()
	var lastErr error
	for {
		select {
		case <-process.done:
			return process.readinessExitError()
		default:
		}

		probeCtx, probeCancel := context.WithTimeout(overall, 250*time.Millisecond)
		err := db.PingContext(probeCtx)
		if err == nil {
			var observedDatabase string
			var observedPort int
			err = db.QueryRowContext(probeCtx, "SELECT DATABASE(), @@port").Scan(
				&observedDatabase,
				&observedPort,
			)
			if err == nil {
				probeCancel()
				if observedDatabase != workspace || observedPort != port {
					return fmt.Errorf(
						"readiness identity = %s:%d, want %s:%d",
						observedDatabase,
						observedPort,
						workspace,
						port,
					)
				}
				select {
				case <-process.done:
					return process.readinessExitError()
				default:
					return nil
				}
			}
		}
		probeCancel()
		lastErr = err

		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-process.done:
			if !timer.Stop() {
				<-timer.C
			}
			return process.readinessExitError()
		case <-overall.Done():
			if !timer.Stop() {
				<-timer.C
			}
			deadlineErr := fmt.Errorf("readiness deadline exceeded: %w", overall.Err())
			if lastErr == nil {
				return deadlineErr
			}
			return errors.Join(deadlineErr, fmt.Errorf("last readiness probe: %w", lastErr))
		case <-timer.C:
		}
	}
}

func (process *ownedDoltProcess) stopAndWait() error {
	var cleanupErrs []error
	select {
	case <-process.done:
		cleanupErrs = append(cleanupErrs, errors.New("server exited before teardown"))
	default:
		if err := process.cmd.Process.Signal(os.Interrupt); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("interrupt server: %w", err))
			if killErr := process.cmd.Process.Kill(); killErr != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("kill after interrupt failure: %w", killErr))
			}
		}

		if !waitForProcessExit(process.done, serverStopTimeout) {
			cleanupErrs = append(cleanupErrs, errors.New("graceful shutdown exceeded 5s; forced kill required"))
			if err := process.cmd.Process.Kill(); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("kill server after graceful timeout: %w", err))
			}
			if !waitForProcessExit(process.done, serverStopTimeout) {
				cleanupErrs = append(cleanupErrs, errors.New("server wait did not return within 5s after kill"))
			}
		}
	}

	select {
	case <-process.done:
		if process.waitErr != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("wait for server: %w", process.waitErr))
		}
	default:
		return errors.Join(cleanupErrs...)
	}

	cleanupErrs = append(cleanupErrs, process.verifyStoppedArtifacts())
	return errors.Join(cleanupErrs...)
}

func (process *ownedDoltProcess) verifyStoppedArtifacts() error {
	return errors.Join(
		process.verifyExitedAttemptArtifacts(),
		requireEndpointAbsent(process.address, 500*time.Millisecond),
	)
}

func (process *ownedDoltProcess) verifyExitedAttemptArtifacts() error {
	select {
	case <-process.done:
	default:
		return errors.New("owned Dolt process has not been waited")
	}

	return errors.Join(
		wrapIfError("close server log", process.logCloseErr),
		requirePathAbsent(process.socketPath, 500*time.Millisecond),
	)
}

func wrapIfError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func waitForProcessExit(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func requireEndpointAbsent(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		connection, err := net.DialTimeout("tcp4", address, 50*time.Millisecond)
		if err != nil {
			return nil
		}
		_ = connection.Close()
		if time.Now().After(deadline) {
			return fmt.Errorf("owned Dolt listener %s still accepts connections", address)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		<-timer.C
	}
}

func requirePathAbsent(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("stat owned Dolt socket %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("owned Dolt socket still exists: %s", path)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		<-timer.C
	}
}

func (process *ownedDoltProcess) logContents() string {
	content, err := os.ReadFile(process.logPath)
	if err != nil {
		return fmt.Sprintf("<read log: %v>", err)
	}
	const maxDiagnosticBytes = 64 << 10
	if len(content) > maxDiagnosticBytes {
		content = content[len(content)-maxDiagnosticBytes:]
		return "<last 64 KiB>\n" + string(content)
	}
	return string(content)
}

func (process *ownedDoltProcess) readinessExitError() error {
	if process.waitErr == nil {
		return errOwnedDoltExited
	}
	return errors.Join(errOwnedDoltExited, fmt.Errorf("wait for server: %w", process.waitErr))
}

func openOwnedAdapter(t *testing.T, config *dolt.Config) *dolt.Adapter {
	t.Helper()
	adapter, err := dolt.New(config)
	if err != nil {
		t.Fatalf("open adapter to owned Dolt: %v", err)
	}
	return adapter
}

func closeOwnedAdapter(t *testing.T, active **dolt.Adapter) {
	t.Helper()
	adapter := *active
	*active = nil
	if adapter == nil {
		return
	}
	if err := adapter.Close(); err != nil {
		t.Fatalf("close adapter to owned Dolt: %v", err)
	}
}

func assertMigrationRegistry(t *testing.T, db *sql.DB) []migrationRegistryRow {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT component, version, name, checksum, applied_at, applied_by,
		       execution_time_ms, tables_created
		FROM agm_migrations
		WHERE component = 'agm'
		ORDER BY version ASC
	`)
	if err != nil {
		t.Fatalf("query owned migration registry: %v", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("close migration registry rows: %v", closeErr)
		}
	}()

	var registry []migrationRegistryRow
	for rows.Next() {
		var row migrationRegistryRow
		var tablesCreated []byte
		if err := rows.Scan(
			&row.Component,
			&row.Version,
			&row.Name,
			&row.Checksum,
			&row.AppliedAt,
			&row.AppliedBy,
			&row.ExecutionTimeMS,
			&tablesCreated,
		); err != nil {
			t.Fatalf("scan owned migration registry: %v", err)
		}
		row.TablesCreated = string(tablesCreated)
		registry = append(registry, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate owned migration registry: %v", err)
	}

	migrations := dolt.AllMigrations()
	if len(migrations) != 19 {
		t.Fatalf("source migration count = %d, want 19", len(migrations))
	}
	if len(registry) != len(migrations) {
		t.Fatalf("live registry row count = %d, want %d", len(registry), len(migrations))
	}
	for index, migration := range migrations {
		wantVersion := index + 1
		if migration.Version != wantVersion {
			t.Fatalf("source migration index %d has version %d, want %d", index, migration.Version, wantVersion)
		}
		row := registry[index]
		if row.Component != "agm" ||
			row.Version != migration.Version ||
			row.Name != migration.Name ||
			row.Checksum != migration.Checksum {
			t.Fatalf(
				"live migration row %d = (%q, %d, %q, %q), want (%q, %d, %q, %q)",
				index,
				row.Component,
				row.Version,
				row.Name,
				row.Checksum,
				"agm",
				migration.Version,
				migration.Name,
				migration.Checksum,
			)
		}
	}
	return registry
}

func assertReservationSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, CHARACTER_MAXIMUM_LENGTH
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'agm_session_name_reservations'
		ORDER BY ORDINAL_POSITION
	`)
	if err != nil {
		t.Fatalf("query reservation columns: %v", err)
	}
	columns := make(map[string]columnShape)
	for rows.Next() {
		var name string
		var shape columnShape
		if err := rows.Scan(&name, &shape.dataType, &shape.nullable, &shape.charWidth); err != nil {
			_ = rows.Close()
			t.Fatalf("scan reservation column: %v", err)
		}
		columns[name] = shape
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("iterate reservation columns: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close reservation column rows: %v", err)
	}

	wantColumns := map[string]columnShape{
		"workspace":  {dataType: "varchar", nullable: "NO", charWidth: sql.NullInt64{Int64: 255, Valid: true}},
		"name":       {dataType: "varchar", nullable: "NO", charWidth: sql.NullInt64{Int64: 255, Valid: true}},
		"session_id": {dataType: "varchar", nullable: "NO", charWidth: sql.NullInt64{Int64: 255, Valid: true}},
		"created_at": {dataType: "timestamp", nullable: "NO"},
		"expires_at": {dataType: "timestamp", nullable: "NO"},
	}
	for name, want := range wantColumns {
		got, ok := columns[name]
		if !ok {
			t.Fatalf("reservation column %q is absent; got %#v", name, columns)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("reservation column %q = %#v, want %#v", name, got, want)
		}
	}

	indexRows, err := db.QueryContext(ctx, `
		SELECT INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'agm_session_name_reservations'
		ORDER BY INDEX_NAME, SEQ_IN_INDEX
	`)
	if err != nil {
		t.Fatalf("query reservation indexes: %v", err)
	}
	indexes := make(map[string][]indexColumn)
	for indexRows.Next() {
		var indexName string
		var part indexColumn
		if err := indexRows.Scan(&indexName, &part.nonUnique, &part.sequence, &part.name); err != nil {
			_ = indexRows.Close()
			t.Fatalf("scan reservation index: %v", err)
		}
		indexes[indexName] = append(indexes[indexName], part)
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		t.Fatalf("iterate reservation indexes: %v", err)
	}
	if err := indexRows.Close(); err != nil {
		t.Fatalf("close reservation index rows: %v", err)
	}

	wantIndexes := map[string][]indexColumn{
		"PRIMARY": {
			{nonUnique: 0, sequence: 1, name: "workspace"},
			{nonUnique: 0, sequence: 2, name: "name"},
		},
		"uq_agm_session_name_reservation_owner": {
			{nonUnique: 0, sequence: 1, name: "workspace"},
			{nonUnique: 0, sequence: 2, name: "session_id"},
		},
		"idx_agm_session_name_reservation_expiry": {
			{nonUnique: 1, sequence: 1, name: "expires_at"},
		},
	}
	for name, want := range wantIndexes {
		got, ok := indexes[name]
		if !ok {
			t.Fatalf("reservation index %q is absent; got %#v", name, indexes)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("reservation index %q = %#v, want %#v", name, got, want)
		}
	}
}

func assertReservationConstraints(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	createdAt := time.Now().UTC().Truncate(time.Second)
	expiresAt := createdAt.Add(time.Hour)
	insert := func(workspace, name, sessionID string) error {
		_, err := db.ExecContext(ctx, `
			INSERT INTO agm_session_name_reservations
			    (workspace, name, session_id, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?)
		`, workspace, name, sessionID, createdAt, expiresAt)
		return err
	}

	if err := insert("workspace-a", "name-a", "session-a"); err != nil {
		t.Fatalf("insert initial reservation: %v", err)
	}
	requireDuplicateKey(t, insert("workspace-a", "name-a", "session-b"), "workspace/name owner")
	requireDuplicateKey(t, insert("workspace-a", "name-b", "session-a"), "workspace/session owner")
	if err := insert("workspace-b", "name-a", "session-a"); err != nil {
		t.Fatalf("insert same name/session in second workspace: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM agm_session_name_reservations
		WHERE name = 'name-a' AND session_id = 'session-a'
	`).Scan(&count); err != nil {
		t.Fatalf("count successful cross-workspace reservations: %v", err)
	}
	if count != 2 {
		t.Fatalf("cross-workspace reservation count = %d, want 2", count)
	}
}

func requireDuplicateKey(t *testing.T, err error, constraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s insert succeeded, want duplicate-key rejection", constraint)
	}
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		t.Fatalf("%s error type = %T (%v), want *mysql.MySQLError", constraint, err, err)
	}
	if mysqlErr.Number != 1062 {
		t.Fatalf("%s MySQL error number = %d, want 1062: %v", constraint, mysqlErr.Number, err)
	}
}

func corruptMigrationChecksum(t *testing.T, db *sql.DB, checksum string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx, `
		UPDATE agm_migrations
		SET checksum = ?
		WHERE component = 'agm' AND version = 19
	`, checksum); err != nil {
		t.Fatalf("corrupt owned migration-19 checksum: %v", err)
	}
	assertStoredChecksum(t, db, checksum)
}

func assertStoredChecksum(t *testing.T, db *sql.DB, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	var got string
	if err := db.QueryRowContext(ctx, `
		SELECT checksum
		FROM agm_migrations
		WHERE component = 'agm' AND version = 19
	`).Scan(&got); err != nil {
		t.Fatalf("query stored migration-19 checksum: %v", err)
	}
	if got != want {
		t.Fatalf("stored migration-19 checksum = %q, want %q", got, want)
	}
}

func requireMigration(t *testing.T, version int) dolt.Migration {
	t.Helper()
	for _, migration := range dolt.AllMigrations() {
		if migration.Version == version {
			return migration
		}
	}
	t.Fatalf("source migration %d is absent", version)
	return dolt.Migration{}
}
