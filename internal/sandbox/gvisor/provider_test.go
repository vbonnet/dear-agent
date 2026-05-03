//go:build linux

package gvisor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	if got := p.Name(); got != "gvisor" {
		t.Errorf("Name() = %q, want %q", got, "gvisor")
	}
}

func TestProvider_ValidateRequest(t *testing.T) {
	p := NewProvider()

	tests := []struct {
		name    string
		req     sandbox.SandboxRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/tmp"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: false,
		},
		{
			name: "missing session ID",
			req: sandbox.SandboxRequest{
				LowerDirs:    []string{"/tmp"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing lower dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing workspace dir",
			req: sandbox.SandboxRequest{
				SessionID: "test",
				LowerDirs: []string{"/tmp"},
			},
			wantErr: true,
		},
		{
			name: "non-existent lower dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/this/does/not/exist"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.validateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProvider_CheckRunscInstalled_Missing(t *testing.T) {
	if _, err := exec.LookPath("runsc"); err == nil {
		t.Skip("runsc is installed; cannot test the missing-binary path")
	}
	p := NewProvider()
	err := p.checkRunscInstalled()
	if err == nil {
		t.Fatal("checkRunscInstalled() should fail when runsc is not on PATH")
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) {
		t.Fatalf("expected *sandbox.Error, got %T", err)
	}
	if sbErr.Code != sandbox.ErrCodeUnsupportedPlatform {
		t.Errorf("expected ErrCodeUnsupportedPlatform, got %v", sbErr.Code)
	}
	if !strings.Contains(sbErr.Message, "runsc") {
		t.Errorf("error message should mention runsc, got %q", sbErr.Message)
	}
}

func TestProvider_Create_FailsWithoutRunsc(t *testing.T) {
	if _, err := exec.LookPath("runsc"); err == nil {
		t.Skip("runsc is installed; cannot test the missing-binary path")
	}

	p := NewProvider()
	tmp := t.TempDir()
	lower := filepath.Join(tmp, "lower")
	if err := os.MkdirAll(lower, 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	req := sandbox.SandboxRequest{
		SessionID:    "no-runsc",
		LowerDirs:    []string{lower},
		WorkspaceDir: filepath.Join(tmp, "ws"),
	}
	sb, err := p.Create(context.Background(), req)
	if err == nil {
		t.Fatal("Create() should fail when runsc is not installed")
	}
	if sb != nil {
		t.Errorf("Create() should return nil sandbox on failure, got %+v", sb)
	}
}

func TestProvider_Destroy_Idempotent(t *testing.T) {
	p := NewProvider()
	if err := p.Destroy(context.Background(), "does-not-exist"); err != nil {
		t.Errorf("Destroy() should be idempotent for unknown id, got %v", err)
	}
}

func TestProvider_Validate_NotFound(t *testing.T) {
	p := NewProvider()
	err := p.Validate(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("Validate() should fail for unknown sandbox id")
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) {
		t.Fatalf("expected *sandbox.Error, got %T", err)
	}
	if sbErr.Code != sandbox.ErrCodeSandboxNotFound {
		t.Errorf("expected ErrCodeSandboxNotFound, got %v", sbErr.Code)
	}
}
