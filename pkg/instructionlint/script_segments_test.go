package instructionlint

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

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
