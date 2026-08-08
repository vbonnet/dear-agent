package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildStampIsVisibleInLegacyInitialize(t *testing.T) {
	const wantVersion = "engram-mcp-test-version"

	root := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "engram-mcp")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	buildCtx, cancelBuild := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancelBuild()

	build := exec.CommandContext(buildCtx, "go", "build",
		"-ldflags=-X github.com/vbonnet/dear-agent/pkg/version.Version="+wantVersion,
		"-o", binary,
		"./engram/cmd/engram-mcp",
	)
	build.Dir = root
	build.Env = append(os.Environ(), "GOENV=off", "GOFLAGS=", "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build engram-mcp: %v\n%s", err, output)
	}

	serverCtx, cancelServer := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelServer()
	command := exec.CommandContext(serverCtx, binary)
	command.Env = append(os.Environ(), "BEADS_DB=", "ENGRAM_ROOT="+t.TempDir())
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("open engram-mcp stdin: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open engram-mcp stdout: %v", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start engram-mcp: %v", err)
	}
	waited := false
	defer func() {
		if waited {
			return
		}
		_ = stdin.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
	}()

	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]string{
				"name":    "engram-mcp-version-test",
				"version": "1.0.0",
			},
		},
	}
	if err := json.NewEncoder(stdin).Encode(request); err != nil {
		t.Fatalf("write initialize request: %v", err)
	}

	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(stdout).Decode(&response); err != nil {
		_ = stdin.Close()
		waitErr := command.Wait()
		waited = true
		t.Fatalf("read initialize response: %v (wait: %v)\nstderr: %s", err, waitErr, stderr.String())
	}
	if response.Error != nil {
		t.Fatalf("initialize error = %d %q", response.Error.Code, response.Error.Message)
	}
	if response.JSONRPC != "2.0" || response.ID != 1 {
		t.Fatalf("initialize envelope = jsonrpc %q id %d", response.JSONRPC, response.ID)
	}
	if got := response.Result.ProtocolVersion; got != "2025-11-25" {
		t.Errorf("protocol version = %q, want %q", got, "2025-11-25")
	}
	if got := response.Result.ServerInfo.Name; got != "engram" {
		t.Errorf("server name = %q, want %q", got, "engram")
	}
	if got := response.Result.ServerInfo.Version; got != wantVersion {
		t.Errorf("server version = %q, want build stamp %q", got, wantVersion)
	}

	if err := stdin.Close(); err != nil {
		t.Fatalf("close engram-mcp stdin: %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("wait for engram-mcp: %v\nstderr: %s", err, stderr.String())
	}
	waited = true
	if !strings.Contains(stderr.String(), "version="+wantVersion) {
		t.Errorf("startup log does not report build stamp %q:\n%s", wantVersion, stderr.String())
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve engram-mcp test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
