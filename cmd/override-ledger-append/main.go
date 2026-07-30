//go:build !windows

// Command override-ledger-append is the fixed privileged boundary for
// dangerous-override audit records on Unix systems.
//
// It accepts no arguments and processes one bounded, canonical launcher-bound
// request on stdin. A request either issues or consumes an exact one-shot
// launch capability, or appends one Use JSONL transaction to the fixed
// production ledger. A transaction has at most one record per override kind.
// A sudoers rule may grant NOPASSWD access to this command without granting
// access to tee, chmod, AGM, or an operator-selected path.
package main

import (
	"bufio"
	"bytes"
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
	maxOperatorLedgerBytes            int64 = 16 << 20
	maxRecordClockSkew                      = time.Minute
	privilegedRateWindow                    = time.Hour
	maxUsesPerKindPerWindow                 = 5
	maxOutstandingLaunchCapabilities        = 256
	launchCapabilityDirectoryLockName       = ".lock"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "override-ledger-append: arguments are not accepted")
		os.Exit(2)
	}
	if err := processInput(
		os.Stdin,
		override.LedgerPath(),
		override.LaunchCapabilityDir(),
		true,
	); err != nil {
		fmt.Fprintf(os.Stderr, "override-ledger-append: %v\n", err)
		os.Exit(1)
	}
}

func processInput(
	input io.Reader,
	ledgerPath, capabilityDir string,
	requireRoot bool,
) error {
	maxBytes := max(
		override.MaxPrivilegedAppendBytes,
		override.MaxPrivilegedLaunchCapabilityBytes,
	)
	limited := &io.LimitedReader{R: input, N: int64(maxBytes + 1)}
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read privileged request: %w", err)
	}
	if len(data) > maxBytes {
		return fmt.Errorf("%w: input exceeds %d bytes",
			override.ErrLedgerRecordTooLarge, maxBytes)
	}
	operation, err := override.PrivilegedRequestOperation(data)
	if err != nil {
		return err
	}
	if operation != override.PrivilegedLaunchCapabilityOperation &&
		operation != override.PrivilegedConsumeLaunchCapabilityOperation {
		return appendInput(bytes.NewReader(data), ledgerPath, requireRoot)
	}
	if requireRoot && os.Geteuid() != 0 {
		return errors.New("must run as root through the installed sudoers rule")
	}
	var capability override.LaunchCapability
	var launcherPID int
	switch operation {
	case override.PrivilegedLaunchCapabilityOperation:
		capability, launcherPID, err =
			override.DecodePrivilegedLaunchCapabilityRequest(data)
	case override.PrivilegedConsumeLaunchCapabilityOperation:
		capability, launcherPID, err =
			override.DecodePrivilegedConsumeLaunchCapabilityRequest(data)
	}
	if err != nil {
		return err
	}
	if requireRoot {
		allowCompanion := operation == override.PrivilegedLaunchCapabilityOperation
		if err := authenticateLauncher(launcherPID, allowCompanion); err != nil {
			return fmt.Errorf("authenticate launch-capability caller: %w", err)
		}
	}
	if operation == override.PrivilegedLaunchCapabilityOperation {
		return issueCapability(capabilityDir, capability, time.Now(), requireRoot)
	}
	return consumeCapability(capabilityDir, capability, time.Now(), requireRoot)
}

func appendInput(input io.Reader, path string, requireRoot bool) error {
	if requireRoot && os.Geteuid() != 0 {
		return errors.New("must run as root through the installed sudoers rule")
	}

	limited := &io.LimitedReader{R: input, N: override.MaxPrivilegedAppendBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read transaction: %w", err)
	}
	if len(data) > override.MaxPrivilegedAppendBytes {
		return fmt.Errorf("%w: input exceeds %d bytes", override.ErrLedgerRecordTooLarge, override.MaxPrivilegedAppendBytes)
	}

	uses, canonical, launcherPID, err := decodeTransaction(data)
	if err != nil {
		return err
	}
	if requireRoot {
		if err := authenticateLauncher(launcherPID, false); err != nil {
			return fmt.Errorf("authenticate launch-bound append caller: %w", err)
		}
	}
	now := uses[0].AtUTC
	if requireRoot {
		now = time.Now()
	}

	return appendRecords(path, canonical, uses, now, requireRoot)
}

func issueCapability(
	dir string,
	capability override.LaunchCapability,
	now time.Time,
	requireRoot bool,
) error {
	if err := capability.Validate(now); err != nil {
		return err
	}
	if err := prepareCapabilityDirectory(dir, requireRoot); err != nil {
		return err
	}
	lock, err := lockCapabilityDirectory(dir, requireRoot)
	if err != nil {
		return err
	}
	defer closeLockedFile(lock)
	count, err := pruneExpiredCapabilities(dir, now, requireRoot)
	if err != nil {
		return err
	}
	if count >= maxOutstandingLaunchCapabilities {
		return fmt.Errorf(
			"launch capability limit reached (%d outstanding); wait for expiry or inspect aborted launches",
			maxOutstandingLaunchCapabilities,
		)
	}
	data, err := override.EncodeLaunchCapability(capability)
	if err != nil {
		return err
	}
	return writeCapabilityFile(
		filepath.Join(dir, capability.ID+".json"),
		data,
	)
}

func prepareCapabilityDirectory(dir string, requireRoot bool) error {
	if requireRoot {
		err := os.Mkdir(dir, 0o755)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create launch capability directory: %w", err)
		}
		if err := validateRootOwnedPath(dir, true); err != nil {
			return fmt.Errorf("validate launch capability directory: %w", err)
		}
		if err := syscall.Chmod(dir, 0o755); err != nil {
			return fmt.Errorf("set launch capability directory mode: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create test launch capability directory: %w", err)
	}
	return nil
}

