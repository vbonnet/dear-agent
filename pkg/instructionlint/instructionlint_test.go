package instructionlint

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

const testContextFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"

func TestSegmentsClassifyExecutableFences(t *testing.T) {
	source := []byte(strings.Join([]string{
		"# W0 prose",
		"Use `git push origin main` only as a quoted bad command.",
		"```markdown",
		"W0 fixture",
		"git push origin fixture",
		"```",
		"```",
		"if gh pr merge 789; then",
		"  echo merged",
		"fi",
		"```",
		"```bash",
		"git push origin main",
		"```",
		"```",
		"env TOKEN=x gh pr merge 123",
		"```",
		"```",
		`gh \`,
		"  pr merge 456",
		"```",
		"",
		"    bd ready",
		"    explanatory fixture text",
	}, "\n"))

	got := parseSegments(source)
	var compact []string
	for _, item := range got {
		compact = append(compact, string(item.Kind)+"|"+item.Text)
	}
	sort.Strings(compact)
	want := []string{
		"inline|git push origin main",
		"prose|# W0 prose",
		"prose|explanatory fixture text",
		"prose|Use `git push origin main` only as a quoted bad command.",
		"prose|W0 fixture",
		"prose|echo merged",
		"prose|fi",
		"shell|git push origin fixture",
		"shell|git push origin main",
		"shell|env TOKEN=x gh pr merge 123",
		"shell|gh pr merge 456",
		"shell|if gh pr merge 789; then",
		"shell|bd ready",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(compact, want) {
		t.Fatalf("segments = %#v, want %#v", compact, want)
	}
}

func TestNonShellFencesRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"```json",
		`"command": "gh pr merge --squash 123"`,
		"```",
		"```yaml",
		"command: git push --force origin main",
		"```",
		"```text",
		"bd ready",
		"```",
	}, "\n"))

	var got []string
	for _, segment := range parseSegments(source) {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			got = append(got, violation.Rule)
		}
	}
	sort.Strings(got)
	want := []string{"bare-beads", "raw-gh-merge", "raw-git-push"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
}

func TestWrappedShellCommandsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"```sh",
		`gh \`,
		"  pr merge 123",
		`git \`,
		"  push origin branch",
		`safe-pr create \`,
		"  --emergency --reason urgent",
		"```",
	}, "\n"))

	var got []string
	for _, segment := range parseSegments(source) {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			got = append(got, violation.Rule)
		}
	}
	sort.Strings(got)
	want := []string{"raw-gh-merge", "raw-git-push", "safe-pr-emergency"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
}

func TestOrdinaryMarkdownContinuationsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		`gh \`,
		"  pr merge 123",
		`git \`,
		"  push origin branch",
		`bd \`,
		"  ready",
	}, "\n"))

	var got []string
	for _, segment := range parseSegments(source) {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			got = append(got, violation.Rule)
		}
	}
	sort.Strings(got)
	want := []string{"bare-beads", "raw-gh-merge", "raw-git-push"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
}

func TestScriptGuidanceAndCommandSubstitutionsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"#!/usr/bin/env bash",
		"# raw gh pr create is discussed here, not instructed",
		"AGM_HELP='Use `agm new worker`.'",
		"guidance='Use `safe-pr create --emergency --reason urgent`.'",
		"merged=$(gh pr merge 42)",
		"ready=$(bd ready)",
	}, "\n"))

	var got []string
	for _, segment := range parseScriptSegments(source) {
		for _, violation := range evaluateSegment("hook", segment) {
			got = append(got, violation.Rule)
		}
	}
	sort.Strings(got)
	want := []string{"agm-root-new", "bare-beads", "raw-gh-merge", "safe-pr-emergency"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
}

func TestScriptDeclarationAssignmentsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"#!/usr/bin/env bash",
		`local msg="BLOCKED: use safe-pr create --emergency --reason bypass`,
		`instead"`,
		`export STATUS="agm status --output json"`,
		`declare -r MERGE="gh pr merge --squash 42"`,
		`readonly READY="bd ready"`,
	}, "\n"))

	var got []string
	for _, segment := range parseScriptSegments(source) {
		for _, violation := range evaluateSegment("hook", segment) {
			got = append(got, violation.Rule)
		}
	}
	sort.Strings(got)
	want := []string{"agm-root-status", "bare-beads", "raw-gh-merge", "safe-pr-emergency"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %v, want %v", got, want)
	}
}

func TestScriptJQGuidanceRemainsPolicyVisible(t *testing.T) {
	source := []byte(`jq -cn --arg msg "Use the standard form 'bd --db <path> close <id>'." '{additionalContext:$msg}'`)
	var rules []string
	for _, segment := range parseScriptSegments(source) {
		for _, violation := range evaluateSegment("hook", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	if !reflect.DeepEqual(rules, []string{"bare-beads"}) {
		t.Fatalf("jq guidance rules = %v, want bare-beads", rules)
	}
}

func TestScriptOutputHelpersRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"emit_json() {",
		`  jq -cn --arg message "$1" '{additionalContext:$message}'`,
		"}",
		"function emit_context {",
		`  emit_json "$1"`,
		"}",
		`emit_context "Run gh pr merge 123 after review.`,
		`Use git push origin main for delivery."`,
	}, "\n"))

	var rules []string
	for _, segment := range parseScriptSegments(source) {
		for _, violation := range evaluateSegment("hook", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"raw-gh-merge", "raw-git-push"}) {
		t.Fatalf("helper-emitted guidance rules = %v", rules)
	}
}

func TestScriptOutputContinuationsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		`printf '%s\n' \`,
		`  'git push origin main'`,
		`echo \`,
		`  "gh pr merge 123"`,
	}, "\n"))

	var rules []string
	for _, segment := range parseScriptSegments(source) {
		for _, violation := range evaluateSegment("hook", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"raw-gh-merge", "raw-git-push"}) {
		t.Fatalf("continued output guidance rules = %v", rules)
	}
}

func TestScriptHeredocVisibilityFollowsOutputRedirection(t *testing.T) {
	source := []byte(strings.Join([]string{
		"cat >&2 <<'VISIBLE'",
		"git push origin main",
		"VISIBLE",
		"cat >fixture <<'FILE_ONLY'",
		"gh pr merge 123",
		"FILE_ONLY",
		"cat <<'AFTER_MARKER' >after-marker-fixture",
		"gh pr close 123",
		"AFTER_MARKER",
		"cat 2>errors <<'STDOUT_VISIBLE'",
		"bd ready",
		"STDOUT_VISIBLE",
		"cat >fixture >&2 <<'RESTORED_VISIBLE'",
		"gh pr reopen 123",
		"RESTORED_VISIBLE",
		"cat >&2 >final-file <<'FINAL_FILE'",
		"safe-pr create --emergency --reason hidden",
		"FINAL_FILE",
	}, "\n"))

	var rules []string
	for _, segment := range parseScriptSegments(source) {
		for _, violation := range evaluateSegment("hook", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"bare-beads", "raw-gh-pr-lifecycle", "raw-git-push"}) {
		t.Fatalf("heredoc rules = %v, want only visible heredoc findings", rules)
	}
}

func TestScriptHeredocVisibilityTracksCommandListsAndDescriptorAliases(t *testing.T) {
	source := []byte(strings.Join([]string{
		"cat >fixture <<'PRINTED_LATER'; cat fixture",
		"git push origin main",
		"PRINTED_LATER",
		"cat 3>fixture >&3 <<'DESCRIPTOR_FILE_ONLY'",
		"gh pr merge 123",
		"DESCRIPTOR_FILE_ONLY",
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	if !slices.Contains(text, "git push origin main") {
		t.Fatalf("command-list heredoc was hidden: %v", text)
	}
	if slices.Contains(text, "gh pr merge 123") {
		t.Fatalf("descriptor-file heredoc was exposed: %v", text)
	}
}

func TestScriptHeredocVisibilityFollowsRoutingAndDeferredReplay(t *testing.T) {
	source := []byte(strings.Join([]string{
		"tee fixture <<'TEE_VISIBLE'",
		"safe-pr create --emergency --reason exposed",
		"TEE_VISIBLE",
		"cat >later-fixture <<'PRINTED_LATER'",
		"git push origin main",
		"PRINTED_LATER",
		"cat later-fixture",
		"cat >stdin-fixture <<'STDIN_REPLAY'",
		"gh pr close 456",
		"STDIN_REPLAY",
		"cat <stdin-fixture",
		"cat <<'PIPE_FILE_ONLY' | jq -R . >pipeline-fixture",
		"gh pr merge 123",
		"PIPE_FILE_ONLY",
		"cat <<'PIPE_VISIBLE' | jq -R .",
		"bd ready",
		"PIPE_VISIBLE",
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{
		"safe-pr create --emergency --reason exposed",
		"git push origin main",
		"gh pr close 456",
		"bd ready",
	} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible heredoc line %q was hidden: %v", visible, text)
		}
	}
	if slices.Contains(text, "gh pr merge 123") {
		t.Fatalf("file-only pipeline heredoc was exposed: %v", text)
	}
}

func TestScriptHeredocVisibilityPreservesQuotesDescriptorsAndFileModes(t *testing.T) {
	source := []byte(strings.Join([]string{
		"tee '>fixture' <<'QUOTED_REDIRECT_NAME'",
		"git push origin quoted-name",
		"QUOTED_REDIRECT_NAME",
		"exec 3>&1",
		"cat >&3 <<'PERSISTENT_DESCRIPTOR'",
		"gh pr merge 123",
		"PERSISTENT_DESCRIPTOR",
		"cat >overwritten-fixture <<'OVERWRITTEN'",
		"safe-pr create --emergency --reason stale",
		"OVERWRITTEN",
		"cat >overwritten-fixture <<'REPLACEMENT'",
		"replacement text",
		"REPLACEMENT",
		"cat overwritten-fixture",
		"cat >appended-fixture <<'APPEND_FIRST'",
		"git push origin append-first",
		"APPEND_FIRST",
		"cat >>appended-fixture <<'APPEND_SECOND'",
		"gh pr close 456",
		"APPEND_SECOND",
		"cat appended-fixture",
		"out=/dev/stderr",
		"cat >\"$out\" <<'DYNAMIC_VISIBLE'",
		"bd ready",
		"DYNAMIC_VISIBLE",
		"cat >head-fixture <<'HEAD_REPLAY'",
		"gh pr reopen 789",
		"HEAD_REPLAY",
		"head -n 1 head-fixture",
		"cat >substitution-fixture <<'SUBSTITUTION_REPLAY'",
		"git push origin substitution",
		"SUBSTITUTION_REPLAY",
		"message=$(cat substitution-fixture)",
		`printf '%s\n' "$message"`,
		"cat >alias-fixture <<'ALIASED_REPLAY'",
		"gh pr merge 987",
		"ALIASED_REPLAY",
		"cat ./alias-fixture",
		"cat <<<EOF",
		"echo 'git push origin after-here-string'",
		`echo "<<PHANTOM"`,
		"echo 'gh pr close 654'",
		"cat << 'END-MARKER'",
		"git push origin spaced-marker",
		"END-MARKER",
		"cat <<\\ESCAPED",
		"gh pr merge 321",
		"ESCAPED",
		"echo ok # <<COMMENT_ONLY",
		"echo 'bd ready'",
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{
		"git push origin quoted-name",
		"gh pr merge 123",
		"git push origin append-first",
		"gh pr close 456",
		"bd ready",
		"gh pr reopen 789",
		"git push origin substitution",
		"gh pr merge 987",
		"echo 'git push origin after-here-string'",
		"echo 'gh pr close 654'",
		"git push origin spaced-marker",
		"gh pr merge 321",
		"echo 'bd ready'",
	} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible heredoc line %q was hidden: %v", visible, text)
		}
	}
	if slices.Contains(text, "safe-pr create --emergency --reason stale") {
		t.Fatalf("overwritten heredoc content remained policy-visible: %v", text)
	}
}

func TestScriptHeredocVisibilityHandlesQueuesArithmeticAndAttachedRedirects(t *testing.T) {
	source := []byte(strings.Join([]string{
		"cat <<'FIRST' <<'SECOND'",
		"safe-pr create --emergency --reason inactive-heredoc",
		"FIRST",
		"gh pr merge 432",
		"SECOND",
		"mask=$((1 << 2))",
		"echo 'git push origin after-arithmetic'",
		"cat>attached-fixture <<'ATTACHED_FILE_ONLY'",
		"safe-pr create --emergency --reason attached-redirect",
		"ATTACHED_FILE_ONLY",
		"cat \\",
		"  <<'CONTINUED_HEREDOC'",
		"gh pr close 876",
		"CONTINUED_HEREDOC",
		"cat >input-only-fixture <<'INPUT_ONLY_SUBSTITUTION'",
		"git push origin input-only-substitution",
		"INPUT_ONLY_SUBSTITUTION",
		"message=$(<input-only-fixture)",
		`printf '%s\n' "$message"`,
		"{ exec 4>&1; }",
		"cat >&4 <<'BRACE_EXEC_DESCRIPTOR'",
		"bd ready",
		"BRACE_EXEC_DESCRIPTOR",
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{
		"gh pr merge 432",
		"echo 'git push origin after-arithmetic'",
		"gh pr close 876",
		"git push origin input-only-substitution",
		"bd ready",
	} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible line %q was hidden: %v", visible, text)
		}
	}
	for _, hidden := range []string{
		"safe-pr create --emergency --reason inactive-heredoc",
		"safe-pr create --emergency --reason attached-redirect",
	} {
		if slices.Contains(text, hidden) {
			t.Errorf("file-only or inactive heredoc line %q was exposed: %v", hidden, text)
		}
	}
}

func TestScriptHeredocVisibilityTracksTeeFilesAndDynamicReplay(t *testing.T) {
	source := []byte(strings.Join([]string{
		"tee tee-fixture >/dev/null <<'TEE_SUPPRESSED_STDOUT'",
		"gh pr reopen 765",
		"TEE_SUPPRESSED_STDOUT",
		"cat tee-fixture",
		"cat >dynamic-replay-fixture <<'DYNAMIC_REPLAY'",
		"safe-pr create --emergency --reason dynamic-replay",
		"DYNAMIC_REPLAY",
		"file=dynamic-replay-fixture",
		`cat "$file"`,
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{
		"gh pr reopen 765",
		"safe-pr create --emergency --reason dynamic-replay",
	} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible replay line %q was hidden: %v", visible, text)
		}
	}
}

func TestScriptHeredocVisibilityTracksDescriptorsPipelinesAndCapturedVariables(t *testing.T) {
	source := []byte(strings.Join([]string{
		"cat <<'STDIN_BODY' 3<<'FD3_BODY'",
		"gh pr merge 246",
		"STDIN_BODY",
		"safe-pr create --emergency --reason unused-fd3",
		"FD3_BODY",
		"cat >pipeline-fixture <<'PIPELINE_FIXTURE'",
		"safe-pr create --emergency --reason hidden-pipeline",
		"PIPELINE_FIXTURE",
		"cat pipeline-fixture | sed 's/x/y/' >pipeline-output",
		"read value <<'SILENT_READ'",
		"git push origin silent-read",
		"SILENT_READ",
		"read message <<'VISIBLE_READ'",
		"gh pr close 135",
		"VISIBLE_READ",
		"copy=$message",
		`printf '%s\n' "$copy"`,
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{"gh pr merge 246", "gh pr close 135"} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible line %q was hidden: %v", visible, text)
		}
	}
	for _, hidden := range []string{
		"safe-pr create --emergency --reason unused-fd3",
		"safe-pr create --emergency --reason hidden-pipeline",
		"git push origin silent-read",
	} {
		if slices.Contains(text, hidden) {
			t.Errorf("non-visible line %q was exposed: %v", hidden, text)
		}
	}
}

func TestScriptHeredocTerminatorsAndReadCapturesMatchShellSemantics(t *testing.T) {
	source := []byte(strings.Join([]string{
		"cat <<'STRICT_TERMINATOR'",
		"safe text",
		" STRICT_TERMINATOR",
		"gh pr reopen 864",
		"STRICT_TERMINATOR",
		"cat <<-'TAB_TERMINATOR'",
		"bd ready",
		"\tTAB_TERMINATOR",
		"read first <<'SINGLE_LINE_READ'",
		"safe first line",
		"git push origin unread-second-line",
		"SINGLE_LINE_READ",
		`printf '%s\n' "$first"`,
		"read msg <<'SHORT_NAME'",
		"gh pr merge 975",
		"SHORT_NAME",
		"message=safe",
		`printf '%s\n' "$message"`,
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{"gh pr reopen 864", "bd ready"} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible line %q was hidden: %v", visible, text)
		}
	}
	for _, hidden := range []string{"git push origin unread-second-line", "gh pr merge 975"} {
		if slices.Contains(text, hidden) {
			t.Errorf("non-visible line %q was exposed: %v", hidden, text)
		}
	}
}

func TestScriptHeredocStateHandlesEmptySupersededAndDeferredInputs(t *testing.T) {
	source := []byte(strings.Join([]string{
		"cat <<''",
		"gh pr merge 123",
		"",
		"cat <<'SUPERSEDED' </dev/null",
		"safe-pr create --emergency --reason superseded-stdin",
		"SUPERSEDED",
		"cat >overwritten-fixture <<'OVERWRITTEN_FIXTURE'",
		"git push origin overwritten-fixture",
		"OVERWRITTEN_FIXTURE",
		"printf safe >overwritten-fixture",
		"cat overwritten-fixture",
		"read -u 3 value <<'ALTERNATE_READ_FD'",
		"gh pr close 456",
		"ALTERNATE_READ_FD",
		`printf '%s\n' "$value"`,
		"cat >shell-payload-fixture <<'SHELL_PAYLOAD'",
		"bd ready",
		"SHELL_PAYLOAD",
		"sh -c 'cat shell-payload-fixture'",
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{"gh pr merge 123", "bd ready"} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible line %q was hidden: %v", visible, text)
		}
	}
	for _, hidden := range []string{
		"safe-pr create --emergency --reason superseded-stdin",
		"git push origin overwritten-fixture",
		"gh pr close 456",
	} {
		if slices.Contains(text, hidden) {
			t.Errorf("non-visible line %q was exposed: %v", hidden, text)
		}
	}
}

func TestScriptHeredocStateHandlesCopiedAndConditionallyRoutedInputs(t *testing.T) {
	source := []byte(strings.Join([]string{
		"tee tee-attached>/dev/null <<'TEE_ATTACHED_REDIRECT'",
		"gh pr reopen 246",
		"TEE_ATTACHED_REDIRECT",
		"cat tee-attached",
		"cat >legacy-substitution <<'LEGACY_SUBSTITUTION'",
		"git push origin legacy-substitution",
		"LEGACY_SUBSTITUTION",
		"message=`cat legacy-substitution`",
		`printf '%s\n' "$message"`,
		"cat >copy-source <<'REDIRECTED_COPY'",
		"gh pr close 135",
		"REDIRECTED_COPY",
		"cat copy-source >copy-destination",
		"cat copy-destination",
		"exec 3>&1",
		"false && exec 3>conditional-fixture",
		"cat >&3 <<'CONDITIONAL_EXEC'",
		"safe-pr create --emergency --reason conditional-exec",
		"CONDITIONAL_EXEC",
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{
		"gh pr reopen 246",
		"git push origin legacy-substitution",
		"gh pr close 135",
		"safe-pr create --emergency --reason conditional-exec",
	} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible line %q was hidden: %v", visible, text)
		}
	}
}

func TestScriptHeredocStateHandlesPipelineSideOutputsAndShellCaptures(t *testing.T) {
	source := []byte(strings.Join([]string{
		"cat <<'PIPELINE_TEE' | tee pipeline-tee >/dev/null",
		"gh pr reopen 864",
		"PIPELINE_TEE",
		"cat pipeline-tee",
		"hidden=$(cat <<'CAPTURED_HIDDEN'",
		"git push origin captured-hidden",
		"CAPTURED_HIDDEN",
		")",
		"visible=$(cat <<'CAPTURED_VISIBLE'",
		"bd ready",
		"CAPTURED_VISIBLE",
		")",
		`printf '%s\n' "$visible"`,
		`quoted="$(cat <<'QUOTED_CAPTURE'`,
		"gh pr close 531",
		"QUOTED_CAPTURE",
		`)"`,
		`printf '%s\n' "$quoted"`,
		"read -p prompt value <<'READ_PROMPT_ONLY'",
		"safe-pr create --emergency --reason prompt-is-not-a-variable",
		"READ_PROMPT_ONLY",
		`printf '%s\n' "$prompt"`,
		"read -p prompt value <<'READ_VALUE'",
		"gh pr merge 975",
		"READ_VALUE",
		`printf '%s\n' "$value"`,
		"read filtered <<'FILTERED_VALUE'",
		"git push origin filtered-value",
		"FILTERED_VALUE",
		`sed 's/^//' <<<"$filtered"`,
		"cat >source-fixture <<'SOURCE_FIXTURE'",
		"gh pr reopen 642",
		"SOURCE_FIXTURE",
		"source source-fixture",
		"cat >shell-script-fixture <<'SHELL_SCRIPT_FIXTURE'",
		"git push origin executed-shell-script",
		"SHELL_SCRIPT_FIXTURE",
		"bash shell-script-fixture",
		"indirect=$(cat <<'INDIRECT_CAPTURE'",
		"gh pr close 753",
		"INDIRECT_CAPTURE",
		")",
		"name=indirect",
		`printf '%s\n' "${!name}"`,
		"cat >silent-substitution-fixture <<'SILENT_SUBSTITUTION'",
		"git push origin silent-substitution",
		"SILENT_SUBSTITUTION",
		"silent=$(cat silent-substitution-fixture)",
		"read original <<'SILENT_HERE_STRING'",
		"safe-pr create --emergency --reason silent-here-string",
		"SILENT_HERE_STRING",
		`read copy <<<"$original"`,
	}, "\n"))

	var text []string
	for _, segment := range parseScriptSegments(source) {
		text = append(text, segment.Text)
	}
	for _, visible := range []string{
		"gh pr reopen 864",
		"git push origin executed-shell-script",
		"gh pr close 531",
		"gh pr merge 975",
		"git push origin filtered-value",
		"gh pr reopen 642",
		"bd ready",
		"gh pr close 753",
	} {
		if !slices.Contains(text, visible) {
			t.Errorf("visible line %q was hidden: %v", visible, text)
		}
	}
	for _, hidden := range []string{
		"git push origin captured-hidden",
		"safe-pr create --emergency --reason prompt-is-not-a-variable",
		"git push origin silent-substitution",
		"safe-pr create --emergency --reason silent-here-string",
	} {
		if slices.Contains(text, hidden) {
			t.Errorf("non-visible line %q was exposed: %v", hidden, text)
		}
	}
}

