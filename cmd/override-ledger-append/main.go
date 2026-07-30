//go:build !windows

// Command override-ledger-append is the fixed privileged boundary for
// dangerous-override audit records on Unix systems.
//
// It accepts no arguments and appends one bounded, canonical Use JSONL
// transaction from stdin to the fixed production ledger. A transaction has at
// most one record per override kind. A sudoers rule may grant NOPASSWD access
// to this command without granting access to tee, chmod, AGM, or an
// operator-selected path.
package main

import (
	"bufio"
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

	limited := &io.LimitedReader{R: input, N: override.MaxLedgerBatchBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read transaction: %w", err)
	}
	if len(data) > override.MaxLedgerBatchBytes {
		return fmt.Errorf("%w: input exceeds %d bytes", override.ErrLedgerRecordTooLarge, override.MaxLedgerBatchBytes)
	}

	uses, canonical, err := decodeTransaction(data)
	if err != nil {
		return err
	}
	now := uses[0].AtUTC
	if requireRoot {
		now = time.Now()
	}

	return appendRecords(path, canonical, uses, now, requireRoot)
}

func decodeTransaction(data []byte) ([]override.Use, []byte, error) {
	uses, err := override.DecodeLedgerUses(data)
	if err != nil {
		return nil, nil, err
	}
	return uses, data, nil
}

func appendRecords(path string, data []byte, uses []override.Use, now time.Time, requireRoot bool) error {
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

	if err := appendLockedRecords(file, path, data, uses, now, requireRoot); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close fixed ledger: %w", err)
	}
	closed = true
	locked = false
	return nil
}

func appendLockedRecords(
	file *os.File,
	path string,
	data []byte,
	uses []override.Use,
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
	if info.Size() > maxOperatorLedgerBytes-int64(len(data)) {
		return fmt.Errorf("fixed ledger size cap of %d bytes would be exceeded", maxOperatorLedgerBytes)
	}
	if requireRoot {
		if err := validatePrivilegedUses(uses, now); err != nil {
			return err
		}
	}
	if err := enforcePrivilegedRateLimits(file, uses, now); err != nil {
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		return fmt.Errorf("secure fixed ledger mode: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("append fixed ledger: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync fixed ledger: %w", err)
	}
	return nil
}

func validatePrivilegedUses(uses []override.Use, now time.Time) error {
	for _, use := range uses {
		if use.AtUTC.Before(now.Add(-maxRecordClockSkew)) || use.AtUTC.After(now.Add(maxRecordClockSkew)) {
			return fmt.Errorf("%w: timestamp is outside the %s append window",
				override.ErrLedgerRecord, maxRecordClockSkew)
		}
		if use.AuthorizationID == "" {
			return fmt.Errorf("%w: authorization ID is required at the privileged boundary",
				override.ErrLedgerRecord)
		}
		grant, err := override.LoadGrant(use.Kind)
		if err != nil {
			return fmt.Errorf("load approval at privileged boundary: %w", err)
		}
		if err := grant.Authorizes(use.Kind, use.Subject, now); err != nil {
			return fmt.Errorf("validate approval at privileged boundary: %w", err)
		}
	}
	return nil
}

func enforcePrivilegedRateLimits(file *os.File, uses []override.Use, now time.Time) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek fixed ledger for rate limit: %w", err)
	}
	additions := make(map[override.Kind]int, len(uses))
	transactionIDs := make(map[string]struct{}, len(uses))
	for _, use := range uses {
		additions[use.Kind]++
		if use.AuthorizationID != "" {
			if _, duplicate := transactionIDs[use.AuthorizationID]; duplicate {
				return fmt.Errorf("%w: transaction repeats authorization ID", override.ErrLedgerRecord)
			}
			transactionIDs[use.AuthorizationID] = struct{}{}
		}
	}
	cutoff := now.Add(-privilegedRateWindow)
	counts, oldest, err := scanRecentUses(file, additions, transactionIDs, cutoff, now)
	if err != nil {
		return err
	}
	for kind, addition := range additions {
		if counts[kind]+addition > maxUsesPerKindPerWindow {
			return fmt.Errorf(
				"%s override rate limit reached (%d uses per %s); retry after %s and audit or revoke an unexpectedly busy grant",
				kind, maxUsesPerKindPerWindow, privilegedRateWindow, oldest[kind].Add(privilegedRateWindow).UTC().Format(time.RFC3339),
			)
		}
	}
	return nil
}

func scanRecentUses(
	file *os.File,
	additions map[override.Kind]int,
	transactionIDs map[string]struct{},
	cutoff, now time.Time,
) (map[override.Kind]int, map[override.Kind]time.Time, error) {
	counts := make(map[override.Kind]int, len(additions))
	oldest := make(map[override.Kind]time.Time, len(additions))
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), override.MaxLedgerBatchBytes)
	for scanner.Scan() {
		recordedUses, err := override.DecodeLedgerUses(append(append([]byte(nil), scanner.Bytes()...), '\n'))
		if err != nil {
			continue
		}
		for _, recorded := range recordedUses {
			if recorded.AuthorizationID != "" {
				if _, duplicate := transactionIDs[recorded.AuthorizationID]; duplicate {
					return nil, nil, fmt.Errorf("%w: authorization ID was already recorded", override.ErrLedgerRecord)
				}
			}
			if additions[recorded.Kind] == 0 ||
				recorded.AtUTC.Before(cutoff) ||
				recorded.AtUTC.After(now.Add(maxRecordClockSkew)) {
				continue
			}
			counts[recorded.Kind]++
			if oldest[recorded.Kind].IsZero() || recorded.AtUTC.Before(oldest[recorded.Kind]) {
				oldest[recorded.Kind] = recorded.AtUTC
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan fixed ledger for rate limit: %w", err)
	}
	return counts, oldest, nil
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