func writeCapabilityFile(path string, data []byte) error {
	fd, err := syscall.Open(
		path,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create root-attested launch capability: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return errors.New("wrap launch capability descriptor")
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o644); err != nil {
		return fmt.Errorf("set launch capability mode: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write launch capability: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync launch capability: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close launch capability: %w", err)
	}
	removeOnFailure = false
	return nil
}

func consumeCapability(
	dir string,
	capability override.LaunchCapability,
	now time.Time,
	requireRoot bool,
) error {
	if err := capability.Validate(now); err != nil {
		return err
	}
	if requireRoot {
		if err := validateRootOwnedPath(dir, true); err != nil {
			return fmt.Errorf("validate launch capability directory: %w", err)
		}
	}
	lock, err := lockCapabilityDirectory(dir, requireRoot)
	if err != nil {
		return err
	}
	defer closeLockedFile(lock)
	path := filepath.Join(dir, capability.ID+".json")
	file, err := openLockedCapability(path)
	if err != nil {
		return err
	}
	defer closeLockedFile(file)
	if err := validateCapabilityFile(path, file, requireRoot); err != nil {
		return err
	}
	if err := matchCapabilityContents(file, capability); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("consume root-attested launch capability: %w", err)
	}
	return nil
}

func lockCapabilityDirectory(dir string, requireRoot bool) (*os.File, error) {
	path := filepath.Join(dir, launchCapabilityDirectoryLockName)
	fd, err := syscall.Open(
		path,
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, fmt.Errorf("open launch capability directory lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("wrap launch capability directory lock")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("set launch capability directory lock mode: %w", err)
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock launch capability directory: %w", err)
	}
	if err := validateCapabilityFile(path, file, requireRoot); err != nil {
		closeLockedFile(file)
		return nil, fmt.Errorf("validate launch capability directory lock: %w", err)
	}
	return file, nil
}

func closeLockedFile(file *os.File) {
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}

func pruneExpiredCapabilities(
	dir string,
	now time.Time,
	requireRoot bool,
) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("list launch capabilities: %w", err)
	}
	active := 0
	for _, entry := range entries {
		if entry.Name() == launchCapabilityDirectoryLockName {
			continue
		}
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			return 0, fmt.Errorf("unexpected entry in launch capability directory: %s", name)
		}
		id := name[:len(name)-len(".json")]
		if _, err := override.LaunchCapabilityPath(id); err != nil {
			return 0, fmt.Errorf("invalid launch capability sidecar %s: %w", name, err)
		}
		path := filepath.Join(dir, name)
		capability, err := readCapabilityFile(path, requireRoot)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("inspect launch capability %s: %w", name, err)
		}
		if !now.Before(capability.ExpiresUTC) {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return 0, fmt.Errorf("prune expired launch capability %s: %w", name, err)
			}
			continue
		}
		active++
	}
	return active, nil
}

func readCapabilityFile(
	path string,
	requireRoot bool,
) (override.LaunchCapability, error) {
	file, err := openLockedCapability(path)
	if err != nil {
		return override.LaunchCapability{}, err
	}
	defer closeLockedFile(file)
	if err := validateCapabilityFile(path, file, requireRoot); err != nil {
		return override.LaunchCapability{}, err
	}
	data, err := readCapabilityContents(file)
	if err != nil {
		return override.LaunchCapability{}, err
	}
	return override.DecodeLaunchCapability(data)
}

func openLockedCapability(path string) (*os.File, error) {
	fd, err := syscall.Open(
		path,
		syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open root-attested launch capability: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("wrap launch capability descriptor")
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock root-attested launch capability: %w", err)
	}
	return file, nil
}

func validateCapabilityFile(path string, file *os.File, requireRoot bool) error {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("revalidate root-attested launch capability path: %w", err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect root-attested launch capability: %w", err)
	}
	if !fileInfo.Mode().IsRegular() || !os.SameFile(pathInfo, fileInfo) {
		return errors.New("root-attested launch capability changed while being consumed")
	}
	if requireRoot {
		if err := validateRootOwnedInfo(path, fileInfo, false); err != nil {
			return fmt.Errorf("validate launch capability sidecar: %w", err)
		}
	}
	return nil
}

func matchCapabilityContents(
	file *os.File,
	capability override.LaunchCapability,
) error {
	data, err := readCapabilityContents(file)
	if err != nil {
		return err
	}
	want, err := override.EncodeLaunchCapability(capability)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, want) {
		return errors.New("root-attested launch capability does not match the consume request")
	}
	return nil
}

func readCapabilityContents(file *os.File) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, override.MaxLaunchCapabilityBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read root-attested launch capability: %w", err)
	}
	if len(data) > override.MaxLaunchCapabilityBytes {
		return nil, errors.New("root-attested launch capability exceeds its size limit")
	}
	return data, nil
}

func decodeTransaction(data []byte) ([]override.Use, []byte, int, error) {
	return override.DecodePrivilegedAppendRequest(data)
}

func appendRecords(
	path string,
	data []byte,
	uses []override.Use,
	now time.Time,
	requireRoot bool,
) error {
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
	if err := requireCompleteLedgerTail(file); err != nil {
		return err
	}
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

func requireCompleteLedgerTail(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect fixed ledger tail: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	var last [1]byte
	if _, err := file.ReadAt(last[:], info.Size()-1); err != nil {
		return fmt.Errorf("read fixed ledger tail: %w", err)
	}
	if last[0] != '\n' {
		return fmt.Errorf("%w: fixed ledger has an incomplete final record", override.ErrLedgerRecord)
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
			return nil, nil, fmt.Errorf("%w: fixed ledger contains a malformed record: %w",
				override.ErrLedgerRecord, err)
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
