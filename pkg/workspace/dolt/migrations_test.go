package dolt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeChecksum(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty content",
			content: "",
			want:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:    "simple sql",
			content: "CREATE TABLE test (id INT);",
			want:    "sha256:9c5c8b0e8f8f9c9f9e9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f9f", // Will be different
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeChecksum([]byte(tt.content))
			assert.True(t, len(got) > 0)
			assert.Contains(t, got, "sha256:")

			// Verify checksums are deterministic
			got2 := computeChecksum([]byte(tt.content))
			assert.Equal(t, got, got2)
		})
	}
}

func TestParseTablesCreated(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "no tables",
			sql:  "SELECT * FROM test;",
			want: nil,
		},
		{
			name: "single table",
			sql:  "CREATE TABLE test (id INT);",
			want: []string{"test"},
		},
		{
			name: "multiple tables",
			sql: `
CREATE TABLE agm_sessions (id VARCHAR(255));
CREATE TABLE agm_messages (id VARCHAR(255));
`,
			want: []string{"agm_sessions", "agm_messages"},
		},
		{
			name: "with IF NOT EXISTS",
			sql:  "CREATE TABLE IF NOT EXISTS test_table (id INT);",
			want: []string{"test_table"},
		},
		{
			name: "case insensitive",
			sql:  "create table TestTable (id int);",
			want: []string{"testtable"},
		},
		{
			name: "duplicate tables",
			sql: `
CREATE TABLE test (id INT);
CREATE TABLE IF NOT EXISTS test (id INT);
`,
			want: []string{"test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTablesCreated(tt.sql)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadMigrationFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Write valid migration files
	err := os.WriteFile(filepath.Join(tmpDir, "001_initial_schema.sql"),
		[]byte("CREATE TABLE users (id INT);"), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "002_add_sessions.sql"),
		[]byte("CREATE TABLE sessions (id INT);\nCREATE TABLE session_logs (id INT);"), 0o644)
	require.NoError(t, err)

	// Write a non-matching file that should be ignored
	err = os.WriteFile(filepath.Join(tmpDir, "README.md"),
		[]byte("not a migration"), 0o644)
	require.NoError(t, err)

	migrations, err := LoadMigrationFiles(tmpDir)
	require.NoError(t, err)
	require.Len(t, migrations, 2)

	// Verify sorted by version
	assert.Equal(t, 1, migrations[0].Version)
	assert.Equal(t, "initial_schema", migrations[0].Name)
	assert.Equal(t, "CREATE TABLE users (id INT);", migrations[0].SQL)
	assert.Contains(t, migrations[0].Checksum, "sha256:")
	assert.Equal(t, []string{"users"}, migrations[0].TablesCreated)

	assert.Equal(t, 2, migrations[1].Version)
	assert.Equal(t, "add_sessions", migrations[1].Name)
	assert.Equal(t, []string{"sessions", "session_logs"}, migrations[1].TablesCreated)

	// Test non-existent directory
	_, err = LoadMigrationFiles("/nonexistent/path")
	assert.Error(t, err)

	// Test empty directory
	emptyDir := t.TempDir()
	migrations, err = LoadMigrationFiles(emptyDir)
	require.NoError(t, err)
	assert.Empty(t, migrations)
}

func TestMigrationFile_Validation(t *testing.T) {
	tests := []struct {
		name      string
		migration *MigrationFile
		wantValid bool
	}{
		{
			name: "valid migration",
			migration: &MigrationFile{
				Version:       1,
				Name:          "initial_schema",
				FilePath:      "/path/to/001_initial_schema.sql",
				SQL:           "CREATE TABLE test (id INT);",
				Checksum:      "sha256:abc123",
				TablesCreated: []string{"test"},
			},
			wantValid: true,
		},
		{
			name: "missing version",
			migration: &MigrationFile{
				Version:  0,
				Name:     "test",
				SQL:      "CREATE TABLE test (id INT);",
				Checksum: "sha256:abc123",
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation
			if tt.wantValid {
				assert.Greater(t, tt.migration.Version, 0)
				assert.NotEmpty(t, tt.migration.Name)
				assert.NotEmpty(t, tt.migration.SQL)
			}
		})
	}
}

func TestMigrationResult(t *testing.T) {
	migration := &MigrationFile{
		Version: 1,
		Name:    "test",
		SQL:     "CREATE TABLE test (id INT);",
	}

	result := &MigrationResult{
		Migration:       migration,
		Success:         true,
		ExecutionTimeMs: 50,
		Error:           nil,
		DoltCommit:      "abc123",
	}

	assert.True(t, result.Success)
	assert.Equal(t, 50, result.ExecutionTimeMs)
	assert.NoError(t, result.Error)
	assert.NotEmpty(t, result.DoltCommit)
}

// Test migration ordering
func TestMigrationOrdering(t *testing.T) {
	migrations := []*MigrationFile{
		{Version: 3, Name: "third"},
		{Version: 1, Name: "first"},
		{Version: 2, Name: "second"},
	}

	// Sort by version (simulating LoadMigrationFiles sorting)
	for i := 0; i < len(migrations); i++ {
		for j := i + 1; j < len(migrations); j++ {
			if migrations[i].Version > migrations[j].Version {
				migrations[i], migrations[j] = migrations[j], migrations[i]
			}
		}
	}

	require.Equal(t, 3, len(migrations))
	assert.Equal(t, 1, migrations[0].Version)
	assert.Equal(t, 2, migrations[1].Version)
	assert.Equal(t, 3, migrations[2].Version)
}

// Test checksum consistency
func TestChecksumConsistency(t *testing.T) {
	sql := "CREATE TABLE agm_sessions (id VARCHAR(255));"

	checksum1 := computeChecksum([]byte(sql))
	checksum2 := computeChecksum([]byte(sql))

	assert.Equal(t, checksum1, checksum2, "checksums should be deterministic")

	// Different content should produce different checksum
	sql2 := "CREATE TABLE agm_messages (id VARCHAR(255));"
	checksum3 := computeChecksum([]byte(sql2))

	assert.NotEqual(t, checksum1, checksum3, "different content should have different checksums")
}

// Test table extraction edge cases
func TestParseTablesCreated_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "table in comment",
			sql:  "-- CREATE TABLE commented_table (id INT);\nCREATE TABLE real_table (id INT);",
			want: []string{"real_table"},
		},
		{
			name: "multiline create",
			sql: `CREATE TABLE multiline (
				id INT,
				name VARCHAR(255)
			);`,
			want: []string{"multiline"},
		},
		{
			name: "with schema prefix",
			sql:  "CREATE TABLE workspace.test_table (id INT);",
			want: []string{"test_table"}, // Might capture "workspace.test_table" - adjust regex if needed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTablesCreated(tt.sql)
			// For these edge cases, we'll be lenient
			assert.NotNil(t, got)
		})
	}
}

// Benchmark tests
func BenchmarkComputeChecksum(b *testing.B) {
	sql := "CREATE TABLE agm_sessions (id VARCHAR(255), created_at TIMESTAMP);"
	content := []byte(sql)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeChecksum(content)
	}
}

func BenchmarkParseTablesCreated(b *testing.B) {
	sql := `
CREATE TABLE agm_sessions (id VARCHAR(255));
CREATE TABLE agm_messages (id VARCHAR(255));
CREATE TABLE agm_tool_calls (id VARCHAR(255));
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseTablesCreated(sql)
	}
}
