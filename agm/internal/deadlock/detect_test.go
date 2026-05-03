package deadlock

import (
	"runtime"
	"strings"
	"testing"
)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:  "MM:SS format",
			input: "05:30",
			want:  330,
		},
		{
			name:  "MM:SS.ss format with decimal",
			input: "02:45.12",
			want:  165,
		},
		{
			name:  "HH:MM:SS format",
			input: "01:30:00",
			want:  5400,
		},
		{
			name:  "zero time MM:SS",
			input: "00:00",
			want:  0,
		},
		{
			name:  "zero time HH:MM:SS",
			input: "00:00:00",
			want:  0,
		},
		{
			name:  "large hours",
			input: "10:05:30",
			want:  36330,
		},
		{
			name:    "single part invalid",
			input:   "12345",
			wantErr: true,
		},
		{
			name:    "four parts invalid",
			input:   "01:02:03:04",
			wantErr: true,
		},
		{
			name:    "non-numeric minutes",
			input:   "abc:30",
			wantErr: true,
		},
		{
			name:    "non-numeric seconds",
			input:   "05:xyz",
			wantErr: true,
		},
		{
			name:    "non-numeric hours",
			input:   "xx:05:30",
			wantErr: true,
		},
		{
			name:    "non-numeric minutes in HH:MM:SS",
			input:   "01:xx:30",
			wantErr: true,
		},
		{
			name:    "non-numeric seconds in HH:MM:SS",
			input:   "01:05:xx",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseTime(%q) expected error, got %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseTime(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("parseTime(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatProcessInfo(t *testing.T) {
	tests := []struct {
		name       string
		info       *ProcessInfo
		wantParts  []string
		wantAbsent []string
	}{
		{
			name: "deadlock detected",
			info: &ProcessInfo{
				PID:         12345,
				CPU:         99.5,
				RuntimeSec:  600,
				State:       "R+",
				WCHAN:       "-",
				IsDeadlock:  true,
				Command:     "node /usr/bin/claude",
				Connections: 3,
			},
			wantParts: []string{
				"PID:         12345",
				"CPU:         99.5%",
				"Runtime:     10m (600s)",
				"State:       R+",
				"Connections: 3",
				"Command:     node /usr/bin/claude",
				"DEADLOCK DETECTED",
			},
		},
		{
			name: "no deadlock - low CPU",
			info: &ProcessInfo{
				PID:         999,
				CPU:         5.0,
				RuntimeSec:  600,
				State:       "S",
				WCHAN:       "poll_schedule",
				IsDeadlock:  false,
				Command:     "node",
				Connections: 0,
			},
			wantParts: []string{
				"No deadlock detected",
				"below threshold",
			},
			wantAbsent: []string{
				"DEADLOCK DETECTED",
			},
		},
		{
			name: "no deadlock - sleeping state",
			info: &ProcessInfo{
				PID:        1000,
				CPU:        50.0,
				RuntimeSec: 600,
				State:      "S",
				IsDeadlock: false,
			},
			wantParts: []string{
				"No deadlock detected",
				"current: S",
			},
		},
		{
			name: "no deadlock - short runtime",
			info: &ProcessInfo{
				PID:        1000,
				CPU:        50.0,
				RuntimeSec: 60,
				State:      "R",
				IsDeadlock: false,
			},
			wantParts: []string{
				"No deadlock detected",
				"below threshold",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatProcessInfo(tt.info)

			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("FormatProcessInfo() missing %q in:\n%s", part, got)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("FormatProcessInfo() should not contain %q in:\n%s", absent, got)
				}
			}
		})
	}
}

func TestProcessInfo_DeadlockCriteria(t *testing.T) {
	// Test the constants are reasonable
	if MinCPUPercent <= 0 {
		t.Errorf("MinCPUPercent should be > 0, got %f", MinCPUPercent)
	}
	if MinRuntimeMinutes <= 0 {
		t.Errorf("MinRuntimeMinutes should be > 0, got %d", MinRuntimeMinutes)
	}
}

func TestCountConnections_NoProcess(t *testing.T) {
	// lsof for PID 0 (or non-existent) should return 0 gracefully
	count, err := countConnections(0)
	if err != nil {
		t.Errorf("countConnections(0) should not error, got: %v", err)
	}
	if count != 0 {
		t.Errorf("countConnections(0) = %d, want 0", count)
	}
}

func TestFindClaudeProcess_NoChildren(t *testing.T) {
	// findClaudeProcess shells out to `ps --no-headers`, a GNU/Linux flag.
	// macOS and BSD ps reject it, so this test only exercises the parsing
	// path on Linux.
	if runtime.GOOS != "linux" {
		t.Skip("findClaudeProcess uses GNU ps --no-headers; Linux-only")
	}
	_, err := findClaudeProcess(999999999)
	if err == nil {
		t.Error("findClaudeProcess with fake PID should return error")
	}
	if !strings.Contains(err.Error(), "no Claude process found") {
		t.Errorf("unexpected error: %v", err)
	}
}
