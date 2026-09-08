//go:build darwin || linux

package steps

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDefaultBuildAuthorityWaitOwnershipLimitsReturnsFreshValue(t *testing.T) {
	mutated := defaultBuildAuthorityWaitOwnershipLimits()
	mutated.maxEntries = 1
	mutated.maxDirectories = 1
	mutated.maxFiles = 1
	mutated.maxFileBytes = 1
	mutated.maxTotalBytes = 1

	got := defaultBuildAuthorityWaitOwnershipLimits()
	want := buildAuthorityWaitOwnershipLimits{
		maxEntries:     32768,
		maxDirectories: 8192,
		maxFiles:       4096,
		maxFileBytes:   1 << 20,
		maxTotalBytes:  32 << 20,
	}
	if got != want {
		t.Fatalf("default limits after caller mutation = %+v, want %+v", got, want)
	}
}

func TestScanBuildAuthorityWaitOwnershipAllowsReviewedExactPIDSeams(t *testing.T) {
	root := t.TempDir()
	writeBuildAuthorityWaitOwnershipFixture(t, root, "internal/specguard/process_wait_linux.go", `package specguard

import "golang.org/x/sys/unix"

func waitForGitCommandExitWithoutReaping(pid int) error {
	return unix.Waitid(unix.P_PID, pid, nil, 0, nil)
}
`)
	writeBuildAuthorityWaitOwnershipFixture(t, root, "internal/specguard/process_wait_darwin.go", `package specguard

import "syscall"

const reviewedPIDType = 1

func waitForGitCommandExitWithoutReaping(pid int) error {
	_, _, errno := (syscall.Syscall9)(syscall.SYS_WAITID, reviewedPIDType, uintptr(pid), 0, 0, 0, 0, 0, 0, 0)
	return errno
}
`)
	writeBuildAuthorityWaitOwnershipFixture(t, root, "internal/specguard/git_exec.go", `package specguard

import "os/exec"

func observeUnrelated(*exec.Cmd) {}

func runGitCommand(command, unrelated *exec.Cmd) error {
	observeUnrelated(unrelated)
	return waitForGitCommandExitWithoutReaping((command.Process).Pid)
}
`)
	writeBuildAuthorityWaitOwnershipFixture(t, root, "safe.go", `package fixture

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

const safeTrap = unix.SYS_PRLIMIT64

func safe(ch chan<- os.Signal) {
	termination := syscall.SIGTERM
	signal.Notify(ch, termination)
	_, _, _ = syscall.RawSyscall6(safeTrap, 0, 0, 0, 0, 0, 0)
}

func stopSafeSignalSubscription() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	signal.Stop(signals)
}

func waitForOwnedCommand() error {
	command := exec.Command("true")
	if err := command.Start(); err != nil {
		return err
	}
	return command.Wait()
}

type localWaiter struct{}

func (*localWaiter) Wait() error { return nil }

func freshLocal[T any]() *T { return new(T) }

func waitForGenericLocalValue() error {
	value := freshLocal[localWaiter]()
	return value.Wait()
}

func inspectForeignProcessWithoutReaping(pid int) {
	process, err := os.FindProcess(pid)
	if err == nil {
		_ = process.Signal(syscall.Signal(0))
		_ = process.Release()
	}
}
`)
	writeBuildAuthorityWaitOwnershipFixture(t, root, "shadowed.go", `package fixture

type fakeSignal struct{}
type fakeUnix struct{}

func (fakeSignal) Notify(any, ...any) {}
func (fakeUnix) Waitid(...any) error { return nil }

func localNamesAreNotImports(signal fakeSignal, unix fakeUnix) {
	signal.Notify(nil)
	_ = unix.Waitid(0)
}
`)

	result, err := scanBuildAuthorityWaitOwnership(
		context.Background(), root, defaultBuildAuthorityWaitOwnershipLimits(),
	)
	if err != nil {
		t.Fatalf("scan reviewed exact-PID seams: %v", err)
	}
	if len(result.violations) != 0 {
		t.Fatalf("reviewed exact-PID seams produced violations: %v", result.violations)
	}
	if result.exactPIDWaits != 2 {
		t.Fatalf("reviewed exact-PID wait evidence = %d, want 2", result.exactPIDWaits)
	}
}

