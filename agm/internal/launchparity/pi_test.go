package launchparity

import (
	"strings"
	"testing"
)

func TestBuildPiCommandIncludesIdentityPolicyAndModeTools(t *testing.T) {
	t.Parallel()
	command := BuildPiCommand(PiCommandSpec{
		WorkDir: "/work dir", ResolvedModel: "anthropic/claude-sonnet-4-6",
		SessionName: "worker", SessionID: "native-id", SessionDir: "/state dir",
		LaunchID:       "launch-id",
		PermissionMode: "default", PermissionExtension: "/ext dir/authorization.js",
		PermissionPolicyFile: "/state dir/policy.json",
	}).Command
	for _, want := range []string{
		"cd '/work dir'", "AGM_SESSION_NAME='worker'", "AGM_PI_PERMISSION_MODE='default'",
		"AGM_PI_LAUNCH_ID='launch-id'",
		"AGM_PI_PERMISSION_POLICY_FILE='/state dir/policy.json'", "pi --session-id 'native-id'",
		"--session-dir '/state dir'", "--extension '/ext dir/authorization.js'",
		"--approve --tools 'read,bash,edit,write,grep,find,ls'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command omits %q: %s", want, command)
		}
	}
	if strings.Contains(command, "Bash(git status)") {
		t.Fatalf("Pi launch inlined the permission policy into terminal input: %s", command)
	}
}

func TestBuildPiCommandSafelyForwardsOptionalCodingAgentDirectory(t *testing.T) {
	t.Parallel()
	configured := BuildPiCommand(PiCommandSpec{
		WorkDir: "/work", SessionName: "worker", SessionID: "native-id",
		SessionDir: "/state", CodingAgentDir: "/tmp/pi agent's config",
	}).Command
	if !strings.Contains(configured, "env -u CLAUDECODE -u PI_CODING_AGENT_DIR") || !strings.Contains(configured, `PI_CODING_AGENT_DIR='/tmp/pi agent'"'"'s config' pi`) {
		t.Fatalf("configured command did not safely forward Pi directory: %s", configured)
	}
	defaulted := BuildPiCommand(PiCommandSpec{
		WorkDir: "/work", SessionName: "worker", SessionID: "native-id", SessionDir: "/state",
	}).Command
	if !strings.Contains(defaulted, "env -u CLAUDECODE -u PI_CODING_AGENT_DIR") || strings.Contains(defaulted, "PI_CODING_AGENT_DIR=") {
		t.Fatalf("default command did not clear inherited Pi configuration: %s", defaulted)
	}
}

func TestPiToolsForMode(t *testing.T) {
	t.Parallel()
	if got := PiToolsForMode("plan"); got != "read,grep,find,ls" {
		t.Fatalf("plan tools = %q", got)
	}
	if got := PiToolsForMode("auto"); strings.Contains(got, "") && !strings.Contains(got, "bash") {
		t.Fatalf("auto tools = %q", got)
	}
}
