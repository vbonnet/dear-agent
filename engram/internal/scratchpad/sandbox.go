// Package scratchpad provides scratchpad-related functionality.
package scratchpad

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	mountProbeFilename         = ".engram-mount-probe"
	mountProbeContainerPath    = "/workspace/" + mountProbeFilename
	mountProbePayloadPrefix    = "engram-scratchpad-bind-visible:"
	mountProbeNonceBytes       = 32
	mountProbeCommandInput     = "engram-scratchpad-probe-command-ready\n"
	sandboxContainerPrefix     = "engram-scratchpad-"
	sandboxContainerNonceBytes = 32
	creationCleanupTimeout     = 10 * time.Second
)

var (
	errBindMountUnavailable = errors.New("scratchpad bind source is not visible inside container")
	errSandboxCleanup       = errors.New("scratchpad cleanup failed")
)

// Sandbox provides isolated code execution for agent probe scripts.
// Uses Docker containers for security isolation (no network, no host filesystem writes).
type Sandbox struct {
	containerID   string
	workdir       string
	mu            sync.Mutex
	maxExecutions int
	execCount     int
}

// ExecuteRequest represents a code execution request.
type ExecuteRequest struct {
	Language string            `json:"language"`      // "python", "bash", "node"
	Code     string            `json:"code"`          // Code to execute
	Timeout  time.Duration     `json:"timeout"`       // Max execution time
	Env      map[string]string `json:"env,omitempty"` // Environment variables
}

// ExecuteResponse contains execution results.
type ExecuteResponse struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// SandboxConfig defines sandbox creation parameters.
type SandboxConfig struct {
	Image         string        // Docker image (e.g., "python:3.11-slim")
	WorkDir       string        // Working directory inside container
	MaxExecutions int           // Max probe executions before cleanup
	CPULimit      string        // CPU limit (e.g., "1.0")
	MemoryLimit   string        // Memory limit (e.g., "512m")
	Timeout       time.Duration // Default execution timeout
}

// NewSandbox creates a new isolated sandbox environment.
func NewSandbox(ctx context.Context, config *SandboxConfig) (*Sandbox, error) {
	probePayload, err := newMountProbePayload()
	if err != nil {
		return nil, fmt.Errorf("failed to generate bind visibility probe: %w", err)
	}
	containerName, err := newSandboxContainerName()
	if err != nil {
		return nil, fmt.Errorf("failed to generate sandbox container identity: %w", err)
	}

	// Create temporary directory for code files
	workdir, err := os.MkdirTemp("", "engram-scratchpad-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create workdir: %w", err)
	}
	probePath := filepath.Join(workdir, mountProbeFilename)
	if err := os.WriteFile(probePath, probePayload, 0o600); err != nil {
		cleanupErr := cleanupSandboxResources(ctx, "", workdir)
		return nil, errors.Join(fmt.Errorf("failed to create bind visibility probe: %w", err), cleanupErr)
	}

	// Launch Docker container with security restrictions
	containerID, err := launchContainer(ctx, config, workdir, containerName)
	if err != nil {
		// docker run can be interrupted after daemon-side creation but before
		// stdout returns the ID. The preassigned random name keeps that
		// ambiguous container addressable for bounded rollback.
		cleanupErr := rollbackSandboxCreation(ctx, containerName, workdir)
		return nil, errors.Join(fmt.Errorf("failed to launch container: %w", err), cleanupErr)
	}

	if err := verifyBindMount(ctx, containerID, probePayload); err != nil {
		cleanupErr := rollbackSandboxCreation(ctx, containerID, workdir)
		return nil, errors.Join(fmt.Errorf("failed to verify bind source %q: %w", workdir, err), cleanupErr)
	}
	if err := os.Remove(probePath); err != nil {
		cleanupErr := rollbackSandboxCreation(ctx, containerID, workdir)
		return nil, errors.Join(fmt.Errorf("failed to remove bind visibility probe: %w", err), cleanupErr)
	}

	return &Sandbox{
		containerID:   containerID,
		workdir:       workdir,
		maxExecutions: config.MaxExecutions,
		execCount:     0,
	}, nil
}

// Execute runs code in the sandbox and returns results.
func (s *Sandbox) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check execution limit
	if s.execCount >= s.maxExecutions {
		return nil, fmt.Errorf("sandbox execution limit reached (%d/%d)", s.execCount, s.maxExecutions)
	}

	// Write code to temporary file
	codeFile, err := s.writeCodeFile(req)
	if err != nil {
		return nil, fmt.Errorf("failed to write code file: %w", err)
	}
	defer os.Remove(codeFile)

	// Execute with timeout
	start := time.Now()
	stdout, stderr, exitCode, execErr := s.executeInContainer(ctx, req, codeFile)
	duration := time.Since(start)

	s.execCount++

	response := &ExecuteResponse{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: duration,
	}

	if execErr != nil {
		response.Error = execErr.Error()
		return response, execErr
	}

	return response, nil
}

// Cleanup destroys the sandbox and cleans up resources.
func (s *Sandbox) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return cleanupSandboxResources(ctx, s.containerID, s.workdir)
}

