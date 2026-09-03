package testcontext

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const maxAuthConfigBytes int64 = 1 << 20

var credentialProjectionPaths = [...]string{
	filepath.Join(".claude", ".credentials.json"),
	filepath.Join(".codex", "auth.json"),
	filepath.Join(".local", "share", "opencode", "auth.json"),
	filepath.Join(".config", "gcloud", "application_default_credentials.json"),
}

var configSnapshotProjectionPaths = [...]string{
	filepath.Join(".config", "gcloud", "configurations", "config_default"),
	filepath.Join(".config", "opencode", "opencode.json"),
	filepath.Join(".config", "opencode", "opencode.jsonc"),
	filepath.Join(".config", "opencode", "tui.json"),
	filepath.Join(".config", "opencode", "tui.jsonc"),
}

// authNamespacePaths is shallow-to-deep so apply never relies on recursive
// creation and rollback can remove exact current-call directories in reverse.
var authNamespacePaths = [...]string{
	".claude",
	".codex",
	".config",
	".local",
	filepath.Join(".config", "gcloud"),
	filepath.Join(".config", "opencode"),
	filepath.Join(".local", "share"),
	filepath.Join(".config", "gcloud", "configurations"),
	filepath.Join(".local", "share", "opencode"),
}

type preparedCredentialLink struct {
	relativePath string
	sourceInfo   os.FileInfo
}

type preparedConfigSnapshot struct {
	relativePath string
	data         []byte
}

type preparedAuthDirectory struct {
	relativePath string
	existingInfo os.FileInfo
}

type authProjectionPlan struct {
	hostHome         string
	selectedHomeInfo os.FileInfo
	links            []preparedCredentialLink
	snapshots        []preparedConfigSnapshot
	directories      []preparedAuthDirectory
}

type authInstallHook func(relativePath string) error

func projectInheritedAuth(hostHome, isolatedHome string) error {
	return projectInheritedAuthWithHook(hostHome, isolatedHome, nil)
}

func projectInheritedAuthWithHook(hostHome, isolatedHome string, hook authInstallHook) error {
	return projectInheritedAuthWithHooks(hostHome, isolatedHome, hook, nil)
}

func projectInheritedAuthWithHooks(
	hostHome, isolatedHome string,
	installHook authInstallHook,
	snapshotHook snapshotReadHook,
) error {
	plan, err := prepareAuthProjectionWithSnapshotHook(hostHome, isolatedHome, snapshotHook)
	if err != nil {
		return err
	}

	tx, err := newAuthProjectionTransaction(isolatedHome, plan.selectedHomeInfo)
	if err != nil {
		return err
	}
	plan.directories, err = tx.preflight(plan.links, plan.snapshots)
	if err != nil {
		return errors.Join(err, tx.close())
	}
	if err := tx.apply(plan, installHook); err != nil {
		return errors.Join(err, tx.rollback(), tx.close())
	}
	return tx.close()
}

func prepareAuthProjectionWithSnapshotHook(
	hostHome, isolatedHome string,
	hook snapshotReadHook,
) (authProjectionPlan, error) {
	selectedHomeInfo, err := validateProjectionRoots(hostHome, isolatedHome)
	if err != nil {
		return authProjectionPlan{}, err
	}

	plan := authProjectionPlan{
		hostHome:         hostHome,
		selectedHomeInfo: selectedHomeInfo,
	}
	for _, relativePath := range credentialProjectionPaths {
		file, info, present, err := openApprovedAuthSource(hostHome, relativePath, true)
		if err != nil {
			return authProjectionPlan{}, err
		}
		if !present {
			continue
		}
		if err := file.Close(); err != nil {
			return authProjectionPlan{}, authPathError("close credential source", relativePath, err)
		}
		plan.links = append(plan.links, preparedCredentialLink{
			relativePath: relativePath,
			sourceInfo:   info,
		})
	}

	for _, relativePath := range configSnapshotProjectionPaths {
		data, present, err := prepareConfigSnapshotWithHook(hostHome, relativePath, hook)
		if err != nil {
			return authProjectionPlan{}, err
		}
		if present {
			plan.snapshots = append(plan.snapshots, preparedConfigSnapshot{
				relativePath: relativePath,
				data:         data,
			})
		}
	}
	return plan, nil
}

