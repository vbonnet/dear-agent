# ADR-004: Shell-Based Command Execution

## Status

Accepted

## Context

The `commands_must_succeed` feature requires executing arbitrary commands specified in DoD files. There are several approaches to command execution:

1. **Direct Execution**: Parse command string, execute binary directly
2. **Shell Execution**: Execute command via shell (sh -c)
3. **Restricted Execution**: Only allow whitelisted commands
4. **Scripting Language**: Embed scripting engine (e.g., Lua, JavaScript)

We need to decide how to execute commands safely and flexibly.

## Decision

We will execute commands via **shell execution** using `sh -c`.

**Implementation**:
```go
cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
output, err := cmd.CombinedOutput()
```

**Rationale**:
- Maximum flexibility for DoD authors
- Natural syntax for command composition (pipes, redirects)
- Consistent with CI/CD environments
- Leverages existing shell knowledge

**Trust Model**: DoD files are trusted sources (version-controlled, code-reviewed), not user input.

## Consequences

### Positive

1. **Full Shell Features**: Commands can use pipes, redirects, variables
   ```yaml
   commands_must_succeed:
     - cmd: "cat file.txt | grep pattern | wc -l"
       exit_code: 1
     - cmd: "test -f output.txt && test -s output.txt"
       exit_code: 0
     - cmd: "export VAR=value && ./script.sh"
       exit_code: 0
   ```

2. **Familiar Syntax**: No need to learn new command syntax
   - Shell commands are universal knowledge
   - Copy-paste from terminal works directly
   - Examples from documentation transfer directly

3. **Composability**: Can combine multiple operations
   - Chaining with `&&` and `||`
   - Process substitution
   - Command grouping with `{}`

4. **Environment Parity**: Same behavior as CI/CD and manual execution
   - GitHub Actions uses shell execution
   - GitLab CI uses shell execution
   - Local testing matches CI environment

5. **Flexibility**: Can call any installed tool
   - No whitelist maintenance
   - Support for project-specific tools (make, custom scripts)
   - Easy to adapt to new requirements

### Negative

1. **Security Risk: Shell Injection**
   - If DoD files contained user input, could execute arbitrary code
   - Example: `cmd: "rm -rf $USER_INPUT"`
   - **Mitigation**: DoD files are trusted (version-controlled)

2. **Platform Dependency**: Assumes Unix shell
   - `sh -c` works on Linux, macOS
   - Fails on Windows (unless using WSL, Git Bash, etc.)
   - Windows users need shell emulation

3. **Parsing Complexity**: Shell parsing is complex
   - Cannot easily extract individual commands
   - Cannot validate command safety before execution
   - Cannot dry-run or simulate execution

4. **Error Messages**: Shell errors can be cryptic
   - "command not found" doesn't indicate which command
   - Exit codes vary by command (no standard)
   - Output may be unclear without context

5. **Resource Exhaustion**: No built-in resource limits
   - Commands can spawn many processes (fork bombs)
   - Can consume unlimited memory
   - Can write unlimited disk
   - **Mitigation**: 30-second timeout prevents infinite loops

## Alternatives Considered

### 1. Direct Execution (No Shell)

**Description**: Parse command into binary + args, execute directly

```go
// Parse "go test -v ./..." into:
cmd := exec.CommandContext(ctx, "go", "test", "-v", "./...")
```

**Rejected because**:
- Requires complex argument parsing (quoting, escaping)
- No support for pipes, redirects, or variable expansion
- Breaks common command patterns (e.g., `find . -name "*.go" | xargs wc -l`)
- Less flexible than shell execution

**Example limitation**:
```yaml
# Cannot support this without shell
commands_must_succeed:
  - cmd: "cat *.txt | grep -v '^#' | sort"
    exit_code: 0
```

### 2. Restricted Command Whitelist

**Description**: Only allow pre-approved commands

```go
allowedCommands := map[string]bool{
    "go":   true,
    "make": true,
    "git":  true,
}

binary := parseCommand(cmdStr)
if !allowedCommands[binary] {
    return fmt.Errorf("command not allowed: %s", binary)
}
```

**Rejected because**:
- Limits flexibility (cannot use custom scripts)
- Whitelist maintenance burden
- Different projects need different tools
- Cannot support composed commands (pipes)

### 3. Scripting Language Execution

**Description**: Embed Lua/JavaScript/etc. for command execution

```go
import "github.com/yuin/gopher-lua"

L := lua.NewState()
defer L.Close()
L.DoString(cmdStr)
```

**Rejected because**:
- Adds significant dependency
- Different syntax from shell (learning curve)
- Less familiar to DevOps users
- Overkill for simple command validation

### 4. Docker-Based Execution

**Description**: Run commands in isolated containers

```yaml
commands_must_succeed:
  - cmd: "make test"
    image: "golang:1.20"
    exit_code: 0
```

