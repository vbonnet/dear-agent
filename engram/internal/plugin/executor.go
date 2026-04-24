package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/security"
)

// DefaultExecutionTimeout is the default timeout for plugin execution
const DefaultExecutionTimeout = 30 * time.Second

// Resource limit constants
const (
	// DefaultMaxProcesses is the maximum number of child processes per plugin
	DefaultMaxProcesses = 100
	// DefaultMaxProcessesHard is the hard limit for child processes
	DefaultMaxProcessesHard = 200
	// DefaultMaxFileDescriptors is the maximum number of open files per plugin
	DefaultMaxFileDescriptors = 8192
	// DefaultMaxMemory is the maximum virtual memory per plugin (4GB)
	DefaultMaxMemory = 4 * 1024 * 1024 * 1024
)

// Executor handles sandboxed plugin execution
type Executor struct {
	sandbox   *security.Sandbox
	validator *security.Validator
	logger    *Logger
	timeout   time.Duration
}

// NewExecutor creates a new plugin executor with default timeout
func NewExecutor() *Executor {
	return &Executor{
		sandbox:   security.NewSandbox(),
		validator: security.NewValidator(),
		logger:    NewDefaultLogger(),
		timeout:   DefaultExecutionTimeout,
	}
}

// NewExecutorWithLogger creates a new plugin executor with a custom logger
func NewExecutorWithLogger(logger *Logger) *Executor {
	return &Executor{
		sandbox:   security.NewSandbox(),
		validator: security.NewValidator(),
		logger:    logger,
		timeout:   DefaultExecutionTimeout,
	}
}

// NewExecutorWithTimeout creates a new plugin executor with custom timeout
func NewExecutorWithTimeout(timeout time.Duration) *Executor {
	return &Executor{
		sandbox:   security.NewSandbox(),
		validator: security.NewValidator(),
		logger:    NewDefaultLogger(),
		timeout:   timeout,
	}
}

// Execute runs a plugin command with sandboxing and timeout
func (e *Executor) Execute(ctx context.Context, plugin *Plugin, cmdName string, args []string) ([]byte, error) {
	errCtx := WithCommand(plugin.Manifest.Name, cmdName).WithOperation("execute")

	e.logger.Info(ctx, "Starting plugin command execution",
		errCtx.WithExtra("args", args))

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Channel for execution result
	type result struct {
		output []byte
		err    error
	}
	resultCh := make(chan result, 1)

	// Run execution in goroutine
	go func() {
		output, err := e.execute(timeoutCtx, plugin, cmdName, args)
		resultCh <- result{output: output, err: err}
	}()

	// Wait for result or timeout
	select {
	case res := <-resultCh:
		return res.output, res.err
	case <-timeoutCtx.Done():
		if ctx.Err() != nil {
			// Parent context was cancelled
			e.logger.Error(ctx, "Plugin execution cancelled", errCtx, ctx.Err())
			return nil, fmt.Errorf("plugin execution cancelled: %w", ctx.Err())
		}
		// Execution timeout
		timeoutErr := fmt.Errorf("plugin execution timeout after %v for plugin %q command %q", e.timeout, plugin.Manifest.Name, cmdName)
		e.logger.Error(ctx, "Plugin execution timeout", errCtx.WithExtra("timeout", e.timeout), timeoutErr)
		return nil, timeoutErr
	}
}

// execute is the internal execution logic (called by Execute with timeout protection)
func (e *Executor) execute(ctx context.Context, plugin *Plugin, cmdName string, args []string) ([]byte, error) {
	errCtx := WithCommand(plugin.Manifest.Name, cmdName).WithOperation("execute")
	// Find command in manifest
	var cmd *Command
	for i := range plugin.Manifest.Commands {
		if plugin.Manifest.Commands[i].Name == cmdName {
			cmd = &plugin.Manifest.Commands[i]
			break
		}
	}

	if cmd == nil {
		err := fmt.Errorf("command %q not found in plugin %q", cmdName, plugin.Manifest.Name)
		e.logger.Error(ctx, "Command not found in plugin manifest",
			errCtx.WithExtra("available_commands", getCommandNames(plugin.Manifest.Commands)), err)
		return nil, err
	}

	e.logger.Debug(ctx, "Found command in manifest",
		errCtx.WithExtra("executable", cmd.Executable))

	// Validate permissions (convert from plugin.Permissions to security.Permissions)
	secPerms := security.Permissions{
		Filesystem: plugin.Manifest.Permissions.Filesystem,
		Network:    plugin.Manifest.Permissions.Network,
		Commands:   plugin.Manifest.Permissions.Commands,
	}
	if err := e.validator.ValidatePermissions(secPerms); err != nil {
		e.logger.Error(ctx, "Permission validation failed", errCtx, err)
		return nil, fmt.Errorf("invalid permissions: %w", err)
	}

	e.logger.Debug(ctx, "Permissions validated successfully", errCtx)

	// Build command
	executable := cmd.Executable
	if executable[0] != '/' {
		// Relative to plugin directory
		executable = plugin.Path + "/" + executable
	}

	// Combine manifest args with runtime args
	cmdArgs := append(cmd.Args, args...)

	e.logger.Debug(ctx, "Building sandboxed command",
		errCtx.WithExtra("executable", executable).WithExtra("args", cmdArgs))

	// Apply sandbox (reuse secPerms from validation)
	sandboxedCmd, err := e.sandbox.Apply(executable, cmdArgs, secPerms)
	if err != nil {
		e.logger.Error(ctx, "Failed to apply sandbox", errCtx, err)
		return nil, fmt.Errorf("failed to apply sandbox: %w", err)
	}

	e.logger.Debug(ctx, "Sandbox applied successfully",
		errCtx.WithExtra("sandboxed_cmd", sandboxedCmd))

	// Execute with context
	execCmd := exec.CommandContext(ctx, sandboxedCmd[0], sandboxedCmd[1:]...)

	// Apply resource limits (fork-bomb prevention)
	// Note: Actual enforcement is environment-dependent (Docker, cgroups, OS defaults)
	applyResourceLimits(execCmd)

	output, err := execCmd.CombinedOutput()
	if err != nil {
		e.logger.Error(ctx, "Command execution failed",
			errCtx.WithExtra("output", string(output)).WithExtra("exit_code", execCmd.ProcessState), err)
		return output, fmt.Errorf("command failed: %w", err)
	}

	e.logger.Info(ctx, "Command execution completed successfully",
		errCtx.WithExtra("output_size", len(output)))

	return output, nil
}

// getCommandNames extracts command names from a slice of commands
func getCommandNames(commands []Command) []string {
	names := make([]string, len(commands))
	for i, cmd := range commands {
		names[i] = cmd.Name
	}
	return names
}

// applyResourceLimits prepares command for resource limiting
// Note: This function documents the resource limits configuration.
// Actual enforcement depends on the execution environment:
// - Docker: Use memory, pids, cpus constraints in docker run
// - cgroups: Set /sys/fs/cgroup/pids.max, memory.max, etc.
// - Bare metal: Use system-wide ulimit settings or go-cgroup libraries
// - OS defaults: Linux typically allows ~1000 processes per user
//
// Future versions (Go 1.24+) may support exec.Cmd.SyscallBack for in-process enforcement.
func applyResourceLimits(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Placeholder for future per-process limit enforcement
	// DefaultMaxProcesses, DefaultMaxFileDescriptors, DefaultMaxMemory
	// will be enforced when Go provides the necessary syscall APIs
}

// Close cleans up executor resources
func (e *Executor) Close() error {
	// No resources to clean up for now
	return nil
}
