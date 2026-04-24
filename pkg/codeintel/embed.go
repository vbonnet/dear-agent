package codeintel

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

//go:embed rules
var embeddedRules embed.FS

var (
	extractedRulesDir string
	extractOnce       sync.Once
	extractErr        error
)

// embeddedRulesDir extracts the embedded rules to a temporary directory
// and returns the path. The extracted directory is the parent of "rules/",
// so callers can join with "rules/<lang>/<file>.yml" as usual.
// The temp directory is created once and reused for the process lifetime.
func embeddedRulesDir() (string, error) {
	extractOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "codeintel-rules-*")
		if err != nil {
			extractErr = fmt.Errorf("creating temp dir for embedded rules: %w", err)
			return
		}

		err = fs.WalkDir(embeddedRules, "rules", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			dest := filepath.Join(tmpDir, path)
			if d.IsDir() {
				return os.MkdirAll(dest, 0o755)
			}
			data, err := embeddedRules.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading embedded %s: %w", path, err)
			}
			return os.WriteFile(dest, data, 0o644)
		})
		if err != nil {
			extractErr = fmt.Errorf("extracting embedded rules: %w", err)
			return
		}

		extractedRulesDir = tmpDir
	})
	return extractedRulesDir, extractErr
}
