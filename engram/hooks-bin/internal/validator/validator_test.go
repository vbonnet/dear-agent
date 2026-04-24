package validator

import "testing"

func TestValidateCommand(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		wantBlock bool
		wantName  string
	}{
		// Core patterns that remain blocked
		{"block cd command", "cd /path", true, "cd command"},
		{"block semicolon", "cmd1; cmd2", true, "command separator (;)"},
		{"block while loop", "while true do sleep 1 done", true, "while loop"},
		{"block find command", "find . -name '*.txt'", true, "find command"},
		{"block python3 one-liner", "python3 -c 'import sys; print(sys.version)'", true, "python one-liner (python3 -c / python -c)"},
		{"block python one-liner", "python -c 'import os; print(os.getcwd())'", true, "python one-liner (python3 -c / python -c)"},
		{"block python3 with complex one-liner", "python3 -c \"import os; files = os.listdir('.'); print(max(files))\"", true, "python one-liner (python3 -c / python -c)"},
		{"block python3 heredoc", "python3 <<EOF", true, "python heredoc (python3 << / python <<)"},
		{"block python heredoc", "python <<EOF", true, "python heredoc (python3 << / python <<)"},
		{"block python3 heredoc with code", "python3 << 'END'", true, "python heredoc (python3 << / python <<)"},

		// Relaxed patterns — now allowed (v3.0)
		{"allow command chaining (relaxed)", "cmd1 && cmd2", false, ""},
		{"allow error suppression (relaxed)", "cmd1 || cmd2", false, ""},
		{"block cat command (re-blocked)", "cat file.txt", true, "cat (standalone)"},
		{"block cat with pipe to grep (cat in first segment)", "cat file | grep text", true, "cat (standalone)"},
		{"block cat with pipe to sed (cat idx 22 before sed idx 24)", "cat file | sed 's/foo/bar/'", true, "cat (standalone)"},
		{"block cat with pipe to awk (cat idx 22 before awk idx 25)", "cat file | awk '{print $1}'", true, "cat (standalone)"},
		{"allow cp command (relaxed)", "cp src dst", false, ""},
		{"allow mv command (relaxed)", "mv src dst", false, ""},
		{"allow touch command (relaxed)", "touch newfile.txt", false, ""},
		{"allow mkdir command", "mkdir dir", false, ""},
		{"allow mkdir -p command", "mkdir -p dir/subdir", false, ""},
		{"allow rm single file (relaxed)", "rm file.txt", false, ""},
		{"allow rm -f single file (relaxed)", "rm -f file.txt", false, ""},
		{"block echo with output redirect (re-blocked)", "echo foo > file.txt", true, "echo/printf (standalone)"},
		{"block echo with append redirect (re-blocked)", "echo bar >> log.txt", true, "echo/printf (standalone)"},
		{"allow heredoc (relaxed)", "wc <<EOF", false, ""},
		{"allow wc (relaxed)", "wc -l file.txt", false, ""},
		{"allow wc with here-string (relaxed)", "wc <<<text", false, ""},
		{"block for loop with echo (re-blocked)", "for f in *.txt do echo $f done", true, "echo/printf (standalone)"},
		{"block echo with command substitution (re-blocked)", "echo $(date)", true, "echo/printf (standalone)"},
		{"block echo with backticks (re-blocked)", "echo `date`", true, "echo/printf (standalone)"},
		{"block echo command (re-blocked)", "echo hello", true, "echo/printf (standalone)"},
		{"block printf command (re-blocked)", "printf 'hello'", true, "echo/printf (standalone)"},
		{"block echo with variable (re-blocked)", `echo "$TMUX"`, true, "echo/printf (standalone)"},
		{"block printf with format (re-blocked)", `printf "%s\n" "value"`, true, "echo/printf (standalone)"},

		// Inline env var prefix (relaxed v3.0)
		{"allow GIT_DIR prefix (relaxed)", "GIT_DIR=/path/.git git worktree add /wt branch", false, ""},
		{"allow GIT_WORK_TREE prefix (relaxed)", "GIT_WORK_TREE=/path git status", false, ""},
		{"allow GOPRIVATE prefix (relaxed)", "GOPRIVATE=github.com/foo/* go build ./...", false, ""},
		{"allow CGO_ENABLED prefix (relaxed)", "CGO_ENABLED=0 go build -o bin/app", false, ""},
		{"allow multi-word env value (relaxed)", "PYTHONPATH=/a:/b python3 script.py", false, ""},
		{"allow bare assignment no cmd", "FOO=bar", false, ""},
		{"allow env var in middle", "go build -ldflags '-X main.version=1.0'", false, ""},

		// Test allowed commands
		{"allow python3 script", "python3 /tmp/script.py", false, ""},
		{"allow python3 module", "python3 -m pytest tests/", false, ""},
		{"allow empty command", "", false, ""},
		{"allow git status", "git status", false, ""},
		{"allow git commit", "git commit -m 'message'", false, ""},
		{"allow go build", "go build ./cmd/hook", false, ""},
		{"allow npm install", "npm install", false, ""},
		{"allow pytest", "pytest tests/", false, ""},
		{"allow docker run", "docker run image", false, ""},

		// Previously relaxed patterns — still allowed
		{"allow pipe", "cmd1 | cmd2", false, ""},
		{"allow stderr redirect", "cmd 2>/dev/null", false, ""},
		{"allow stderr append", "cmd 2>>error.log", false, ""},
		{"allow input redirect", "cmd < file", false, ""},
		{"allow here-string", "cmd <<<text", false, ""},

		// Edge cases
		{"allow cd in middle of word", "abcd efg", false, ""},
		{"block cd with double space", "cd  /path", true, "cd command"},
		{"allow command without spaces", "gitpush", false, ""},

		// NEW: rm-recursive pattern (blocks rm -r, rm -rf, rm -fr)
		{"block rm -rf", "rm -rf /path/to/dir", true, "recursive rm (rm -r / rm -rf)"},
		{"block rm -r", "rm -r directory/", true, "recursive rm (rm -r / rm -rf)"},
		{"block rm -fr", "rm -fr /tmp/build", true, "recursive rm (rm -r / rm -rf)"},
		{"allow rm single file", "rm file1.go", false, ""},
		{"allow rm -f single file", "rm -f file1.go", false, ""},

		// NEW: sed-in-place pattern (blocks sed -i, sed -ni, sed --in-place)
		{"block sed -i", "sed -i 's/foo/bar/' file.txt", true, "sed in-place edit (sed -i)"},
		{"block sed -ni", "sed -ni '/pattern/p' file.txt", true, "sed in-place edit (sed -i)"},
		{"block sed --in-place", "sed --in-place 's/a/b/' file", true, "sed in-place edit (sed -i)"},
		{"block sed read-only (re-blocked)", "sed 's/foo/bar/' file.txt", true, "sed (standalone)"},
		{"block sed -n (re-blocked)", "sed -n '/pattern/p' file.txt", true, "sed (standalone)"},
		{"block sed -e (re-blocked)", "sed -e 's/foo/bar/' file.txt", true, "sed (standalone)"},

		// NEW: redirect-system-path pattern
		{"block redirect to /etc", "echo cfg > /etc/hosts", true, "redirection to system path"},
		{"block redirect to /var", "cmd > /var/log/custom.log", true, "redirection to system path"},
		{"block redirect to /usr", "cmd >> /usr/local/bin/script", true, "redirection to system path"},
		{"block echo to project path (echo re-blocked)", "echo hello > output.txt", true, "echo/printf (standalone)"},
		{"allow redirect to /tmp", "cmd > /tmp/output.log", false, ""},
		{"allow redirect to home", "cmd > /tmp/test/file.txt", false, ""},

		// NEW: sensitive-dotdir-access pattern
		{"block cat ~/.ssh/", "cat ~/.ssh/id_rsa", true, "sensitive dotdir access (~/.ssh, ~/.aws, ~/.gnupg)"},
		{"block ls ~/.aws/", "ls ~/.aws/credentials", true, "sensitive dotdir access (~/.ssh, ~/.aws, ~/.gnupg)"},
		{"block cp ~/.gnupg/", "cp ~/.gnupg/pubring.kbx /tmp/", true, "sensitive dotdir access (~/.ssh, ~/.aws, ~/.gnupg)"},
		{"block ~/.gpg/", "cat ~/.gpg/trustdb.gpg", true, "sensitive dotdir access (~/.ssh, ~/.aws, ~/.gnupg)"},
		{"block cat ~/.claude/ (cat re-blocked)", "cat ~/.claude/CLAUDE.md", true, "cat (standalone)"},
		{"block ls ~/.config/ (ls re-blocked)", "ls ~/.config/settings.json", true, "ls (standalone)"},
		{"allow ssh command (not dotdir)", "ssh user@host", false, ""},

		// Text processing commands re-blocked
		{"block standalone ls", "ls -la /path", true, "ls (standalone)"},
		{"block standalone grep", "grep -l 'pattern' /path/*.md", true, "grep/rg (standalone)"},
		{"block standalone sed (read-only)", "sed 's/foo/bar/' file.txt", true, "sed (standalone)"},
		{"block standalone awk", "awk '{print $1}' file.txt", true, "awk (standalone)"},
		{"block standalone head", "head -n 20 file.txt", true, "head/tail (standalone)"},
		{"block standalone tail", "tail -n +8 file.txt", true, "head/tail (standalone)"},
		{"allow standalone wc (still relaxed)", "wc -l file.txt", false, ""},
		{"allow standalone cut (still relaxed)", "cut -d, -f1 file.csv", false, ""},
		{"allow standalone sort (still relaxed)", "sort -u file.txt", false, ""},
		{"allow standalone uniq (still relaxed)", "uniq -c file.txt", false, ""},

		// Bare text commands re-blocked
		{"block bare ls command", "ls", true, "ls (standalone)"},
		{"block bare grep command", "grep", true, "grep/rg (standalone)"},
		{"block bare sed command", "sed", true, "sed (standalone)"},
		{"block bare awk command", "awk", true, "awk (standalone)"},
		{"block bare head command", "head", true, "head/tail (standalone)"},
		{"block bare tail command", "tail", true, "head/tail (standalone)"},
		{"allow bare wc command (still relaxed)", "wc", false, ""},
		{"allow lsblk (word boundary)", "lsblk -f", false, ""},
		{"allow mygrep (word boundary)", "mygrep pattern file", false, ""},
		{"allow awkward (word boundary)", "awkward command", false, ""},
		{"allow healhead (word boundary)", "healhead start", false, ""},

		// Hyphenated tool names — word boundary fix (v3.1)
		{"allow ast-grep (hyphenated tool)", "ast-grep -p 'pattern'", false, ""},
		{"allow ast-grep with lang flag", "ast-grep --lang go 'func main'", false, ""},
		{"allow auto-cd (hyphenated)", "auto-cd /path", false, ""},
		{"allow mini-awk (hyphenated)", "mini-awk '{print}'", false, ""},
		{"allow tom-cat (hyphenated)", "tom-cat file.txt", false, ""},
		{"allow figure-head (hyphenated)", "figure-head start", false, ""},
		{"allow dog-tail (hyphenated)", "dog-tail end", false, ""},
		{"allow mean-while (hyphenated)", "mean-while waiting", false, ""},
		{"allow path-find (hyphenated)", "path-find /dir", false, ""},
		{"block ls with tab separator", "ls\t-la", true, "ls (standalone)"},
		{"block grep with multiple tabs", "grep\t\t-rni pattern", true, "grep/rg (standalone)"},
		{"block grep with complex flags", "grep -rniE 'pattern' --exclude-dir=.git .", true, "grep/rg (standalone)"},
		{"block awk with complex program", "awk 'BEGIN {FS=\",\"} {print $1,$3}' file.csv", true, "awk (standalone)"},

		// Complex real violations - updated expectations for v3.0
		{"block complex chaining - test catches first", "test -f ~/bin/bow-core && echo \"installed\" || echo \"not found\"", true, "bash test ([/[[/test)"},
		{"block for loop with grep - semicolon catches", "for f in /path/*/STATUS.md; do echo \"===\"; grep \"pattern\" \"$f\"; done", true, "command separator (;)"},
		{"block cd with ls", "cd /tmp/worktree/agent && ls -la path/ 2>/dev/null || echo \"No dir\"", true, "cd command"},
		{"block tail with sha256sum (checksum catches first)", "tail -n +8 ~/.claude/skills/file.md | sha256sum | awk '{print $1}'", true, "checksum (sha256sum/sha1sum/md5sum/cksum)"},
		{"block go build with command substitution", "go build -C /path $(go list ./...)", true, "command substitution $()"},

		// Command substitution $() (re-blocked)
		{"block $() in agm prompt", `agm send msg --prompt "$(cat file)"`, true, "command substitution $()"},
		{"block bare $() assignment", "result=$(git log)", true, "command substitution $()"},
		{"block $() in echo", "echo $(date)", true, "echo/printf (standalone)"},
		{"allow command without $()", "go build -C /path ./...", false, ""},

		// Quoted string exemption (pattern 3 fix)
		{"allow agm with loop in prompt", `agm send msg --prompt "check the loop"`, false, ""},
		{"allow agm with while in prompt", `agm send msg --prompt "fix the while issue"`, false, ""},
		{"allow git commit mentioning loop", `git commit -m "fix: refactor the while loop handler"`, false, ""},
		{"allow git commit mentioning cd", `git commit -m "docs: explain cd behavior"`, false, ""},
		{"allow agm with sed in prompt", `agm send msg --prompt "use sed to replace text"`, false, ""},
		{"allow agm with find in prompt", `agm send msg --prompt "find the bug"`, false, ""},
		{"allow agm with echo in prompt", `agm send msg --prompt "echo back the result"`, false, ""},
		{"allow single-quoted prompt with loop", `agm send msg --prompt 'check the loop'`, false, ""},
		{"block actual while loop", "while true; do sleep 1; done", true, "while loop"},
		{"block actual cd command", "cd /path/to/dir", true, "cd command"},
		{"block $() inside double quotes (executable)", `agm send msg --prompt "$(cat file)"`, true, "command substitution $()"},

		// git checkout main/master (worktree safety)
		{"block git checkout main", "git checkout main", true, "git checkout main/master (worktree safety)"},
		{"block git checkout master", "git checkout master", true, "git checkout main/master (worktree safety)"},
		{"block git -C checkout main", "git -C /repo checkout main", true, "git checkout main/master (worktree safety)"},
		{"allow git checkout -b feature", "git checkout -b feature-branch", false, ""},
		{"allow git checkout feature-branch", "git checkout feature-branch", false, ""},
		{"allow git checkout -- file", "git checkout -- file.txt", false, ""},

		// git switch and git stash
		{"block git switch", "git switch feature", true, "git switch (branch-level operation)"},
		{"block git switch -c", "git switch -c new-branch", true, "git switch (branch-level operation)"},
		{"block git switch with -C repo", "git -C /repo switch main", true, "git switch (branch-level operation)"},
		{"block git stash", "git stash", true, "git stash (contaminates shared state)"},
		{"block git stash push", "git stash push -m 'wip'", true, "git stash (contaminates shared state)"},
		{"block git stash pop", "git stash pop", true, "git stash (contaminates shared state)"},
		{"allow git show (not switch)", "git show HEAD", false, ""},
		{"allow git status (not stash)", "git status", false, ""},

		// git add (unchanged)
		{"allow git add specific file", "git add file.go", false, ""},
		{"block git add -A", "git add -A", true, "git add (broad staging: . -A --all -u --update)"},
		{"block git add dot", "git add .", true, "git add (broad staging: . -A --all -u --update)"},
		{"allow git add with -C specific file", "git -C /repo add file.go", false, ""},
		{"allow git worktree add", "git worktree add ~/worktrees/foo branch", false, ""},
		{"allow git remote add", "git remote add origin url", false, ""},
		{"allow git submodule add", "git submodule add url path", false, ""},
		{"allow git worktree add with -C", "git -C /repo worktree add ~/wt branch", false, ""},

		// stat command (unchanged)
		{"block stat with format string", "stat -c '%s %n' /tmp/file.output", true, "stat (causes shell expansion permission prompt)"},
		{"block stat bare", "stat /path/to/file", true, "stat (causes shell expansion permission prompt)"},
		{"block stat with -f flag", "stat -f %z file.txt", true, "stat (causes shell expansion permission prompt)"},
		{"allow thermostat (word boundary)", "thermostat check", false, ""},
		{"allow git diff --stat", "git diff --cached --stat", false, ""},
		{"allow git log --stat", "git log --oneline --stat", false, ""},

		// checksum commands (unchanged)
		{"block sha256sum", "sha256sum /tmp/test/src/ws/oss/repos/engram/file.md", true, "checksum (sha256sum/sha1sum/md5sum/cksum)"},
		{"block sha256sum bare", "sha256sum file.txt", true, "checksum (sha256sum/sha1sum/md5sum/cksum)"},
		{"block sha1sum", "sha1sum file.txt", true, "checksum (sha256sum/sha1sum/md5sum/cksum)"},
		{"block sha512sum", "sha512sum file.txt", true, "checksum (sha256sum/sha1sum/md5sum/cksum)"},
		{"block md5sum", "md5sum file.txt", true, "checksum (sha256sum/sha1sum/md5sum/cksum)"},
		{"block cksum", "cksum file.txt", true, "checksum (sha256sum/sha1sum/md5sum/cksum)"},

		// AGM_SKIP_TEST_GATE bypass prevention (unchanged)
		{"block export AGM_SKIP_TEST_GATE", "export AGM_SKIP_TEST_GATE=1", true, "AGM_SKIP_TEST_GATE (test gate bypass)"},
		{"block bare AGM_SKIP_TEST_GATE", "AGM_SKIP_TEST_GATE=1", true, "AGM_SKIP_TEST_GATE (test gate bypass)"},
		{"allow normal git commit (no skip gate)", "git commit -m 'fix test gate'", false, ""},

		// Pipe target exemption — tools after | should be allowed
		{"allow tail as pipe target", "tmux capture-pane -t session -p | tail -20", false, ""},
		{"allow grep as pipe target", "agm session list 2>/dev/null | grep pattern", false, ""},
		{"allow head as pipe target", "some-command | head -5", false, ""},
		{"allow awk as pipe target", "some-command | awk '{print $1}'", false, ""},
		{"allow sed as pipe target", "some-command | sed 's/foo/bar/'", false, ""},
		{"allow grep in multi-pipe", "some-command | grep pattern | head -5", false, ""},
		{"block standalone tail (not a pipe target)", "tail file.txt", true, "head/tail (standalone)"},
		{"block standalone grep (not a pipe target)", "grep pattern file", true, "grep/rg (standalone)"},
		{"block standalone cat (not a pipe target)", "cat file", true, "cat (standalone)"},
		{"block ls before pipe", "ls /path | grep pattern", true, "ls (standalone)"},
		{"allow find as pipe target", "some-command | find . -name foo", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOk, gotName, _ := ValidateCommand(tt.command)
			gotBlock := !gotOk

			if gotBlock != tt.wantBlock {
				t.Errorf("ValidateCommand(%q) blocked = %v, want %v", tt.command, gotBlock, tt.wantBlock)
			}
			if gotName != tt.wantName {
				t.Errorf("ValidateCommand(%q) pattern = %q, want %q", tt.command, gotName, tt.wantName)
			}
		})
	}
}

// TestPatternNameInvariant ensures forbiddenPatterns and patternNames stay in sync.
// If a pattern is added without a corresponding name (or vice versa), this fails.
func TestPatternNameInvariant(t *testing.T) {
	if len(forbiddenPatterns) != len(patternNames) {
		t.Fatalf("Pattern/name mismatch: %d patterns but %d names",
			len(forbiddenPatterns), len(patternNames))
	}
	if len(forbiddenPatterns) != len(remediations) {
		t.Fatalf("Pattern/remediation mismatch: %d patterns but %d remediations",
			len(forbiddenPatterns), len(remediations))
	}
}

func BenchmarkValidateCommand(b *testing.B) {
	testCases := []struct {
		name    string
		command string
	}{
		{"allowed git", "git status"},
		{"allowed go", "go build ./cmd/hook"},
		{"allowed npm", "npm install"},
		{"blocked cd", "cd /path"},
		{"blocked rm -rf", "rm -rf /tmp/dir"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ValidateCommand(tc.command)
			}
		})
	}
}
