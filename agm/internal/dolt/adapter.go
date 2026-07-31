package dolt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver for Dolt
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver for isolated AGM test environments
)

func init() {
	// Initialize lookupEnv with os.LookupEnv
	lookupEnv = os.LookupEnv
}

// isRunningInTest detects both go test binaries and built subprocesses whose
// caller explicitly enables the test protocol.
func isRunningInTest() bool {
	testMode := getEnv("ENGRAM_TEST_MODE", "")
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}
	return testExecution(executable, testMode)
}

func testExecution(executable, testMode string) bool {
	return testModeEnabled(testMode) || strings.Contains(executable, ".test")
}

func testModeEnabled(value string) bool {
	return value == "1" || value == "true"
}

// Adapter provides Dolt-based storage for AGM sessions
// Replaces dual-layer SQLite+JSONL storage with single Dolt database
type Adapter struct {
	conn              *sql.DB
	workspace         string
	port              string
	migrationsApplied bool
	testStore         bool
}

// NewSQLiteAdapter opens the persistent session store used only by an AGM test
// environment. It deliberately exposes the same Adapter contract as Dolt so
// lifecycle commands do not gain a second, file-backed manifest path.
func NewSQLiteAdapter(path string) (*Adapter, error) {
	if path == "" {
		return nil, fmt.Errorf("SQLite database path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create SQLite database directory: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open SQLite session store: %w", err)
	}
	// The SQLite adapter is a test-environment stand-in for Dolt. Keep its
	// writes on one connection so concurrent callers queue in database/sql
	// instead of racing separate SQLite writers into SQLITE_BUSY. The durable
	// name reservation still arbitrates interleaved create lifecycles.
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil { //nolint:noctx // connection validation
		_ = conn.Close()
		return nil, fmt.Errorf("ping SQLite session store: %w", err)
	}

	if _, err := conn.Exec(sqliteSessionSchema); err != nil { //nolint:noctx // schema initialization
		_ = conn.Close()
		return nil, fmt.Errorf("initialize SQLite session store: %w", err)
	}
	if err := upgradeSQLiteSessionSchema(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &Adapter{
		conn:              conn,
		workspace:         "test",
		migrationsApplied: true,
		testStore:         true,
	}, nil
}

func upgradeSQLiteSessionSchema(conn *sql.DB) error {
	hasRevision, err := sqliteSessionColumnExists(conn, "tmux_session_revision")
	if err != nil {
		return fmt.Errorf("inspect SQLite session store schema: %w", err)
	}
	if hasRevision {
		return nil
	}
	if _, err := conn.Exec(`ALTER TABLE agm_sessions ADD COLUMN tmux_session_revision TEXT`); err != nil { //nolint:noctx // startup schema upgrade
		// Another opener may have completed the idempotent upgrade after the
		// first inspection. Verify the postcondition before reporting failure.
		upgraded, verifyErr := sqliteSessionColumnExists(conn, "tmux_session_revision")
		if verifyErr == nil && upgraded {
			return nil
		}
		return fmt.Errorf("upgrade SQLite session store with tmux ownership revision: %w", err)
	}
	return nil
}

func sqliteSessionColumnExists(conn *sql.DB, column string) (bool, error) {
	rows, err := conn.Query(`PRAGMA table_info(agm_sessions)`) //nolint:noctx // startup schema inspection
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			columnID     int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(&columnID, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

// IsTestStore reports whether this adapter backs an isolated AGM test
// environment rather than a Dolt workspace.
func (a *Adapter) IsTestStore() bool { return a.testStore }

const sqliteSessionSchema = `
CREATE TABLE IF NOT EXISTS agm_sessions (
  id TEXT PRIMARY KEY,
  created_at TIMESTAMP,
  updated_at TIMESTAMP,
  status TEXT,
  workspace TEXT NOT NULL,
  model TEXT,
  name TEXT,
  harness TEXT,
  context_project TEXT,
  context_purpose TEXT,
  context_tags TEXT,
  context_notes TEXT,
  claude_uuid TEXT,
  tmux_session_name TEXT,
  tmux_session_revision TEXT,
  metadata TEXT,
  permission_mode TEXT,
  permission_mode_updated_at TIMESTAMP,
  permission_mode_source TEXT,
  parent_session_id TEXT,
  is_test BOOLEAN NOT NULL DEFAULT TRUE,
  access_count INTEGER NOT NULL DEFAULT 0,
  last_accessed_at TIMESTAMP,
  context_total_tokens INTEGER,
  context_used_tokens INTEGER,
  context_percentage_used REAL,
  monitors TEXT
);
CREATE INDEX IF NOT EXISTS idx_agm_sessions_workspace_updated
  ON agm_sessions(workspace, updated_at DESC);
CREATE TABLE IF NOT EXISTS agm_session_name_reservations (
  workspace TEXT NOT NULL,
  name TEXT NOT NULL,
  session_id TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  PRIMARY KEY (workspace, name),
  UNIQUE (workspace, session_id)
);
CREATE INDEX IF NOT EXISTS idx_agm_session_name_reservation_expiry
  ON agm_session_name_reservations(expires_at);
CREATE TABLE IF NOT EXISTS agm_harness_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  switched_at TIMESTAMP NOT NULL,
  from_harness TEXT NOT NULL,
  to_harness TEXT NOT NULL
);
`

// Config holds Dolt connection configuration
type Config struct {
	Workspace   string // Workspace name (e.g., "oss", "acme")
	Port        string // Dolt server port (e.g., "3307" for oss, "3308" for acme)
	Host        string // Dolt server host (default: "127.0.0.1")
	Database    string // Database name (default: "workspace")
	User        string // Database user (default: "root")
	Password    string // Database password (default: "")
	StartScript string // Path to auto-start script (empty = disabled)
}

// agmConfigPath is the path to the AGM workspace config, overridable in tests.
var agmConfigPath = "~/.agm/config.yaml"

// readDefaultWorkspaceFromConfig reads default_workspace from the AGM config file.
// Returns empty string on any error (file missing, malformed YAML, etc.).
func readDefaultWorkspaceFromConfig() string {
	return readDefaultWorkspaceFromConfigAt(agmConfigPath)
}

func readDefaultWorkspaceFromConfigAt(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, path[2:])
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg struct {
		DefaultWorkspace string `yaml:"default_workspace"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return cfg.DefaultWorkspace
}

// DefaultConfig returns default configuration from environment
func DefaultConfig() (*Config, error) {
	return defaultConfigAt(agmConfigPath)
}

func defaultConfigAt(workspaceConfigPath string) (*Config, error) {
	workspace := selectedWorkspaceAt(workspaceConfigPath)
	if workspace == "" {
		return nil, fmt.Errorf("WORKSPACE environment variable not set (workspace protocol not activated)\n" +
			"Hint: Set WORKSPACE=<name> or configure default_workspace in ~/.agm/config.yaml")
	}
	return configForWorkspace(workspace)
}

func selectedWorkspaceAt(workspaceConfigPath string) string {
	workspace := getEnv("WORKSPACE", "")
	if testModeEnabled(getEnv("ENGRAM_TEST_MODE", "")) {
		if testWorkspace := getEnv("ENGRAM_TEST_WORKSPACE", ""); testWorkspace != "" {
			workspace = testWorkspace
		}
	}
	if workspace == "" {
		// Fall back to default_workspace from ~/.agm/config.yaml so the MCP
		// server works when invoked outside a workspace context (e.g. Dispatch
		// or Cowork where WORKSPACE is not set in the environment).
		workspace = readDefaultWorkspaceFromConfigAt(workspaceConfigPath)
	}
	return workspace
}

func configForWorkspace(workspace string) (*Config, error) {
	database := getEnv("DOLT_DATABASE", workspace)
	if workspace == "" {
		return nil, fmt.Errorf("WORKSPACE environment variable not set (workspace protocol not activated)\n" +
			"Hint: Set WORKSPACE=<name> or configure default_workspace in ~/.agm/config.yaml")
	}

	// CRITICAL: Fail-fast enforcement to prevent test pollution
	// Tests MUST set ENGRAM_TEST_MODE=1 and use a test-specific workspace
	if isRunningInTest() {
		if err := validateTestExecutionTarget(workspace, database); err != nil {
			return nil, err
		}
	}

	port := getEnv("DOLT_PORT", "3307")
	host := getEnv("DOLT_HOST", "127.0.0.1")
	// Default database name to workspace name for proper workspace isolation
	// In production: oss → database "oss", acme → database "acme"
	// In tests: test → database "test"
	user := getEnv("DOLT_USER", "root")
	password := getEnv("DOLT_PASSWORD", "")

	startScript := getEnv("AGM_DOLT_START_SCRIPT", "")
	if startScript == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			defaultPath := filepath.Join(home, ".local", "bin", "ensure-dolt-agm.sh")
			if _, err := os.Stat(defaultPath); err == nil {
				startScript = defaultPath
			}
		}
	}

	return &Config{
		Workspace:   workspace,
		Port:        port,
		Host:        host,
		Database:    database,
		User:        user,
		Password:    password,
		StartScript: startScript,
	}, nil
}

// ConfiguredWorkspaceConfigs returns every enabled workspace store that
// destructive cross-repository maintenance must query before it can treat its
// active-session set as complete. An explicit DOLT_DATABASE is accepted only
// when it describes a single workspace; applying one database name to multiple
// workspace configs would silently invent a cross-workspace mapping. Without
// that override, each workspace uses its conventional same-name database.
func ConfiguredWorkspaceConfigs() ([]*Config, error) {
	return ConfiguredWorkspaceConfigsAt(agmConfigPath)
}

// ConfiguredWorkspaceConfigsAt is the explicit-path variant used by AGM
// commands after loading their configured workspace registry. Destructive
// inventory must query the same registry that session creation uses.
func ConfiguredWorkspaceConfigsAt(workspaceConfigPath string) ([]*Config, error) {
	path := expandTilde(workspaceConfigPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		base, baseErr := defaultConfigAt(workspaceConfigPath)
		if baseErr != nil {
			return nil, baseErr
		}
		return []*Config{base}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read AGM workspace config: %w", err)
	}
	enabled, hasRegistryEntries, err := parseEnabledWorkspaceNames(data)
	if err != nil {
		return nil, err
	}
	if len(enabled) == 0 {
		if hasRegistryEntries {
			return nil, fmt.Errorf("AGM workspace config has no enabled workspaces")
		}
		base, baseErr := defaultConfigAt(workspaceConfigPath)
		if baseErr != nil {
			return nil, baseErr
		}
		return []*Config{base}, nil
	}
	return configuredWorkspaceConfigsFromRegistry(enabled, data)
}

func configuredWorkspaceConfigsFromRegistry(enabled []string, data []byte) ([]*Config, error) {
	base, err := configForWorkspace(enabled[0])
	if err != nil {
		return nil, err
	}
	endpoints, err := parseWorkspaceDoltEndpoints(data)
	if err != nil {
		return nil, err
	}
	_, sharedPortExplicit := lookupEnv("DOLT_PORT")
	explicitDatabase, databaseIsExplicit := lookupEnv("DOLT_DATABASE")
	databaseIsExplicit = databaseIsExplicit && explicitDatabase != ""
	seen := make(map[string]bool, len(enabled))
	configs := make([]*Config, 0, len(enabled))
	add := func(workspace string) {
		if workspace == "" || seen[workspace] {
			return
		}
		seen[workspace] = true
		workspaceConfig := *base
		workspaceConfig.Workspace = workspace
		endpoint := endpoints[workspace]
		applyWorkspaceDoltEndpoint(&workspaceConfig, endpoint)
		if endpoint.Port == "" && len(enabled) > 1 && !sharedPortExplicit {
			return
		}
		if databaseIsExplicit {
			workspaceConfig.Database = explicitDatabase
		} else if endpoints[workspace].Database == "" {
			workspaceConfig.Database = workspace
		}
		configs = append(configs, &workspaceConfig)
	}
	for _, workspace := range enabled {
		add(workspace)
	}
	if len(configs) != len(enabled) {
		return nil, fmt.Errorf("multiple enabled workspaces require either an explicit shared DOLT_PORT or a per-workspace dolt.port in the AGM registry")
	}
	return validateConfiguredWorkspaceConfigs(configs, base, explicitDatabase, databaseIsExplicit)
}

func applyWorkspaceDoltEndpoint(config *Config, endpoint workspaceDoltEndpoint) {
	if endpoint.Port != "" {
		config.Port = endpoint.Port
	}
	if endpoint.Host != "" {
		config.Host = endpoint.Host
	}
	if endpoint.User != "" {
		config.User = endpoint.User
	}
	if endpoint.Password != "" {
		config.Password = endpoint.Password
	}
	if endpoint.StartScript != "" {
		config.StartScript = expandTilde(endpoint.StartScript)
	}
	if endpoint.Database != "" {
		config.Database = endpoint.Database
	}
}

type workspaceDoltEndpoint struct {
	Host        string `yaml:"host"`
	Port        string `yaml:"port"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	StartScript string `yaml:"start_script"`
	Database    string `yaml:"database"`
}

func parseWorkspaceDoltEndpoints(data []byte) (map[string]workspaceDoltEndpoint, error) {
	var cfg struct {
		Workspaces []struct {
			Name string                `yaml:"name"`
			Dolt workspaceDoltEndpoint `yaml:"dolt"`
		} `yaml:"workspaces"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse AGM workspace Dolt endpoints: %w", err)
	}
	endpoints := make(map[string]workspaceDoltEndpoint, len(cfg.Workspaces))
	for _, workspace := range cfg.Workspaces {
		if name := strings.TrimSpace(workspace.Name); name != "" {
			endpoints[name] = workspace.Dolt
		}
	}
	return endpoints, nil
}

func parseEnabledWorkspaceNames(data []byte) ([]string, bool, error) {
	var cfg struct {
		Workspaces []struct {
			Name    string `yaml:"name"`
			Enabled *bool  `yaml:"enabled"`
		} `yaml:"workspaces"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, false, fmt.Errorf("parse AGM workspace config: %w", err)
	}
	enabled := make([]string, 0, len(cfg.Workspaces))
	for index, workspace := range cfg.Workspaces {
		if workspace.Enabled == nil || !*workspace.Enabled {
			continue
		}
		name := strings.TrimSpace(workspace.Name)
		if name == "" {
			return nil, true, fmt.Errorf("AGM workspace config entry %d has no name", index+1)
		}
		enabled = append(enabled, name)
	}
	return enabled, len(cfg.Workspaces) > 0, nil
}

func validateConfiguredWorkspaceConfigs(configs []*Config, base *Config, explicitDatabase string, databaseIsExplicit bool) ([]*Config, error) {
	if len(configs) == 0 {
		return []*Config{base}, nil
	}
	if databaseIsExplicit && len(configs) > 1 {
		return nil, fmt.Errorf(
			"DOLT_DATABASE=%q scopes one store but %d enabled workspaces are configured; cannot prove a complete cross-workspace active-session inventory",
			explicitDatabase,
			len(configs),
		)
	}
	return configs, nil
}

func validateTestExecutionTarget(workspace, database string) error {
	if !testModeEnabled(getEnv("ENGRAM_TEST_MODE", "")) {
		return fmt.Errorf("TEST POLLUTION BLOCKED: Tests must set ENGRAM_TEST_MODE=1\n\n"+
			"Why: Without test isolation, tests write to production databases causing data pollution.\n\n"+
			"Fix: Run tests with proper isolation:\n"+
			"  ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test go test ./...\n\n"+
			"Or use testutil.SetupTestEnvironment(t) in your test setup function.\n\n"+
			"Current workspace: %s (attempted during test)", workspace)
	}
	return validateTestTarget(workspace, database, getEnv("ENGRAM_TEST_WORKSPACE", ""))
}

func validateTestTarget(workspace, database, testWorkspace string) error {
	if testWorkspace == "" {
		return fmt.Errorf("TEST POLLUTION BLOCKED: ENGRAM_TEST_WORKSPACE must explicitly select the isolated test workspace")
	}
	if workspace != testWorkspace {
		return fmt.Errorf("TEST POLLUTION BLOCKED: resolved workspace %q does not match ENGRAM_TEST_WORKSPACE %q", workspace, testWorkspace)
	}
	if database != testWorkspace && database != "agm_test" {
		return fmt.Errorf("TEST POLLUTION BLOCKED: resolved database %q is not the selected test workspace %q or the shared agm_test database", database, testWorkspace)
	}
	return nil
}

// New creates a new Dolt adapter with the given configuration
func New(config *Config) (*Adapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if config.Workspace == "" {
		return nil, fmt.Errorf("workspace cannot be empty")
	}
	if config.Port == "" {
		return nil, fmt.Errorf("port cannot be empty")
	}
	if isRunningInTest() {
		if err := validateTestExecutionTarget(config.Workspace, config.Database); err != nil {
			return nil, err
		}
	}

	// Build MySQL DSN for Dolt connection
	// Format: user:password@tcp(host:port)/database
	dsn := buildDSN(config)

	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open dolt connection: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil { //nolint:noctx // TODO(context): plumb ctx through this layer
		conn.Close()

		// Try auto-start if script configured and not in test
		if config.StartScript != "" && !isRunningInTest() {
			startErr := tryAutoStart(config.StartScript)
			if startErr != nil {
				return nil, fmt.Errorf("failed to connect to Dolt: %w\nAuto-start failed: %w", err, startErr)
			}

			// Retry with backoff — the auto-start script waits for TCP,
			// but there can be a brief gap before the MySQL protocol is ready
			for attempt := 0; attempt < 10; attempt++ {
				time.Sleep(500 * time.Millisecond)
				conn2, err2 := sql.Open("mysql", dsn)
				if err2 != nil {
					continue
				}
				if err3 := conn2.Ping(); err3 == nil { //nolint:noctx // TODO(context): plumb ctx through this layer
					return &Adapter{
						conn:      conn2,
						workspace: config.Workspace,
						port:      config.Port,
					}, nil
				}
				conn2.Close()
			}
			return nil, fmt.Errorf("failed to connect to Dolt after auto-start "+
				"(script succeeded but server not responding after 5s): %w", err)
		}

		hint := fmt.Sprintf("Hint: Ensure Dolt server is running on port %s", config.Port)
		if config.StartScript != "" {
			hint += fmt.Sprintf("\nStart with: %s", config.StartScript)
		}
		return nil, fmt.Errorf("failed to connect to Dolt (is it running?): %w\n%s", err, hint)
	}

	adapter := &Adapter{
		conn:      conn,
		workspace: config.Workspace,
		port:      config.Port,
	}

	return adapter, nil
}

// expandTilde expands ~ prefix to user home directory
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// tryAutoStart attempts to start the Dolt server using the configured script
func tryAutoStart(scriptPath string) error {
	expanded := expandTilde(scriptPath)

	info, err := os.Stat(expanded)
	if err != nil {
		return fmt.Errorf("auto-start script not found: %s", expanded)
	}
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("auto-start script not executable: %s", expanded)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, expanded)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("auto-start script timed out after 10s: %s", expanded)
	}
	if err != nil {
		return fmt.Errorf("auto-start script failed: %w\nOutput: %s",
			err, strings.TrimSpace(string(output)))
	}

	return nil
}

// Close closes the Dolt connection
func (a *Adapter) Close() error {
	if a.conn == nil {
		return nil
	}
	return a.conn.Close()
}

// Conn returns the underlying database connection for direct access
func (a *Adapter) Conn() *sql.DB {
	return a.conn
}

// BeginTx starts a new transaction
func (a *Adapter) BeginTx() (*sql.Tx, error) {
	return a.conn.Begin() //nolint:noctx // TODO(context): plumb ctx through this layer
}

// Workspace returns the current workspace name
func (a *Adapter) Workspace() string {
	return a.workspace
}

// ExecSQL executes a SQL statement with the given arguments
// This is a helper method for admin commands that need direct SQL access
func (a *Adapter) ExecSQL(ctx context.Context, query string, args ...interface{}) error {
	_, err := a.conn.ExecContext(ctx, query, args...)
	return err
}

// buildDSN constructs a MySQL DSN for Dolt connection
func buildDSN(config *Config) string {
	// Format: user:password@tcp(host:port)/database?parseTime=true
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		config.User,
		config.Password,
		config.Host,
		config.Port,
		config.Database,
	)

	// Add parseTime=true to automatically parse DATE and DATETIME values to time.Time
	dsn += "?parseTime=true"

	return dsn
}

