package launchparity

import "testing"

func TestActiveHarnessContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		harness     string
		mode        string
		interactive string
		modeToken   string
	}{
		{harness: "claude-code", mode: "auto", interactive: "claude", modeToken: "--permission-mode auto"},
		{harness: "codex-cli", mode: "auto", interactive: "codex", modeToken: "-a never"},
		{harness: "agy", mode: "auto", interactive: "--prompt-interactive", modeToken: "--dangerously-skip-permissions"},
		{harness: "opencode-cli", mode: "plan", interactive: "opencode attach"},
	}
	for _, tt := range tests {
		t.Run(tt.harness, func(t *testing.T) {
			t.Parallel()
			contract, err := Resolve(tt.harness, tt.mode, true)
			if err != nil {
				t.Fatal(err)
			}
			if contract.InteractiveToken != tt.interactive || contract.ModeToken != tt.modeToken {
				t.Fatalf("contract = %+v, want interactive=%q mode=%q", contract, tt.interactive, tt.modeToken)
			}
			if contract.ExitSuffix != "" {
				t.Fatalf("persistent exit suffix = %q, want empty", contract.ExitSuffix)
			}
		})
	}
}

func TestCodexPermissionModesUseSupportedCLIOptions(t *testing.T) {
	tests := []struct {
		mode     string
		approval string
		sandbox  string
	}{
		{mode: "auto", approval: "-a never", sandbox: "workspace-write"},
		{mode: "plan", approval: "-a untrusted", sandbox: "read-only"},
		{mode: "default", approval: "", sandbox: "workspace-write"},
	}
	for _, tt := range tests {
		if got := CodexPermissionModeFlag(tt.mode); got != tt.approval {
			t.Errorf("CodexPermissionModeFlag(%q) = %q, want %q", tt.mode, got, tt.approval)
		}
		if got := CodexSandboxMode(tt.mode); got != tt.sandbox {
			t.Errorf("CodexSandboxMode(%q) = %q, want %q", tt.mode, got, tt.sandbox)
		}
	}
}

func TestNonPersistentContractExitsPaneShell(t *testing.T) {
	t.Parallel()
	contract, err := Resolve("codex-cli", "default", false)
	if err != nil {
		t.Fatal(err)
	}
	if contract.ExitSuffix != " && exit" {
		t.Fatalf("exit suffix = %q", contract.ExitSuffix)
	}
}