func TestCheckRepositoryGovernsSkillAgentMetadata(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: .agents/skills/**/agents/*.yaml
      owner: skill-agents
`)
	writeTestFile(t, repo, ".agents/skills/example/agents/openai.yaml", `interface:
  default_prompt: |
    Review the current state first.
    gh pr merge 123
  followup_prompt: "Run git push origin main for delivery."
`)
	runGit(t, repo, "add", ".")

	result, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, violation := range violations {
		rules = append(rules, violation.Rule)
	}
	sort.Strings(rules)
	if result.Files != 1 || !reflect.DeepEqual(rules, []string{"raw-gh-merge", "raw-git-push"}) {
		t.Fatalf("result=%+v rules=%v, want governed skill-agent prompts", result, rules)
	}
}

func TestCheckRepositoryGovernsMultilineGoPrompts(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: cmd/worker/main.go
      owner: worker-prompts
`)
	writeTestFile(t, repo, "cmd/worker/main.go", "package main\n\nvar prompt = `# Worker\n\n` + (\"gh pr merge 123\") + `\n`\nvar runtimeCommand = \"git push origin main\"\n")
	runGit(t, repo, "add", ".")

	result, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 || len(violations) != 1 || violations[0].Rule != "raw-gh-merge" {
		t.Fatalf("result=%+v violations=%v, want only the multiline prompt governed", result, violations)
	}
}

func TestEvalPayloadsRemainPolicyVisible(t *testing.T) {
	source := []byte("```\neval 'git push origin main'\neval 'gh pr merge 123'\neval 'bd ready'\n```\n")
	var rules []string
	for _, segment := range parseSegments(source) {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	want := []string{"bare-beads", "raw-gh-merge", "raw-git-push"}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("eval payload rules = %v, want %v", rules, want)
	}
}

func TestFoldedYAMLScalarsPreservePhysicalCommandLines(t *testing.T) {
	source := []byte("interface:\n  default_prompt: >\n    Review the current state first.\n    gh pr merge 123\n")
	segments, err := parseYAMLSegments(source)
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("agent.yaml", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	if !reflect.DeepEqual(rules, []string{"raw-gh-merge"}) {
		t.Fatalf("folded scalar rules = %v, want raw-gh-merge", rules)
	}
}

func TestQuotedAndPlainYAMLScalarsPreservePhysicalCommandLines(t *testing.T) {
	source := []byte("quoted: \"Review the current state first.\n  gh pr merge 123\"\nplain: Review the current state first.\n  git push origin main\n")
	segments, err := parseYAMLSegments(source)
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("agent.yaml", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"raw-gh-merge", "raw-git-push"}) {
		t.Fatalf("multiline scalar rules = %v, want physical command lines", rules)
	}
}

func TestMarkdownContainerCommandsRemainPolicyVisible(t *testing.T) {
	source := []byte(strings.Join([]string{
		"- git push origin main",
		"1. gh pr merge 123",
		"> bd ready",
		"- [ ] agm status --output json",
	}, "\n"))
	var rules []string
	for _, segment := range parseSegments(source) {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	want := []string{"agm-root-status", "bare-beads", "raw-gh-merge", "raw-git-push"}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("Markdown container rules = %v, want %v", rules, want)
	}
}

func TestRuleViolationsTeachCanonicalReplacements(t *testing.T) {
	segments := []Segment{
		{Kind: SegmentProse, Line: 1, Text: "Create W0-charter.md before D1."},
		{Kind: SegmentShell, Line: 2, Text: "bd ready"},
		{Kind: SegmentShell, Line: 3, Text: "git push -u origin branch"},
		{Kind: SegmentShell, Line: 4, Text: "gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 5, Text: "safe-pr create --emergency --reason urgent"},
		{Kind: SegmentShell, Line: 6, Text: "agm session health --json"},
		{Kind: SegmentShell, Line: 7, Text: "agm status --output json"},
		{Kind: SegmentShell, Line: 8, Text: "agm search positional-query"},
		{Kind: SegmentShell, Line: 9, Text: "agm new --workspace oss"},
		{Kind: SegmentShell, Line: 10, Text: "agm session send worker hello"},
		{Kind: SegmentShell, Line: 11, Text: "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on ready"},
		{Kind: SegmentShell, Line: 12, Text: "bd --db=/tmp/wrong ready"},
		{Kind: SegmentShell, Line: 13, Text: "bd --db=~/beads/context-engine/.beads ready"},
		{Kind: SegmentShell, Line: 14, Text: "env TOKEN=x gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 15, Text: "echo ok && gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 16, Text: "env WORKSPACE=oss bd ready"},
		{Kind: SegmentShell, Line: 17, Text: "echo ok && bd ready"},
		{Kind: SegmentShell, Line: 18, Text: "timeout 5s bd ready"},
		{Kind: SegmentShell, Line: 19, Text: "WORKSPACE=oss bd ready"},
		{Kind: SegmentShell, Line: 20, Text: "env WORKSPACE=oss bd --db ~/beads/context-engine/.beads ready"},
		{Kind: SegmentShell, Line: 21, Text: "bd -db ~/beads/context-engine/.beads ready"},
		{Kind: SegmentShell, Line: 22, Text: "sudo gh pr merge --squash 123"},
		{Kind: SegmentShell, Line: 23, Text: "command bd ready"},
		{Kind: SegmentShell, Line: 24, Text: "nohup git push origin branch"},
		{Kind: SegmentShell, Line: 25, Text: "timeout --signal TERM 5s bd ready"},
		{Kind: SegmentShell, Line: 26, Text: "env WORKSPACE=oss agm status --output json"},
		{Kind: SegmentShell, Line: 27, Text: "cd repo && agm session send worker hello"},
		{Kind: SegmentShell, Line: 28, Text: "if gh pr merge 123; then"},
		{Kind: SegmentShell, Line: 29, Text: "gh pr create --title test"},
		{Kind: SegmentShell, Line: 30, Text: "gh pr close 123"},
		{Kind: SegmentShell, Line: 31, Text: "gh pr reopen 123"},
		{Kind: SegmentShell, Line: 32, Text: "merged=$(gh pr merge 123)"},
		{Kind: SegmentShell, Line: 33, Text: "ready=$(bd ready)"},
		{Kind: SegmentShell, Line: 34, Text: `agm escalate --action="create PR" --reason blocked`},
	}

	var got []Violation
	for _, segment := range segments {
		got = append(got, evaluateSegment("AGENTS.md", segment)...)
	}
	if len(got) != 33 {
		t.Fatalf("violations = %v, want 33", got)
	}
	for _, item := range got {
		if item.Rule == "" || item.Replacement == "" {
			t.Fatalf("violation lacks actionable rule/replacement: %+v", item)
		}
	}
}

func TestGitHubAPIMergesRemainPolicyVisible(t *testing.T) {
	commands := []string{
		"gh api -X PUT repos/owner/repo/pulls/1/merge",
		`gh api graphql -f query='mutation { mergePullRequest(input:{pullRequestId:"PR_id"}) { pullRequest { state } } }'`,
		`gh api graphql -f query='mutation { enablePullRequestAutoMerge(input:{pullRequestId:"PR_id"}) { pullRequest { state } } }'`,
	}
	for _, command := range commands {
		violations := evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: command})
		if len(violations) != 1 || violations[0].Rule != "raw-gh-merge" {
			t.Errorf("%q violations = %v, want raw-gh-merge", command, violations)
		}
	}
	if violations := evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: "gh api repos/owner/repo/pulls/1/merge"}); len(violations) != 0 {
		t.Errorf("read-only merge-status request violations = %v, want none", violations)
	}
}

func TestGitHubAPIPRLifecycleRemainsPolicyVisible(t *testing.T) {
	commands := []string{
		"gh api -X POST repos/owner/repo/pulls -f title=test -f head=feature -f base=main",
		"gh api repos/owner/repo/pulls -f title=test -f head=feature -f base=main",
		"gh api -XPATCH repos/owner/repo/pulls/1 -f=state=closed",
		"gh api --method=PATCH repos/owner/repo/pulls/1 --raw-field=state=open",
		"gh api -X PATCH repos/owner/repo/pulls/$PR -f state=closed",
		`gh api graphql -f query='mutation { createPullRequest(input:{repositoryId:"R_id"}) { pullRequest { id } } }'`,
		`gh api graphql -f query='mutation { closePullRequest(input:{pullRequestId:"PR_id"}) { pullRequest { state } } }'`,
		`gh api graphql -f query='mutation { reopenPullRequest(input:{pullRequestId:"PR_id"}) { pullRequest { state } } }'`,
	}
	for _, command := range commands {
		violations := evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: command})
		if len(violations) != 1 || violations[0].Rule != "raw-gh-pr-lifecycle" {
			t.Errorf("%q violations = %v, want raw-gh-pr-lifecycle", command, violations)
		}
	}

	for _, command := range []string{
		"gh api repos/owner/repo/pulls",
		"gh api -X PATCH repos/owner/repo/pulls/1 -f title=updated",
	} {
		if violations := evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: command}); len(violations) != 0 {
			t.Errorf("%q violations = %v, want none", command, violations)
		}
	}
}

func TestExecLaunchedCommandsRemainPolicyVisible(t *testing.T) {
	commands := []string{
		"exec gh pr merge 123",
		"exec -a delivery git push origin main",
		"exec -cl bd ready",
	}
	var rules []string
	for _, command := range commands {
		for _, segment := range parseSegments([]byte("```text\n" + command + "\n```\n")) {
			for _, violation := range evaluateSegment("AGENTS.md", segment) {
				rules = append(rules, violation.Rule)
			}
		}
	}
	sort.Strings(rules)
	want := []string{"bare-beads", "raw-gh-merge", "raw-git-push"}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("exec-launched rules = %v, want %v", rules, want)
	}
}

func TestBackgroundShellCommandsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte("```\necho preparing & gh pr merge 123\n```\n"))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	if !reflect.DeepEqual(rules, []string{"raw-gh-merge"}) {
		t.Fatalf("background command rules = %v, want raw-gh-merge", rules)
	}

	want := []string{"echo preparing 2>&1", `printf 'a & b'`}
	for _, input := range want {
		if got := splitShellCommands(input); !reflect.DeepEqual(got, []string{input}) {
			t.Errorf("splitShellCommands(%q) = %v, want one unsplit command", input, got)
		}
	}
}

func TestExecutablePathsRemainPolicyVisible(t *testing.T) {
	segments := []Segment{
		{Kind: SegmentShell, Text: "/usr/bin/env bd ready"},
		{Kind: SegmentShell, Text: "/opt/homebrew/bin/bd ready"},
		{Kind: SegmentShell, Text: "/usr/local/bin/gh pr merge 123"},
		{Kind: SegmentShell, Text: "/usr/local/bin/git push origin main"},
		{Kind: SegmentShell, Text: "/usr/local/bin/agm status --output json"},
	}
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	want := []string{"agm-root-status", "bare-beads", "bare-beads", "raw-gh-merge", "raw-git-push"}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("path-qualified command rules = %v, want %v", rules, want)
	}
}

func TestShellCommandPayloadsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte(strings.Join([]string{
		"```",
		`bash -c 'git push origin main'`,
		`/bin/sh -c "gh pr merge 123"`,
		`env /opt/homebrew/bin/bash -lc 'bd ready'`,
		"```",
	}, "\n")))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	want := []string{"bare-beads", "raw-gh-merge", "raw-git-push"}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("shell -c rules = %v, want %v", rules, want)
	}
}

func TestGitGlobalOptionsRemainPolicyVisible(t *testing.T) {
	for _, command := range []string{
		"git -C ~/src/dear-agent push origin main",
		"/usr/bin/git --git-dir=.git push origin main",
		"git -c credential.helper= push origin main",
	} {
		violations := evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: command})
		if len(violations) != 1 || violations[0].Rule != "raw-git-push" {
			t.Errorf("%q violations = %v, want raw-git-push", command, violations)
		}
	}
}

func TestGitHubGlobalOptionsRemainPolicyVisible(t *testing.T) {
	segments := []Segment{
		{Kind: SegmentShell, Text: "gh -R owner/repo pr merge 123"},
		{Kind: SegmentShell, Text: "gh --repo=owner/repo pr create --title test"},
	}
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"raw-gh-merge", "raw-gh-pr-lifecycle"}) {
		t.Fatalf("GitHub global-option rules = %v", rules)
	}
}

func TestInlinePRLifecycleGuidanceRemainsPolicyVisible(t *testing.T) {
	var violations []Violation
	for _, segment := range parseSegments([]byte("Run `gh pr create --title test` after review.\n")) {
		violations = append(violations, evaluateSegment("AGENTS.md", segment)...)
	}
	if len(violations) != 1 || violations[0].Rule != "raw-gh-pr-lifecycle" {
		t.Fatalf("inline PR lifecycle violations = %v, want raw-gh-pr-lifecycle", violations)
	}
}

func TestShellBraceGroupsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte("```\n{ gh pr merge 123; }\n{ git push origin main; }\n```\n"))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"raw-gh-merge", "raw-git-push"}) {
		t.Fatalf("brace-group rules = %v", rules)
	}
}

func TestOrdinaryProseCommandsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte("Run gh pr merge 123 after review.\nUse git push origin main for delivery.\nExecute bd ready next.\n"))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"bare-beads", "raw-gh-merge", "raw-git-push"}) {
		t.Fatalf("ordinary prose rules = %v", rules)
	}
}

func TestRetiredWayfinderVocabularyIsCaseInsensitive(t *testing.T) {
	for _, token := range []string{"Wayfinder v1", "Wayfinder v1.", "Wayfinder w0", "Wayfinder d1", "Wayfinder s1", "Create W0-charter.md", "Use Wayfinder/V1", "follow the Wayfinder V1.x workflow"} {
		if !retiredWayfinderToken(token) {
			t.Errorf("retiredWayfinderToken does not match %q", token)
		}
	}
	for _, currentVersion := range []string{"ai.md/v1", "Use the Foo API V1", "v1.2", "Use an AWS S3 bucket", "Store state in a Cloudflare D1 database", "Select the S1 tier"} {
		if retiredWayfinderToken(currentVersion) {
			t.Errorf("retiredWayfinderToken unexpectedly matches %q", currentVersion)
		}
	}
	violations := evaluateSegment("docs/policies/wayfinder-v2-canonical.ai.md", Segment{Kind: SegmentProse, Text: "V1's 13-phase model is retired."})
	if len(violations) != 1 || violations[0].Rule != "wayfinder-v1" {
		t.Errorf("Wayfinder-path violations = %v, want wayfinder-v1", violations)
	}
}

func TestEnvSplitStringCommandsRemainPolicyVisible(t *testing.T) {
	commands := []string{
		`env -S 'git push origin main'`,
		`env --split-string='gh pr merge 123'`,
		`/usr/bin/env -S 'bd ready'`,
		`env -S git\ push origin main`,
	}
	var rules []string
	for _, command := range commands {
		for _, violation := range evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: command}) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	want := []string{"bare-beads", "raw-gh-merge", "raw-git-push", "raw-git-push"}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("env split-string rules = %v, want %v", rules, want)
	}
}

func TestIndentedRetiredWayfinderGuidanceRemainsPolicyVisible(t *testing.T) {
	source := []byte("    Use Wayfinder V1\n    Create W0-charter.md\n")
	var rules []string
	for _, segment := range parseSegments(source) {
		for _, violation := range evaluateSegment("AGENTS.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	if !reflect.DeepEqual(rules, []string{"wayfinder-v1", "wayfinder-v1"}) {
		t.Fatalf("indented guidance rules = %v, want two wayfinder-v1 violations", rules)
	}
}

func TestCheckRepositoryGovernsJSONManifests(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: hooks.json
      owner: harness-policy
`)
	writeTestFile(t, repo, "hooks.json", `{
  "permissions": {"allow": ["Bash(git push *)"]},
  "hooks": [{"command": "/usr/local/bin/gh pr merge 123"}]
}`)
	runGit(t, repo, "add", ".dear-agent.yml", "hooks.json")

	result, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, violation := range violations {
		rules = append(rules, violation.Rule)
	}
	sort.Strings(rules)
	if result.Files != 1 || !reflect.DeepEqual(rules, []string{"raw-gh-merge", "raw-git-push"}) {
		t.Fatalf("result=%+v rules=%v, want governed JSON commands", result, rules)
	}
}

func TestCheckRepositoryGovernsAgentReadableYAML(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: .dear-agent.yml
      owner: repository-policy
pr-policy:
  emergency: safe-pr create --emergency --reason bypass
`)
	runGit(t, repo, "add", ".dear-agent.yml")

	result, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 1 || len(violations) != 1 || violations[0].Rule != "safe-pr-emergency" {
		t.Fatalf("result=%+v violations=%v, want governed YAML emergency violation", result, violations)
	}
}

func TestYAMLCommentsRemainPolicyVisible(t *testing.T) {
	segments, err := parseYAMLSegments([]byte("# Do not instruct agents to run `safe-pr create --emergency --reason bypass`.\npolicy: active # `agm status` is retired\n"))
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment(".dear-agent.yml", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"agm-root-status", "safe-pr-emergency"}) {
		t.Fatalf("YAML comment rules = %v", rules)
	}
}

func TestMarkdownFrontmatterCommandsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte("---\nallowed-tools: Bash(gh pr merge:*), Bash(agm status *)\n---\n\n# Command\n"))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("command.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"agm-root-status", "raw-gh-merge"}) {
		t.Fatalf("frontmatter command rules = %v", rules)
	}
}

func TestMarkdownFrontmatterSequenceCommandsRemainPolicyVisible(t *testing.T) {
	segments := parseSegments([]byte("---\nallowed-tools:\n  - Bash(gh pr merge:*)\n  - Bash(agm status *)\n---\n\n# Command\n"))
	var rules []string
	for _, segment := range segments {
		for _, violation := range evaluateSegment("command.md", segment) {
			rules = append(rules, violation.Rule)
		}
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"agm-root-status", "raw-gh-merge"}) {
		t.Fatalf("sequence frontmatter command rules = %v", rules)
	}
}

func TestAGMPersistentFlagsDoNotHideLegacyRootCommands(t *testing.T) {
	for _, input := range []string{"agm -o json status", "agm -C repo new worker"} {
		var rules []string
		for _, violation := range evaluateSegment("AGENTS.md", Segment{Kind: SegmentShell, Text: input}) {
			rules = append(rules, violation.Rule)
		}
		if len(rules) != 1 {
			t.Fatalf("%q rules = %v, want one legacy AGM finding", input, rules)
		}
	}
}

func TestCheckRepositoryRejectsImportOutsideGovernedInventory(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
`)
	writeTestFile(t, repo, "AGENTS.md", "@import hidden.md\n")
	writeTestFile(t, repo, "hidden.md", "# Hidden instructions\n")
	runGit(t, repo, "add", ".")

	_, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Rule != "ungoverned-import" {
		t.Fatalf("violations = %v, want ungoverned import", violations)
	}
}

func TestCheckRepositoryValidatesGovernedSymlinkTargets(t *testing.T) {
	t.Run("governed target", func(t *testing.T) {
		repo := t.TempDir()
		runGit(t, repo, "init", "-q")
		writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: wayfinder/SKILL.md
      owner: wayfinder
    - match: wayfinder/skills/wayfinder/SKILL.md
      owner: plugin
`)
		writeTestFile(t, repo, "wayfinder/SKILL.md", "# Governed skill\n")
		if err := os.MkdirAll(filepath.Join(repo, "wayfinder", "skills", "wayfinder"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../SKILL.md", filepath.Join(repo, "wayfinder", "skills", "wayfinder", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", ".")

		result, violations, err := CheckRepository(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		if result.Files != 2 || len(violations) != 0 {
			t.Fatalf("result=%+v violations=%v, want two governed paths", result, violations)
		}
	})

	t.Run("ungoverned target", func(t *testing.T) {
		repo := t.TempDir()
		runGit(t, repo, "init", "-q")
		writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: skills/current/SKILL.md
      owner: plugin
`)
		writeTestFile(t, repo, "hidden.md", "# Hidden instructions\n")
		if err := os.MkdirAll(filepath.Join(repo, "skills", "current"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../hidden.md", filepath.Join(repo, "skills", "current", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", ".")

		_, _, err := CheckRepository(context.Background(), repo)
		if err == nil || !strings.Contains(err.Error(), "must be tracked and match exactly one instruction surface") {
			t.Fatalf("error = %v, want ungoverned symlink target rejection", err)
		}
	})
}

func TestExclusionRatchetRejectsNewAndIncreasedDebt(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
  exclusions-file: .instruction-policy-exclusions.yml
`)
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n")
	writeTestFile(t, repo, ".instruction-policy-exclusions.yml", `exclusions:
  - path: AGENTS.md
    rule: bare-beads
    excerpt: bd ready
    context: "0000000000000000000000000000000000000000000000000000000000000000"
    count: 1
    owner: test
    reason: fixture
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "baseline")
	baseline := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, ".instruction-policy-exclusions.yml", `exclusions:
  - path: AGENTS.md
    rule: bare-beads
    excerpt: bd ready
    context: "0000000000000000000000000000000000000000000000000000000000000000"
    count: 2
    owner: test
    reason: fixture
  - path: AGENTS.md
    rule: raw-git-push
    excerpt: git push
    context: "0000000000000000000000000000000000000000000000000000000000000000"
    count: 1
    owner: test
    reason: fixture
`)
	violations, err := CheckExclusionRatchet(context.Background(), repo, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("violations = %v, want one increased and one new exclusion", violations)
	}
	for _, violation := range violations {
		if violation.Rule != "exclusion-growth" {
			t.Fatalf("unexpected ratchet violation: %+v", violation)
		}
	}
}

func TestExclusionRatchetRejectsSurfaceRemoval(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
    - match: CODEX.md
      owner: codex
`)
	writeTestFile(t, repo, "AGENTS.md", "# Root instructions\n")
	writeTestFile(t, repo, "CODEX.md", "# Codex instructions\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "baseline")
	baseline := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
`)
	violations, err := CheckExclusionRatchet(context.Background(), repo, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Rule != "surface-removal" || !strings.Contains(violations[0].Excerpt, "CODEX.md") {
		t.Fatalf("violations = %v, want removed CODEX.md surface", violations)
	}
}

func TestExclusionRatchetAllowsBootstrapAndShrink(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
  exclusions-file: .instruction-policy-exclusions.yml
`)
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "pre-policy")
	bootstrap := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	writeTestFile(t, repo, ".instruction-policy-exclusions.yml", `exclusions:
  - path: AGENTS.md
    rule: bare-beads
    excerpt: bd ready
    context: "0000000000000000000000000000000000000000000000000000000000000000"
    count: 2
    owner: test
    reason: fixture
`)
	if violations, err := CheckExclusionRatchet(context.Background(), repo, bootstrap); err != nil || len(violations) != 0 {
		t.Fatalf("bootstrap violations=%v err=%v", violations, err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "policy")
	baseline := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))
	writeTestFile(t, repo, ".instruction-policy-exclusions.yml", `exclusions:
  - path: AGENTS.md
    rule: bare-beads
    excerpt: bd ready
    context: "0000000000000000000000000000000000000000000000000000000000000000"
    count: 1
    owner: test
    reason: fixture
`)
	if violations, err := CheckExclusionRatchet(context.Background(), repo, baseline); err != nil || len(violations) != 0 {
		t.Fatalf("shrink violations=%v err=%v", violations, err)
	}
}

func TestExclusionRatchetComparesFileBaselineWithInlineCurrentPolicy(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", "instruction-policy:\n  surfaces:\n    - match: AGENTS.md\n      owner: root\n  exclusions-file: .instruction-policy-exclusions.yml\n")
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n")
	writeTestFile(t, repo, ".instruction-policy-exclusions.yml", "exclusions:\n  - path: AGENTS.md\n    rule: bare-beads\n    excerpt: bd ready\n    context: \""+testContextFingerprint+"\"\n    count: 1\n    owner: test\n    reason: fixture\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "baseline")
	baseline := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "HEAD"))

	writeTestFile(t, repo, ".dear-agent.yml", "instruction-policy:\n  surfaces:\n    - match: AGENTS.md\n      owner: root\n  exclusions:\n    - path: AGENTS.md\n      rule: bare-beads\n      excerpt: bd ready\n      context: \""+testContextFingerprint+"\"\n      count: 2\n      owner: test\n      reason: fixture\n    - path: AGENTS.md\n      rule: raw-git-push\n      excerpt: git push\n      context: \""+testContextFingerprint+"\"\n      count: 1\n      owner: test\n      reason: fixture\n")
	violations, err := CheckExclusionRatchet(context.Background(), repo, baseline)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("violations = %v, want one increased and one new inline exclusion", violations)
	}
	for _, violation := range violations {
		if violation.Path != ".dear-agent.yml" || violation.Rule != "exclusion-growth" {
			t.Fatalf("unexpected inline ratchet violation: %+v", violation)
		}
	}
}

func TestCheckRepositoryRejectsUntrackedImportMatchingBroadSurface(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
    - match: skills/**/SKILL.md
      owner: skills
`)
	writeTestFile(t, repo, "AGENTS.md", "@import skills/missing/SKILL.md\n")
	writeTestFile(t, repo, "skills/existing/SKILL.md", "# Existing\n")
	runGit(t, repo, "add", ".")

	_, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Rule != "ungoverned-import" {
		t.Fatalf("violations = %v, want untracked import finding", violations)
	}
}

func TestApplyExclusionsIsExactAndStaleDetecting(t *testing.T) {
	findings := []Violation{
		{Path: "AGENTS.md", Line: 3, Rule: "bare-beads", Excerpt: "bd ready", Context: testContextFingerprint, Replacement: "canonical bd"},
		{Path: "AGENTS.md", Line: 4, Rule: "bare-beads", Excerpt: "bd ready", Context: "1111111111111111111111111111111111111111111111111111111111111111", Replacement: "canonical bd"},
	}
	exclusions := []Exclusion{
		{Path: "AGENTS.md", Rule: "bare-beads", Excerpt: "bd ready", Context: testContextFingerprint, Count: 1, Owner: "#930", Reason: "root router"},
		{Path: "AGENTS.md", Rule: "raw-git-push", Excerpt: "git push", Context: testContextFingerprint, Count: 1, Owner: "#930", Reason: "root router"},
	}

	got := applyExclusions(findings, exclusions)
	if len(got) != 2 {
		t.Fatalf("violations = %v, want one excess and one stale exclusion", got)
	}
	rules := []string{got[0].Rule, got[1].Rule}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"bare-beads", "stale-exclusion"}) {
		t.Fatalf("unexpected violations: %v", got)
	}
}

func TestCheckRepositoryUsesTrackedGovernedInventory(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	legacy := []byte("# Instructions\n\n```bash\nbd ready\n```\n")
	legacyContext := contextFingerprint(legacy, 4)
	writeTestFile(t, repo, ".dear-agent.yml", fmt.Sprintf(`instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
  exclusions:
    - path: AGENTS.md
      rule: bare-beads
      excerpt: bd ready
      context: %q
      count: 1
      owner: "#930"
      reason: root router owns the exact legacy line
`, legacyContext))
	writeTestFile(t, repo, "AGENTS.md", string(legacy))
	writeTestFile(t, repo, "untracked.md", "Create W0-charter.md.\n")
	runGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")

	result, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 || result.Files != 1 || result.Exclusions != 1 {
		t.Fatalf("result=%+v violations=%v", result, violations)
	}

	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n\nCreate W0-charter.md.\n\n```bash\nbd ready\n```\n")
	_, violations, err = CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, violation := range violations {
		rules = append(rules, violation.Rule)
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"bare-beads", "stale-exclusion", "wayfinder-v1"}) {
		t.Fatalf("violations = %v, want moved debt plus unexcluded Wayfinder V1 token", violations)
	}
}

func TestExclusionContextDoesNotSuppressMovedGuidance(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	legacy := []byte("# Safety\n\nNever run `git push` directly.\n")
	writeTestFile(t, repo, ".dear-agent.yml", fmt.Sprintf(`instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
  exclusions:
    - path: AGENTS.md
      rule: raw-git-push
      excerpt: git push
      context: %q
      count: 1
      owner: test
      reason: negative reference fixture
`, contextFingerprint(legacy, 3)))
	writeTestFile(t, repo, "AGENTS.md", string(legacy))
	runGit(t, repo, "add", ".")

	if _, violations, err := CheckRepository(context.Background(), repo); err != nil || len(violations) != 0 {
		t.Fatalf("legacy violations=%v err=%v", violations, err)
	}
	writeTestFile(t, repo, "AGENTS.md", "# Delivery\n\nRun `git push` now.\n")
	_, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, violation := range violations {
		rules = append(rules, violation.Rule)
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"raw-git-push", "stale-exclusion"}) {
		t.Fatalf("moved guidance rules = %v, want new finding plus stale exclusion", rules)
	}
}

func TestExclusionContextIncludesPhysicalLine(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	legacy := []byte("# Safety\n\nBefore.\n\nNever run `git push` directly.\n\nAfter.\n")
	writeTestFile(t, repo, ".dear-agent.yml", fmt.Sprintf(`instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
  exclusions:
    - path: AGENTS.md
      rule: raw-git-push
      excerpt: git push
      context: %q
      count: 1
      owner: test
      reason: negative reference fixture
`, contextFingerprint(legacy, 5)))
	writeTestFile(t, repo, "AGENTS.md", string(legacy))
	runGit(t, repo, "add", ".")

	if _, violations, err := CheckRepository(context.Background(), repo); err != nil || len(violations) != 0 {
		t.Fatalf("legacy violations=%v err=%v", violations, err)
	}
	writeTestFile(t, repo, "AGENTS.md", "\n"+string(legacy))
	_, violations, err := CheckRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	var rules []string
	for _, violation := range violations {
		rules = append(rules, violation.Rule)
	}
	sort.Strings(rules)
	if !reflect.DeepEqual(rules, []string{"raw-git-push", "stale-exclusion"}) {
		t.Fatalf("moved guidance rules = %v, want new finding plus stale exclusion", rules)
	}
}

func TestCheckRepositoryRejectsOverlappingSurfaces(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
    - match: "*.md"
      owner: catch-all
`)
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n")
	runGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")
	if _, _, err := CheckRepository(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "multiple instruction surfaces") {
		t.Fatalf("overlapping surfaces error = %v", err)
	}
}

