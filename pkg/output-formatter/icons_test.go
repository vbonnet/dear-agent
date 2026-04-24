package outputformatter

import "testing"

func TestIconMapper_GetIcon_Emoji(t *testing.T) {
	mapper := NewIconMapper(false) // Use emoji icons

	tests := []struct {
		name   string
		status StatusLevel
		want   string
	}{
		{
			name:   "ok status",
			status: StatusOK,
			want:   "✅",
		},
		{
			name:   "success status (alias for ok)",
			status: StatusSuccess,
			want:   "✅",
		},
		{
			name:   "info status",
			status: StatusInfo,
			want:   "ℹ️ ",
		},
		{
			name:   "warning status",
			status: StatusWarning,
			want:   "⚠️ ",
		},
		{
			name:   "error status",
			status: StatusError,
			want:   "❌",
		},
		{
			name:   "failed status (alias for error)",
			status: StatusFailed,
			want:   "❌",
		},
		{
			name:   "unknown status",
			status: StatusUnknown,
			want:   "❓",
		},
		{
			name:   "invalid status",
			status: StatusLevel("invalid"),
			want:   "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapper.GetIcon(tt.status)
			if got != tt.want {
				t.Errorf("GetIcon(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestIconMapper_GetIcon_PlainText(t *testing.T) {
	mapper := NewIconMapper(true) // Use plain text icons

	tests := []struct {
		name   string
		status StatusLevel
		want   string
	}{
		{
			name:   "ok status",
			status: StatusOK,
			want:   "[OK]",
		},
		{
			name:   "success status",
			status: StatusSuccess,
			want:   "[OK]",
		},
		{
			name:   "info status",
			status: StatusInfo,
			want:   "[INFO]",
		},
		{
			name:   "warning status",
			status: StatusWarning,
			want:   "[WARN]",
		},
		{
			name:   "error status",
			status: StatusError,
			want:   "[ERROR]",
		},
		{
			name:   "failed status",
			status: StatusFailed,
			want:   "[ERROR]",
		},
		{
			name:   "unknown status",
			status: StatusUnknown,
			want:   "[?]",
		},
		{
			name:   "invalid status",
			status: StatusLevel("invalid"),
			want:   "     ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapper.GetIcon(tt.status)
			if got != tt.want {
				t.Errorf("GetIcon(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestIconMapper_FormatWithIcon(t *testing.T) {
	tests := []struct {
		name    string
		noColor bool
		status  StatusLevel
		message string
		want    string
	}{
		{
			name:    "emoji ok with message",
			noColor: false,
			status:  StatusOK,
			message: "All checks passed",
			want:    "✅ All checks passed",
		},
		{
			name:    "emoji warning with message",
			noColor: false,
			status:  StatusWarning,
			message: "Low disk space",
			want:    "⚠️  Low disk space",
		},
		{
			name:    "plain text error with message",
			noColor: true,
			status:  StatusError,
			message: "Connection failed",
			want:    "[ERROR] Connection failed",
		},
		{
			name:    "plain text ok with message",
			noColor: true,
			status:  StatusOK,
			message: "Config valid",
			want:    "[OK] Config valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewIconMapper(tt.noColor)
			got := mapper.FormatWithIcon(tt.status, tt.message)
			if got != tt.want {
				t.Errorf("FormatWithIcon() = %q, want %q", got, tt.want)
			}
		})
	}
}
