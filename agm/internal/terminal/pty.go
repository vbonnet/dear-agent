// Package terminal provides terminal functionality.
package terminal

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// PTYProvider abstracts PTY allocation for testing
type PTYProvider interface {
	StartWithPTY(cmd *exec.Cmd) error
	Close() error
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
}

// RealPTY wraps creack/pty
type RealPTY struct {
	ptmx *os.File
}

// NewRealPTY creates a new real PTY provider
func NewRealPTY() *RealPTY {
	return &RealPTY{}
}

// StartWithPTY starts a command with a PTY attached
func (r *RealPTY) StartWithPTY(cmd *exec.Cmd) error {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	r.ptmx = ptmx
	return nil
}

// Close closes the PTY
func (r *RealPTY) Close() error {
	if r.ptmx != nil {
		return r.ptmx.Close()
	}
	return nil
}

// Read reads from the PTY
func (r *RealPTY) Read(p []byte) (n int, err error) {
	if r.ptmx == nil {
		return 0, io.EOF
	}
	return r.ptmx.Read(p)
}

// Write writes to the PTY
func (r *RealPTY) Write(p []byte) (n int, err error) {
	if r.ptmx == nil {
		return 0, io.EOF
	}
	return r.ptmx.Write(p)
}

// MockPTY for testing
type MockPTY struct {
	StartErr error
	CloseErr error
	ReadData []byte
	ReadErr  error
	WriteErr error
	Written  []byte
}

// NewMockPTY creates a new mock PTY provider
func NewMockPTY() *MockPTY {
	return &MockPTY{}
}

// StartWithPTY mocks starting a command with PTY
func (m *MockPTY) StartWithPTY(cmd *exec.Cmd) error {
	if m.StartErr != nil {
		return m.StartErr
	}
	// Start the command normally for testing purposes
	return cmd.Start()
}

// Close mocks closing the PTY
func (m *MockPTY) Close() error {
	return m.CloseErr
}

// Read mocks reading from the PTY
func (m *MockPTY) Read(p []byte) (n int, err error) {
	if m.ReadErr != nil {
		return 0, m.ReadErr
	}
	n = copy(p, m.ReadData)
	if n < len(m.ReadData) {
		m.ReadData = m.ReadData[n:]
	} else {
		m.ReadData = nil
	}
	return n, nil
}

// Write mocks writing to the PTY
func (m *MockPTY) Write(p []byte) (n int, err error) {
	if m.WriteErr != nil {
		return 0, m.WriteErr
	}
	m.Written = append(m.Written, p...)
	return len(p), nil
}
