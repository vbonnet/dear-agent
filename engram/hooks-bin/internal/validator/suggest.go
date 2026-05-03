package validator

import (
	"fmt"
	"regexp"
	"strings"
)

// Regexes for extracting arguments from denied commands
var (
	// find . -name '*.txt' → extract path and pattern
	findArgsRe = regexp.MustCompile(`\bfind\s+(\S+)\s+(?:-name\s+)?['"]?([^'"]+)['"]?`)

	// sed -i 's/old/new/' file.txt → extract old, new, file
	sedArgsRe = regexp.MustCompile(`\bsed\s+-[a-zA-Z]*i[a-zA-Z]*\s+'s/([^/]*)/([^/]*)/'?\s+(\S+)`)

	// cd /path → extract path
	cdArgsRe = regexp.MustCompile(`\bcd\s+(\S+)`)

	// git switch branch → extract branch
	gitSwitchRe = regexp.MustCompile(`\bgit\s+(?:-C\s+\S+\s+)?switch\s+(?:-c\s+)?(\S+)`)


	// git add . / git add -A → extract what was being added
	gitAddBroadRe = regexp.MustCompile(`\bgit\s+(?:-C\s+(\S+)\s+)?add\s+(\.|--all|--update|-A|-u)`)

	// rm -rf /path → extract path
	rmRecursiveRe = regexp.MustCompile(`\brm\s+-[a-zA-Z]*r[a-zA-Z]*\s+(\S+)`)

	// cat ~/.ssh/id_rsa → extract path
	sensitiveDotdirRe = regexp.MustCompile(`(~|/home/\w+)/\.(ssh|aws|gnupg|gpg)/(\S+)`)

	// echo cfg > /etc/hosts → extract path
	systemRedirectRe = regexp.MustCompile(`>\s*(/(etc|sys|proc|boot|dev|usr|var|sbin)/\S+)`)

	// stat file → extract path
	statArgsRe = regexp.MustCompile(`(?:^|\s)stat\s+(\S+)`)

	// python3 -c 'code' → extract code snippet
	pythonCRe = regexp.MustCompile(`\b(python3?)\s+-c\s+['"]?(.+?)['"]?\s*$`)

	// cmd1; cmd2 → extract both commands
	semicolonRe = regexp.MustCompile(`^(.+?)\s*;\s*(.+)$`)

	// git --no-verify → extract the git subcommand
	gitNoVerifyRe = regexp.MustCompile(`\bgit\s+(\S+)`)
)

