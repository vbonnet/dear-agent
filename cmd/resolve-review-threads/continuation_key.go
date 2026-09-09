package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
)

const (
	continuationReceiptKeyBytes = 32
	continuationReceiptKeyFile  = "continuation-receipt-hmac-v1.key"
)

var continuationReceiptPlatformTestOverride *bool

func continuationReceiptKeyPath() (string, error) {
	stateRoot, _, err := continuationReceiptStateRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(
		stateRoot,
		"dear-agent",
		"resolve-review-threads",
		continuationReceiptKeyFile,
	), nil
}

func continuationReceiptStateRoot() (string, bool, error) {
	if err := validateContinuationReceiptPlatform(runtime.GOOS, continuationReceiptPlatformIsSupported()); err != nil {
		return "", false, err
	}
	stateRoot := os.Getenv("XDG_STATE_HOME")
	if stateRoot != "" {
		if !filepath.IsAbs(stateRoot) {
			return "", false, errors.New("resolve continuation receipt state home must be an absolute path")
		}
		return filepath.Clean(stateRoot), true, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolve continuation receipt state home: %w", err)
	}
	if !filepath.IsAbs(home) {
		return "", false, errors.New("resolve continuation receipt home must be an absolute path")
	}
	return filepath.Join(home, ".local", "state"), false, nil
}

func continuationReceiptPlatformIsSupported() bool {
	if continuationReceiptPlatformTestOverride != nil {
		return *continuationReceiptPlatformTestOverride
	}
	return continuationReceiptPlatformDefaultSupported
}

func validateContinuationReceiptPlatform(goos string, supported bool) error {
	if supported {
		return nil
	}
	return fmt.Errorf(
		"resolve continuation receipts are unsupported on %s because owner-private key state and durable directory entries cannot be established",
		goos,
	)
}

func loadOrCreateContinuationReceiptKey() ([]byte, error) {
	path, err := continuationReceiptKeyPath()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := ensurePrivateContinuationDirectory(dir); err != nil {
		return nil, fmt.Errorf("prepare continuation receipt key directory: %w", err)
	}
	key, err := readContinuationReceiptKey(path)
	if err == nil {
		if syncErr := syncContinuationReceiptKeyDirectory(dir); syncErr != nil {
			return nil, syncErr
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return createContinuationReceiptKey(path, dir)
}

func createContinuationReceiptKey(path, dir string) ([]byte, error) {
	if err := validateContinuationReceiptKeyDirectory(dir); err != nil {
		return nil, err
	}
	key := make([]byte, continuationReceiptKeyBytes)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate continuation receipt key: %w", err)
	}
	tempPath, err := writeContinuationReceiptKeyCandidate(dir, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tempPath) }()
	return installContinuationReceiptKey(path, dir, tempPath)
}

func writeContinuationReceiptKeyCandidate(dir string, key []byte) (string, error) {
	file, err := os.CreateTemp(dir, ".continuation-receipt-key-*")
	if err != nil {
		return "", fmt.Errorf("create continuation receipt key candidate: %w", err)
	}
	tempPath := file.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("protect continuation receipt key candidate: %w", err)
	}
	written, writeErr := file.Write(key)
	if writeErr == nil && written != len(key) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return "", fmt.Errorf("write continuation receipt key: %w", writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close continuation receipt key: %w", closeErr)
	}
	keep = true
	return tempPath, nil
}

func installContinuationReceiptKey(path, dir, tempPath string) ([]byte, error) {
	// Linking a complete, fsynced candidate installs the key without replacing
	// a concurrent winner. Readers can observe either no final path or the whole
	// immutable key, never a partially written final file.
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			if syncErr := syncContinuationReceiptKeyDirectory(dir); syncErr != nil {
				return nil, syncErr
			}
			return readContinuationReceiptKey(path)
		}
		return nil, fmt.Errorf("install continuation receipt key without replacement: %w", err)
	}
	if err := syncContinuationReceiptKeyDirectory(dir); err != nil {
		return nil, err
	}
	return readContinuationReceiptKey(path)
}

func loadContinuationReceiptKey() ([]byte, error) {
	path, err := continuationReceiptKeyPath()
	if err != nil {
		return nil, err
	}
	return readContinuationReceiptKey(path)
}