func TestScanBuildAuthorityWaitOwnershipRepository(t *testing.T) {
	result, err := scanBuildAuthorityWaitOwnership(
		context.Background(), packageSpecBDDRepoRoot(), defaultBuildAuthorityWaitOwnershipLimits(),
	)
	if err != nil {
		t.Fatalf("scan repository wait ownership: %v", err)
	}
	if len(result.violations) != 0 {
		t.Fatalf("repository wait-ownership violations:\n%s", strings.Join(result.violations, "\n"))
	}
	wantSites := []string{
		"agm/test/bdd/steps/spec_governance_process_wait_darwin.go:waitForSpecAuditCommandExitWithoutReaping",
		"agm/test/bdd/steps/spec_governance_process_wait_linux.go:waitForSpecAuditCommandExitWithoutReaping",
		"internal/specguard/process_wait_darwin.go:waitForGitCommandExitWithoutReaping",
		"internal/specguard/process_wait_linux.go:waitForGitCommandExitWithoutReaping",
	}
	gotSites := slices.Clone(result.exactPIDSites)
	slices.Sort(gotSites)
	if !slices.Equal(gotSites, wantSites) {
		t.Fatalf("repository exact-PID sites = %v, want %v", gotSites, wantSites)
	}
}

func TestScanBuildAuthorityWaitOwnershipExcludesOnlyExactOwnerFiles(t *testing.T) {
	t.Run("exact owner", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "internal/buildauthority/process_darwin.go", `package buildauthority

import (
	"os/signal"
	"syscall"
)

func ownDisposition() { signal.Ignore(syscall.SIGCHLD) }
`)
		writeBuildAuthorityWaitOwnershipFixture(t, root, "internal/buildauthority/native.c", "void owned(void) {}\n")

		result, err := scanBuildAuthorityWaitOwnership(
			context.Background(), root, defaultBuildAuthorityWaitOwnershipLimits(),
		)
		if err != nil {
			t.Fatalf("scan exact owner: %v", err)
		}
		if result.files != 0 || len(result.violations) != 0 {
			t.Fatalf("exact owner files were scanned: files=%d violations=%v", result.files, result.violations)
		}
	})

	t.Run("owner subpackage", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "internal/buildauthority/subpackage/process.go", `package subpackage

import (
	"os/signal"
	"syscall"
)

func stealDisposition() { signal.Ignore(syscall.SIGCHLD) }
`)

		result := scanBuildAuthorityWaitOwnershipFixture(t, root)
		assertBuildAuthorityViolation(t, result, "SIGCHLD")
	})
}

