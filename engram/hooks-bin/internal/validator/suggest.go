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

// suggestionFn turns a denied command into an exact tool-call recommendation.
// Returning "" means no specific suggestion (caller falls back to generic
// remediation).
type suggestionFn func(string) string

// suggestionByPattern is a per-pattern dispatch table for SuggestToolCall.
// Indices align with the pattern catalog.
//
//nolint:gochecknoglobals // intentional immutable lookup table
var suggestionByPattern = map[int]suggestionFn{
	0:  suggestSubshellCD,
	1:  suggestCD,
	2:  func(string) string { return "Run as separate Bash calls or use TaskCreate for polling" },
	3:  suggestPythonOneLiner,
	4:  func(string) string { return "Use: Write(file_path='/tmp/script.py', content='...') then Bash(command='python3 /tmp/script.py')" },
	5:  suggestSemicolon,
	6:  func(string) string { return "Use: Read(file_path='...') to check file existence, or Grep(pattern='...') for searching" },
	7:  suggestSystemRedirect,
	8:  suggestRecursiveRm,
	9:  suggestSedI,
	10: suggestFind,
	11: func(string) string { return "Do not checkout main in worktrees. Use git -C for read-only operations on main." },
	12: suggestGitSwitch,
	13: func(string) string { return "Use a git worktree instead: Bash(command='git worktree add /path/to/worktree -b <branch>')" },
	14: suggestGitAddBroad,
	15: suggestGitNoVerify,
	16: suggestStat,
	17: func(string) string { return "Use: Read(file_path='<file>') to read file contents for verification" },
	18: func(string) string { return "Fix failing tests instead. Use AskUserQuestion to escalate if blocked" },
	19: suggestSensitiveDotdir,
	20: func(string) string { return "Use Glob tool (for ls/find), Read tool (for head/tail/cat)" },
	21: func(string) string { return "Use Grep tool (for grep), Glob tool (for find)" },
	22: func(string) string { return "Use Read tool to view file contents" },
	23: func(string) string { return "Use Read tool (with offset/limit for partial reads)" },
	24: func(string) string { return "Use Read tool with offset/limit parameters, or Edit tool for modifications" },
	25: func(string) string { return "Use Edit tool for structured text processing" },
	26: func(string) string { return "Use Write tool to create files" },
	27: func(string) string { return "Use --prompt-file flag instead of command substitution in arguments" },
}

// SuggestToolCall generates an exact tool call suggestion based on the pattern
// index and the denied command. Returns empty string if no specific suggestion
// can be generated (falls back to generic remediation).
func SuggestToolCall(patternIdx int, command string) string {
	if fn, ok := suggestionByPattern[patternIdx]; ok {
		return fn(command)
	}
	return ""
}

func suggestSubshellCD(command string) string {
	if m := cdArgsRe.FindStringSubmatch(command); m != nil {
		return fmt.Sprintf("Use: Bash(command='git -C %s <subcommand>')", m[1])
	}
	return ""
}

func suggestCD(command string) string {
	m := cdArgsRe.FindStringSubmatch(command)
	if m == nil {
		return ""
	}
	path := m[1]
	parts := strings.SplitN(command, "&&", 2)
	if len(parts) != 2 {
		return fmt.Sprintf("Use absolute paths or -C flag instead of 'cd %s'", path)
	}
	followCmd := strings.TrimSpace(parts[1])
	for _, prefix := range []string{"git ", "go ", "make "} {
		if strings.HasPrefix(followCmd, prefix) {
			tool := strings.TrimSpace(prefix)
			rest := strings.TrimPrefix(followCmd, prefix)
			return fmt.Sprintf("Use: Bash(command='%s -C %s %s')", tool, path, rest)
		}
	}
	return fmt.Sprintf("Use: Bash(command='%s') with absolute paths, not cd", followCmd)
}

func suggestPythonOneLiner(command string) string {
	m := pythonCRe.FindStringSubmatch(command)
	if m == nil {
		return ""
	}
	snippet := m[2]
	if len(snippet) > 60 {
		snippet = snippet[:60] + "..."
	}
	return fmt.Sprintf("Use: Write(file_path='/tmp/script.py', content='%s') then Bash(command='python3 /tmp/script.py')", snippet)
}

func suggestSemicolon(command string) string {
	m := semicolonRe.FindStringSubmatch(command)
	if m == nil {
		return ""
	}
	cmd1 := strings.TrimSpace(m[1])
	cmd2 := strings.TrimSpace(m[2])
	return fmt.Sprintf("Use: Bash(command='%s') and Bash(command='%s') as separate calls", cmd1, cmd2)
}

func suggestSystemRedirect(command string) string {
	if m := systemRedirectRe.FindStringSubmatch(command); m != nil {
		return fmt.Sprintf("Do not write to %s. Use: Write(file_path='/tmp/output.txt', content='...') instead", m[1])
	}
	return ""
}

func suggestRecursiveRm(command string) string {
	if m := rmRecursiveRe.FindStringSubmatch(command); m != nil {
		return fmt.Sprintf("Do not use recursive rm on '%s'. Use git rm for tracked files, or ask the user", m[1])
	}
	return ""
}

func suggestSedI(command string) string {
	if m := sedArgsRe.FindStringSubmatch(command); m != nil {
		oldText, newText, file := m[1], m[2], m[3]
		return fmt.Sprintf("Use: Edit(file_path='%s', old_string='%s', new_string='%s')", file, oldText, newText)
	}
	return "Use: Edit(file_path='<file>', old_string='<old>', new_string='<new>') instead of sed -i"
}

func suggestFind(command string) string {
	m := findArgsRe.FindStringSubmatch(command)
	if m == nil {
		return "Use: Glob(pattern='**/<pattern>') instead of find"
	}
	path := m[1]
	pattern := m[2]
	if !strings.Contains(pattern, "/") {
		return fmt.Sprintf("Use: Glob(pattern='**/%s', path='%s')", pattern, path)
	}
	return fmt.Sprintf("Use: Glob(pattern='%s', path='%s')", pattern, path)
}

func suggestGitSwitch(command string) string {
	if m := gitSwitchRe.FindStringSubmatch(command); m != nil {
		return fmt.Sprintf("Do not switch branches in worktrees. Use: Bash(command='git checkout -b %s') for new branches", m[1])
	}
	return "Do not switch branches in worktrees. Use git -C for read-only operations on main."
}

func suggestGitAddBroad(command string) string {
	if m := gitAddBroadRe.FindStringSubmatch(command); m != nil {
		if cPath := m[1]; cPath != "" {
			return fmt.Sprintf("Use: Bash(command='git -C %s add file1.go file2.go') with specific file paths", cPath)
		}
	}
	return "Use: Bash(command='git add file1.go file2.go') with specific file paths"
}

func suggestGitNoVerify(command string) string {
	if m := gitNoVerifyRe.FindStringSubmatch(command); m != nil {
		return fmt.Sprintf("Fix the issue that git %s verification caught, then retry without --no-verify", m[1])
	}
	return ""
}

func suggestStat(command string) string {
	if m := statArgsRe.FindStringSubmatch(command); m != nil {
		return fmt.Sprintf("Use: Read(file_path='%s') to check file existence/contents", m[1])
	}
	return ""
}

func suggestSensitiveDotdir(command string) string {
	if m := sensitiveDotdirRe.FindStringSubmatch(command); m != nil {
		return fmt.Sprintf("Do not access ~/.%s/. Ask the user to perform credential operations manually", m[2])
	}
	return ""
}
