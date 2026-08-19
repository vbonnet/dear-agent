package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"golang.org/x/sys/unix"
)

func clearLoadOverrideEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{
		"AGM_SESSIONS_DIR",
		"AGM_LOG_LEVEL",
		"AGM_LOG_FILE",
		"OPENCODE_SERVER_URL",
		"OPENCODE_ADAPTER_ENABLED",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadRejectsAmbiguousYAMLDocuments(t *testing.T) {
	clearLoadOverrideEnv(t)
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown root key",
			content: "sandox: {}\n",
			want:    "field sandox not found",
		},
		{
			name: "unknown sandbox key",
			content: `sandbox:
  repoz: []
`,
			want: "field repoz not found",
		},
		{
			name: "unknown deeply nested key",
			content: `adapters:
  opencode:
    reconnect:
      multiplir: 3
`,
			want: "field multiplir not found",
		},
		{
			name:    "unknown UI key",
			content: "ui:\n  picker_heigth: 20\n",
			want:    "field picker_heigth not found",
		},
		{
			name: "malformed known value",
			content: `timeout:
  tmux_commands: eventually
`,
			want: "cannot unmarshal",
		},
		{
			name:    "second document",
			content: "log_level: debug\n---\nlog_level: warn\n",
			want:    "multiple YAML documents",
		},
		{
			name:    "trailing empty document",
			content: "log_level: debug\n---\n",
			want:    "multiple YAML documents",
		},
		{
			name:    "empty file",
			content: "",
			want:    "configuration file is empty",
		},
		{
			name:    "comments only",
			content: "# no configuration\n",
			want:    "configuration file is empty",
		},
		{
			name:    "non mapping root",
			content: "- sandbox\n- repos\n",
			want:    "canonical YAML mapping",
		},
		{
			name:    "null root",
			content: "null\n",
			want:    "canonical YAML mapping",
		},
		{
			name:    "tilde null root",
			content: "~\n",
			want:    "canonical YAML mapping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			cfg, err := Load(path)
			if err == nil {
				t.Fatalf("Load() = %#v, nil; want error containing %q", cfg, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %q, want substring %q", err, tt.want)
			}
			if cfg != nil {
				t.Fatalf("Load() config = %#v, want nil on parse failure", cfg)
			}
		})
	}
}

func TestLoadStrictPartialConfigPreservesDefaultsAndExplicitEmptyRepos(t *testing.T) {
	clearLoadOverrideEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `log_level: debug
sandbox:
  repos: []
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.Timeout.TmuxCommands != 5*time.Second || !cfg.Timeout.Enabled {
		t.Fatalf("Timeout = %#v, want seeded defaults", cfg.Timeout)
	}
	if !cfg.Sandbox.Enabled {
		t.Fatal("Sandbox.Enabled = false, want seeded default true")
	}
	if len(cfg.Sandbox.Repos) != 0 {
		t.Fatalf("Sandbox.Repos = %v, want explicit empty compatibility", cfg.Sandbox.Repos)
	}
}

func TestLoadAcceptsOneMixedRuntimeAndUISnapshot(t *testing.T) {
	clearLoadOverrideEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := `log_level: debug
defaults:
  interactive: false
  cleanup_threshold_days: 60
ui:
  theme: agm-light
  picker_height: 20
  show_tags: false
advanced:
  tmux_timeout: 9s
sandbox:
  enabled: false
  provider: mock
  repos: []
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.LogLevel != "debug" || got.UISettings.UI.Theme != "agm-light" ||
		got.UISettings.UI.PickerHeight != 20 || got.UISettings.Defaults.Interactive ||
		got.UISettings.Defaults.CleanupThresholdDays != 60 || got.UISettings.UI.ShowTags ||
		got.UISettings.Advanced.TmuxTimeout != "9s" || got.Sandbox.Enabled ||
		got.Sandbox.Provider != "mock" {
		t.Fatalf("mixed snapshot = %#v", got)
	}
	if !got.UISettings.UI.ShowProjectPaths || !got.UISettings.UI.FuzzySearch {
		t.Fatalf("omitted UI defaults were not preserved: %#v", got.UISettings.UI)
	}
}

