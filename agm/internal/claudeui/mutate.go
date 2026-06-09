package claudeui

import (
	"fmt"
	"os"
	"path/filepath"
)

// Backup copies the session file verbatim into backupDir, preserving its
// filename. The copy is byte-identical to the original so a manual restore
// reproduces the exact pre-mutation file. backupDir is created lazily.
func (s *Session) Backup(backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	dst := filepath.Join(backupDir, filepath.Base(s.Path))
	info, err := os.Stat(s.Path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, s.raw, info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	return dst, nil
}

// SetArchived flips isArchived to value, writing the file atomically while
// preserving every other field and the original key order/formatting.
//
// It is a surgical replacement of the single isArchived token, so:
//   - the write is byte-minimal (only true<->false changes);
//   - it is idempotent: if the file is already at value it is a no-op and
//     returns changed=false without touching disk or taking a backup;
//   - --unarchive is exactly byte-reversible against --apply.
//
// When backup is true the original is copied to backupDir before mutation.
func (s *Session) SetArchived(value, backup bool, backupDir string) (changed bool, backupPath string, err error) {
	if s.IsArchived == value {
		return false, "", nil // idempotent no-op
	}

	if backup {
		backupPath, err = s.Backup(backupDir)
		if err != nil {
			return false, "", err
		}
	}

	newBool := "false"
	if value {
		newBool = "true"
	}
	updated := archivedTokenRe.ReplaceAll(s.raw, []byte(`"isArchived":`+newBool))

	if err := atomicWrite(s.Path, updated); err != nil {
		return false, backupPath, err
	}

	s.raw = updated
	s.IsArchived = value
	return true, backupPath, nil
}

// atomicWrite writes data to a temp file in the target's directory and renames
// it over the destination, so a crash mid-write cannot leave a truncated file.
// The destination's permission bits are preserved.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".claudeui-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
