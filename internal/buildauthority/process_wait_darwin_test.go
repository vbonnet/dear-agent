//go:build darwin && arm64

package buildauthority

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestDarwinProcessABILayouts(t *testing.T) {
	if got := unsafe.Sizeof(darwinOldSigaction{}); got != 16 {
		t.Fatalf("sigaction size = %d", got)
	}
	if got := unsafe.Alignof(darwinOldSigaction{}); got != 8 {
		t.Fatalf("sigaction alignment = %d", got)
	}
	if unsafe.Offsetof(darwinOldSigaction{}.handler) != 0 ||
		unsafe.Offsetof(darwinOldSigaction{}.mask) != 8 ||
		unsafe.Offsetof(darwinOldSigaction{}.flags) != 12 {
		t.Fatal("sigaction field offsets drifted")
	}

	if got := unsafe.Sizeof(darwinSiginfo{}); got != 104 {
		t.Fatalf("siginfo size = %d", got)
	}
	if got := unsafe.Alignof(darwinSiginfo{}); got != 8 {
		t.Fatalf("siginfo alignment = %d", got)
	}
	offsets := [...]uintptr{
		unsafe.Offsetof(darwinSiginfo{}.signo),
		unsafe.Offsetof(darwinSiginfo{}.errno),
		unsafe.Offsetof(darwinSiginfo{}.code),
		unsafe.Offsetof(darwinSiginfo{}.pid),
		unsafe.Offsetof(darwinSiginfo{}.uid),
		unsafe.Offsetof(darwinSiginfo{}.status),
		unsafe.Offsetof(darwinSiginfo{}.addr),
		unsafe.Offsetof(darwinSiginfo{}.value),
		unsafe.Offsetof(darwinSiginfo{}.band),
		unsafe.Offsetof(darwinSiginfo{}.pad),
	}
	want := [...]uintptr{0, 4, 8, 12, 16, 20, 24, 32, 40, 48}
	if offsets != want {
		t.Fatalf("siginfo offsets = %v, want %v", offsets, want)
	}
	if unsafe.Sizeof(unix.KinfoProc{}) != uintptr(unix.SizeofKinfoProc) {
		t.Fatalf("KinfoProc size = %d, x/sys constant = %d", unsafe.Sizeof(unix.KinfoProc{}), unix.SizeofKinfoProc)
	}
	if got := len(darwinGroupBuffer{}.rows); got != 4_096 {
		t.Fatalf("group buffer rows = %d", got)
	}
}

func TestDecodeDarwinWaitAcceptsOnlyExactTerminalPairs(t *testing.T) {
	const pid = 42
	for _, status := range []int{0, 1, 254, 255} {
		terminal, noResult, valid := decodeDarwinWait(validDarwinInfo(pid, darwinChildExited, status), pid)
		if !valid || noResult || terminal == nil || terminal.class != processTerminalExited || terminal.status != status {
			t.Fatalf("exit status %d decoded as terminal=%+v noResult=%v valid=%v", status, terminal, noResult, valid)
		}
	}
	for _, status := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 24, 25, 26, 27, 30, 31} {
		terminal, noResult, valid := decodeDarwinWait(validDarwinInfo(pid, darwinChildKilled, status), pid)
		if !valid || noResult || terminal == nil || terminal.class != processTerminalKilled || terminal.status != status {
			t.Fatalf("killed status %d decoded as terminal=%+v noResult=%v valid=%v", status, terminal, noResult, valid)
		}
	}
	for _, status := range []int{3, 4, 5, 6, 7, 8, 10, 11, 12} {
		terminal, noResult, valid := decodeDarwinWait(validDarwinInfo(pid, darwinChildDumped, status), pid)
		if !valid || noResult || terminal == nil || terminal.class != processTerminalDumped || terminal.status != status {
			t.Fatalf("dumped status %d decoded as terminal=%+v noResult=%v valid=%v", status, terminal, noResult, valid)
		}
	}

	invalid := []darwinSiginfo{
		validDarwinInfo(pid, darwinChildExited, -1),
		validDarwinInfo(pid, darwinChildExited, 256),
		validDarwinInfo(pid, darwinChildKilled, 0),
		validDarwinInfo(pid, darwinChildKilled, 16),
		validDarwinInfo(pid, darwinChildKilled, 23),
		validDarwinInfo(pid, darwinChildKilled, 28),
		validDarwinInfo(pid, darwinChildKilled, 29),
		validDarwinInfo(pid, darwinChildKilled, 32),
		validDarwinInfo(pid, darwinChildDumped, 1),
		validDarwinInfo(pid, darwinChildDumped, 9),
		validDarwinInfo(pid, 4, 19),
	}
	wrongSigno := validDarwinInfo(pid, darwinChildExited, 0)
	wrongSigno.signo = 19
	invalid = append(invalid, wrongSigno)
	wrongErrno := validDarwinInfo(pid, darwinChildExited, 0)
	wrongErrno.errno = 1
	invalid = append(invalid, wrongErrno)
	wrongPID := validDarwinInfo(pid+1, darwinChildExited, 0)
	invalid = append(invalid, wrongPID)
	for index, info := range invalid {
		if terminal, noResult, valid := decodeDarwinWait(info, pid); valid || noResult || terminal != nil {
			t.Fatalf("invalid row %d decoded as terminal=%+v noResult=%v valid=%v", index, terminal, noResult, valid)
		}
	}

	noResult := darwinSiginfo{signo: 99, pid: 0, status: 99}
	if terminal, absent, valid := decodeDarwinWait(noResult, pid); !valid || !absent || terminal != nil {
		t.Fatalf("pid-zero result = terminal=%+v absent=%v valid=%v", terminal, absent, valid)
	}
}

