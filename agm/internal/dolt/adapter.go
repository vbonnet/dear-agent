package dolt

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver for Dolt
)

func init() {
	// Initialize lookupEnv with os.LookupEnv
	lookupEnv = os.LookupEnv
}

// isRunningInTest detects if code is executing within a test context
// This is determined by checking if the executable name contains ".test"
// which is characteristic of binaries compiled by `go test`
func isRunningInTest() bool {
	// Get the executable path
	executable, err := os.Executable()
	if err != nil {
		return false
	}

	// Check if executable name contains ".test" (characteristic of go test binaries)
	return strings.Contains(executable, ".test")
}

// Adapter provides Dolt-based storage for AGM sessions
// Replaces dual-layer SQLite+JSONL storage with single Dolt database
type Adapter struct {
	conn              *sql.DB
	workspace         string
	port              string
	migrationsApplied bool
}

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

// DefaultConfig returns default configuration from environment
func DefaultConfig() (*Config, error) {
	workspace := getEnv("WORKSPACE", "")
	if workspace == "" {
		return nil, fmt.Errorf("WORKSPACE environment variable not set (workspace protocol not activated)")
	}

	// CRITICAL: Fail-fast enforcement to prevent test pollution
	// Tests MUST set ENGRAM_TEST_MODE=1 and use a test-specific workspace
	if isRunningInTest() {
		testMode := getEnv("ENGRAM_TEST_MODE", "")
		testWorkspace := getEnv("ENGRAM_TEST_WORKSPACE", "")

		// Require explicit test mode
		if testMode != "1" && testMode != "true" {
			return nil, fmt.Errorf("TEST POLLUTION BLOCKED: Tests must set ENGRAM_TEST_MODE=1\n\n"+
				"Why: Without test isolation, tests write to production databases causing data pollution.\n\n"+
				"Fix: Run tests with proper isolation:\n"+
				"  ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test go test ./...\n\n"+
				"Or use testutil.SetupTestEnvironment(t) in your test setup function.\n\n"+
				"Current workspace: %s (attempted during test)", workspace)
		}

		// Block production workspace names during tests
		// Production workspaces include: oss, acme, prod, production, main, etc.
		isProductionWorkspace := workspace == "oss" ||
			workspace == "acme" ||
			workspace == "prod" ||
			workspace == "production" ||
			workspace == "main"

		if isProductionWorkspace {
			//nolint:staticcheck // multi-line CLI-facing help text
			return nil, fmt.Errorf("TEST POLLUTION BLOCKED: Tests cannot write to production workspace '%s'\n\n"+
				"Why: Production workspaces contain real data that tests would corrupt.\n\n"+
				"Fix: Set ENGRAM_TEST_WORKSPACE to a test-specific value:\n"+
				"  ENGRAM_TEST_MODE=1 ENGRAM_TEST_WORKSPACE=test go test ./...\n\n"+
				"Or use testutil.SetupTestEnvironment(t) which auto-sets workspace='test'.\n\n"+
				"Note: WORKSPACE env var is set to '%s' - this is a production workspace.\n"+
				"      Tests detected production name, blocking to prevent pollution.", workspace, workspace)
		}

		// Warn if ENGRAM_TEST_WORKSPACE is not set (using inherited WORKSPACE)
		if testWorkspace == "" {
			fmt.Fprintf(os.Stderr, "WARNING: ENGRAM_TEST_WORKSPACE not set, using WORKSPACE=%s\n", workspace)
			fmt.Fprintf(os.Stderr, "         Set ENGRAM_TEST_WORKSPACE=test for explicit test isolation\n")
		}
	}

	port := getEnv("DOLT_PORT", "3307")
	host := getEnv("DOLT_HOST", "127.0.0.1")
	// Default database name to workspace name for proper workspace isolation
	// In production: oss → database "oss", acme → database "acme"
	// In tests: test → database "test"
	database := getEnv("DOLT_DATABASE", workspace)
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
	Limit           int      // Max number of results (0 = no limit)
	Offset          int      // Number of results to skip
}
