package agenticreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentic-review.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigReadsEveryKnob(t *testing.T) {
	path := writeConfig(t, `
families:
  - claude
  - codex
  - gemini
quorum: 2
verdict-timeout: 45m
dispatch-timeout: 30m
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Families) != 3 || cfg.Families[1] != FamilyCodex {
		t.Fatalf("families = %v, want the three defaults in order", cfg.Families)
	}
	if cfg.Quorum != 2 {
		t.Fatalf("quorum = %d, want 2", cfg.Quorum)
	}
	if cfg.VerdictTimeout != 45*time.Minute || cfg.DispatchTimeout != 30*time.Minute {
		t.Fatalf("timeouts = %s/%s, want 45m/30m", cfg.VerdictTimeout, cfg.DispatchTimeout)
	}
}

// A configuration that could never pass is refused at load, not at merge time.
func TestLoadConfigRejectsUnsatisfiableQuorum(t *testing.T) {
	path := writeConfig(t, `
families: [claude, gemini]
quorum: 3
verdict-timeout: 45m
dispatch-timeout: 30m
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig accepted a quorum larger than the family set")
	}
	if !strings.Contains(err.Error(), "quorum") {
		t.Fatalf("error %q should name the quorum", err)
	}
}

// Every knob is required. An omitted timeout must not silently become zero,
// which would age every started family out instantly and open the gate.
func TestLoadConfigRequiresEveryKnob(t *testing.T) {
	for name, body := range map[string]string{
		"no families":         "quorum: 1\nverdict-timeout: 45m\ndispatch-timeout: 30m\n",
		"no quorum":           "families: [claude]\nverdict-timeout: 45m\ndispatch-timeout: 30m\n",
		"no verdict timeout":  "families: [claude]\nquorum: 1\ndispatch-timeout: 30m\n",
		"no dispatch timeout": "families: [claude]\nquorum: 1\nverdict-timeout: 45m\n",
	} {
		if _, err := LoadConfig(writeConfig(t, body)); err == nil {
			t.Errorf("%s: LoadConfig accepted an incomplete configuration", name)
		}
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, "families: [claude]\nquorum: 1\nverdict-timeout: 45m\ndispatch-timeout: 30m\nqorum: 2\n")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig accepted a misspelled key, so a typo could silently change policy")
	}
}

func TestLoadConfigReportsMissingFile(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yml")); err == nil {
		t.Fatal("LoadConfig accepted a missing configuration file")
	}
}

// The checked-in repository policy has to be loadable and has to match the
// schema the gate enforces, or the gate ships red on its own first run.
func TestRepositoryConfigIsValid(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", DefaultConfigPath))
	if err != nil {
		t.Fatalf("load %s: %v", DefaultConfigPath, err)
	}
	if len(cfg.Families) != 3 {
		t.Fatalf("repository config has %d families, want the three documented ones", len(cfg.Families))
	}
	if cfg.Quorum != 2 {
		t.Fatalf("repository quorum = %d, want 2 so a single reviewer outage cannot wedge the queue", cfg.Quorum)
	}
}