func TestDarwinSnapshotLengthAndGroupClassification(t *testing.T) {
	rowBytes := unsafe.Sizeof(unix.KinfoProc{})
	capacityBytes := unsafe.Sizeof(darwinGroupBuffer{}.rows)
	for _, test := range []struct {
		name   string
		result darwinSnapshotResult
		count  int
		ok     bool
	}{
		{name: "empty", result: darwinSnapshotResult{}, ok: true},
		{name: "one", result: darwinSnapshotResult{byteLength: rowBytes}, count: 1, ok: true},
		{name: "exact capacity", result: darwinSnapshotResult{byteLength: capacityBytes}, count: processGroupMemberLimit, ok: true},
		{name: "misaligned", result: darwinSnapshotResult{byteLength: rowBytes + 1}},
		{name: "one row over", result: darwinSnapshotResult{byteLength: capacityBytes + rowBytes}},
		{name: "kernel overflow", result: darwinSnapshotResult{byteLength: capacityBytes, overflow: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			count, ok := test.result.count()
			if count != test.count || ok != test.ok {
				t.Fatalf("count = %d, %v; want %d, %v", count, ok, test.count, test.ok)
			}
		})
	}

	const pgid = 71
	tests := []struct {
		name        string
		members     []darwinTestMember
		valid       bool
		leaderOnly  bool
		hasSurvivor bool
	}{
		{name: "empty"},
		{name: "leader zombie", members: []darwinTestMember{{pgid, pgid, darwinProcessZombie}}, valid: true, leaderOnly: true},
		{name: "leader and survivor", members: []darwinTestMember{{pgid, pgid, darwinProcessZombie}, {pgid + 1, pgid, 2}}, valid: true, hasSurvivor: true},
		{name: "missing leader", members: []darwinTestMember{{pgid + 1, pgid, 2}}, hasSurvivor: true},
		{name: "live leader", members: []darwinTestMember{{pgid, pgid, 2}}},
		{name: "stopped leader", members: []darwinTestMember{{pgid, pgid, 3}}},
		{name: "wrong group", members: []darwinTestMember{{pgid, pgid + 1, darwinProcessZombie}}},
		{name: "zero pid", members: []darwinTestMember{{0, pgid, darwinProcessZombie}}, hasSurvivor: true},
		{name: "duplicate", members: []darwinTestMember{{pgid, pgid, darwinProcessZombie}, {pgid, pgid, darwinProcessZombie}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buffer darwinGroupBuffer
			for index, member := range test.members {
				buffer.rows[index] = unix.KinfoProc{
					Proc:  unix.ExternProc{P_pid: int32(member.pid), P_stat: member.state},
					Eproc: unix.Eproc{Pgid: int32(member.pgid)},
				}
			}
			got := classifyDarwinGroup(&buffer, len(test.members), pgid)
			if got.valid != test.valid || got.leaderOnly != test.leaderOnly || got.hasSurvivor != test.hasSurvivor {
				t.Fatalf("classification = %+v", got)
			}
		})
	}
}

func TestRealDarwinSigactionWaitidAndFixedGroupSnapshot(t *testing.T) {
	first, err := realDarwinSigaction()
	if err != nil {
		t.Fatalf("query SIGCHLD action: %v", err)
	}
	second, err := realDarwinSigaction()
	if err != nil {
		t.Fatalf("repeat SIGCHLD action query: %v", err)
	}
	if first != second || !first.admitted() {
		t.Fatalf("SIGCHLD action changed or unsafe: first=%+v second=%+v", first, second)
	}

	command := exec.Command("/bin/sh", "-c", "exit 7")
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}
	if err := command.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	pid := command.Process.Pid
	reaped := false
	defer func() {
		if !reaped {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = command.Wait()
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	var terminal *processTerminal
	for time.Now().Before(deadline) {
		info, waitErr := realDarwinWaitID(pid)
		if waitErr != nil {
			t.Fatalf("waitid: %v", waitErr)
		}
		var valid bool
		terminal, _, valid = decodeDarwinWait(info, pid)
		if !valid {
			t.Fatalf("raw waitid record = %+v", info)
		}
		if terminal != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if terminal == nil || terminal.class != processTerminalExited || terminal.status != 7 {
		t.Fatalf("terminal = %+v", terminal)
	}

	var buffer darwinGroupBuffer
	snapshot, err := realDarwinGroupSnapshot(pid, &buffer)
	if err != nil {
		t.Fatalf("kern.proc.pgrp snapshot: %v (bytes=%d overflow=%v)", err, snapshot.byteLength, snapshot.overflow)
	}
	count, ok := snapshot.count()
	if !ok {
		t.Fatalf("snapshot length = %d overflow=%v", snapshot.byteLength, snapshot.overflow)
	}
	classification := classifyDarwinGroup(&buffer, count, pid)
	if !classification.valid || !classification.leaderOnly || classification.hasSurvivor {
		t.Fatalf("pinned group classification = %+v (rows=%d)", classification, count)
	}

	if err := command.Wait(); err == nil {
		t.Fatal("exit 7 Wait unexpectedly returned nil")
	}
	reaped = true
}

func validDarwinInfo(pid, code, status int) darwinSiginfo {
	return darwinSiginfo{
		signo:  darwinSIGCHLD,
		code:   int32(code),
		pid:    int32(pid),
		status: int32(status),
	}
}

type darwinTestMember struct {
	pid   int
	pgid  int
	state int8
}