func readContinuationReceiptKey(path string) ([]byte, error) {
	dir := filepath.Dir(path)
	if err := validateContinuationReceiptKeyDirectory(dir); err != nil {
		return nil, err
	}
	file, err := openContinuationReceiptKey(path)
	if err != nil {
		return nil, fmt.Errorf("open continuation receipt key: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened continuation receipt key: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("continuation receipt key must be a regular file, not a symlink or special object")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("continuation receipt key permissions must be 0600, got %04o", info.Mode().Perm())
	}
	key, err := io.ReadAll(io.LimitReader(file, continuationReceiptKeyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read continuation receipt key: %w", err)
	}
	if len(key) != continuationReceiptKeyBytes {
		return nil, fmt.Errorf("continuation receipt key must contain exactly %d bytes", continuationReceiptKeyBytes)
	}
	return key, nil
}

func validateContinuationReceiptKeyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect continuation receipt key directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("continuation receipt key directory must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("continuation receipt key directory permissions %04o are not private", info.Mode().Perm())
	}
	return nil
}

func ensurePrivateContinuationDirectory(path string) error {
	plan, err := buildContinuationReceiptDirectoryPlan(path)
	if err != nil {
		return err
	}
	managed := []string{filepath.Dir(plan.target), plan.target}
	if err := validateExistingContinuationDirectories(managed); err != nil {
		return err
	}
	if err := ensurePrivateContinuationDirectoryWithSync(
		plan.anchor,
		plan.target,
		syncContinuationReceiptKeyDirectory,
	); err != nil {
		return err
	}
	// The configured XDG hierarchy can contain provider-owned or user-selected
	// ancestors, but these two command-owned directories must never be symlinks
	// or shared paths, including when they predate this invocation.
	for _, path := range managed {
		if err := validateContinuationReceiptKeyDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func validateExistingContinuationDirectories(paths []string) error {
	for _, path := range paths {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect continuation receipt key directory: %w", err)
		}
		if err := validateContinuationReceiptKeyDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

type continuationReceiptDirectoryPlan struct {
	anchor string
	target string
}

func buildContinuationReceiptDirectoryPlan(path string) (continuationReceiptDirectoryPlan, error) {
	stateRoot, explicitXDG, err := continuationReceiptStateRoot()
	if err != nil {
		return continuationReceiptDirectoryPlan{}, err
	}
	target := filepath.Join(stateRoot, "dear-agent", "resolve-review-threads")
	if filepath.Clean(path) != target {
		return continuationReceiptDirectoryPlan{}, fmt.Errorf(
			"continuation receipt directory %s does not match configured state location %s",
			path,
			target,
		)
	}
	if explicitXDG {
		volume := filepath.VolumeName(target)
		anchor := volume + string(filepath.Separator)
		if volume == "" {
			anchor = string(filepath.Separator)
		}
		return continuationReceiptDirectoryPlan{anchor: anchor, target: target}, nil
	}
	// The user's home is the documented pre-existing boundary for the fallback
	// ~/.local/state location. Everything below it may have been created by a
	// prior issuance attempt and therefore needs its parent entry re-synced.
	home := filepath.Dir(filepath.Dir(stateRoot))
	return continuationReceiptDirectoryPlan{anchor: home, target: target}, nil
}

func ensurePrivateContinuationDirectoryWithSync(
	anchor string,
	path string,
	syncDirectory func(string) error,
) error {
	anchor = filepath.Clean(anchor)
	path = filepath.Clean(path)
	relative, err := filepath.Rel(anchor, path)
	if err != nil {
		return fmt.Errorf("relate continuation state directory to durability anchor: %w", err)
	}
	if relative == ".." || filepath.IsAbs(relative) {
		return fmt.Errorf("continuation state directory %s is outside durability anchor %s", path, anchor)
	}
	firstRelativeComponent := ".." + string(filepath.Separator)
	if len(relative) >= len(firstRelativeComponent) && relative[:len(firstRelativeComponent)] == firstRelativeComponent {
		return fmt.Errorf("continuation state directory %s is outside durability anchor %s", path, anchor)
	}
	anchorInfo, err := os.Stat(anchor)
	if err != nil {
		return fmt.Errorf("inspect continuation state durability anchor %s: %w", anchor, err)
	}
	if !anchorInfo.IsDir() {
		return fmt.Errorf("continuation state durability anchor %s is not a directory", anchor)
	}

	components := make([]string, 0, 6)
	for current := path; current != anchor; current = filepath.Dir(current) {
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("continuation state directory %s is outside durability anchor %s", path, anchor)
		}
		components = append(components, current)
	}
	for _, current := range slices.Backward(components) {
		wasMissing, err := ensureContinuationDirectoryComponent(current)
		if err != nil {
			return err
		}
		if wasMissing {
			if err := validateContinuationReceiptKeyDirectory(current); err != nil {
				return err
			}
		}
		parent := filepath.Dir(current)
		if err := syncDirectory(parent); err != nil {
			return fmt.Errorf("persist continuation state directory entry %s: %w", current, err)
		}
	}
	return validateContinuationReceiptKeyDirectory(path)
}

func ensureContinuationDirectoryComponent(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("continuation state parent %s is not a directory", path)
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect continuation state directory %s: %w", path, err)
	}
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return false, fmt.Errorf("create continuation state directory %s: %w", path, err)
	}
	info, err = os.Lstat(path)
	if err != nil {
		return false, fmt.Errorf("inspect created continuation state directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("continuation state parent %s is not a directory", path)
	}
	return true, nil
}