// getEnv retrieves environment variable with fallback
func getEnv(key, fallback string) string {
	if value, exists := lookupEnv(key); exists {
		return value
	}
	return fallback
}

// lookupEnv is a wrapper for os.LookupEnv to enable testing
var lookupEnv = func(key string) (string, bool) {
	// Imported from os package, exposed as variable for testing
	// In production, this will use os.LookupEnv
	// In tests, this can be mocked
	return "", false // Placeholder - will be replaced with actual os.LookupEnv in init
}

// SessionFilter provides options for filtering sessions in queries
type SessionFilter struct {
	Lifecycle       string   // Filter by lifecycle state ("", "archived")
	Harness         string   // Filter by harness type ("claude-code", "gemini-cli", "codex-cli", "opencode-cli")
	ParentSessionID *string  // Filter by parent session ID (nil = no filter)
	Workspace       string   // Filter by workspace name (optional, auto-set from adapter)
	ExcludeArchived bool     // Exclude archived sessions (status != 'archived')
	ExcludeTest     bool     // Exclude test sessions (is_test != true)
	Tags            []string // Filter by context tags (all must match)
	StableOrder     bool     // Order by immutable creation key for offset pagination
	Limit           int      // Max number of results (0 = no limit)
	Offset          int      // Number of results to skip
}