// SuggestToolCall generates an exact tool call suggestion based on the pattern
// index and the denied command. Returns empty string if no specific suggestion
// can be generated (falls back to generic remediation).
func SuggestToolCall(patternIdx int, command string) string {
	switch patternIdx {
	case 0: // subshell with cd
		if m := cdArgsRe.FindStringSubmatch(command); m != nil {
			path := m[1]
			return fmt.Sprintf("Use: Bash(command='git -C %s <subcommand>')", path)
		}

	case 1: // cd command
		if m := cdArgsRe.FindStringSubmatch(command); m != nil {
			path := m[1]
			// Try to detect what command follows cd (e.g., "cd /path && go test")
			parts := strings.SplitN(command, "&&", 2)
			if len(parts) == 2 {
				followCmd := strings.TrimSpace(parts[1])
				// Check if it's a git/go/make command that supports -C
				for _, prefix := range []string{"git ", "go ", "make "} {
					if strings.HasPrefix(followCmd, prefix) {
						tool := strings.TrimSpace(prefix)
						rest := strings.TrimPrefix(followCmd, prefix)
						return fmt.Sprintf("Use: Bash(command='%s -C %s %s')", tool, path, rest)
					}
				}
				return fmt.Sprintf("Use: Bash(command='%s') with absolute paths, not cd", followCmd)
			}
			return fmt.Sprintf("Use absolute paths or -C flag instead of 'cd %s'", path)
		}

	case 2: // while loop
		return "Run as separate Bash calls or use TaskCreate for polling"

	case 3: // python one-liner
		if m := pythonCRe.FindStringSubmatch(command); m != nil {
			snippet := m[2]
			if len(snippet) > 60 {
				snippet = snippet[:60] + "..."
			}
			return fmt.Sprintf("Use: Write(file_path='/tmp/script.py', content='%s') then Bash(command='python3 /tmp/script.py')", snippet)
		}

	case 4: // python heredoc
		return "Use: Write(file_path='/tmp/script.py', content='...') then Bash(command='python3 /tmp/script.py')"

	case 5: // semicolon
		if m := semicolonRe.FindStringSubmatch(command); m != nil {
			cmd1 := strings.TrimSpace(m[1])
			cmd2 := strings.TrimSpace(m[2])
			return fmt.Sprintf("Use: Bash(command='%s') and Bash(command='%s') as separate calls", cmd1, cmd2)
		}

	case 6: // bash test
		return "Use: Read(file_path='...') to check file existence, or Grep(pattern='...') for searching"

	case 7: // redirect to system path
		if m := systemRedirectRe.FindStringSubmatch(command); m != nil {
			return fmt.Sprintf("Do not write to %s. Use: Write(file_path='/tmp/output.txt', content='...') instead", m[1])
		}

	case 8: // recursive rm
		if m := rmRecursiveRe.FindStringSubmatch(command); m != nil {
			return fmt.Sprintf("Do not use recursive rm on '%s'. Use git rm for tracked files, or ask the user", m[1])
		}

	case 9: // sed -i
		if m := sedArgsRe.FindStringSubmatch(command); m != nil {
			oldText, newText, file := m[1], m[2], m[3]
			return fmt.Sprintf("Use: Edit(file_path='%s', old_string='%s', new_string='%s')", file, oldText, newText)
		}
		// Fallback: at least mention Edit tool
		return "Use: Edit(file_path='<file>', old_string='<old>', new_string='<new>') instead of sed -i"

	case 10: // find command
		if m := findArgsRe.FindStringSubmatch(command); m != nil {
			path := m[1]
			pattern := m[2]
			// Convert find-style pattern to glob if it looks like a name pattern
			if !strings.Contains(pattern, "/") {
				return fmt.Sprintf("Use: Glob(pattern='**/%s', path='%s')", pattern, path)
			}
			return fmt.Sprintf("Use: Glob(pattern='%s', path='%s')", pattern, path)
		}
		return "Use: Glob(pattern='**/<pattern>') instead of find"

	case 11: // git checkout main/master
		return "Do not checkout main in worktrees. Use git -C for read-only operations on main."

	case 12: // git switch
		if m := gitSwitchRe.FindStringSubmatch(command); m != nil {
			branch := m[1]
			return fmt.Sprintf("Do not switch branches in worktrees. Use: Bash(command='git checkout -b %s') for new branches", branch)
		}
		return "Do not switch branches in worktrees. Use git -C for read-only operations on main."

	case 13: // git stash
		return "Use a git worktree instead: Bash(command='git worktree add /path/to/worktree -b <branch>')"

	case 14: // git add broad
		if m := gitAddBroadRe.FindStringSubmatch(command); m != nil {
			cPath := m[1]
			if cPath != "" {
				return fmt.Sprintf("Use: Bash(command='git -C %s add file1.go file2.go') with specific file paths", cPath)
			}
		}
		return "Use: Bash(command='git add file1.go file2.go') with specific file paths"

	case 15: // git --no-verify
		if m := gitNoVerifyRe.FindStringSubmatch(command); m != nil {
			return fmt.Sprintf("Fix the issue that git %s verification caught, then retry without --no-verify", m[1])
		}

	case 16: // stat
		if m := statArgsRe.FindStringSubmatch(command); m != nil {
			return fmt.Sprintf("Use: Read(file_path='%s') to check file existence/contents", m[1])
		}

	case 17: // checksum
		return "Use: Read(file_path='<file>') to read file contents for verification"

	case 18: // AGM_SKIP_TEST_GATE
		return "Fix failing tests instead. Use AskUserQuestion to escalate if blocked"

	case 19: // sensitive dotdir
		if m := sensitiveDotdirRe.FindStringSubmatch(command); m != nil {
			dir := m[2]
			return fmt.Sprintf("Do not access ~/.%s/. Ask the user to perform credential operations manually", dir)
		}

	case 20: // ls
		return "Use Glob tool (for ls/find), Read tool (for head/tail/cat)"

	case 21: // grep/rg
		return "Use Grep tool (for grep), Glob tool (for find)"

	case 22: // cat
		return "Use Read tool to view file contents"

	case 23: // head/tail
		return "Use Read tool (with offset/limit for partial reads)"

	case 24: // sed
		return "Use Read tool with offset/limit parameters, or Edit tool for modifications"

	case 25: // awk
		return "Use Edit tool for structured text processing"

	case 26: // echo/printf
		return "Use Write tool to create files"

	case 27: // command substitution $()
		return "Use --prompt-file flag instead of command substitution in arguments"
	}

	return "" // no specific suggestion; caller falls back to generic remediation
}
