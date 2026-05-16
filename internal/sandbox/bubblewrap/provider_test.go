package bubblewrap

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// These tests cover the cross-platform, pure-logic surface of the Bubblewrap
// provider. The real bwrap enforcement path is Linux-only (see
// integration_test.go, which t.Skip()s on non-Linux), so the
// security-relevant input gate validateRequest — which decides whether a
// sandbox is even attempted and which host paths get exposed — is the part
// that must be protected by tests that run everywhere.

func TestProvider_Name(t *testing.T) {
	assert.Equal(t, "bubblewrap", NewProvider().Name())
}

// errCode extracts the structured sandbox error code, failing the test if
// err is not a *sandbox.Error.
func errCode(t *testing.T, err error) sandbox.ErrorCode {
	t.Helper()
	var sErr *sandbox.Error
	require.True(t, errors.As(err, &sErr), "expected *sandbox.Error, got %T (%v)", err, err)
	return sErr.Code
}

func TestProvider_validateRequest(t *testing.T) {
	// An existing directory to satisfy the LowerDirs stat check on the
	// happy path and the "workspace empty" case.
	existing := t.TempDir()

	tests := []struct {
		name     string
		req      sandbox.SandboxRequest
		wantErr  bool
		wantCode sandbox.ErrorCode
	}{
		{
			name: "empty session id",
			req: sandbox.SandboxRequest{
				LowerDirs:    []string{existing},
				WorkspaceDir: "/tmp/ws",
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "no lower dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "s1",
				WorkspaceDir: "/tmp/ws",
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "lower dir does not exist",
			req: sandbox.SandboxRequest{
				SessionID:    "s1",
				LowerDirs:    []string{"/definitely/not/a/real/path/xyzzy"},
				WorkspaceDir: "/tmp/ws",
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeRepoNotFound,
		},
		{
			name: "empty workspace dir",
			req: sandbox.SandboxRequest{
				SessionID: "s1",
				LowerDirs: []string{existing},
			},
			wantErr:  true,
			wantCode: sandbox.ErrCodeInvalidConfig,
		},
		{
			name: "valid request",
			req: sandbox.SandboxRequest{
				SessionID:    "s1",
				LowerDirs:    []string{existing},
				WorkspaceDir: "/tmp/ws",
			},
			wantErr: false,
		},
	}

	p := NewProvider()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.validateRequest(tt.req)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, errCode(t, err))
		})
	}
}

func TestProvider_validateRequest_multipleLowerDirs_oneMissing(t *testing.T) {
	// Every LowerDir must exist; a single missing entry must be rejected so
	// a sandbox is never created with a silently-dropped mount.
	p := NewProvider()
	err := p.validateRequest(sandbox.SandboxRequest{
		SessionID:    "s1",
		LowerDirs:    []string{t.TempDir(), "/definitely/not/real/abc"},
		WorkspaceDir: "/tmp/ws",
	})
	require.Error(t, err)
	assert.Equal(t, sandbox.ErrCodeRepoNotFound, errCode(t, err))
}

func TestProvider_checkBubblewrapInstalled(t *testing.T) {
	err := NewProvider().checkBubblewrapInstalled()
	if _, lookErr := exec.LookPath("bwrap"); lookErr != nil {
		// bwrap absent (the usual case on darwin): must report the
		// structured unsupported-platform error, not a nil/opaque one.
		require.Error(t, err)
		assert.Equal(t, sandbox.ErrCodeUnsupportedPlatform, errCode(t, err))
	} else {
		assert.NoError(t, err)
	}
}
