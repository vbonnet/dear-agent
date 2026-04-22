package healthchecker

import "testing"

func TestResult_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "OK status is healthy",
			result: Result{Status: StatusOK},
			want:   true,
		},
		{
			name:   "Info status is healthy",
			result: Result{Status: StatusInfo},
			want:   true,
		},
		{
			name:   "Warning status is not healthy",
			result: Result{Status: StatusWarning},
			want:   false,
		},
		{
			name:   "Error status is not healthy",
			result: Result{Status: StatusError},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsHealthy()
			if got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResult_IsIssue(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "OK status is not an issue",
			result: Result{Status: StatusOK},
			want:   false,
		},
		{
			name:   "Info status is not an issue",
			result: Result{Status: StatusInfo},
			want:   false,
		},
		{
			name:   "Warning status is an issue",
			result: Result{Status: StatusWarning},
			want:   true,
		},
		{
			name:   "Error status is an issue",
			result: Result{Status: StatusError},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsIssue()
			if got != tt.want {
				t.Errorf("IsIssue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResult_IsCritical(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "OK status is not critical",
			result: Result{Status: StatusOK},
			want:   false,
		},
		{
			name:   "Info status is not critical",
			result: Result{Status: StatusInfo},
			want:   false,
		},
		{
			name:   "Warning status is not critical",
			result: Result{Status: StatusWarning},
			want:   false,
		},
		{
			name:   "Error status is critical",
			result: Result{Status: StatusError},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsCritical()
			if got != tt.want {
				t.Errorf("IsCritical() = %v, want %v", got, tt.want)
			}
		})
	}
}
