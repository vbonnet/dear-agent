package hookparity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCodeLegacyProjectionIsInactive(t *testing.T) {
	for _, test := range []struct {
		name     string
		manifest string
		present  bool
		want     bool
	}{
		{name: "absent", want: true},
		{name: "retirement tombstone", present: true, want: true, manifest: `{"_dear_agent":{"status":"retired","replacement":".opencode/plugins/spec-contract-guard.mjs","runtime_claim":"none"}}`},
		{name: "legacy hooks", present: true, manifest: `{"hooks":{"Stop":[]}}`},
		{name: "tombstone with hooks", present: true, manifest: `{"_dear_agent":{"status":"retired","replacement":".opencode/plugins/spec-contract-guard.mjs","runtime_claim":"none"},"hooks":{}}`},
		{name: "unapproved replacement", present: true, manifest: `{"_dear_agent":{"status":"retired","replacement":"other.mjs","runtime_claim":"none"}}`},
		{name: "runtime claim", present: true, manifest: `{"_dear_agent":{"status":"retired","replacement":".opencode/plugins/spec-contract-guard.mjs","runtime_claim":"installed"}}`},
		{name: "extra metadata", present: true, manifest: `{"_dear_agent":{"status":"retired","replacement":".opencode/plugins/spec-contract-guard.mjs","runtime_claim":"none","extra":true}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.present {
				directory := filepath.Join(root, ".opencode")
				if err := os.Mkdir(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "hooks.json"), []byte(test.manifest), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := OpenCodeLegacyProjectionIsInactive(root)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("OpenCodeLegacyProjectionIsInactive() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestOpenCodeLegacyProjectionRejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, ".opencode")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "hooks.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if inactive, err := OpenCodeLegacyProjectionIsInactive(root); err == nil || inactive {
		t.Fatalf("malformed projection = (%t, %v), want explicit error", inactive, err)
	}
}

func TestOpenCodeLegacyProjectionRejectsDuplicateKeys(t *testing.T) {
	for _, manifest := range []string{
		`{"_dear_agent":{"status":"active","status":"retired","replacement":".opencode/plugins/spec-contract-guard.mjs","runtime_claim":"none"}}`,
		`{"_dear_agent":{"status":"active"},"_dear_agent":{"status":"retired","replacement":".opencode/plugins/spec-contract-guard.mjs","runtime_claim":"none"}}`,
	} {
		root := t.TempDir()
		directory := filepath.Join(root, ".opencode")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "hooks.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		if inactive, err := OpenCodeLegacyProjectionIsInactive(root); err == nil || inactive {
			t.Fatalf("duplicate-key projection = (%t, %v), want explicit error", inactive, err)
		}
	}
}

func TestOpenCodeLegacyProjectionRejectsSymlinkIdentities(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
	}{
		{name: "valid target", target: "tombstone.json"},
		{name: "dangling target", target: "missing.json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, ".opencode")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if test.target == "tombstone.json" {
				body := `{"_dear_agent":{"status":"retired","replacement":".opencode/plugins/spec-contract-guard.mjs","runtime_claim":"none"}}`
				if err := os.WriteFile(filepath.Join(root, test.target), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(filepath.Join(root, test.target), filepath.Join(directory, "hooks.json")); err != nil {
				t.Skipf("symlink fixture unavailable: %v", err)
			}
			if inactive, err := OpenCodeLegacyProjectionIsInactive(root); err != nil || inactive {
				t.Fatalf("symlink projection = (%t, %v), want present and inactive=false", inactive, err)
			}
		})
	}
}
