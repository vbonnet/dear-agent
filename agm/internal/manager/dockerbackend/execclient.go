package dockerbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExecClient implements ContainerClient by shelling out to the docker CLI.
// This avoids pulling in the Docker SDK as a dependency.
type ExecClient struct{}

// NewExecClient returns a ContainerClient that uses the docker CLI.
func NewExecClient() *ExecClient {
	return &ExecClient{}
}

func (c *ExecClient) CreateContainer(ctx context.Context, opts ContainerCreateOpts) (string, error) {
	args := []string{"create", "--name", opts.Name}

	for k, v := range opts.Labels {
		args = append(args, "--label", k+"="+v)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", k+"="+v)
	}
	for _, m := range opts.Mounts {
		mountStr := m.Source + ":" + m.Target
		if m.ReadOnly {
			mountStr += ":ro"
		}
		args = append(args, "-v", mountStr)
	}

	// Network isolation
	args = append(args, "--network", "none")

	args = append(args, opts.Image)
	args = append(args, opts.Cmd...)

	out, err := runDocker(ctx, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *ExecClient) StartContainer(ctx context.Context, containerID string) error {
	_, err := runDocker(ctx, "start", containerID)
	return err
}

func (c *ExecClient) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 1
	}
	_, err := runDocker(ctx, "stop", "-t", fmt.Sprintf("%d", secs), containerID)
	return err
}

func (c *ExecClient) RemoveContainer(ctx context.Context, containerID string) error {
	_, err := runDocker(ctx, "rm", "-f", containerID)
	return err
}

func (c *ExecClient) InspectContainer(ctx context.Context, containerID string) (ContainerState, error) {
	out, err := runDocker(ctx, "inspect", "--format", "{{json .State}}", containerID)
	if err != nil {
		return ContainerState{}, err
	}

	var raw struct {
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		return ContainerState{}, fmt.Errorf("parse inspect output: %w", err)
	}

	state := ContainerState{
		Running:  raw.Running,
		ExitCode: raw.ExitCode,
	}
	if t, err := time.Parse(time.RFC3339Nano, raw.StartedAt); err == nil {
		state.StartedAt = t
	}
	if t, err := time.Parse(time.RFC3339Nano, raw.FinishedAt); err == nil {
		state.FinishedAt = t
	}
	return state, nil
}

func (c *ExecClient) ListContainers(ctx context.Context, labels map[string]string) ([]ContainerInfo, error) {
	args := []string{"ps", "-a", "--format", "{{json .}}"}
	for k, v := range labels {
		args = append(args, "--filter", "label="+k+"="+v)
	}

	out, err := runDocker(ctx, args...)
	if err != nil {
		return nil, err
	}

	var results []ContainerInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var raw struct {
			ID     string `json:"ID"`
			Names  string `json:"Names"`
			Labels string `json:"Labels"`
			State  string `json:"State"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		results = append(results, ContainerInfo{
			ID:    raw.ID,
			Name:  raw.Names,
			State: raw.State,
		})
	}
	return results, nil
}

func (c *ExecClient) Exec(ctx context.Context, containerID string, cmd []string, stdin string) (string, error) {
	args := []string{"exec"}
	if stdin != "" {
		args = append(args, "-i")
	}
	args = append(args, containerID)
	args = append(args, cmd...)

	dockerCmd := exec.CommandContext(ctx, "docker", args...)
	if stdin != "" {
		dockerCmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	dockerCmd.Stdout = &stdout
	dockerCmd.Stderr = &stderr

	if err := dockerCmd.Run(); err != nil {
		return "", fmt.Errorf("docker exec %v: %w: %s", cmd, err, stderr.String())
	}
	return stdout.String(), nil
}

// runDocker executes a docker CLI command.
func runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, stderr.String())
	}
	return stdout.String(), nil
}
