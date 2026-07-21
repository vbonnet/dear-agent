package launchparity

import (
	"strings"
	"testing"
)

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
		{harness: "agy", mode: "auto", interactive: "agy", modeToken: "--dangerously-skip-permissions"},
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

func TestBuildAgyCommandOwnsLaunchAndResumePolicy(t *testing.T) {
	command := BuildAgyCommand(AgyCommandSpec{
		WorkDir:        "/tmp/agy work's",
		ResolvedModel:  "Claude Sonnet 4.6 (Thinking)",
		PermissionMode: "auto",
		ConversationID: "117ff898-a964-4a9f-b460-1be4a8a49b17",
		ExtraAddDirs:   []string{"/tmp/extra dir"},
	})
	for _, want := range []string{
		"cd '/tmp/agy work'\"'\"'s' && agy --model 'Claude Sonnet 4.6 (Thinking)'",
		"--dangerously-skip-permissions",
		"--conversation '117ff898-a964-4a9f-b460-1be4a8a49b17'",
		"--add-dir '/tmp/extra dir'",
		"&& exit",
	} {
		if !strings.Contains(command.Command, want) {
			t.Errorf("AGY command %q missing %q", command.Command, want)
		}
	}
	if !command.ModeAppliedAtStartup {
		t.Fatal("auto mode should be reported as applied at startup")
	}
}
