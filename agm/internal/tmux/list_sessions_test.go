package tmux

import "testing"

// tmux exits non-zero both when it observed an empty server and when it could
// not reach one, so the exit status alone cannot tell them apart. Classifying
// on *exec.ExitError turned a permission-denied socket into a successful
// observation of zero sessions — and a caller that trusts that empty list then
// marks every live manifest stopped (ce-0zng9).
func TestIsEmptyServerOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "no server running", output: "no server running on /tmp/agm.sock", want: true},
		{name: "no sessions", output: "no sessions", want: true},
		{name: "socket absent", output: "error connecting to /tmp/agm.sock (No such file or directory)", want: true},
		{name: "mixed case is still recognized", output: "No Server Running On /tmp/agm.sock", want: true},

		{name: "permission denied is not an observation", output: "error connecting to /tmp/agm.sock (Permission denied)", want: false},
		{name: "connection refused is not an observation", output: "error connecting to /tmp/agm.sock (Connection refused)", want: false},
		{name: "unexpected diagnostic is not an observation", output: "lost server", want: false},
		{name: "empty output proves nothing", output: "", want: false},
		{name: "whitespace proves nothing", output: "   \n ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmptyServerOutput(tt.output); got != tt.want {
				t.Errorf("isEmptyServerOutput(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}
