package validator

import (
	"strings"
	"testing"
)

func TestSuggestToolCall_FindCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "find with name pattern",
			command: "find . -name '*.txt'",
			want:    "Glob(pattern='**/*.txt', path='.')",
		},
		{
			name:    "find with specific path",
			command: "find /tmp/test/src -name '*.go'",
			want:    "Glob(pattern='**/*.go', path='/tmp/test/src')",
		},
		{
			name:    "find without parseable args",
			command: "find /tmp",
			want:    "Glob(pattern=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SuggestToolCall(10, tt.command) // index 10 = find
			if !strings.Contains(got, tt.want) {
				t.Errorf("SuggestToolCall(10, %q) = %q, want containing %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestSuggestToolCall_SedInPlace(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "sed -i with substitution",
			command: "sed -i 's/old/new/' file.txt",
			want:    "Edit(file_path='file.txt', old_string='old', new_string='new')",
		},
		{
			name:    "sed -i without parseable substitution",
			command: "sed -i 'd' file.txt",
			want:    "Edit(file_path=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SuggestToolCall(9, tt.command) // index 9 = sed -i
			if !strings.Contains(got, tt.want) {
				t.Errorf("SuggestToolCall(9, %q) = %q, want containing %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestSuggestToolCall_CdCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "cd with git follow-up",
			command: "cd /path && git status",
			want:    "Bash(command='git -C /path status')",
		},
		{
			name:    "cd with go follow-up",
			command: "cd /project && go test ./...",
			want:    "Bash(command='go -C /project test ./...')",
		},
		{
			name:    "cd with make follow-up",
			command: "cd /build && make all",
			want:    "Bash(command='make -C /build all')",
		},
		{
			name:    "cd alone",
			command: "cd /path",
			want:    "absolute paths or -C flag",
		},
		{
			name:    "cd with non-standard follow-up",
			command: "cd /dir && npm install",
			want:    "Bash(command='npm install') with absolute paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SuggestToolCall(1, tt.command) // index 1 = cd
			if !strings.Contains(got, tt.want) {
				t.Errorf("SuggestToolCall(1, %q) = %q, want containing %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestSuggestToolCall_GitCheckoutMain(t *testing.T) {
	got := SuggestToolCall(11, "git checkout main")
	if !strings.Contains(got, "Do not checkout main") {
		t.Errorf("SuggestToolCall(11, 'git checkout main') = %q, want containing 'Do not checkout main'", got)
	}
}

func TestSuggestToolCall_GitSwitch(t *testing.T) {
	got := SuggestToolCall(12, "git switch feature")
	want := "git checkout -b feature"
	if !strings.Contains(got, want) {
		t.Errorf("SuggestToolCall(12, 'git switch feature') = %q, want containing %q", got, want)
	}
}

func TestSuggestToolCall_GitStash(t *testing.T) {
	got := SuggestToolCall(13, "git stash")
	if !strings.Contains(got, "git worktree add") {
		t.Errorf("SuggestToolCall(13, 'git stash') = %q, want containing 'git worktree add'", got)
	}
}

func TestSuggestToolCall_Semicolon(t *testing.T) {
	got := SuggestToolCall(5, "cmd1; cmd2")
	if !strings.Contains(got, "Bash(command='cmd1')") || !strings.Contains(got, "Bash(command='cmd2')") {
		t.Errorf("SuggestToolCall(5, 'cmd1; cmd2') = %q, want both commands in separate Bash calls", got)
	}
}

func TestSuggestToolCall_SystemRedirect(t *testing.T) {
	got := SuggestToolCall(7, "echo cfg > /etc/hosts")
	if !strings.Contains(got, "/etc/hosts") || !strings.Contains(got, "Write(file_path=") {
		t.Errorf("SuggestToolCall(7, ...) = %q, want /etc/hosts mention and Write suggestion", got)
	}
}

func TestSuggestToolCall_RecursiveRm(t *testing.T) {
	got := SuggestToolCall(8, "rm -rf /tmp/dir")
	if !strings.Contains(got, "/tmp/dir") {
		t.Errorf("SuggestToolCall(8, 'rm -rf /tmp/dir') = %q, want containing '/tmp/dir'", got)
	}
}

func TestSuggestToolCall_SensitiveDotdir(t *testing.T) {
	got := SuggestToolCall(19, "cat ~/.ssh/id_rsa")
	if !strings.Contains(got, "~/.ssh/") {
		t.Errorf("SuggestToolCall(19, 'cat ~/.ssh/id_rsa') = %q, want containing '~/.ssh/'", got)
	}
}

func TestSuggestToolCall_GitAddBroad(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{
			name:    "git add dot",
			command: "git add .",
			want:    "git add file1.go file2.go",
		},
		{
			name:    "git add with -C flag",
			command: "git -C /path add .",
			want:    "git -C /path add file1.go file2.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SuggestToolCall(14, tt.command)
			if !strings.Contains(got, tt.want) {
				t.Errorf("SuggestToolCall(14, %q) = %q, want containing %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestSuggestToolCall_Stat(t *testing.T) {
	got := SuggestToolCall(16, "stat myfile.txt")
	if !strings.Contains(got, "Read(file_path='myfile.txt')") {
		t.Errorf("SuggestToolCall(16, 'stat myfile.txt') = %q, want Read suggestion", got)
	}
}

// TestValidateCommandIncludesToolCall verifies the full integration:
// ValidateCommand returns specific tool calls in remediation.
func TestValidateCommandIncludesToolCall(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		wantContains string
	}{
		{
			name:         "find includes Glob suggestion",
			command:      "find . -name '*.go'",
			wantContains: "Glob(pattern=",
		},
		{
			name:         "sed -i includes Edit suggestion",
			command:      "sed -i 's/foo/bar/' main.go",
			wantContains: "Edit(file_path='main.go'",
		},
		{
			name:         "cd && git includes -C suggestion",
			command:      "cd /repo && git log",
			wantContains: "git -C /repo log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _, remediation := ValidateCommand(tt.command)
			if ok {
				t.Fatalf("Expected command to be blocked: %s", tt.command)
			}
			if !strings.Contains(remediation, tt.wantContains) {
				t.Errorf("ValidateCommand(%q) remediation = %q, want containing %q",
					tt.command, remediation, tt.wantContains)
			}
		})
	}
}
