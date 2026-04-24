package cmd

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func createTestExceptionDB(t *testing.T) (string, func()) {
	t.Helper()

	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_exceptions.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	// Create schema
	schema := `
		CREATE TABLE exceptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_name TEXT NOT NULL,
			file_path TEXT NOT NULL,
			reason TEXT NOT NULL,
			approver TEXT NOT NULL,
			approved_date DATE NOT NULL DEFAULT CURRENT_DATE,
			sunset_date DATE NOT NULL,
			github_issue_url TEXT,
			status TEXT NOT NULL DEFAULT 'approved' CHECK(status IN ('pending', 'approved', 'expired', 'resolved', 'active')),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(rule_name, file_path)
		);

		CREATE INDEX idx_exceptions_rule_name ON exceptions(rule_name);
		CREATE INDEX idx_exceptions_file_path ON exceptions(file_path);
		CREATE INDEX idx_exceptions_status ON exceptions(status);
		CREATE INDEX idx_exceptions_sunset_date ON exceptions(sunset_date);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test data
	testData := `
		INSERT INTO exceptions (rule_name, file_path, reason, approver, sunset_date, status)
		VALUES
			('bash-20-line-limit', 'scripts/deploy.sh', 'Complex deployment script', 'maintainer@example.com', DATE('now', '+45 days'), 'active'),
			('bash-20-line-limit', 'scripts/old.sh', 'Legacy script', 'maintainer@example.com', DATE('now', '+15 days'), 'active'),
			('python-import-justification', 'tools/build.py', 'Build script', 'maintainer@example.com', DATE('now', '+60 days'), 'active'),
			('validator-location', 'lib/old_validator.py', 'Deprecation in progress', 'maintainer@example.com', DATE('now', '+5 days'), 'active'),
			('bash-20-line-limit', 'scripts/archived.sh', 'Archived', 'maintainer@example.com', DATE('now', '-10 days'), 'expired'),
			('python-import-justification', 'tools/fixed.py', 'Fixed', 'maintainer@example.com', DATE('now', '+30 days'), 'resolved');
	`

	if _, err := db.Exec(testData); err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	cleanup := func() {
		os.Remove(dbPath)
	}

	return dbPath, cleanup
}

func TestGetExceptionDBPath(t *testing.T) {
	tests := []struct {
		name     string
		flagPath string
		envPath  string
		want     string
	}{
		{
			name:     "flag takes precedence",
			flagPath: "/custom/path.db",
			envPath:  "/env/path.db",
			want:     "/custom/path.db",
		},
		{
			name:     "env var used when flag empty",
			flagPath: "",
			envPath:  "/env/path.db",
			want:     "/env/path.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original state
			origFlag := policyDBPath
			origEnv := os.Getenv("ENGRAM_EXCEPTION_DB")

			// Set test values
			policyDBPath = tt.flagPath
			if tt.envPath != "" {
				os.Setenv("ENGRAM_EXCEPTION_DB", tt.envPath)
			}

			// Test
			got := getExceptionDBPath()

			// Restore original state
			policyDBPath = origFlag
			if origEnv != "" {
				os.Setenv("ENGRAM_EXCEPTION_DB", origEnv)
			} else {
				os.Unsetenv("ENGRAM_EXCEPTION_DB")
			}

			if got != tt.want {
				t.Errorf("getExceptionDBPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetExceptionSummary(t *testing.T) {
	dbPath, cleanup := createTestExceptionDB(t)
	defer cleanup()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	var summary ExceptionSummary
	if err := getExceptionSummary(db, &summary); err != nil {
		t.Fatalf("getExceptionSummary() error = %v", err)
	}

	// Verify counts (4 active, 1 expired, 1 resolved, 0 pending, 6 total)
	if summary.TotalActive != 4 {
		t.Errorf("TotalActive = %d, want 4", summary.TotalActive)
	}
	if summary.TotalExpired != 1 {
		t.Errorf("TotalExpired = %d, want 1", summary.TotalExpired)
	}
	if summary.TotalResolved != 1 {
		t.Errorf("TotalResolved = %d, want 1", summary.TotalResolved)
	}
	if summary.TotalPending != 0 {
		t.Errorf("TotalPending = %d, want 0", summary.TotalPending)
	}
	if summary.Total != 6 {
		t.Errorf("Total = %d, want 6", summary.Total)
	}
}

func TestGetExpiringExceptions(t *testing.T) {
	dbPath, cleanup := createTestExceptionDB(t)
	defer cleanup()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name      string
		days      int
		wantCount int
		wantFirst string // file_path of first result
	}{
		{
			name:      "30 days",
			days:      30,
			wantCount: 2, // 5 days and 15 days exceptions
			wantFirst: "lib/old_validator.py",
		},
		{
			name:      "10 days",
			days:      10,
			wantCount: 1, // Only 5 days exception
			wantFirst: "lib/old_validator.py",
		},
		{
			name:      "60 days",
			days:      60,
			wantCount: 4, // All active exceptions except one at +60
			wantFirst: "lib/old_validator.py",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var exceptions []ExceptionDetail
			err := getExpiringExceptions(db, &exceptions, tt.days)
			if err != nil {
				t.Fatalf("getExpiringExceptions() error = %v", err)
			}

			if len(exceptions) != tt.wantCount {
				t.Errorf("got %d exceptions, want %d", len(exceptions), tt.wantCount)
			}

			if len(exceptions) > 0 && exceptions[0].FilePath != tt.wantFirst {
				t.Errorf("first exception file = %s, want %s", exceptions[0].FilePath, tt.wantFirst)
			}

			// Verify sorted by sunset date (ascending)
			for i := 1; i < len(exceptions); i++ {
				if exceptions[i].SunsetDate < exceptions[i-1].SunsetDate {
					t.Errorf("exceptions not sorted by sunset date: %s before %s",
						exceptions[i-1].SunsetDate, exceptions[i].SunsetDate)
				}
			}
		})
	}
}

func TestGetRuleBreakdown(t *testing.T) {
	dbPath, cleanup := createTestExceptionDB(t)
	defer cleanup()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	var breakdown []RuleBreakdown
	if err := getRuleBreakdown(db, &breakdown); err != nil {
		t.Fatalf("getRuleBreakdown() error = %v", err)
	}

	// Should have 3 rules
	if len(breakdown) != 3 {
		t.Errorf("got %d rules, want 3", len(breakdown))
	}

	// Verify bash-20-line-limit counts
	var bashRule *RuleBreakdown
	for i := range breakdown {
		if breakdown[i].RuleName == "bash-20-line-limit" {
			bashRule = &breakdown[i]
			break
		}
	}

	if bashRule == nil {
		t.Fatal("bash-20-line-limit rule not found")
	}

	if bashRule.Active != 2 {
		t.Errorf("bash Active = %d, want 2", bashRule.Active)
	}
	if bashRule.Expired != 1 {
		t.Errorf("bash Expired = %d, want 1", bashRule.Expired)
	}
	if bashRule.Resolved != 0 {
		t.Errorf("bash Resolved = %d, want 0", bashRule.Resolved)
	}
	if bashRule.Total != 3 {
		t.Errorf("bash Total = %d, want 3", bashRule.Total)
	}
}

func TestShortenRuleName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bash-20-line-limit",
			input: "bash-20-line-limit",
			want:  "bash-20-line",
		},
		{
			name:  "python-import-justification",
			input: "python-import-justification",
			want:  "python-import",
		},
		{
			name:  "validator-location",
			input: "validator-location",
			want:  "validator-loc",
		},
		{
			name:  "unknown rule",
			input: "custom-rule",
			want:  "custom-rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortenRuleName(tt.input); got != tt.want {
				t.Errorf("shortenRuleName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		maxLen int
		want   string
	}{
		{
			name:   "short path unchanged",
			path:   "scripts/deploy.sh",
			maxLen: 30,
			want:   "scripts/deploy.sh",
		},
		{
			name:   "long path shortened",
			path:   "very/long/path/to/some/deeply/nested/file.sh",
			maxLen: 20,
			want:   "...nested/file.sh",
		},
		{
			name:   "very long filename",
			path:   "short/verylongfilenamethatexceedsmaxlength.sh",
			maxLen: 20,
			want:   "...ceedsmaxlength.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenPath(tt.path, tt.maxLen)
			if got != tt.want {
				t.Errorf("shortenPath() = %v, want %v", got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("shortenPath() result length %d exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "short string",
			maxLen: 20,
			want:   "short string",
		},
		{
			name:   "long string truncated",
			input:  "this is a very long string that needs truncation",
			maxLen: 20,
			want:   "this is a very lo...",
		},
		{
			name:   "exact length unchanged",
			input:  "exactly twenty chars",
			maxLen: 20,
			want:   "exactly twenty chars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate() = %v, want %v", got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("truncate() result length %d exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}

func TestGetComplianceRates(t *testing.T) {
	dbPath, cleanup := createTestExceptionDB(t)
	defer cleanup()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	defer db.Close()

	var rates ComplianceRates
	if err := getComplianceRates(db, &rates); err != nil {
		t.Fatalf("getComplianceRates() error = %v", err)
	}

	// Verify rates are set (simplified implementation just shows counts)
	if rates.BashCompliance == "" {
		t.Error("BashCompliance should not be empty")
	}
	if rates.PythonCompliance == "" {
		t.Error("PythonCompliance should not be empty")
	}
	if rates.ValidatorCompliance == "" {
		t.Error("ValidatorCompliance should not be empty")
	}
}
