//go:build darwin || linux

package marketplaceparity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJSONWithinAppliesManifestLimitAtRoot(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"padding":"` + strings.Repeat("x", int(maxManifestBytes)) + `"}`)
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	err := readJSONWithin(root, "plugin.json", &decoded)
	want := fmt.Sprintf("exceeds the %d-byte bound", maxManifestBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("readJSONWithin() error = %v, want %q", err, want)
	}
}