func TestScanBuildAuthorityWaitOwnershipRejectsBypasses(t *testing.T) {
	tests := []struct {
		name string
		path string
		src  string
		want string
	}{
		{
			name: "P_PID with foreign target",
			path: "forbidden.go",
			src: `package fixture
import "golang.org/x/sys/unix"
func reap(pid int) { _ = unix.Waitid(unix.P_PID, pid, nil, 0, nil) }
`,
			want: "outside an explicitly reviewed exact-PID wait seam",
		},
		{
			name: "arbitrary Pid selector",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
type foreign struct{ Pid int }
func reap(child foreign) { _, _ = syscall.Wait4(child.Pid, nil, 0, nil) }
`,
			want: "child-reaping API",
		},
		{
			name: "positive literal",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func reap() { _, _ = syscall.Wait4(123, nil, 0, nil) }
`,
			want: "child-reaping API",
		},
		{
			name: "parenthesized wait",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func reap(pid int) { _, _ = (syscall.Wait4)(pid, nil, 0, nil) }
`,
			want: "child-reaping API",
		},
		{
			name: "wait function alias",
			path: "forbidden.go",
			src: `package fixture
import "golang.org/x/sys/unix"
func reap() { wait := unix.Waitid; _ = wait(unix.P_ALL, 0, nil, 0, nil) }
`,
			want: "function value",
		},
		{
			name: "os FindProcess Wait",
			path: "forbidden.go",
			src: `package fixture
import "os"
func reap(pid int) error {
	process, _ := os.FindProcess(pid)
	_, err := process.Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "os FindProcess Wait function value",
			path: "forbidden.go",
			src: `package fixture
import "os"
func reap(pid int) error {
	process, _ := os.FindProcess(pid)
	wait := process.Wait
	_, err := wait()
	return err
}
`,
			want: "process Wait method as a function value",
		},
		{
			name: "os FindProcess hidden behind local helper",
			path: "forbidden.go",
			src: `package fixture
import "os"
func locate(pid int) *os.Process {
	process, _ := os.FindProcess(pid)
	return process
}
func reap(pid int) error {
	process := locate(pid)
	_, err := process.Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "os FindProcess result stored through opaque container",
			path: "forbidden.go",
			src: `package fixture
import "os"
type holder struct { process *os.Process }
func retain(pid int) holder {
	process, _ := os.FindProcess(pid)
	return holder{process: process}
}
`,
			want: "opaque path that could enable foreign-PID reaping",
		},
		{
			name: "opaque exec Cmd factory Wait",
			path: "forbidden.go",
			src: `package fixture
import "os/exec"
func reap(factory func() *exec.Cmd) error { return factory().Wait() }
`,
			want: "dynamically replaced process Wait receiver",
		},
		{
			name: "generic os Process factory",
			path: "forbidden.go",
			src: `package fixture
import "os"
func fresh[T any]() *T { return new(T) }
func reap() error {
	process := fresh[os.Process]()
	process.Pid = -1
	_, err := process.Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "generic os Process factory alias chain",
			path: "forbidden.go",
			src: `package fixture
import "os"
func fresh[T any]() *T { return new(T) }
func reap() error {
	factory := fresh[os.Process]
	alias := factory
	process := alias()
	process.Pid = -1
	_, err := process.Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "generic os Process method factory",
			path: "forbidden.go",
			src: `package fixture
import "os"
type factory[T any] struct{}
func (factory[T]) fresh() *T { return new(T) }
func reap() error {
	maker := factory[os.Process]{}
	process := maker.fresh()
	process.Pid = -1
	_, err := process.Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "generic os Process multi-argument factory",
			path: "forbidden.go",
			src: `package fixture
import "os"
func fresh[Marker, Value any]() *Value { return new(Value) }
func reap() error {
	process := fresh[int, os.Process]()
	process.Pid = -1
	_, err := process.Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "generic exec Cmd factory",
			path: "forbidden.go",
			src: `package fixture
import "os/exec"
func fresh[T any]() *T { return new(T) }
func reap() error {
	command := fresh[exec.Cmd]()
	return command.Wait()
}
`,
			want: "forged exec.Cmd",
		},
		{
			name: "indexed os Process receiver",
			path: "forbidden.go",
			src: `package fixture
import "os"
func reap(processes []*os.Process) error {
	_, err := processes[0].Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "asserted os Process receiver",
			path: "forbidden.go",
			src: `package fixture
import "os"
func reap(value any) error {
	process := value.(*os.Process)
	_, err := process.Wait()
	return err
}
`,
			want: "os.Process obtained outside an owned exec.Cmd",
		},
		{
			name: "os Process Wait method expression",
			path: "forbidden.go",
			src: `package fixture
import "os"
func reap(process *os.Process) error {
	wait := (*os.Process).Wait
	_, err := wait(process)
	return err
}
`,
			want: "process Wait method as a function value",
		},
		{
			name: "os Process parameter hidden in container",
			path: "forbidden.go",
			src: `package fixture
import "os"
type holder struct { value any }
func retain(process *os.Process) holder {
	return holder{value: process}
}
`,
			want: "opaque path that could enable foreign-PID reaping",
		},
		{
			name: "forged exec Cmd Wait",
			path: "forbidden.go",
			src: `package fixture
import (
	"os"
	"os/exec"
)
func reap() error {
	command := &exec.Cmd{Process: &os.Process{Pid: 123}}
	return command.Wait()
}
`,
			want: "forged exec.Cmd",
		},
		{
			name: "empty forged os Process hidden in container",
			path: "forbidden.go",
			src: `package fixture
import "os"
type holder struct { process *os.Process }
func retain(pid int) holder {
	process := &os.Process{}
	process.Pid = pid
	return holder{process: process}
}
`,
			want: "constructs a forged os.Process",
		},
		{
			name: "replaced exec Cmd Process Wait",
			path: "forbidden.go",
			src: `package fixture
import (
	"os"
	"os/exec"
)
func reap() error {
	var command exec.Cmd
	command.Process = &os.Process{Pid: 123}
	return command.Wait()
}
`,
			want: "dynamically replaced process Wait receiver",
		},
		{
			name: "owned exec Cmd Process replaced from FindProcess",
			path: "forbidden.go",
			src: `package fixture
import (
	"os"
	"os/exec"
)
func reap(pid int) error {
	command := exec.Command("true")
	process, _ := os.FindProcess(pid)
	command.Process = process
	return command.Wait()
}
`,
			want: "dynamically replaced process Wait receiver",
		},
		{
			name: "owned exec Cmd alias may replace Process",
			path: "forbidden.go",
			src: `package fixture
import (
	"os"
	"os/exec"
)
func reap(pid int) error {
	command := exec.Command("true")
	alias := command
	process, _ := os.FindProcess(pid)
	alias.Process = process
	return command.Wait()
}
`,
			want: "dynamically replaced process Wait receiver",
		},
		{
			name: "reviewed seam call unknown target",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
func runGitCommand(pid int) error { return waitForGitCommandExitWithoutReaping(pid) }
`,
			want: "must receive an *exec.Cmd parameter's Process.Pid",
		},
		{
			name: "reviewed seam reassigned command",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func runGitCommand(command, foreign *exec.Cmd) error {
	command = foreign
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "receives a reassigned command or Process",
		},
		{
			name: "reviewed seam reassigned Process",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func runGitCommand(command, foreign *exec.Cmd) error {
	command.Process = foreign.Process
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "receives a reassigned command or Process",
		},
		{
			name: "reviewed seam overwritten command value",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func runGitCommand(command, foreign *exec.Cmd) error {
	*command = *foreign
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "receives a reassigned command or Process",
		},
		{
			name: "reviewed seam aliases Process pointer",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func runGitCommand(command, foreign *exec.Cmd) error {
	process := command.Process
	process.Pid = foreign.Process.Pid
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "receives a reassigned command or Process",
		},
		{
			name: "reviewed seam aliases Process pointer with var",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func runGitCommand(command, foreign *exec.Cmd) error {
	var process = command.Process
	process.Pid = foreign.Process.Pid
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "receives a reassigned command or Process",
		},
		{
			name: "reviewed seam passes command to opaque helper",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func mutate(*exec.Cmd) {}
func runGitCommand(command *exec.Cmd) error {
	mutate(command)
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "opaque use of its command, Process, or PID",
		},
		{
			name: "reviewed seam passes command alias to opaque helper",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func mutate(*exec.Cmd) {}
func runGitCommand(command *exec.Cmd) error {
	alias := command
	mutate(alias)
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "opaque use of its command, Process, or PID",
		},
		{
			name: "reviewed seam passes Process to opaque helper",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import (
	"os"
	"os/exec"
)
func mutate(*os.Process) {}
func runGitCommand(command *exec.Cmd) error {
	mutate(command.Process)
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "opaque use of its command, Process, or PID",
		},
		{
			name: "reviewed seam passes PID-derived value to opaque helper",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func mutate(int) {}
func runGitCommand(command *exec.Cmd) error {
	pid := command.Process.Pid
	mutate(pid)
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "opaque use of its command, Process, or PID",
		},
		{
			name: "reviewed seam sends command to opaque receiver",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
func runGitCommand(command *exec.Cmd, commands chan<- *exec.Cmd) error {
	commands <- command
	return waitForGitCommandExitWithoutReaping(command.Process.Pid)
}
`,
			want: "opaque use of its command, Process, or PID",
		},
		{
			name: "reviewed seam wrong id type",
			path: "internal/specguard/process_wait_linux.go",
			src: `package specguard
import "golang.org/x/sys/unix"
func waitForGitCommandExitWithoutReaping(pid int) error {
	return unix.Waitid(unix.P_ALL, pid, nil, 0, nil)
}
`,
			want: "both P_PID and its exact pid parameter",
		},
		{
			name: "reviewed seam foreign target",
			path: "internal/specguard/process_wait_linux.go",
			src: `package specguard
import "golang.org/x/sys/unix"
func waitForGitCommandExitWithoutReaping(pid int) error {
	return unix.Waitid(unix.P_PID, 123, nil, 0, nil)
}
`,
			want: "both P_PID and its exact pid parameter",
		},
		{
			name: "reviewed seam local uintptr function",
			path: "internal/specguard/process_wait_darwin.go",
			src: `package specguard
import "syscall"
func uintptr(value int) int { return value }
func waitForGitCommandExitWithoutReaping(pid int) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, 1, uintptr(pid), 0, 0, 0, 0)
	return errno
}
`,
			want: "both P_PID and its exact pid parameter",
		},
		{
			name: "raw wait trap alias",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
const trap = syscall.SYS_WAITID
func reap(pid int) { _, _, _ = syscall.Syscall6(trap, 1, uintptr(pid), 0, 0, 0, 0) }
`,
			want: "outside an explicitly reviewed exact-PID wait seam",
		},
		{
			name: "numeric raw trap",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func call() { _, _, _ = syscall.Syscall6(173, 1, 123, 0, 0, 0, 0) }
`,
			want: "numeric or dynamic raw syscall trap",
		},
		{
			name: "mutable raw trap alias",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
var trap = syscall.SYS_PRLIMIT64
func call() { _, _, _ = syscall.RawSyscall6(trap, 0, 0, 0, 0, 0, 0) }
`,
			want: "numeric or dynamic raw syscall trap",
		},
		{
			name: "Syscall9",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func reap() { _, _, _ = syscall.Syscall9(syscall.SYS_WAITID, 1, 123, 0, 0, 0, 0, 0, 0, 0) }
`,
			want: "outside an explicitly reviewed exact-PID wait seam",
		},
		{
			name: "raw function alias",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func reap() { call := syscall.Syscall6; _, _, _ = call(syscall.SYS_WAITID, 1, 123, 0, 0, 0, 0) }
`,
			want: "function value",
		},
		{
			name: "AllThreadsSyscall wait",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func reap() { _, _, _ = syscall.AllThreadsSyscall(syscall.SYS_WAIT4, 0, 0, 0) }
`,
			want: "raw child-reaping syscall",
		},
		{
			name: "AllThreadsSyscall6 waitid",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func reap() { _, _, _ = syscall.AllThreadsSyscall6(syscall.SYS_WAITID, 1, 123, 0, 0, 0, 0) }
`,
			want: "outside an explicitly reviewed exact-PID wait seam",
		},
		{
			name: "dynamic AllThreadsSyscall trap",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func reap(trap uintptr) { _, _, _ = syscall.AllThreadsSyscall(trap, 0, 0, 0) }
`,
			want: "numeric or dynamic raw syscall trap",
		},
		{
			name: "raw sigaction",
			path: "forbidden.go",
			src: `package fixture
import "syscall"
func mutate() { _, _, _ = syscall.RawSyscall(syscall.SYS_SIGACTION, 0, 0) }
`,
			want: "raw signal-disposition syscall",
		},
		{
			name: "signal function alias",
			path: "forbidden.go",
			src: `package fixture
import "os/signal"
func mutate() { ignore := signal.Ignore; ignore() }
`,
			want: "function value",
		},
		{
			name: "parenthesized SIGCHLD",
			path: "forbidden.go",
			src: `package fixture
import (
	"os/signal"
	"syscall"
)
func mutate() { (signal.Ignore)(syscall.SIGCHLD) }
`,
			want: "SIGCHLD",
		},
		{
			name: "numeric signal",
			path: "forbidden.go",
			src: `package fixture
import (
	"os/signal"
	"syscall"
)
func mutate() { signal.Ignore(syscall.Signal(20)) }
`,
			want: "dynamic or numeric signal",
		},
		{
			name: "helper returned signal",
			path: "forbidden.go",
			src: `package fixture
import (
	"os"
	"os/signal"
)
func childSignal() os.Signal { return nil }
func mutate() { signal.Ignore(childSignal()) }
`,
			want: "cannot be audited to exclude SIGCHLD",
		},
		{
			name: "signal Stop without proven subscription",
			path: "forbidden.go",
			src: `package fixture
import (
	"os"
	"os/signal"
)
func mutate(signals chan os.Signal) { signal.Stop(signals) }
`,
			want: "not proven to contain only non-SIGCHLD subscriptions",
		},
		{
			name: "signal Stop on caller-owned channel",
			path: "forbidden.go",
			src: `package fixture
import (
	"os"
	"os/signal"
	"syscall"
)
func mutate(signals chan os.Signal) {
	signal.Notify(signals, syscall.SIGTERM)
	signal.Stop(signals)
}
`,
			want: "not proven to contain only non-SIGCHLD subscriptions",
		},
		{
			name: "signal Stop function alias",
			path: "forbidden.go",
			src: `package fixture
import "os/signal"
func mutate() { stop := signal.Stop; _ = stop }
`,
			want: "function value",
		},
		{
			name: "scoped SIGCHLD alias",
			path: "forbidden.go",
			src: `package fixture
import (
	"os/signal"
	"syscall"
)
func bad() { selected := syscall.SIGCHLD; signal.Ignore(selected) }
func good() { selected := syscall.SIGTERM; signal.Ignore(selected) }
`,
			want: "SIGCHLD",
		},
		{
			name: "reviewed signal seam SIGCHLD callsite",
			path: "agm/cmd/agm/caller.go",
			src: `package main
import (
	"context"
	"syscall"
)
func call(run func(context.Context) error) error {
	return executeWithSignalContext(context.Background(), run, syscall.SIGCHLD)
}
`,
			want: "receives SIGCHLD",
		},
		{
			name: "reviewed wait seam reassigns pid parameter",
			path: "internal/specguard/process_wait_linux.go",
			src: `package specguard
import "golang.org/x/sys/unix"
func waitForGitCommandExitWithoutReaping(pid int) error {
	pid = 123
	return unix.Waitid(unix.P_PID, pid, nil, 0, nil)
}
`,
			want: "reassigns or exposes its pid parameter",
		},
		{
			name: "reviewed wait seam exposes pid parameter pointer",
			path: "internal/specguard/process_wait_linux.go",
			src: `package specguard
import "golang.org/x/sys/unix"
func mutate(*int) {}
func waitForGitCommandExitWithoutReaping(pid int) error {
	mutate(&pid)
	return unix.Waitid(unix.P_PID, pid, nil, 0, nil)
}
`,
			want: "reassigns or exposes its pid parameter",
		},
		{
			name: "reviewed lifecycle constructor mutates Process",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import (
	"os"
	"os/exec"
)
type gitProcessGroupLifecycle struct {
	command *exec.Cmd
	enabled bool
}
func newGitProcessGroupLifecycle(command *exec.Cmd) *gitProcessGroupLifecycle {
	command.Process = &os.Process{Pid: 123}
	return &gitProcessGroupLifecycle{command: command, enabled: true}
}
`,
			want: "lifecycle constructor may replace",
		},
		{
			name: "reviewed lifecycle constructor changes its result seam",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import "os/exec"
type gitProcessGroupLifecycle struct {
	command *exec.Cmd
	enabled bool
}
func newGitProcessGroupLifecycle(command *exec.Cmd) any {
	return &gitProcessGroupLifecycle{command: command, enabled: true}
}
`,
			want: "lifecycle constructor may replace",
		},
		{
			name: "reviewed lifecycle method mutates Process",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import (
	"os"
	"os/exec"
)
type gitProcessGroupLifecycle struct { command *exec.Cmd }
func (lifecycle *gitProcessGroupLifecycle) disable() {
	lifecycle.command.Process = &os.Process{Pid: 123}
}
`,
			want: "lifecycle method may mutate",
		},
		{
			name: "reviewed lifecycle generic mutates Process",
			path: "internal/specguard/git_exec.go",
			src: `package specguard
import (
	"os"
	"os/exec"
)
type gitProcessGroupLifecycle struct { command *exec.Cmd }
func replace[T any](target *T, value T) { *target = value }
func (lifecycle *gitProcessGroupLifecycle) disable() {
	replace(&lifecycle.command.Process, &os.Process{Pid: 123})
}
`,
			want: "lifecycle method may mutate",
		},
		{
			name: "reviewed process helper mutates Process",
			path: "internal/specguard/process_unix.go",
			src: `package specguard
import "os"
func killProcessGroup(process *os.Process) error {
	process.Pid = 123
	return nil
}
`,
			want: "process-group helper may mutate",
		},
		{
			name: "reviewed signal slice element mutation",
			path: "agm/cmd/agm/main.go",
			src: `package main
import (
	"context"
	"os"
	"os/signal"
	"syscall"
)
func executeWithSignalContext(parent context.Context, run func(context.Context) error, signals ...os.Signal) error {
	signals[0] = syscall.SIGCHLD
	ctx, stop := signal.NotifyContext(parent, signals...)
	defer stop()
	return run(ctx)
}
`,
			want: "must forward only its audited variadic signal parameter",
		},
		{
			name: "reviewed signal slice append",
			path: "agm/cmd/agm/main.go",
			src: `package main
import (
	"context"
	"os"
	"os/signal"
	"syscall"
)
func executeWithSignalContext(parent context.Context, run func(context.Context) error, signals ...os.Signal) error {
	signals = append(signals, syscall.SIGCHLD)
	ctx, stop := signal.NotifyContext(parent, signals...)
	defer stop()
	return run(ctx)
}
`,
			want: "must forward only its audited variadic signal parameter",
		},
		{
			name: "reviewed signal slice opaque helper",
			path: "agm/cmd/agm/main.go",
			src: `package main
import (
	"context"
	"os"
	"os/signal"
)
func mutate([]os.Signal) {}
func executeWithSignalContext(parent context.Context, run func(context.Context) error, signals ...os.Signal) error {
	mutate(signals)
	ctx, stop := signal.NotifyContext(parent, signals...)
	defer stop()
	return run(ctx)
}
`,
			want: "must forward only its audited variadic signal parameter",
		},
		{
			name: "import C",
			path: "forbidden.go",
			src: `package fixture
/* #include <signal.h> */
import "C"
func mutate() { C.signal(C.SIGCHLD, C.SIG_IGN) }
`,
			want: "imports C",
		},
		{
			name: "go linkname raw syscall",
			path: "forbidden.go",
			src: `package fixture
import _ "unsafe"
//go:linkname rawSyscall syscall.RawSyscall
func rawSyscall(trap, first, second uintptr) (uintptr, uintptr, error)
func mutate() { _, _, _ = rawSyscall(173, 1, 0) }
`,
			want: "uses go:linkname",
		},
		{
			name: "dot import",
			path: "forbidden.go",
			src: `package fixture
import . "syscall"
func mutate() { _, _ = Wait4(-1, nil, 0, nil) }
`,
			want: "dot imports",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeBuildAuthorityWaitOwnershipFixture(t, root, test.path, test.src)
			result := scanBuildAuthorityWaitOwnershipFixture(t, root)
			assertBuildAuthorityViolation(t, result, test.want)
		})
	}

	t.Run("cross-file generic process factory alias", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "factory.go", `package fixture
import "os"
func fresh[T any]() *T { return new(T) }
var processFactory = fresh[os.Process]
`)
		writeBuildAuthorityWaitOwnershipFixture(t, root, "reap.go", `package fixture
func reap() error {
	process := processFactory()
	process.Pid = -1
	_, err := process.Wait()
	return err
}
`)
		result := scanBuildAuthorityWaitOwnershipFixture(t, root)
		assertBuildAuthorityViolation(t, result, "generic type argument")
	})

	for _, test := range []struct {
		name        string
		declaration string
		conversion  string
	}{
		{
			name:        "cross-file package int type",
			declaration: "type int uint64\n",
			conversion:  "uintptr(int(pid))",
		},
		{
			name:        "cross-file package uintptr function",
			declaration: "func uintptr(value int) uint64 { return uint64(value) }\n",
			conversion:  "uintptr(pid)",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeBuildAuthorityWaitOwnershipFixture(
				t, root, "internal/specguard/shadow.go", "package specguard\n"+test.declaration,
			)
			writeBuildAuthorityWaitOwnershipFixture(
				t,
				root,
				"internal/specguard/process_wait_darwin.go",
				`package specguard
import "syscall"
func waitForGitCommandExitWithoutReaping(pid int) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, 1, `+test.conversion+`, 0, 0, 0, 0)
	return errno
}
`,
			)
			result := scanBuildAuthorityWaitOwnershipFixture(t, root)
			assertBuildAuthorityViolation(t, result, "both P_PID and its exact pid parameter")
		})
	}
}

func TestScanBuildAuthorityWaitOwnershipRejectsClosedSourceEscapes(t *testing.T) {
	t.Run("vendor", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "pkg/vendor/hidden/hidden.go", "package hidden\n")
		result := scanBuildAuthorityWaitOwnershipFixture(t, root)
		assertBuildAuthorityViolation(t, result, "vendored production source")
	})

	for _, extension := range []string{
		".c", ".cc", ".cpp", ".cxx", ".m", ".mm",
		".h", ".hh", ".hpp", ".hxx",
		".f", ".F", ".for", ".f90", ".s", ".S", ".sx",
		".swig", ".swigcxx", ".syso",
	} {
		t.Run("native "+extension, func(t *testing.T) {
			root := t.TempDir()
			writeBuildAuthorityWaitOwnershipFixture(t, root, "pkg/native"+extension, "native production payload\n")
			result := scanBuildAuthorityWaitOwnershipFixture(t, root)
			assertBuildAuthorityViolation(t, result, "native production source")
		})
	}

	for _, directory := range []string{"node_modules", "testdata"} {
		t.Run("reachable "+directory, func(t *testing.T) {
			root := t.TempDir()
			writeBuildAuthorityWaitOwnershipFixture(t, root, directory+"/hidden.go", `package hidden
import "syscall"
func reap() { _, _ = syscall.Wait4(-1, nil, 0, nil) }
`)
			result := scanBuildAuthorityWaitOwnershipFixture(t, root)
			assertBuildAuthorityViolation(t, result, "child-reaping API")
		})
	}
}

func TestScanBuildAuthorityWaitOwnershipEnforcesBoundsAndCancellation(t *testing.T) {
	source := "package fixture\n"

	t.Run("exact entry count", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.txt", "one\n")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "two.txt", "two\n")
		limits := buildAuthorityWaitOwnershipTestLimits(1, 1024, 1024)
		limits.maxEntries = 2
		limits.maxDirectories = 1
		if _, err := scanBuildAuthorityWaitOwnership(context.Background(), root, limits); err != nil {
			t.Fatalf("exact entry boundary: %v", err)
		}
	})

	t.Run("entry count overflow", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.txt", "one\n")
		writeBuildAuthorityWaitOwnershipFixture(t, root, "two.txt", "two\n")
		limits := buildAuthorityWaitOwnershipTestLimits(1, 1024, 1024)
		limits.maxEntries = 1
		limits.maxDirectories = 1
		_, err := scanBuildAuthorityWaitOwnership(context.Background(), root, limits)
		if err == nil || !strings.Contains(err.Error(), "entry count exceeds bounded scan limit 1") {
			t.Fatalf("entry-bound error = %v", err)
		}
	})

	t.Run("exact directory count", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "one"), 0o755); err != nil {
			t.Fatalf("create first directory: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "two"), 0o755); err != nil {
			t.Fatalf("create second directory: %v", err)
		}
		limits := buildAuthorityWaitOwnershipTestLimits(1, 1024, 1024)
		limits.maxEntries = 2
		limits.maxDirectories = 3
		if _, err := scanBuildAuthorityWaitOwnership(context.Background(), root, limits); err != nil {
			t.Fatalf("exact directory boundary: %v", err)
		}
	})

	t.Run("directory count overflow", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "one"), 0o755); err != nil {
			t.Fatalf("create first directory: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "two"), 0o755); err != nil {
			t.Fatalf("create second directory: %v", err)
		}
		limits := buildAuthorityWaitOwnershipTestLimits(1, 1024, 1024)
		limits.maxEntries = 2
		limits.maxDirectories = 2
		_, err := scanBuildAuthorityWaitOwnership(context.Background(), root, limits)
		if err == nil || !strings.Contains(err.Error(), "directory count exceeds bounded scan limit 2") {
			t.Fatalf("directory-bound error = %v", err)
		}
	})

	t.Run("exact file count", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.go", source)
		result, err := scanBuildAuthorityWaitOwnership(context.Background(), root,
			buildAuthorityWaitOwnershipTestLimits(1, 1024, 1024))
		if err != nil || result.files != 1 {
			t.Fatalf("exact count boundary: files=%d err=%v", result.files, err)
		}
	})

	t.Run("file count overflow", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.go", source)
		writeBuildAuthorityWaitOwnershipFixture(t, root, "two.go", source)
		_, err := scanBuildAuthorityWaitOwnership(context.Background(), root,
			buildAuthorityWaitOwnershipTestLimits(1, 1024, 2048))
		if err == nil || !strings.Contains(err.Error(), "source count exceeds 1") {
			t.Fatalf("count-bound error = %v", err)
		}
	})

	t.Run("exact byte bounds", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.go", source)
		limit := int64(len(source))
		result, err := scanBuildAuthorityWaitOwnership(context.Background(), root,
			buildAuthorityWaitOwnershipTestLimits(1, limit, limit))
		if err != nil || result.bytes != limit {
			t.Fatalf("exact byte boundary: bytes=%d err=%v", result.bytes, err)
		}
	})

	t.Run("per file overflow", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "large.go", source)
		_, err := scanBuildAuthorityWaitOwnership(context.Background(), root,
			buildAuthorityWaitOwnershipTestLimits(1, int64(len(source)-1), 2048))
		if err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("file-byte-bound error = %v", err)
		}
	})

	t.Run("aggregate overflow", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.go", source)
		writeBuildAuthorityWaitOwnershipFixture(t, root, "two.go", source)
		_, err := scanBuildAuthorityWaitOwnership(context.Background(), root,
			buildAuthorityWaitOwnershipTestLimits(2, 1024, int64(len(source)*2-1)))
		if err == nil || !strings.Contains(err.Error(), "source bytes exceed") {
			t.Fatalf("aggregate-byte-bound error = %v", err)
		}
	})

	t.Run("limit addition overflow", func(t *testing.T) {
		root := t.TempDir()
		limits := buildAuthorityWaitOwnershipTestLimits(1, math.MaxInt64, math.MaxInt64)
		_, err := scanBuildAuthorityWaitOwnership(context.Background(), root,
			limits)
		if err == nil || !strings.Contains(err.Error(), "invalid production-source scan limits") {
			t.Fatalf("overflow-bound error = %v", err)
		}
	})

	t.Run("pre-cancelled", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.go", source)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := scanBuildAuthorityWaitOwnership(ctx, root, defaultBuildAuthorityWaitOwnershipLimits())
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-cancellation error = %v", err)
		}
	})

	t.Run("cancel after descriptor open", func(t *testing.T) {
		root := t.TempDir()
		writeBuildAuthorityWaitOwnershipFixture(t, root, "one.go", source)
		ctx, cancel := context.WithCancel(context.Background())
		_, err := scanBuildAuthorityWaitOwnershipWithHooks(
			ctx,
			root,
			defaultBuildAuthorityWaitOwnershipLimits(),
			buildAuthorityWaitOwnershipHooks{afterSourceOpen: func(_, _ string) error {
				cancel()
				return nil
			}},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("mid-read cancellation error = %v", err)
		}
	})
}

func scanBuildAuthorityWaitOwnershipFixture(
	t *testing.T,
	root string,
) buildAuthorityWaitOwnershipScan {
	t.Helper()
	result, err := scanBuildAuthorityWaitOwnership(
		context.Background(), root, defaultBuildAuthorityWaitOwnershipLimits(),
	)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	return result
}

func buildAuthorityWaitOwnershipTestLimits(
	maxFiles int,
	maxFileBytes int64,
	maxTotalBytes int64,
) buildAuthorityWaitOwnershipLimits {
	return buildAuthorityWaitOwnershipLimits{
		maxEntries:     128,
		maxDirectories: 64,
		maxFiles:       maxFiles,
		maxFileBytes:   maxFileBytes,
		maxTotalBytes:  maxTotalBytes,
	}
}

func assertBuildAuthorityViolation(
	t *testing.T,
	result buildAuthorityWaitOwnershipScan,
	want string,
) {
	t.Helper()
	for _, violation := range result.violations {
		if strings.Contains(violation, want) {
			return
		}
	}
	t.Fatalf("violations = %v, want substring %q", result.violations, want)
}

func writeBuildAuthorityWaitOwnershipFixture(t *testing.T, root, name, source string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}