func validateProjectionRoots(hostHome, isolatedHome string) (os.FileInfo, error) {
	for _, root := range []struct {
		label string
		path  string
	}{
		{label: "host home", path: hostHome},
		{label: "selected home", path: isolatedHome},
	} {
		if !filepath.IsAbs(root.path) || filepath.Clean(root.path) != root.path {
			return nil, fmt.Errorf("%s must be a canonical absolute path", root.label)
		}
	}

	hostInfo, err := os.Lstat(hostHome)
	if err != nil {
		return nil, authPathError("inspect host home", ".", err)
	}
	if err := validateOwnedDirectoryInfo(hostInfo, false, "host home"); err != nil {
		return nil, err
	}
	isolatedInfo, err := os.Lstat(isolatedHome)
	if err != nil {
		return nil, authPathError("inspect selected home", ".", err)
	}
	if err := validateOwnedDirectoryInfo(isolatedInfo, true, "selected home"); err != nil {
		return nil, err
	}
	if os.SameFile(hostInfo, isolatedInfo) {
		return nil, errors.New("host home and selected home must be distinct")
	}

	resolvedHost, err := filepath.EvalSymlinks(hostHome)
	if err != nil {
		return nil, authPathError("resolve host home", ".", err)
	}
	resolvedIsolated, err := filepath.EvalSymlinks(isolatedHome)
	if err != nil {
		return nil, authPathError("resolve selected home", ".", err)
	}
	if pathsOverlap(resolvedHost, resolvedIsolated) {
		return nil, errors.New("host home and selected home must not contain one another")
	}
	return isolatedInfo, nil
}

func pathsOverlap(left, right string) bool {
	contains := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		if err != nil || filepath.IsAbs(relative) {
			return false
		}
		return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)))
	}
	return contains(left, right) || contains(right, left)
}

func validateOwnedDirectoryInfo(info os.FileInfo, exactPrivate bool, label string) error {
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a real directory", label)
	}
	if !ownedByEffectiveUser(info) {
		return fmt.Errorf("%s must be owned by the effective user", label)
	}
	if exactPrivate {
		if info.Mode().Perm() != 0700 {
			return fmt.Errorf("%s must have mode 0700", label)
		}
	} else if info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("%s must not be writable by group or other", label)
	}
	return nil
}

func validateAuthLeafInfo(info os.FileInfo, credential bool, relativePath string) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("auth source %s must be a regular file", relativePath)
	}
	if !ownedByEffectiveUser(info) {
		return fmt.Errorf("auth source %s must be owned by the effective user", relativePath)
	}
	permissions := info.Mode().Perm()
	if credential && permissions&0077 != 0 {
		return fmt.Errorf("credential source %s must be owner-private", relativePath)
	}
	if !credential && permissions&0022 != 0 {
		return fmt.Errorf("configuration source %s must not be writable by group or other", relativePath)
	}
	return nil
}

func ownedByEffectiveUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	return stat.Uid == uint32(os.Geteuid())
}

func sameAuthFileState(left, right os.FileInfo) bool {
	return os.SameFile(left, right) &&
		left.Mode() == right.Mode() &&
		left.Size() == right.Size() &&
		left.ModTime().Equal(right.ModTime())
}

func authDirectoryNeeded(
	relativeDirectory string,
	links []preparedCredentialLink,
	snapshots []preparedConfigSnapshot,
) bool {
	prefix := relativeDirectory + string(os.PathSeparator)
	for _, link := range links {
		if strings.HasPrefix(link.relativePath, prefix) {
			return true
		}
	}
	for _, snapshot := range snapshots {
		if strings.HasPrefix(snapshot.relativePath, prefix) {
			return true
		}
	}
	return false
}

func allAuthLeafPaths() []string {
	paths := make([]string, 0, len(credentialProjectionPaths)+len(configSnapshotProjectionPaths))
	paths = append(paths, credentialProjectionPaths[:]...)
	paths = append(paths, configSnapshotProjectionPaths[:]...)
	return paths
}

func authPathError(operation, relativePath string, err error) error {
	if pathError, ok := errors.AsType[*os.PathError](err); ok {
		err = pathError.Err
	}
	if linkError, ok := errors.AsType[*os.LinkError](err); ok {
		err = linkError.Err
	}
	return fmt.Errorf("%s %s: %w", operation, relativePath, err)
}