func TestCheckRepositoryRejectsSurfaceWithNoTrackedMatches(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	writeTestFile(t, repo, ".dear-agent.yml", `instruction-policy:
  surfaces:
    - match: AGENTS.md
      owner: root
    - match: CODEX.md
      owner: codex
`)
	writeTestFile(t, repo, "AGENTS.md", "# Instructions\n")
	runGit(t, repo, "add", ".dear-agent.yml", "AGENTS.md")
	if _, _, err := CheckRepository(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "CODEX.md") {
		t.Fatalf("zero-match surface error = %v", err)
	}
}

func TestRepositoryInstructionPolicyIsConformant(t *testing.T) {
	root := repositoryRoot(t)
	result, violations, err := CheckRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("repository instruction-policy violations: %v", violations)
	}
	if result.Files < 10 {
		t.Fatalf("governed files = %d, want a nontrivial active inventory", result.Files)
	}
}

func writeTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	gittest.Run(t, root, args...)
	if len(args) > 0 && args[0] == "init" {
		for _, identity := range [][2]string{{"user.name", "instructionlint tests"}, {"user.email", "instructionlint@example.invalid"}} {
			gittest.Run(t, root, "config", "--local", identity[0], identity[1])
		}
	}
}

func runGitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := gittest.Command(t, root, args...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(output)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		t.Skipf("not in a Git worktree: %v", err)
	}
	return strings.TrimSpace(string(output))
}
