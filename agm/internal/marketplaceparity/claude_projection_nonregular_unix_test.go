//go:build darwin || linux

package marketplaceparity

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestProjectionRejectsFIFOAndSocketPromptly(t *testing.T) {
	for _, target := range projectionTestAuthorityTargets() {
		t.Run(target.name+"/fifo", func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			path := target.path(fixture)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := unix.Mkfifo(path, 0o600); err != nil {
				t.Skipf("FIFOs unavailable: %v", err)
			}
			projectionTestRequirePromptError(t, target.name+" FIFO", func() error {
				return target.validate(fixture.root)
			}, func() {
				fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK, 0)
				if err == nil {
					_ = unix.Close(fd)
				}
			})
		})

		t.Run(target.name+"/socket", func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			path := target.path(fixture)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Skipf("Unix sockets unavailable: %v", err)
			}
			defer listener.Close()
			projectionTestRequirePromptError(t, target.name+" socket", func() error {
				return target.validate(fixture.root)
			}, nil)
		})
	}
}

func projectionTestRequirePromptError(t *testing.T, operation string, validate func() error, unblock func()) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		result <- validate()
	}()

	timer := time.NewTimer(1500 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-result:
		if err == nil {
			t.Fatalf("%s accepted a nonregular path", operation)
		}
	case <-timer.C:
		if unblock != nil {
			unblock()
		}
		select {
		case <-result:
		case <-time.After(time.Second):
		}
		t.Fatalf("%s did not reject within 1.5s", operation)
	}
}

func TestProjectionRejectsSocketNestedBelowSource(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	path := filepath.Join(fixture.root, "spec-governance", "skills", "audit-specs", "references", "socket.md")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	defer listener.Close()
	projectionTestRequirePromptError(t, "nested exported socket", func() error {
		return ValidateCatalog(fixture.root)
	}, nil)
}
