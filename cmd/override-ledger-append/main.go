//go:build !windows

// Command override-ledger-append is the fixed privileged boundary for
// dangerous-override audit records on Unix systems.
//
// It accepts no arguments and appends exactly one bounded, canonical Use JSONL
// record from stdin to the fixed production ledger. A sudoers rule may grant
// NOPASSWD access to this command without granting access to tee, chmod, AGM,
// or an operator-selected path.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/pkg/override"
)

const (
	maxOperatorLedgerBytes  int64 = 16 << 20
	maxRecordClockSkew            = time.Minute
	privilegedRateWindow          = time.Hour
	maxUsesPerKindPerWindow       = 5
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "override-ledger-append: arguments are not accepted")
		os.Exit(2)
	}
	if err := appendInput(os.Stdin, override.LedgerPath(), true); err != nil {
		fmt.Fprintf(os.Stderr, "override-ledger-append: %v\n", err)
		os.Exit(1)
	}
}

func appendInput(input io.Reader, path string, requireRoot bool) error {
	if requireRoot && os.Geteuid() != 0 {
		return errors.New("must run as root through the installed sudoers rule")
	}

	limited := &io.LimitedReader{R: input, N: override.MaxLedgerRecordBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read record: %w", err)
	}
	if len(data) > override.MaxLedgerRecordBytes {
		return fmt.Errorf("%w: input exceeds %d bytes", override.ErrLedgerRecordTooLarge, override.MaxLedgerRecordBytes)
	}

	var use override.Use
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&use); err != nil {
		return fmt.Errorf("decode record: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	canonical, err := override.EncodeLedgerUse(use)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("%w: input must be one canonical JSONL record", override.ErrLedgerRecord)
	}
	now := use.AtUTC
	if requireRoot {
		now = time.Now()
		if use.AtUTC.Before(now.Add(-maxRecordClockSkew)) || use.AtUTC.After(now.Add(maxRecordClockSkew)) {
			return fmt.Errorf("%w: timestamp is outside the %s append window",
				override.ErrLedgerRecord, maxRecordClockSkew)
		}
		grant, err := override.LoadGrant(use.Kind)
		if err != nil {
			return fmt.Errorf("load approval at privileged boundary: %w", err)
		}
		if err := grant.Active(use.Kind, now); err != nil {
			return fmt.Errorf("validate approval at privileged boundary: %w", err)
		}
	}

	return appendRecord(path, canonical, use, now, requireRoot)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("%w: multiple JSON values are not accepted", override.ErrLedgerRecord)
	}
	return fmt.Errorf("decode trailing input: %w", err)
}

func appendRecord(path string, line []byte, use override.Use, now time.Time, requireRoot bool) error {
	if requireRoot {
		if err := validateRootOwnedPath(filepath.Dir(path), true); err != nil {
			return fmt.Errorf("validate ledger directory: %w", err)
		}
	}

	fd, err := syscall.Open(path,
		syscall.O_RDWR|syscall.O_APPEND|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("open fixed ledger: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return errors.New("wrap fixed ledger descriptor")
	}

	locked := false
	closed := false
	defer func() {
		if locked && !closed {
			_ = syscall.Flock(fd, syscall.LOCK_UN)
		}
		if !closed {
			_ = file.Close()
		}
	}()
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock fixed ledger: %w", err)
	}
	locked = true

	if err := appendLockedRecord(file, path, line, use, now, requireRoot); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close fixed ledger: %w", err)
	}
	closed = true
	locked = false
	return nil
}

func appendLockedRecord(
	file *os.File,
	path string,
	line []byte,
	use override.Use,
	now time.Time,
	requireRoot bool,
) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect fixed ledger: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("fixed ledger is not a regular file")
	}
	if requireRoot {
		if err := validateRootOwnedInfo(path, info, false); err != nil {
			return err
		}
	}
	if info.Size() > maxOperatorLedgerBytes-int64(len(line)) {
		return fmt.Errorf("fixed ledger size cap of %d bytes would be exceeded", maxOperatorLedgerBytes)
	}
	if err := enforcePrivilegedRateLimit(file, use.Kind, now); err != nil {
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		return fmt.Errorf("secure fixed ledger mode: %w", err)
	}
	if _, err := file.Write(line); err != nil {
		return fmt.Errorf("append fixed ledger: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync fixed ledger: %w", err)
	}
	return nil
}

func enforcePrivilegedRateLimit(file *os.File, kind override.Kind, now time.Time) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek fixed ledger for rate limit: %w", err)
	}
	cutoff := now.Add(-privilegedRateWindow)
	count := 0
	var oldest time.Time
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), override.MaxLedgerRecordBytes)
	for scanner.Scan() {
		var recorded override.Use
		if json.Unmarshal(scanner.Bytes(), &recorded) != nil ||
			recorded.Kind != kind ||
			recorded.AtUTC.Before(cutoff) ||
			recorded.AtUTC.After(now.Add(maxRecordClockSkew)) {
			continue
		}
		count++
		if oldest.IsZero() || recorded.AtUTC.Before(oldest) {
			oldest = recorded.AtUTC
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan fixed ledger for rate limit: %w", err)
	}
	if count >= maxUsesPerKindPerWindow {
		return fmt.Errorf(
			"%s override rate limit reached (%d uses per %s); retry after %s and audit or revoke an unexpectedly busy grant",
			kind, maxUsesPerKindPerWindow, privilegedRateWindow, oldest.Add(privilegedRateWindow).UTC().Format(time.RFC3339),
		)
	}
	return nil
}

func validateRootOwnedPath(path string, wantDir bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	return validateRootOwnedInfo(path, info, wantDir)
}

func validateRootOwnedInfo(path string, info os.FileInfo, wantDir bool) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", path)
	}
	if wantDir && !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if !wantDir && !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot verify owner of %s", path)
	}
	if stat.Uid != 0 {
		return fmt.Errorf("%s is owned by uid %d, want root", path, stat.Uid)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by group or others (mode %04o)", path, info.Mode().Perm())
	}
	return nil
}
