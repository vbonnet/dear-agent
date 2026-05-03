package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTokenUsageFromReminder(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantUsed  int
		wantTotal int
		wantNil   bool
	}{
		{
			name:      "basic reminder",
			text:      "<system-reminder>Token usage: 45000/200000; 155000 remaining</system-reminder>",
			wantUsed:  45000,
			wantTotal: 200000,
		},
		{
			name:      "reminder with noise",
			text:      "Some output\n<system-reminder>Token usage: 12345/200000; 187655 remaining</system-reminder>\nMore output",
			wantUsed:  12345,
			wantTotal: 200000,
		},
		{
			name:    "no reminder",
			text:    "Just some regular output with no token info",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{}
			got := monitor.extractTokenUsageFromReminder(tt.text)

			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantUsed, got.Used)
				assert.Equal(t, tt.wantTotal, got.Total)
			}
		})
	}
}

func TestExtractTokenUsageFromJSON(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		wantUsed  int
		wantTotal int
		wantNil   bool
	}{
		{
			name: "complete JSON",
			data: map[string]interface{}{
				"token_usage": map[string]interface{}{
					"input_tokens":  float64(10000),
					"output_tokens": float64(2345),
					"total_tokens":  float64(12345),
				},
				"max_context_tokens": float64(200000),
			},
			wantUsed:  12345,
			wantTotal: 200000,
		},
		{
			name: "JSON with default max",
			data: map[string]interface{}{
				"token_usage": map[string]interface{}{
					"total_tokens": float64(50000),
				},
			},
			wantUsed:  50000,
			wantTotal: 200000, // default
		},
		{
			name: "no token usage",
			data: map[string]interface{}{
				"some": "other",
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{}
			got := monitor.extractTokenUsageFromJSON(tt.data)

			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantUsed, got.Used)
				assert.Equal(t, tt.wantTotal, got.Total)
			}
		})
	}
}

func TestCalculatePercentage(t *testing.T) {
	tests := []struct {
		name  string
		used  int
		total int
		want  float64
	}{
		{
			name:  "basic percentage",
			used:  50000,
			total: 200000,
			want:  25.0,
		},
		{
			name:  "with rounding",
			used:  12345,
			total: 200000,
			want:  6.2, // Rounded to 1 decimal
		},
		{
			name:  "zero total",
			used:  100,
			total: 0,
			want:  0.0,
		},
		{
			name:  "high usage",
			used:  198000,
			total: 200000,
			want:  99.0,
		},
		{
			name:  "full usage",
			used:  200000,
			total: 200000,
			want:  100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{}
			got := monitor.calculatePercentage(tt.used, tt.total)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldUpdate(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name       string
		setupCache func(string) // setup function to create cache
		percentage float64
		wantUpdate bool
	}{
		{
			name:       "no cache exists",
			setupCache: nil,
			percentage: 50.0,
			wantUpdate: true,
		},
		{
			name: "interval not elapsed",
			setupCache: func(cacheFile string) {
				cache := CacheEntry{
					Percentage: 50.0,
					Timestamp:  time.Now(),
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(cacheFile, data, 0644)
			},
			percentage: 51.0,
			wantUpdate: false,
		},
		{
			name: "interval elapsed with significant change",
			setupCache: func(cacheFile string) {
				cache := CacheEntry{
					Percentage: 50.0,
					Timestamp:  time.Now().Add(-15 * time.Second),
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(cacheFile, data, 0644)
			},
			percentage: 55.0,
			wantUpdate: true,
		},
		{
			name: "interval elapsed but change too small",
			setupCache: func(cacheFile string) {
				cache := CacheEntry{
					Percentage: 50.0,
					Timestamp:  time.Now().Add(-15 * time.Second),
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(cacheFile, data, 0644)
			},
			percentage: 50.5,
			wantUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{
				sessionID: "test-session-123",
				cacheDir:  tempDir,
			}

			cacheFile := monitor.getCacheFile()
			if tt.setupCache != nil {
				tt.setupCache(cacheFile)
			}

			got := monitor.shouldUpdate(tt.percentage)
			assert.Equal(t, tt.wantUpdate, got)
		})
	}
}

func TestUpdateCache(t *testing.T) {
	tempDir := t.TempDir()

	monitor := &ContextMonitor{
		sessionID: "test-session-456",
		cacheDir:  tempDir,
	}

	monitor.updateCache(75.5)

	cacheFile := monitor.getCacheFile()
	require.FileExists(t, cacheFile)

	data, err := os.ReadFile(cacheFile)
	require.NoError(t, err)

	var cache CacheEntry
	err = json.Unmarshal(data, &cache)
	require.NoError(t, err)

	assert.Equal(t, 75.5, cache.Percentage)
	assert.WithinDuration(t, time.Now(), cache.Timestamp, time.Second)
}

func TestFindAGMSession(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		setupManifest func(string) // setup function to create manifest
		wantSession   string
	}{
		{
			name: "AGM session exists",
			setupManifest: func(manifestPath string) {
				os.MkdirAll(filepath.Dir(manifestPath), 0755)
				content := `session_id: test-session-456
agm_session_name: my-agm-session
workspace: oss
`
				os.WriteFile(manifestPath, []byte(content), 0644)
			},
			wantSession: "my-agm-session",
		},
		{
			name: "not AGM-managed",
			setupManifest: func(manifestPath string) {
				os.MkdirAll(filepath.Dir(manifestPath), 0755)
				content := `session_id: test-session-456
workspace: personal
`
				os.WriteFile(manifestPath, []byte(content), 0644)
			},
			wantSession: "",
		},
		{
			name:          "no manifest",
			setupManifest: nil,
			wantSession:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp home directory structure
			sessionID := "test-session-456"
			homeDir := tempDir
			manifestPath := filepath.Join(homeDir, ".claude", "sessions", sessionID, "manifest.yaml")

			if tt.setupManifest != nil {
				tt.setupManifest(manifestPath)
			}

			// Override home dir for testing
			t.Setenv("HOME", homeDir)

			monitor := &ContextMonitor{
				sessionID: sessionID,
			}

			got := monitor.findAGMSession()
			assert.Equal(t, tt.wantSession, got)
		})
	}
}

func TestRun_NoTokenUsage(t *testing.T) {
	monitor := &ContextMonitor{
		toolResult: "Just some output without tokens",
	}

	exitCode := monitor.Run()
	assert.Equal(t, 0, exitCode)
}

func TestRun_NonBashTool(t *testing.T) {
	// Context monitor doesn't filter by tool name, but let's test basic flow
	monitor := &ContextMonitor{
		toolResult: "<system-reminder>Token usage: 50000/200000; 150000 remaining</system-reminder>",
		sessionID:  "test-session",
		cacheDir:   t.TempDir(),
	}

	// Without AGM session, should succeed but do nothing
	exitCode := monitor.Run()
	assert.Equal(t, 0, exitCode)
}