// verifyBindMount proves that the Docker daemon observes the exact client-side
// workdir contents before the sandbox is published to callers. A successful
// docker run is insufficient for VM or remote daemons: they may resolve the
// bind source to a different, empty directory.
func verifyBindMount(ctx context.Context, containerID string, probePayload []byte) error {
	// Use container UID 0 for this infrastructure check so image USER and source
	// interpreter permissions do not masquerade as host bind visibility.
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", "-u", "0", containerID, "cat", "-", mountProbeContainerPath)
	cmd.Stdin = strings.NewReader(mountProbeCommandInput)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("bind visibility probe interrupted: %w", ctxErr)
		}
		detail := strings.TrimSpace(stderr.String())
		if !bytes.HasPrefix(stdout.Bytes(), []byte(mountProbeCommandInput)) {
			if detail == "" {
				return fmt.Errorf("bind visibility probe command failed before reading its input: %w", err)
			}
			return fmt.Errorf("bind visibility probe command failed before reading its input: %w: %s", err, detail)
		}
		if detail == "" {
			return fmt.Errorf("%w: mount probe failed: %w", errBindMountUnavailable, err)
		}
		return fmt.Errorf("%w: mount probe failed: %w: %s", errBindMountUnavailable, err, detail)
	}
	expectedOutput := append([]byte(mountProbeCommandInput), probePayload...)
	if !bytes.Equal(stdout.Bytes(), expectedOutput) {
		return fmt.Errorf("%w: mount probe bytes differed", errBindMountUnavailable)
	}
	return nil
}

func newMountProbePayload() ([]byte, error) {
	nonce := make([]byte, mountProbeNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	payload := make([]byte, 0, len(mountProbePayloadPrefix)+hex.EncodedLen(len(nonce))+1)
	payload = append(payload, mountProbePayloadPrefix...)
	payload = hex.AppendEncode(payload, nonce)
	payload = append(payload, '\n')
	return payload, nil
}

func newSandboxContainerName() (string, error) {
	nonce := make([]byte, sandboxContainerNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return sandboxContainerPrefix + hex.EncodeToString(nonce), nil
}

func rollbackSandboxCreation(ctx context.Context, containerID, workdir string) error {
	return rollbackSandboxCreationWithin(ctx, containerID, workdir, creationCleanupTimeout)
}

func rollbackSandboxCreationWithin(ctx context.Context, containerID, workdir string, cleanupTimeout time.Duration) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	return cleanupSandboxResources(cleanupCtx, containerID, workdir)
}

// cleanupSandboxResources attempts both resource classes so one cleanup
// failure never hides or prevents the other.
func cleanupSandboxResources(ctx context.Context, containerID, workdir string) error {
	var cleanupErrs []error
	if containerID != "" {
		cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
		if err := cmd.Run(); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = errors.Join(err, ctxErr)
			}
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to remove container: %w", err))
		}
	}
	if workdir != "" {
		if err := os.RemoveAll(workdir); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to remove workdir: %w", err))
		}
	}
	cleanupErr := errors.Join(cleanupErrs...)
	if cleanupErr == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errSandboxCleanup, cleanupErr)
}

// launchContainer creates and starts a Docker container with security restrictions.
func launchContainer(ctx context.Context, config *SandboxConfig, workdir, containerName string) (string, error) {
	args := []string{
		"run",
		"-d",   // Detached mode
		"--rm", // Auto-remove on exit
		"--name", containerName,
		"--network", "none", // No network access
		"--read-only",                                // Read-only root filesystem
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=100m", // Writable /tmp (limited)
		"--cpu-quota", "100000", // CPU limit (100% of 1 core)
		"--memory", config.MemoryLimit,
		"--pids-limit", "100", // Process limit
		"-v", fmt.Sprintf("%s:/workspace:ro", workdir), // Mount workdir read-only
		"-w", "/workspace",
		config.Image,
		"sleep", "3600", // Keep container alive
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = errors.Join(err, ctxErr)
		}
		return "", fmt.Errorf("docker run failed: %w", err)
	}

	containerID := strings.TrimSpace(string(output))
	return containerID, nil
}

// writeCodeFile writes code to a temporary file in the workdir.
func (s *Sandbox) writeCodeFile(req *ExecuteRequest) (string, error) {
	ext := getFileExtension(req.Language)
	codeFile := filepath.Join(s.workdir, fmt.Sprintf("probe%s", ext))

	if err := os.WriteFile(codeFile, []byte(req.Code), 0o600); err != nil {
		return "", err
	}

	return codeFile, nil
}

// executeInContainer runs code in the Docker container with timeout.
func (s *Sandbox) executeInContainer(ctx context.Context, req *ExecuteRequest, codeFile string) (stdout string, stderr string, exitCode int, err error) {
	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Build command args based on language
	var cmdArgs []string
	switch req.Language {
	case "python":
		cmdArgs = []string{"exec", s.containerID, "python3", "/workspace/" + filepath.Base(codeFile)}
	case "bash":
		cmdArgs = []string{"exec", s.containerID, "bash", "/workspace/" + filepath.Base(codeFile)}
	case "node":
		cmdArgs = []string{"exec", s.containerID, "node", "/workspace/" + filepath.Base(codeFile)}
	default:
		return "", "", -1, fmt.Errorf("unsupported language: %s", req.Language)
	}

	cmd := exec.CommandContext(timeoutCtx, "docker", cmdArgs...)

	// Set environment variables
	if len(req.Env) > 0 {
		var envVars []string
		for k, v := range req.Env {
			envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = envVars
	}

	// Capture stdout and stderr
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Run command
	err = cmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// Timeout or other error
			exitCode = -1
			return stdout, stderr, exitCode, err
		}
	}

	return stdout, stderr, exitCode, nil
}

// getFileExtension returns the file extension for a given language.
func getFileExtension(language string) string {
	switch language {
	case "python":
		return ".py"
	case "bash":
		return ".sh"
	case "node":
		return ".js"
	default:
		return ".txt"
	}
}
