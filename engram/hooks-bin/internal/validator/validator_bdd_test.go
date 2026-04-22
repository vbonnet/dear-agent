package validator_test

import (
	"testing"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/validator"
)

func TestValidator(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Validator BDD Suite")
}

var _ = ginkgo.Describe("Pattern Consolidation", func() {
	ginkgo.Context("when using alternation patterns", func() {
		ginkgo.Describe("bash conditionals (relaxed v3.0)", func() {
			ginkgo.It("should allow conditional keywords (relaxed)", func() {
				// Conditionals are relaxed in v3.0, but commands with ; are still blocked
				// by the command-separator pattern
				cmd := "if true; then cmd; fi"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("command separator (;)"),
					"With conditionals relaxed, semicolon catches this")
			})
		})

		ginkgo.Describe("bash test commands (test|[|[[)", func() {
			ginkgo.It("should block all test command variations", func() {
				testCommands := []struct {
					cmd       string
					variation string
				}{
					{"test -f ~/bin/bow-core", "test"},
					{"[ -f file ]", "["},
					{"[[ -f file ]]", "[["},
				}

				for _, tc := range testCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected %s to be blocked", tc.variation)
					gomega.Expect(patternName).To(gomega.Equal("bash test ([/[[/test)"),
						"All test variations should match same pattern")
				}
			})

			ginkgo.It("should block test in complex expressions", func() {
				cmd := "test -f ~/bin/bow-core && echo 'installed' || echo 'not_found'"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				// With && and || relaxed, test pattern catches this
				gomega.Expect(patternName).To(gomega.Equal("bash test ([/[[/test)"))
			})
		})

		ginkgo.Describe("echo/printf commands (re-blocked)", func() {
			ginkgo.It("should block plain echo and printf", func() {
				blockedCommands := []string{
					"echo hello",
					"printf 'hello world'",
					`echo "$TMUX"`,
					`printf "%s\n" "value"`,
				}

				for _, cmd := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", cmd)
					gomega.Expect(patternName).To(gomega.Equal("echo/printf (standalone)"))
				}
			})

			ginkgo.It("should block echo/printf with non-system-path redirection", func() {
				blockedCommands := []struct {
					cmd  string
					desc string
				}{
					{`echo "foo" > file`, "echo with > to local file"},
					{`echo "bar" >> log`, "echo with >> to local file"},
					{`printf "%s" val > out`, "printf with > to local file"},
				}

				for _, tc := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked (re-blocked)", tc.desc)
					gomega.Expect(patternName).To(gomega.Equal("echo/printf (standalone)"))
				}
			})

			ginkgo.It("should block echo with redirection to system path (system-path pattern catches first)", func() {
				cmd := `echo "config" > /etc/hosts`
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("redirection to system path"))
			})
		})
	})

	ginkgo.Context("when checking word boundaries", func() {
		ginkgo.Describe("text processing commands (re-blocked)", func() {
			ginkgo.It("should block standalone text processing commands", func() {
				blockedCommands := []struct {
					cmd         string
					patternName string
				}{
					{"ls", "ls (standalone)"},
					{"grep", "grep/rg (standalone)"},
					{"sed", "sed (standalone)"},
					{"awk", "awk (standalone)"},
					{"head", "head/tail (standalone)"},
					{"tail", "head/tail (standalone)"},
				}

				for _, tc := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected bare '%s' to be blocked", tc.cmd)
					gomega.Expect(patternName).To(gomega.Equal(tc.patternName))
				}
			})

			ginkgo.It("should still allow wc, cut, sort, uniq", func() {
				allowedCommands := []string{"wc", "cut", "sort", "uniq"}
				for _, cmd := range allowedCommands {
					ok, _, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected bare '%s' to still be allowed", cmd)
				}
			})

			ginkgo.It("should block commands with arguments", func() {
				blockedCommands := []struct {
					cmd         string
					patternName string
				}{
					{"ls -la /path", "ls (standalone)"},
					{"grep -rni 'pattern' .", "grep/rg (standalone)"},
					{"sed 's/foo/bar/' file.txt", "sed (standalone)"},
					{"awk '{print $1}' data.csv", "awk (standalone)"},
					{"head -n 20 file.txt", "head/tail (standalone)"},
					{"tail -f /var/log/syslog", "head/tail (standalone)"},
				}

				for _, tc := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", tc.cmd)
					gomega.Expect(patternName).To(gomega.Equal(tc.patternName))
				}
			})

			ginkgo.It("should allow wc with arguments (still relaxed)", func() {
				ok, _, _ := validator.ValidateCommand("wc -l file.txt")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should still block sed -i (in-place, caught by specific pattern first)", func() {
				cmd := "sed -i 's/foo/bar/' file.txt"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("sed in-place edit (sed -i)"))
			})
		})

		ginkgo.Describe("pattern as substring in different word", func() {
			ginkgo.It("should allow commands where pattern is part of a different word", func() {
				allowedCommands := []struct {
					cmd         string
					contains    string
					description string
				}{
					{"lsblk -f", "ls", "lsblk is not ls"},
					{"mygrep pattern file", "grep", "mygrep is not grep"},
					{"awkward command", "awk", "awkward is not awk"},
					{"healhead start", "head", "healhead is not head"},
					{"ast-grep -p 'pattern'", "grep", "ast-grep is not grep (hyphenated)"},
					{"ast-grep --lang go 'func main'", "grep", "ast-grep with flags"},
					{"auto-cd /path", "cd", "auto-cd is not cd (hyphenated)"},
					{"mini-awk '{print}'", "awk", "mini-awk is not awk (hyphenated)"},
					{"tom-cat file.txt", "cat", "tom-cat is not cat (hyphenated)"},
					{"figure-head start", "head", "figure-head is not head (hyphenated)"},
					{"mean-while waiting", "while", "mean-while is not while (hyphenated)"},
					{"path-find /dir", "find", "path-find is not find (hyphenated)"},
				}

				for _, tc := range allowedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed: %s", tc.cmd, tc.description)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})
		})

		ginkgo.Describe("whitespace variations", func() {
			ginkgo.It("should block text processing commands with tabs (re-blocked)", func() {
				tabCommands := []struct {
					cmd         string
					patternName string
				}{
					{"ls\t-la", "ls (standalone)"},
					{"grep\t\t-rni pattern", "grep/rg (standalone)"},
				}

				for _, tc := range tabCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected command with tabs to be blocked (re-blocked)")
					gomega.Expect(patternName).To(gomega.Equal(tc.patternName))
				}
			})

			ginkgo.It("should block commands with multiple spaces", func() {
				cmd := "cd  /path"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("cd command"))
			})
		})
	})

	ginkgo.Context("when handling command chaining and combinations", func() {
		ginkgo.Describe("multiple violations in single command", func() {
			ginkgo.It("should report first matching pattern", func() {
				cmd := "cd /path && git status"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("cd command"),
					"cd should match before && due to pattern order")
			})

			ginkgo.It("should block cat | grep (cat in first segment, grep exempt as pipe target)", func() {
				cmd := "cat file | grep text"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse(),
					"cat in first segment is still blocked")
				gomega.Expect(patternName).To(gomega.Equal("cat (standalone)"),
					"grep is exempt as pipe target, cat catches in first segment")
			})
		})

		ginkgo.Describe("complex chaining scenarios", func() {
			ginkgo.It("should block complex test expressions via bash-test pattern", func() {
				cmd := "test -f ~/bin/bow-core && echo \"installed\" || echo \"not_found\""
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("bash test ([/[[/test)"))
			})

			ginkgo.It("should block for loops with semicolons via semicolon pattern (for loop relaxed)", func() {
				cmd := "for f in /path/*/STATUS.md; do echo \"===\"; grep \"pattern\" \"$f\"; done"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("command separator (;)"))
			})

			ginkgo.It("should block multi-stage pipe chains by first forbidden pattern (ls catches first after quote stripping)", func() {
				// With quote stripping, '[0-9]' inside single quotes is removed,
				// so ls (index 20) is the first matching pattern
				cmd := "ls ~/dir/ | grep -E '^violation-[0-9]+\\.md$' | sed 's/violation-//' | tail -1"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("ls (standalone)"))
			})
		})

		ginkgo.Describe("command substitution (re-blocked)", func() {
			ginkgo.It("should block $() substitution", func() {
				cmd := "go build -C /path $(go list ./...)"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse(), "Command substitution is re-blocked")
				gomega.Expect(patternName).To(gomega.Equal("command substitution $()"))
			})

			ginkgo.It("should block $() inside double-quoted args (executable in bash)", func() {
				cmd := `agm send msg --prompt "$(cat file)"`
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("command substitution $()"))
			})

			ginkgo.It("should block echo with backtick substitution (echo re-blocked)", func() {
				cmd := "echo `date`"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse(), "echo is re-blocked")
				gomega.Expect(patternName).To(gomega.Equal("echo/printf (standalone)"))
			})
		})

		ginkgo.Describe("quoted string argument exemption", func() {
			ginkgo.It("should allow English words inside --prompt double quotes", func() {
				allowedCommands := []string{
					`agm send msg --prompt "check the loop"`,
					`agm send msg --prompt "fix the while issue"`,
					`agm send msg --prompt "use sed to replace text"`,
					`agm send msg --prompt "find the bug"`,
					`agm send msg --prompt "echo back the result"`,
				}

				for _, cmd := range allowedCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(),
						"Expected '%s' to be allowed (words inside quotes are not bash syntax)", cmd)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})

			ginkgo.It("should allow English words inside single quotes", func() {
				cmd := `agm send msg --prompt 'check the loop'`
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeTrue())
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})

			ginkgo.It("should allow git commit messages mentioning bash keywords", func() {
				allowedCommands := []string{
					`git commit -m "fix: refactor the while loop handler"`,
					`git commit -m "docs: explain cd behavior"`,
					`git commit -m "test: add echo validation"`,
				}

				for _, cmd := range allowedCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(),
						"Expected '%s' to be allowed (commit message content)", cmd)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})

			ginkgo.It("should still block actual bash constructs outside quotes", func() {
				blockedCommands := []struct {
					cmd         string
					patternName string
				}{
					{"while true; do sleep 1; done", "while loop"},
					{"cd /path/to/dir", "cd command"},
					{"find . -name '*.go'", "find command"},
				}

				for _, tc := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(),
						"Expected '%s' to still be blocked", tc.cmd)
					gomega.Expect(patternName).To(gomega.Equal(tc.patternName))
				}
			})
		})
	})

	ginkgo.Context("when testing pattern order dependency", func() {
		ginkgo.Describe("specific patterns before general patterns", func() {
			ginkgo.It("should match test pattern before input redirection", func() {
				cmd := "[ -f file ]"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("bash test ([/[[/test)"),
					"Test pattern should match before < redirection pattern")
			})

			ginkgo.It("should match semicolon for conditionals (conditionals relaxed v3.0)", func() {
				cmd := "if true; then cmd; fi"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("command separator (;)"),
					"With conditionals relaxed, semicolon pattern catches this")
			})

			ginkgo.It("should match while before semicolon", func() {
				whileCmd := "while true do sleep 1 done"
				ok, patternName, _ := validator.ValidateCommand(whileCmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("while loop"),
					"While loop should match before semicolon")
			})
		})

		ginkgo.Describe("redirection precedence (relaxed v3.0)", func() {
			ginkgo.It("should allow general redirection (relaxed v3.0)", func() {
				// All general redirection patterns are relaxed
				cmds := []string{
					"cmd &>>output.log",
					"cmd &>/dev/null",
					"cmd >>output.log",
					"cmd >output.txt",
				}

				for _, cmd := range cmds {
					ok, _, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed (redirection relaxed v3.0)", cmd)
				}
			})

			ginkgo.It("should still block redirection to system paths", func() {
				cmd := "echo config > /etc/hosts"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("redirection to system path"))
			})

			ginkgo.It("should allow 2> and 2>> (relaxed)", func() {
				errRedirectCmd := "cmd 2>/dev/null"
				ok, _, _ := validator.ValidateCommand(errRedirectCmd)
				gomega.Expect(ok).To(gomega.BeTrue(), "2> should be allowed (relaxed)")

				errAppendCmd := "cmd 2>>error.log"
				ok, _, _ = validator.ValidateCommand(errAppendCmd)
				gomega.Expect(ok).To(gomega.BeTrue(), "2>> should be allowed (relaxed)")
			})

			ginkgo.It("should handle relaxed <<< and < and <<", func() {
				// <<< (here-string) relaxed
				hereStringAllowed := "cmd <<<text"
				ok, _, _ := validator.ValidateCommand(hereStringAllowed)
				gomega.Expect(ok).To(gomega.BeTrue(), "<<< should be allowed (relaxed)")

				// << (heredoc) with cat is now blocked (cat re-blocked)
				heredocCmd := "cat <<EOF"
				ok, patternName, _ := validator.ValidateCommand(heredocCmd)
				gomega.Expect(ok).To(gomega.BeFalse(), "cat <<EOF should be blocked (cat re-blocked)")
				gomega.Expect(patternName).To(gomega.Equal("cat (standalone)"))

				// < (input redirection) relaxed
				inputCmd := "cmd < file"
				ok, _, _ = validator.ValidateCommand(inputCmd)
				gomega.Expect(ok).To(gomega.BeTrue(), "Input redirection < should be allowed (relaxed)")
			})
		})
	})

	ginkgo.Context("when blocking permission bypass patterns", func() {
		ginkgo.Describe("ENV=value command prefix (relaxed v3.0)", func() {
			ginkgo.It("should allow commands with inline env var prefix (relaxed v3.0)", func() {
				prefixCommands := []string{
					"GIT_DIR=/path/.git git worktree add /wt branch",
					"GIT_WORK_TREE=/path git status",
					"GOPRIVATE=github.com/foo/* go build ./...",
					"CGO_ENABLED=0 go build -o bin/app",
					"PYTHONPATH=/a:/b python3 script.py",
				}

				for _, cmd := range prefixCommands {
					ok, _, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed (relaxed v3.0)", cmd)
				}
			})

			ginkgo.It("should allow bare assignments without commands", func() {
				ok, patternName, _ := validator.ValidateCommand("FOO=bar")
				gomega.Expect(ok).To(gomega.BeTrue(), "Bare assignment should be allowed")
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})

			ginkgo.It("should allow env-like syntax in the middle of commands", func() {
				ok, patternName, _ := validator.ValidateCommand("go build -ldflags '-X main.version=1.0'")
				gomega.Expect(ok).To(gomega.BeTrue(), "= in middle of command should be allowed")
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})
		})

		ginkgo.Describe("git add subcommand discrimination", func() {
			ginkgo.It("should block broad git add patterns", func() {
				broadCommands := []string{
					"git add -A",
					"git add .",
					"git add --all",
					"git add -u",
				}

				for _, cmd := range broadCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", cmd)
					gomega.Expect(patternName).To(gomega.Equal("git add (broad staging: . -A --all -u --update)"))
				}
			})

			ginkgo.It("should allow git add with specific files", func() {
				specificCommands := []string{
					"git add file.go",
					"git -C /repo add file.go",
					"git add src/main.go tests/main_test.go",
				}

				for _, cmd := range specificCommands {
					ok, _, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed", cmd)
				}
			})

			ginkgo.It("should allow git subcommand add (worktree, remote, submodule)", func() {
				allowedCommands := []string{
					"git worktree add ~/worktrees/foo branch",
					"git remote add origin url",
					"git submodule add url path",
					"git -C /repo worktree add ~/wt branch",
				}

				for _, cmd := range allowedCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed", cmd)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})
		})
	})

	ginkgo.Context("when returning remediation advice", func() {
		ginkgo.It("should return remediation for cd command", func() {
			_, _, remediation := validator.ValidateCommand("cd /path")
			gomega.Expect(remediation).To(gomega.ContainSubstring("absolute paths"))
		})

		ginkgo.It("should allow pipe operator (relaxed)", func() {
			ok, patternName, _ := validator.ValidateCommand("cmd1 | cmd2")
			gomega.Expect(ok).To(gomega.BeTrue(), "Pipe operator is relaxed and should be allowed")
			gomega.Expect(patternName).To(gomega.BeEmpty())
		})

		ginkgo.It("should return remediation for git add broad staging with exact tool call", func() {
			_, _, remediation := validator.ValidateCommand("git add .")
			gomega.Expect(remediation).To(gomega.ContainSubstring("git add file1.go file2.go"))
		})

		ginkgo.It("should return remediation for sed -i with exact Edit tool call", func() {
			_, _, remediation := validator.ValidateCommand("sed -i 's/old/new/' file.txt")
			gomega.Expect(remediation).To(gomega.ContainSubstring("Edit(file_path='file.txt', old_string='old', new_string='new')"))
		})

		ginkgo.It("should return remediation for rm -rf with extracted path", func() {
			_, _, remediation := validator.ValidateCommand("rm -rf /tmp/dir")
			gomega.Expect(remediation).To(gomega.ContainSubstring("/tmp/dir"))
		})

		ginkgo.It("should return remediation for system path redirect with exact Write suggestion", func() {
			_, _, remediation := validator.ValidateCommand("echo x > /etc/hosts")
			gomega.Expect(remediation).To(gomega.ContainSubstring("/etc/hosts"))
			gomega.Expect(remediation).To(gomega.ContainSubstring("Write(file_path="))
		})

		ginkgo.It("should return remediation for sensitive dotdir with extracted dir", func() {
			_, _, remediation := validator.ValidateCommand("cat ~/.ssh/id_rsa")
			gomega.Expect(remediation).To(gomega.ContainSubstring("~/.ssh/"))
		})
	})

	ginkgo.Context("when validating new targeted patterns (v3.0)", func() {
		ginkgo.Describe("rm-recursive pattern", func() {
			ginkgo.It("should block rm -rf, rm -r, rm -fr", func() {
				blockedCommands := []struct {
					cmd  string
					desc string
				}{
					{"rm -rf /path/to/dir", "rm -rf"},
					{"rm -r directory/", "rm -r"},
					{"rm -fr /tmp/build", "rm -fr"},
				}
				for _, tc := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", tc.desc)
					gomega.Expect(patternName).To(gomega.Equal("recursive rm (rm -r / rm -rf)"))
				}
			})

			ginkgo.It("should allow non-recursive rm", func() {
				allowedCommands := []string{
					"rm file.txt",
					"rm -f file.txt",
					"rm file1.go file2.go",
				}
				for _, cmd := range allowedCommands {
					ok, _, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed", cmd)
				}
			})
		})

		ginkgo.Describe("sed-in-place pattern", func() {
			ginkgo.It("should block sed -i and variants", func() {
				blockedCommands := []struct {
					cmd  string
					desc string
				}{
					{"sed -i 's/foo/bar/' file.txt", "sed -i"},
					{"sed -ni '/pattern/p' file.txt", "sed -ni"},
					{"sed --in-place 's/a/b/' file", "sed --in-place"},
				}
				for _, tc := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", tc.desc)
					gomega.Expect(patternName).To(gomega.Equal("sed in-place edit (sed -i)"))
				}
			})

			ginkgo.It("should block read-only sed (sed re-blocked)", func() {
				blockedCommands := []string{
					"sed 's/foo/bar/' file.txt",
					"sed -n '/pattern/p' file.txt",
					"sed -e 's/foo/bar/' file.txt",
				}
				for _, cmd := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", cmd)
					gomega.Expect(patternName).To(gomega.Equal("sed (standalone)"))
				}
			})
		})

		ginkgo.Describe("redirect-system-path pattern", func() {
			ginkgo.It("should block redirection to system directories", func() {
				blockedCommands := []string{
					"echo cfg > /etc/hosts",
					"cmd > /var/log/custom.log",
					"cmd >> /usr/local/bin/script",
				}
				for _, cmd := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", cmd)
					gomega.Expect(patternName).To(gomega.Equal("redirection to system path"))
				}
			})

			ginkgo.It("should allow redirection to safe paths (non-blocked commands)", func() {
				allowedCommands := []string{
					"cmd > /tmp/output.log",
					"cmd > /tmp/test/file.txt",
				}
				for _, cmd := range allowedCommands {
					ok, _, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed", cmd)
				}
			})

			ginkgo.It("should block echo with safe-path redirection (echo re-blocked)", func() {
				cmd := "echo hello > output.txt"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("echo/printf (standalone)"))
			})
		})

		ginkgo.Describe("sensitive-dotdir-access pattern", func() {
			ginkgo.It("should block access to sensitive directories", func() {
				blockedCommands := []string{
					"cat ~/.ssh/id_rsa",
					"ls ~/.aws/credentials",
					"cp ~/.gnupg/pubring.kbx /tmp/",
					"cat ~/.gpg/trustdb.gpg",
				}
				for _, cmd := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeFalse(), "Expected '%s' to be blocked", cmd)
					gomega.Expect(patternName).To(gomega.Equal("sensitive dotdir access (~/.ssh, ~/.aws, ~/.gnupg)"))
				}
			})

			ginkgo.It("should block cat/ls to non-sensitive dirs (cat/ls re-blocked)", func() {
				ok, patternName, _ := validator.ValidateCommand("cat ~/.claude/CLAUDE.md")
				gomega.Expect(ok).To(gomega.BeFalse(), "cat is re-blocked")
				gomega.Expect(patternName).To(gomega.Equal("cat (standalone)"))

				ok, patternName, _ = validator.ValidateCommand("ls ~/.config/settings.json")
				gomega.Expect(ok).To(gomega.BeFalse(), "ls is re-blocked")
				gomega.Expect(patternName).To(gomega.Equal("ls (standalone)"))
			})

			ginkgo.It("should allow ssh command (not a blocked tool)", func() {
				ok, _, _ := validator.ValidateCommand("ssh user@host")
				gomega.Expect(ok).To(gomega.BeTrue(), "Expected 'ssh user@host' to be allowed")
			})
		})
	})

	ginkgo.Context("when validating edge cases", func() {
		ginkgo.Describe("empty and whitespace commands", func() {
			ginkgo.It("should allow empty command", func() {
				ok, patternName, _ := validator.ValidateCommand("")

				gomega.Expect(ok).To(gomega.BeTrue())
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})

			ginkgo.It("should allow whitespace-only command", func() {
				ok, patternName, _ := validator.ValidateCommand("   ")

				gomega.Expect(ok).To(gomega.BeTrue())
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})
		})

		ginkgo.Describe("allowed development commands", func() {
			ginkgo.It("should allow git commands", func() {
				gitCommands := []string{
					"git status",
					"git commit -m 'message'",
					"git -C /path log",
				}

				for _, cmd := range gitCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed", cmd)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})

			ginkgo.It("should allow go commands", func() {
				goCommands := []string{
					"go build ./cmd/hook",
					"go run main.go",
					"go mod tidy",
				}

				for _, cmd := range goCommands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed", cmd)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})

			ginkgo.It("should allow npm and docker commands", func() {
				commands := []string{
					"npm install",
					"npm run build",
					"docker run -it image",
					"docker ps",
				}

				for _, cmd := range commands {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(), "Expected '%s' to be allowed", cmd)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})
		})

		ginkgo.Describe("complex real-world violations", func() {
			ginkgo.It("should block cd with ls and error redirect", func() {
				cmd := "cd /tmp/worktree/agent && ls -la path/ 2>/dev/null || echo \"No dir\""
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("cd command"))
			})

			ginkgo.It("should block tail piped to sha256sum (checksum pattern at lower index catches first)", func() {
				cmd := "tail -n +8 ~/.claude/skills/file.md | sha256sum | awk '{print $1}'"
				ok, patternName, _ := validator.ValidateCommand(cmd)

				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("checksum (sha256sum/sha1sum/md5sum/cksum)"),
					"Checksum pattern (index 16) checked before head/tail (index 22)")
			})
		})
	})

	ginkgo.Context("git --no-verify flag prevention", func() {
		ginkgo.Describe("blocking --no-verify flag", func() {
			ginkgo.It("should block git commit --no-verify", func() {
				cmd := "git commit -m 'test' --no-verify"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git commit with --no-verify at start", func() {
				cmd := "git commit --no-verify"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git push --no-verify", func() {
				cmd := "git push origin main --no-verify"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git merge --no-verify", func() {
				cmd := "git merge feature-branch --no-verify"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git commit -n (short form)", func() {
				cmd := "git commit -m 'test' -n"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git push -n", func() {
				cmd := "git push origin main -n"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git -C with --no-verify", func() {
				cmd := "git -C /path/to/repo commit --no-verify"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git rebase --no-verify", func() {
				cmd := "git rebase main --no-verify"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git am --no-verify", func() {
				cmd := "git am patch.diff --no-verify"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})

			ginkgo.It("should block git -C path commit -n", func() {
				cmd := "git -C /some/repo commit -m 'msg' -n"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("git --no-verify flag (hook bypass)"))
			})
		})

		ginkgo.Describe("allowing normal git commands (no false positives)", func() {
			ginkgo.It("should allow git commit with message", func() {
				ok, _, _ := validator.ValidateCommand("git commit -m 'fix: update function'")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git push normal", func() {
				ok, _, _ := validator.ValidateCommand("git push origin main")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git merge normal", func() {
				ok, _, _ := validator.ValidateCommand("git merge feature-branch")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git status", func() {
				ok, _, _ := validator.ValidateCommand("git status")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git log", func() {
				ok, _, _ := validator.ValidateCommand("git log --oneline")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git diff", func() {
				ok, _, _ := validator.ValidateCommand("git diff HEAD~1")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git log -n (number of commits, not --no-verify)", func() {
				ok, _, _ := validator.ValidateCommand("git log -n 5")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git log -n5 (no space)", func() {
				ok, _, _ := validator.ValidateCommand("git log -n5")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git -C path status", func() {
				ok, _, _ := validator.ValidateCommand("git -C /path/to/repo status")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git branch --show-current", func() {
				ok, _, _ := validator.ValidateCommand("git branch --show-current")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git -C path branch --show-current", func() {
				ok, _, _ := validator.ValidateCommand("git -C /path/to/repo branch --show-current")
				gomega.Expect(ok).To(gomega.BeTrue())
			})

			ginkgo.It("should allow git commit with message mentioning no-verify", func() {
				ok, _, _ := validator.ValidateCommand("git -C /path commit -m 'docs: explain no-verify prevention'")
				gomega.Expect(ok).To(gomega.BeTrue())
			})
		})
	})

	ginkgo.Context("when blocking test gate bypass patterns", func() {
		ginkgo.Describe("AGM_SKIP_TEST_GATE prevention", func() {
			ginkgo.It("should block export of AGM_SKIP_TEST_GATE", func() {
				cmd := "export AGM_SKIP_TEST_GATE=1"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("AGM_SKIP_TEST_GATE (test gate bypass)"))
			})

			ginkgo.It("should block bare AGM_SKIP_TEST_GATE assignment", func() {
				cmd := "AGM_SKIP_TEST_GATE=1"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse())
				gomega.Expect(patternName).To(gomega.Equal("AGM_SKIP_TEST_GATE (test gate bypass)"))
			})

			ginkgo.It("should return actionable remediation", func() {
				_, _, remediation := validator.ValidateCommand("export AGM_SKIP_TEST_GATE=1")
				gomega.Expect(remediation).To(gomega.ContainSubstring("Fix failing tests"))
			})

			ginkgo.It("should not flag unrelated commands", func() {
				ok, _, _ := validator.ValidateCommand("go test ./...")
				gomega.Expect(ok).To(gomega.BeTrue())
			})
		})
	})

	ginkgo.Context("when commands use pipes", func() {
		ginkgo.Describe("pipe target exemption for standalone tool patterns", func() {
			ginkgo.It("should allow tail as a pipe target", func() {
				cmd := "tmux capture-pane -t session -p | tail -20"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeTrue(),
					"tail after pipe should be allowed")
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})

			ginkgo.It("should allow grep as a pipe target", func() {
				cmd := "agm session list 2>/dev/null | grep pattern"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeTrue(),
					"grep after pipe should be allowed")
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})

			ginkgo.It("should allow head as a pipe target", func() {
				cmd := "some-command | head -5"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeTrue(),
					"head after pipe should be allowed")
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})

			ginkgo.It("should allow awk and sed as pipe targets", func() {
				cmds := []string{
					"some-command | awk '{print $1}'",
					"some-command | sed 's/foo/bar/'",
				}
				for _, cmd := range cmds {
					ok, patternName, _ := validator.ValidateCommand(cmd)
					gomega.Expect(ok).To(gomega.BeTrue(),
						"Expected '%s' to be allowed as pipe target", cmd)
					gomega.Expect(patternName).To(gomega.BeEmpty())
				}
			})

			ginkgo.It("should allow chained pipe targets", func() {
				cmd := "some-command | grep pattern | head -5"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeTrue(),
					"chained pipe targets should be allowed")
				gomega.Expect(patternName).To(gomega.BeEmpty())
			})

			ginkgo.It("should still block standalone commands (not pipe targets)", func() {
				blockedCommands := []struct {
					cmd         string
					patternName string
				}{
					{"tail file.txt", "head/tail (standalone)"},
					{"grep pattern file", "grep/rg (standalone)"},
					{"cat file", "cat (standalone)"},
					{"head -20 file.txt", "head/tail (standalone)"},
				}
				for _, tc := range blockedCommands {
					ok, patternName, _ := validator.ValidateCommand(tc.cmd)
					gomega.Expect(ok).To(gomega.BeFalse(),
						"Expected standalone '%s' to still be blocked", tc.cmd)
					gomega.Expect(patternName).To(gomega.Equal(tc.patternName))
				}
			})

			ginkgo.It("should still block tools in the first pipe segment", func() {
				cmd := "ls /path | grep pattern"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse(),
					"ls in first segment should still be blocked")
				gomega.Expect(patternName).To(gomega.Equal("ls (standalone)"))
			})

			ginkgo.It("should not confuse || (logical OR) with pipe", func() {
				cmd := "cmd1 || grep pattern"
				ok, patternName, _ := validator.ValidateCommand(cmd)
				gomega.Expect(ok).To(gomega.BeFalse(),
					"grep after || is not a pipe target — it's a standalone command")
				gomega.Expect(patternName).To(gomega.Equal("grep/rg (standalone)"))
			})
		})
	})
})
