package dolt

import (
	"os"
	"strings"
	"testing"
)

// `agm sandbox gc` is the only collector for ~/.agm/sandboxes. On 2026-09-04 it
// refused to run at all, with "multiple enabled workspaces require either an
// explicit shared DOLT_PORT or a per-workspace dolt.port in the AGM registry",
// while 3.9 GB of sandboxes sat uncollected on a host at 97% used.
//
// The refusal was over-strict rather than protective. Cross-workspace isolation
// here is by database name (each workspace uses its same-name database), not by
// port, and every workspace already resolves the same conventional default port
// when nothing overrides it. Refusing the whole inventory in that state buys no
// safety and costs the only sandbox collector the host has.

func withRegistry(t *testing.T, yaml string, env map[string]string) {
	t.Helper()
	originalLookupEnv := lookupEnv
	originalAgmConfigPath := agmConfigPath
	t.Cleanup(func() {
		lookupEnv = originalLookupEnv
		agmConfigPath = originalAgmConfigPath
	})

	configPath := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	agmConfigPath = configPath

	lookupEnv = func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

const twoEnabledNoPorts = `version: 1
default_workspace: personal
workspaces:
  - name: personal
    enabled: true
  - name: oss
    enabled: true
`

// TestConfiguredWorkspaceConfigs_DerivesDefaultPortForEveryWorkspace is the
// receipt for defect 2: two enabled workspaces with no port configured anywhere
// must resolve, not refuse.
func TestConfiguredWorkspaceConfigs_DerivesDefaultPortForEveryWorkspace(t *testing.T) {
	withRegistry(t, twoEnabledNoPorts, map[string]string{
		"ENGRAM_TEST_MODE":      "1",
		"ENGRAM_TEST_WORKSPACE": "personal",
		"WORKSPACE":             "personal",
	})

	configs, err := ConfiguredWorkspaceConfigs()
	if err != nil {
		t.Fatalf("ConfiguredWorkspaceConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("configured stores = %d, want 2: %#v", len(configs), configs)
	}
	for index, want := range []string{"personal", "oss"} {
		got := configs[index]
		if got.Workspace != want || got.Database != want {
			t.Errorf("config[%d] = workspace %q database %q, want %q/%q",
				index, got.Workspace, got.Database, want, want)
		}
		// Isolation is by database, so a shared derived port is correct, but
		// an empty port would silently produce an unusable DSN.
		if got.Port != "3307" {
			t.Errorf("config[%d].Port = %q, want the derived default 3307", index, got.Port)
		}
	}
}

// TestConfiguredWorkspaceConfigsIncludingDisabled_DerivesDefaultPort covers the
// path `agm sandbox gc` actually calls: destructive inventory must include
// disabled stores, and that call was the one failing on the host.
func TestConfiguredWorkspaceConfigsIncludingDisabled_DerivesDefaultPort(t *testing.T) {
	withRegistry(t, twoEnabledNoPorts+`  - name: retired
    enabled: false
`, map[string]string{
		"ENGRAM_TEST_MODE":      "1",
		"ENGRAM_TEST_WORKSPACE": "personal",
		"WORKSPACE":             "personal",
	})

	configs, err := ConfiguredWorkspaceConfigsIncludingDisabledAt(agmConfigPath)
	if err != nil {
		t.Fatalf("ConfiguredWorkspaceConfigsIncludingDisabledAt: %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("configured stores = %d, want 3 (disabled included): %#v", len(configs), configs)
	}
	for _, c := range configs {
		if c.Port == "" {
			t.Errorf("workspace %q resolved an empty port", c.Workspace)
		}
	}
}

// TestConfiguredWorkspaceConfigs_PerWorkspacePortStillWins: deriving a default
// must not flatten a registry that genuinely runs its workspaces on separate
// ports.
func TestConfiguredWorkspaceConfigs_PerWorkspacePortStillWins(t *testing.T) {
	withRegistry(t, `version: 1
default_workspace: personal
workspaces:
  - name: personal
    enabled: true
    dolt:
      port: "3307"
  - name: oss
    enabled: true
    dolt:
      port: "3400"
`, map[string]string{
		"ENGRAM_TEST_MODE":      "1",
		"ENGRAM_TEST_WORKSPACE": "personal",
		"WORKSPACE":             "personal",
	})

	configs, err := ConfiguredWorkspaceConfigs()
	if err != nil {
		t.Fatalf("ConfiguredWorkspaceConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("configured stores = %d, want 2", len(configs))
	}
	if configs[0].Port != "3307" || configs[1].Port != "3400" {
		t.Fatalf("ports = %q/%q, want 3307/3400", configs[0].Port, configs[1].Port)
	}
}

// TestConfiguredWorkspaceConfigs_PartialPortsComposeWithTheDerivedDefault: one
// workspace pinned to its own port and one unpinned is the shape a half-migrated
// registry has. It must resolve both rather than refuse the inventory.
func TestConfiguredWorkspaceConfigs_PartialPortsComposeWithTheDerivedDefault(t *testing.T) {
	withRegistry(t, `version: 1
default_workspace: personal
workspaces:
  - name: personal
    enabled: true
  - name: oss
    enabled: true
    dolt:
      port: "3400"
`, map[string]string{
		"ENGRAM_TEST_MODE":      "1",
		"ENGRAM_TEST_WORKSPACE": "personal",
		"WORKSPACE":             "personal",
	})

	configs, err := ConfiguredWorkspaceConfigs()
	if err != nil {
		t.Fatalf("ConfiguredWorkspaceConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("configured stores = %d, want 2", len(configs))
	}
	if configs[0].Port != "3307" {
		t.Errorf("unpinned workspace port = %q, want the derived default 3307", configs[0].Port)
	}
	if configs[1].Port != "3400" {
		t.Errorf("pinned workspace port = %q, want 3400", configs[1].Port)
	}
}

// TestConfiguredWorkspaceConfigs_ExplicitDatabaseAcrossWorkspacesStillRefuses
// pins the guard that is genuinely load-bearing: one DOLT_DATABASE cannot
// describe several workspaces, so that inventory really is unprovable. Relaxing
// the port rule must not relax this one.
func TestConfiguredWorkspaceConfigs_ExplicitDatabaseAcrossWorkspacesStillRefuses(t *testing.T) {
	withRegistry(t, twoEnabledNoPorts, map[string]string{
		"ENGRAM_TEST_MODE":      "1",
		"ENGRAM_TEST_WORKSPACE": "personal",
		"WORKSPACE":             "personal",
		"DOLT_DATABASE":         "personal",
	})

	_, err := ConfiguredWorkspaceConfigs()
	if err == nil {
		t.Fatal("an explicit DOLT_DATABASE across two workspaces must still refuse")
	}
	if !strings.Contains(err.Error(), "DOLT_DATABASE") {
		t.Fatalf("error should name the offending override, got %v", err)
	}
}
