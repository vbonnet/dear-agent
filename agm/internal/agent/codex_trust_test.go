package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

// TestEnsureCodexWorkdirTrusted covers the trust pre-write that keeps Codex
// launches from bricking in fresh non-git sandbox dirs (ce-cmsq): the entry is
// created when missing, existing user decisions are never overwritten, and a
// broken config is never touched.
func TestEnsureCodexWorkdirTrusted(t *testing.T) {
	tests := []struct {
		name string
		// initial is the pre-existing config.toml content; nil means no file.
		// The literal {DIR} is replaced with the workdir under test.
		initial *string
		// wantChange is whether the config file is expected to be modified.
		wantChange bool
		// wantErr is a substring of the expected error ("" = no error).
		wantErr string
	}{
		{
			name:       "no config file: creates one with a trusted entry",
			initial:    nil,
			wantChange: true,
		},
		{
			name:       "empty config: appends a trusted entry",
			initial:    new(""),
			wantChange: true,
		},
		{
			name: "config without entry: appends and preserves existing content",
			initial: new("model = \"gpt-5.5\"\n\n[projects.\"/some/other/dir\"]\n" +
				"trust_level = \"trusted\"\n"),
			wantChange: true,
		},
		{
			name:       "config without trailing newline: still valid after append",
			initial:    new("model = \"gpt-5.5\""),
			wantChange: true,
		},
		{
			name:       "entry already trusted: no-op",
			initial:    new("[projects.\"{DIR}\"]\ntrust_level = \"trusted\"\n"),
			wantChange: false,
		},
		{
			name:       "entry explicitly untrusted: user decision is preserved",
			initial:    new("[projects.\"{DIR}\"]\ntrust_level = \"untrusted\"\n"),
			wantChange: false,
		},
		{
			name:       "malformed config: error and file untouched",
			initial:    new("[projects.\"broken\"\nnot toml"),
			wantChange: false,
			wantErr:    "parse codex config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codexHome := t.TempDir()
			t.Setenv("CODEX_HOME", codexHome)
			workDir := t.TempDir()
			configPath := filepath.Join(codexHome, "config.toml")

			var before string
			if tt.initial != nil {
				before = strings.ReplaceAll(*tt.initial, "{DIR}", workDir)
				if err := os.WriteFile(configPath, []byte(before), 0o600); err != nil {
					t.Fatalf("seed config: %v", err)
				}
			}

			err := EnsureCodexWorkdirTrusted(workDir)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("EnsureCodexWorkdirTrusted: %v", err)
			}

			after, readErr := os.ReadFile(configPath)
			if readErr != nil {
				if tt.initial == nil && !tt.wantChange {
					return // no file before, none expected after
				}
				t.Fatalf("read config after: %v", readErr)
			}

			if !tt.wantChange {
				if string(after) != before {
					t.Fatalf("config changed unexpectedly:\nbefore: %q\nafter:  %q", before, string(after))
				}
				return
			}

			// The result must be valid TOML with the workdir trusted and all
			// prior entries intact.
			assertTrusted(t, after, workDir)
			if tt.initial != nil && strings.Contains(before, "/some/other/dir") {
				assertTrusted(t, after, "/some/other/dir")
			}
			if tt.initial != nil && strings.Contains(before, "model = ") {
				if !strings.Contains(string(after), `model = "gpt-5.5"`) {
					t.Fatalf("existing config content lost: %q", string(after))
				}
			}
		})
	}
}