**Rejected because**:
- Requires Docker installation
- Adds significant complexity
- Slower execution (container startup)
- Overkill for local validation

### 5. Command Plugins

**Description**: Extensible plugin system for custom validators

```go
type Validator interface {
    Validate(args []string) error
}

registry.Register("go-test", &GoTestValidator{})
```

**Rejected because**:
- Much more complex architecture
- Requires plugin development for each tool
- Harder to author DoD files
- Doesn't support arbitrary tools

## Security Considerations

### Threat Model

**Trusted Inputs**: DoD files are treated as trusted code
- Stored in version control
- Reviewed via pull requests
- Modified only by authorized developers

**Not Suitable for Untrusted Inputs**: Never execute DoD from:
- User uploads
- External API responses
- Unverified sources

### Attack Vectors

1. **Malicious DoD File**:
   ```yaml
   commands_must_succeed:
     - cmd: "rm -rf / --no-preserve-root"
       exit_code: 0
   ```
   **Mitigation**: Code review, trusted sources only

2. **Environment Variable Injection**:
   ```yaml
   commands_must_succeed:
     - cmd: "echo $SECRET_KEY > /tmp/leak.txt"
       exit_code: 0
   ```
   **Mitigation**: Review DoD files, restrict environment variables

3. **Resource Exhaustion**:
   ```yaml
   commands_must_succeed:
     - cmd: ":(){ :|:& };:"  # Fork bomb
       exit_code: 0
   ```
   **Mitigation**: 30-second timeout, trusted sources

### Security Best Practices

1. **DoD File Review**: Treat DoD files as code (require PR reviews)
2. **Least Privilege**: Run validation with minimal permissions
3. **Timeout Protection**: 30-second timeout prevents runaway processes
4. **Audit Logging**: Log all command executions for security monitoring
5. **Sandboxing** (future): Consider containerized execution for high-security environments

## Platform Compatibility

### Linux/macOS

**Works**: `sh -c` is standard
```yaml
commands_must_succeed:
  - cmd: "test -f file.txt"
    exit_code: 0
```

### Windows

**Problem**: No `sh` by default

**Workarounds**:
1. **WSL (Windows Subsystem for Linux)**: Run in WSL environment
2. **Git Bash**: Use Git for Windows shell
3. **Cygwin**: Unix-like environment on Windows
4. **Future**: Detect OS and use `cmd /c` on Windows

**Future Enhancement**:
```go
func getShell() (string, string) {
    if runtime.GOOS == "windows" {
        return "cmd", "/c"
    }
    return "sh", "-c"
}

cmd := exec.CommandContext(ctx, getShell(), cmdStr)
```

## Implementation Notes

**Current Implementation**:
```go
func executeCommand(cmdStr string, timeout time.Duration) (int, string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
    output, err := cmd.CombinedOutput()

    if ctx.Err() == context.DeadlineExceeded {
        return -1, string(output), fmt.Errorf("command timed out after %v", timeout)
    }

    exitCode := 0
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            exitCode = exitErr.ExitCode()
        } else {
            return -1, string(output), fmt.Errorf("command execution error: %w", err)
        }
    }

    return exitCode, string(output), nil
}
```

**Key Features**:
- Context-based timeout (30 seconds)
- Combined output (stdout + stderr)
- Exit code extraction
- Timeout detection and reporting

## Design Philosophy

**Trust the User**: DoD authors are developers, not adversaries.

**Flexibility Over Safety**: Shell execution enables powerful validations.

**Defense in Depth**: Timeouts + trusted sources + code review.

**Pragmatism Over Purity**: Shell is imperfect but practical.

## Future Enhancements

### 1. Platform-Specific Shells

```go
func getShellCommand(cmdStr string) *exec.Cmd {
    if runtime.GOOS == "windows" {
        return exec.Command("cmd", "/c", cmdStr)
    }
    return exec.Command("sh", "-c", cmdStr)
}
```

### 2. Sandboxed Execution

```yaml
commands_must_succeed:
  - cmd: "make test"
    exit_code: 0
    sandbox:
      readonly_paths: ["/etc", "/usr"]
      tmpfs: true
      network: false
```

### 3. Dry Run Mode

```go
func (d *BeadDoD) ValidateDryRun() []string {
    // Return commands that would be executed without running them
}
```

### 4. Command Audit Log

```go
type CommandAuditLog struct {
    Timestamp time.Time
    Command   string
    ExitCode  int
    Duration  time.Duration
    User      string
}
```

## References

- [OWASP Command Injection](https://owasp.org/www-community/attacks/Command_Injection)
- [Go exec Package](https://pkg.go.dev/os/exec)
- [Bash Reference Manual](https://www.gnu.org/software/bash/manual/)
- [GitHub Actions: Run Shell Commands](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#jobsjob_idstepsrun)