func TestLoadRejectsNoncanonicalSandboxAuthority(t *testing.T) {
	clearLoadOverrideEnv(t)
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "null sandbox", raw: "sandbox: null\n", want: "sandbox must be a canonical mapping"},
		{name: "yaml 1.1 bool spelling", raw: "sandbox: {enabled: no}\n", want: "canonical true or false boolean"},
		{name: "null bool", raw: "sandbox: {enabled: null}\n", want: "canonical true or false boolean"},
		{name: "tagged bool", raw: "sandbox: {enabled: !authority false}\n", want: "canonical true or false boolean"},
		{name: "blank repos", raw: "sandbox:\n  repos:\n", want: "sandbox.repos must be a sequence"},
		{name: "null repos", raw: "sandbox: {repos: null}\n", want: "sandbox.repos must be a sequence"},
		{name: "null repo item", raw: "sandbox: {repos: [null]}\n", want: "sandbox.repos[0]"},
		{name: "aliased null repo item", raw: "sandbox:\n  secrets: {nothing: &nothing null}\n  repos: [*nothing]\n", want: "sandbox.repos[0]"},
		{name: "binary repo item", raw: "sandbox: {repos: [!!binary Lw==]}\n", want: "sandbox.repos[0]"},
		{name: "tagged repo sequence", raw: "sandbox: {repos: !authority [/one]}\n", want: "sandbox.repos must be a sequence"},
		{name: "null writable dirs", raw: "sandbox: {writable_dirs: null}\n", want: "sandbox.writable_dirs must be a sequence"},
		{name: "tagged writable item", raw: "sandbox: {writable_dirs: [!authority /one]}\n", want: "sandbox.writable_dirs[0]"},
		{name: "null provider", raw: "sandbox: {provider: null}\n", want: "sandbox.provider must be a non-empty canonical string"},
		{name: "binary provider", raw: "sandbox: {provider: !!binary bW9jaw==}\n", want: "sandbox.provider must be a non-empty canonical string"},
		{name: "empty provider", raw: "sandbox: {provider: ''}\n", want: "sandbox.provider must be a non-empty canonical string"},
		{name: "merged null repos", raw: "sandbox:\n  <<: {repos: null}\n", want: "sandbox.repos must be a sequence"},
		{name: "merged null writable dirs", raw: "sandbox:\n  <<: {writable_dirs: null}\n", want: "sandbox.writable_dirs must be a sequence"},
		{name: "merged null provider", raw: "sandbox:\n  <<: {provider: null}\n", want: "sandbox.provider must be a non-empty canonical string"},
		{name: "aliased merge with null repos", raw: "sandbox:\n  <<: [ &bad {repos: null}, *bad ]\n", want: "sandbox.repos must be a sequence"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := Load(path)
			if err == nil || got != nil {
				t.Fatalf("Load() = %#v, %v; want rejection", got, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %q, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadPreservesSandboxAliasesAndMergePrecedence(t *testing.T) {
	clearLoadOverrideEnv(t)
	tests := []struct {
		name         string
		raw          string
		wantProvider string
		wantRepos    []string
		wantWritable []string
	}{
		{
			name:         "sequence and provider aliases",
			raw:          "sandbox:\n  provider: &provider overlayfs-native\n  repos: &paths [/one, /two]\n  writable_dirs: *paths\n  secrets: {provider_copy: *provider}\n",
			wantProvider: "overlayfs-native",
			wantRepos:    []string{"/one", "/two"},
			wantWritable: []string{"/one", "/two"},
		},
		{
			name:      "direct sequence overrides merged null",
			raw:       "sandbox:\n  <<: &authority {repos: null}\n  repos: []\n",
			wantRepos: []string{},
		},
		{
			name:      "merge sequence first wins",
			raw:       "sandbox:\n  <<: [ &valid {repos: [/first]}, &null {repos: null} ]\n",
			wantRepos: []string{"/first"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !slices.Equal(got.Sandbox.Repos, tt.wantRepos) {
				t.Fatalf("Sandbox.Repos = %v, want %v", got.Sandbox.Repos, tt.wantRepos)
			}
			if got.Sandbox.Repos == nil && tt.wantRepos != nil {
				t.Fatal("explicit empty repository sequence collapsed to nil")
			}
			if got.Sandbox.Provider != tt.wantProvider && tt.wantProvider != "" {
				t.Fatalf("Sandbox.Provider = %q, want %q", got.Sandbox.Provider, tt.wantProvider)
			}
			if !slices.Equal(got.Sandbox.WritableDirs, tt.wantWritable) {
				t.Fatalf("Sandbox.WritableDirs = %v, want %v", got.Sandbox.WritableDirs, tt.wantWritable)
			}
		})
	}
}

func TestLoadNormalizesLegacyUIZeroValues(t *testing.T) {
	clearLoadOverrideEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := `defaults:
  cleanup_threshold_days: 0
  archive_threshold_days: 0
ui:
  theme: ""
  picker_height: 0
  show_tags: false
`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultUISettings()
	if got.UISettings.UI.Theme != want.UI.Theme ||
		got.UISettings.UI.PickerHeight != want.UI.PickerHeight ||
		got.UISettings.Defaults.CleanupThresholdDays != want.Defaults.CleanupThresholdDays ||
		got.UISettings.Defaults.ArchiveThresholdDays != want.Defaults.ArchiveThresholdDays {
		t.Fatalf("normalized UI settings = %#v, want legacy scalar defaults %#v", got.UISettings, want)
	}
	if got.UISettings.UI.ShowTags {
		t.Fatal("explicit false UI boolean was not preserved")
	}
}

func TestLoadRejectsMissingOrDanglingSelectedSource(t *testing.T) {
	clearLoadOverrideEnv(t)
	root := t.TempDir()
	missing := filepath.Join(root, "missing.yaml")
	if got, err := Load(missing); err == nil || got != nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("Load(explicit missing) = %#v, %v; want path-qualified error", got, err)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "agm")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(configDir, "config.yaml")
	if err := os.Symlink(filepath.Join(configDir, "gone.yaml"), selected); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if got, err := Load(""); err == nil || got != nil || !strings.Contains(err.Error(), selected) {
		t.Fatalf("Load(dangling canonical source) = %#v, %v; want rejection", got, err)
	}

	intermediateHome := t.TempDir()
	t.Setenv("HOME", intermediateHome)
	if err := os.Symlink(filepath.Join(intermediateHome, "gone"), filepath.Join(intermediateHome, ".config")); err != nil {
		t.Skipf("intermediate symlink unavailable: %v", err)
	}
	if got, err := Load(""); err == nil || got != nil || !strings.Contains(err.Error(), "config.yaml") {
		t.Fatalf("Load(dangling intermediate source) = %#v, %v; want rejection", got, err)
	}
}

func TestLoadAcceptsRegularFileThroughSymlink(t *testing.T) {
	clearLoadOverrideEnv(t)
	root := t.TempDir()
	realPath := filepath.Join(root, "real.yaml")
	selectedPath := filepath.Join(root, "selected.yaml")
	if err := os.WriteFile(realPath, []byte("sandbox: {repos: []}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPath, selectedPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	got, err := Load(selectedPath)
	if err != nil {
		t.Fatalf("Load(regular symlink) error = %v", err)
	}
	if got == nil || got.Sandbox.Repos == nil || len(got.Sandbox.Repos) != 0 {
		t.Fatalf("Load(regular symlink) = %#v, want explicit empty repositories", got)
	}
}

func TestLoadUsesPhysicalHomeForSandboxAuthority(t *testing.T) {
	clearLoadOverrideEnv(t)
	root := t.TempDir()
	physical := filepath.Join(root, "physical")
	logical := filepath.Join(root, "logical")
	if err := os.MkdirAll(physical, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(physical, logical); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	physical, err := filepath.EvalSymlinks(physical)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", logical)
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, []byte("sandbox: {repos: [~/repo], writable_dirs: [~/work]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(physical, "repo")}; !slices.Equal(got.Sandbox.Repos, want) {
		t.Fatalf("Sandbox.Repos = %v, want %v", got.Sandbox.Repos, want)
	}
	if want := []string{filepath.Join(physical, "work")}; !slices.Equal(got.Sandbox.WritableDirs, want) {
		t.Fatalf("Sandbox.WritableDirs = %v, want %v", got.Sandbox.WritableDirs, want)
	}
}

func TestLoadRejectsUnsafeSandboxPathSpelling(t *testing.T) {
	clearLoadOverrideEnv(t)
	for _, raw := range []string{
		"sandbox: {repos: [relative/repo]}\n",
		"sandbox: {repos: [~/../escape]}\n",
		"sandbox: {writable_dirs: [./relative]}\n",
	} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := Load(path); err == nil || got != nil {
			t.Fatalf("Load(%q) = %#v, %v; want path rejection", raw, got, err)
		}
	}
}

func TestLoadRejectsUnboundedOrNonregularSource(t *testing.T) {
	clearLoadOverrideEnv(t)
	oversize := filepath.Join(t.TempDir(), "oversize.yaml")
	if err := os.WriteFile(oversize, bytes.Repeat([]byte{'x'}, maxConfigBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(oversize); err == nil || got != nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Load(oversize) = %#v, %v; want size rejection", got, err)
	}

	fifo := filepath.Join(t.TempDir(), "config.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}
	if got, err := Load(fifo); err == nil || got != nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Load(FIFO) = %#v, %v; want nonregular rejection", got, err)
	}
}

func TestReadBoundedConfigPropagatesPartialReadError(t *testing.T) {
	sentinel := errors.New("sentinel read failure")
	reader := io.MultiReader(strings.NewReader("{}\n"), iotest.ErrReader(sentinel))
	got, err := readBoundedConfig(reader, 3)
	if !errors.Is(err, sentinel) || got != nil {
		t.Fatalf("readBoundedConfig() = %q, %v; want nil and sentinel", got, err)
	}
}

func TestReadBoundedConfigEnforcesObservedSize(t *testing.T) {
	exact := bytes.Repeat([]byte{'x'}, maxConfigBytes)
	got, err := readBoundedConfig(bytes.NewReader(exact), int64(len(exact)))
	if err != nil || !bytes.Equal(got, exact) {
		t.Fatalf("readBoundedConfig(exact limit) = %d bytes, %v; want success", len(got), err)
	}

	growing := bytes.Repeat([]byte{'x'}, maxConfigBytes+1)
	got, err = readBoundedConfig(bytes.NewReader(growing), maxConfigBytes)
	if err == nil || got != nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("readBoundedConfig(growing source) = %d bytes, %v; want bounded rejection", len(got), err)
	}
}

func TestReadBoundedConfigRejectsTruncatedRestrictiveSnapshot(t *testing.T) {
	restrictive := []byte("log_level: info\nsandbox:\n  repos: [/safe]\n")
	permissivePrefix := []byte("log_level: info\n")
	got, err := readBoundedConfig(bytes.NewReader(permissivePrefix), int64(len(restrictive)))
	if err == nil || got != nil || !strings.Contains(err.Error(), "changed while reading") {
		t.Fatalf(
			"readBoundedConfig(truncated authority) = %q, %v; want size-stability rejection",
			got, err,
		)
	}
}

func TestLoadPropagatesNonMissingReadFailures(t *testing.T) {
	clearLoadOverrideEnv(t)
	cfg, err := Load(t.TempDir())
	if err == nil {
		t.Fatalf("Load(directory) = %#v, nil; want read failure", cfg)
	}
	if !strings.Contains(err.Error(), "failed to read config file") {
		t.Fatalf("Load(directory) error = %q, want read failure context", err)
	}
	if cfg != nil {
		t.Fatalf("Load(directory) config = %#v, want nil", cfg)
	}
}