// TestEnsureCodexWorkdirTrusted_Idempotent verifies a second call does not
// duplicate the projects table (a duplicate TOML table would break every
// subsequent codex launch).
func TestEnsureCodexWorkdirTrusted_Idempotent(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	workDir := t.TempDir()

	for i := range 2 {
		if err := EnsureCodexWorkdirTrusted(workDir); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	header := fmt.Sprintf("[projects.%s]", tomlQuoteKey(workDir))
	if got := strings.Count(string(data), header); got != 1 {
		t.Fatalf("projects table appears %d times, want 1:\n%s", got, string(data))
	}
	assertTrusted(t, data, workDir)
}

// TestEnsureCodexWorkdirTrusted_EscapesSpecialPathChars verifies TOML key
// escaping for directories containing quotes or backslashes.
func TestEnsureCodexWorkdirTrusted_EscapesSpecialPathChars(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	workDir := filepath.Join(t.TempDir(), `we"ird\name`)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := EnsureCodexWorkdirTrusted(workDir); err != nil {
		t.Fatalf("EnsureCodexWorkdirTrusted: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	assertTrusted(t, data, workDir)
}

// TestEnsureCodexWorkdirTrusted_DefaultsToHomeCodex verifies the default
// config location is ~/.codex/config.toml when CODEX_HOME is unset.
func TestEnsureCodexWorkdirTrusted_DefaultsToHomeCodex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	workDir := t.TempDir()

	if err := EnsureCodexWorkdirTrusted(workDir); err != nil {
		t.Fatalf("EnsureCodexWorkdirTrusted: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("config not written under ~/.codex: %v", err)
	}
	assertTrusted(t, data, workDir)
}

// TestEnsureCodexWorkdirTrusted_StealsStaleLock verifies a lock left behind by
// a crashed holder does not brick trust writes forever: a lock older than the
// staleness window is stolen and the write proceeds.
func TestEnsureCodexWorkdirTrusted_StealsStaleLock(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	workDir := t.TempDir()

	lockPath := filepath.Join(codexHome, "config.toml.agm-lock")
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	stale := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatalf("age lock: %v", err)
	}

	if err := EnsureCodexWorkdirTrusted(workDir); err != nil {
		t.Fatalf("EnsureCodexWorkdirTrusted with stale lock: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	assertTrusted(t, data, workDir)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("stale lock not cleaned up: %v", err)
	}
}

// TestCodexCreateSessionPreTrustsWorkdir verifies the adapter launch path
// records trust for the workdir before the codex command is sent (ce-cmsq).
func TestCodexCreateSessionPreTrustsWorkdir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)

	origLookPath := lookPath
	origHasSession := codexHasSession
	origNewSession := codexNewSession
	origSendCommand := codexSendCommand
	origWaitForPrompt := codexWaitForPrompt
	t.Cleanup(func() {
		lookPath = origLookPath
		codexHasSession = origHasSession
		codexNewSession = origNewSession
		codexSendCommand = origSendCommand
		codexWaitForPrompt = origWaitForPrompt
	})

	lookPath = func(file string) (string, error) { return "/fake/" + file, nil }
	codexHasSession = func(string) (bool, error) { return false, nil }
	codexNewSession = func(string, string) error { return nil }
	codexWaitForPrompt = func(string, time.Duration) error { return nil }

	workDir := t.TempDir()
	trustedWhenSent := false
	codexSendCommand = func(_ string, cmd string) error {
		if strings.Contains(cmd, "agm __exec-codex") {
			data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
			trustedWhenSent = err == nil && projectTrusted(t, data, workDir)
		}
		return nil
	}

	adapter := &CodexCLIAdapter{sessionStore: &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}}
	_, err := adapter.CreateSession(SessionContext{
		Name:             "codex-trust-test",
		WorkingDirectory: workDir,
		Environment:      map[string]string{"AGM_MODEL": "5.4"},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if !trustedWhenSent {
		t.Fatal("workdir was not trusted in codex config before the launch command was sent")
	}
}

func assertTrusted(t *testing.T, data []byte, dir string) {
	t.Helper()
	if !projectTrusted(t, data, dir) {
		t.Fatalf("config does not trust %s:\n%s", dir, string(data))
	}
}

func projectTrusted(t *testing.T, data []byte, dir string) bool {
	t.Helper()
	var cfg struct {
		Projects map[string]struct {
			TrustLevel string `toml:"trust_level"`
		} `toml:"projects"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("appended config is not valid TOML: %v\n%s", err, string(data))
	}
	return cfg.Projects[dir].TrustLevel == "trusted"
}
