package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// providerStep is one response from the fake gh boundary. Tests deliberately
// provide one step per expected request so an unexpected retry or mutation
// cannot hide behind a reusable canned response.
type providerStep struct {
	stdout string
	stderr string
	exit   int
}

type sequencedProvider struct {
	t     *testing.T
	dir   string
	steps int
}

func installSequencedProvider(t *testing.T, steps ...providerStep) *sequencedProvider {
	t.Helper()
	dir := t.TempDir()
	for i, step := range steps {
		for name, contents := range map[string]string{
			fmt.Sprintf("stdout.%d", i): step.stdout,
			fmt.Sprintf("stderr.%d", i): step.stderr,
			fmt.Sprintf("exit.%d", i):   strconv.Itoa(step.exit),
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
				t.Fatalf("write fake-provider %s: %v", name, err)
			}
		}
	}
	script := `#!/bin/sh
set -eu
root="$GH_SEQUENCE_DIR"
index=0
if [ -f "$root/index" ]; then
  index="$(cat "$root/index")"
fi
cat > "$root/request.$index"
printf '%s' "$((index + 1))" > "$root/index"
cat "$root/stdout.$index"
cat "$root/stderr.$index" >&2
exit "$(cat "$root/exit.$index")"
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_SEQUENCE_DIR", dir)
	return &sequencedProvider{t: t, dir: dir, steps: len(steps)}
}

func (p *sequencedProvider) capture(run func() int) (code int, stdout, stderr string) {
	p.t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		p.t.Fatalf("open stdout capture: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		p.t.Fatalf("open stderr capture: %v", err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	restored := false
	defer func() {
		if restored {
			return
		}
		os.Stdout, os.Stderr = oldOut, oldErr
		_ = outW.Close()
		_ = errW.Close()
		_ = outR.Close()
		_ = errR.Close()
	}()
	code = run()
	os.Stdout, os.Stderr = oldOut, oldErr
	restored = true
	if err := outW.Close(); err != nil {
		p.t.Fatalf("close stdout writer: %v", err)
	}
	if err := errW.Close(); err != nil {
		p.t.Fatalf("close stderr writer: %v", err)
	}
	out, err := io.ReadAll(outR)
	if err != nil {
		p.t.Fatalf("read stdout capture: %v", err)
	}
	errOut, err := io.ReadAll(errR)
	if err != nil {
		p.t.Fatalf("read stderr capture: %v", err)
	}
	_ = outR.Close()
	_ = errR.Close()
	return code, string(out), string(errOut)
}

type recoveryGraphQLRequest struct {
	Query     string
	Variables map[string]json.RawMessage
}

func (p *sequencedProvider) requests() []recoveryGraphQLRequest {
	p.t.Helper()
	count := p.requestCount()
	requests := make([]recoveryGraphQLRequest, 0, count)
	for i := range count {
		raw, err := os.ReadFile(filepath.Join(p.dir, fmt.Sprintf("request.%d", i)))
		if err != nil {
			p.t.Fatalf("read request %d: %v", i, err)
		}
		var request recoveryGraphQLRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			p.t.Fatalf("decode request %d: %v\n%s", i, err, raw)
		}
		requests = append(requests, request)
	}
	return requests
}

func (p *sequencedProvider) requestCount() int {
	p.t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.dir, "index"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		p.t.Fatalf("read provider request count: %v", err)
	}
	count, err := strconv.Atoi(string(raw))
	if err != nil {
		p.t.Fatalf("decode provider request count %q: %v", raw, err)
	}
	return count
}

func (p *sequencedProvider) assertExhausted() {
	p.t.Helper()
	if got := p.requestCount(); got != p.steps {
		p.t.Fatalf("provider received %d requests, want exactly %d", got, p.steps)
	}
}

func queryKinds(requests []recoveryGraphQLRequest) []string {
	kinds := make([]string, 0, len(requests))
	for _, request := range requests {
		switch {
		case strings.Contains(request.Query, "reviewThreads(first:100"):
			kinds = append(kinds, "list")
		case strings.Contains(request.Query, "addPullRequestReviewThreadReply"):
			kinds = append(kinds, "reply")
		case strings.Contains(request.Query, "unresolveReviewThread"):
			kinds = append(kinds, "unresolve")
		case strings.Contains(request.Query, "resolveReviewThread"):
			kinds = append(kinds, "resolve")
		case strings.Contains(request.Query, "comments(first:100"):
			kinds = append(kinds, "history")
		case strings.Contains(request.Query, "node(id:$id)"):
			kinds = append(kinds, "thread")
		default:
			kinds = append(kinds, "unknown")
		}
	}
	return kinds
}

type providerComment struct {
	id    string
	login string
	body  string
}

func threadResponse(threadID string, resolved bool, comments ...providerComment) string {
	return fmt.Sprintf(`{"data":{"node":%s}}`, threadNodeResponse(threadID, resolved, comments...))
}

func threadNodeResponse(threadID string, resolved bool, comments ...providerComment) string {
	opening := ""
	if len(comments) > 0 {
		opening = fmt.Sprintf(`{"author":{"login":%q},"body":%q}`, comments[0].login, comments[0].body)
	}
	recent := make([]string, 0, len(comments))
	for _, comment := range comments {
		recent = append(recent, fmt.Sprintf(`{"id":%q,"author":{"login":%q},"body":%q}`,
			comment.id, comment.login, comment.body))
	}
	return fmt.Sprintf(`{"id":%q,"isResolved":%t,"isOutdated":false,"path":"review.go","opening":{"totalCount":%d,"nodes":[%s]},"recent":{"nodes":[%s]}}`,
		threadID, resolved, len(comments), opening, strings.Join(recent, ","))
}

func listResponse(nodes ...string) string {
	return fmt.Sprintf(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[%s]}}}}}`, strings.Join(nodes, ","))
}

func historyResponse(comments ...providerComment) string {
	nodes := make([]string, 0, len(comments))
	for _, comment := range comments {
		nodes = append(nodes, fmt.Sprintf(`{"id":%q,"author":{"login":%q},"body":%q}`,
			comment.id, comment.login, comment.body))
	}
	return fmt.Sprintf(`{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[%s]}}}}`, strings.Join(nodes, ","))
}

func replyResponse(commentID string) string {
	return fmt.Sprintf(`{"data":{"addPullRequestReviewThreadReply":{"comment":{"id":%q}}}}`, commentID)
}

func resolveResponse(threadID, lastCommentID string) string {
	return resolveResponseWithState(threadID, lastCommentID, true)
}

func resolveResponseWithState(threadID, lastCommentID string, resolved bool) string {
	nodes := ""
	if lastCommentID != "" {
		nodes = fmt.Sprintf(`{"id":%q}`, lastCommentID)
	}
	return fmt.Sprintf(`{"data":{"resolveReviewThread":{"thread":{"id":%q,"isResolved":%t,"comments":{"nodes":[%s]}}}}}`, threadID, resolved, nodes)
}

func unresolveResponse(threadID string) string {
	return unresolveResponseWithState(threadID, false)

}

func unresolveResponseWithState(threadID string, resolved bool) string {
	return fmt.Sprintf(`{"data":{"unresolveReviewThread":{"thread":{"id":%q,"isResolved":%t}}}}`, threadID, resolved)
}

func malformedMutationResponse(field, variant, requestedID, lastCommentID string, resolved bool) string {
	switch variant {
	case "missing":
		return fmt.Sprintf(`{"data":{%q:{}}}`, field)
	case "null":
		return fmt.Sprintf(`{"data":{%q:{"thread":null}}}`, field)
	case "mismatched":
		otherID := requestedID + "_other"
		if field == "resolveReviewThread" {
			return resolveResponseWithState(otherID, lastCommentID, resolved)
		}
		return unresolveResponseWithState(otherID, resolved)
	case "state-missing", "state-null":
		state := ""
		if variant == "state-null" {
			state = `,"isResolved":null`
		}
		comments := ""
		if field == "resolveReviewThread" {
			comments = fmt.Sprintf(`,"comments":{"nodes":[{"id":%q}]}`, lastCommentID)
		}
		return fmt.Sprintf(`{"data":{%q:{"thread":{"id":%q%s%s}}}}`,
			field, requestedID, state, comments)
	default:
		panic("unknown malformed mutation variant: " + variant)
	}
}

func assertQueryKinds(t *testing.T, provider *sequencedProvider, want ...string) {
	t.Helper()
	got := queryKinds(provider.requests())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider query order = %q, want %q", got, want)
	}
}

func assertQueryTargets(t *testing.T, provider *sequencedProvider, threadID string) {
	t.Helper()
	for i, request := range provider.requests() {
		key := "id"
		if strings.Contains(request.Query, "mutation(") {
			key = "threadId"
		}
		raw, ok := request.Variables[key]
		if !ok {
			t.Errorf("provider request %d omitted %q target variable", i, key)
			continue
		}
		var got string
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("decode provider request %d target %q: %v", i, raw, err)
			continue
		}
		if got != threadID {
			t.Errorf("provider request %d target = %q, want %q", i, got, threadID)
		}
	}
}

func assertContainsAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func assertContainsNone(t *testing.T, got string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		if strings.Contains(got, value) {
			t.Errorf("output contains forbidden %q:\n%s", value, got)
		}
	}
}

func TestPostReplyRecoveryMatrixPreservesOriginalTail(t *testing.T) {
	const (
		threadID    = "PRRT_recovery"
		originalID  = "PRRC_original"
		recoveredID = "PRRC_recovered"
		body        = "Addressed with a descriptor-bound read."
		customPath  = "/private/tmp/custom reply's body"
		canonical   = "/tmp/resolve-review-thread.ABC123"
	)
	original := providerComment{id: originalID, login: "reviewer", body: "P2: close the race"}
	recovered := providerComment{id: recoveredID, login: "author", body: body}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "This still races."}

	t.Run("missing id recovers exact reply and verifies against original tail", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: replyResponse("")},
			providerStep{stdout: historyResponse(original, recovered)},
			providerStep{stdout: threadResponse(threadID, false, original, recovered)},
		)
		var gotID string
		code, _, diagnostics := provider.capture(func() int {
			var postCode int
			gotID, postCode = postReplyOrExit(
				context.Background(), threadID, body, originalID, customPath,
			)
			if postCode >= 0 {
				return postCode
			}
			return verifyReplyPlacement(
				context.Background(), threadID, originalID, gotID, customPath,
			)
		})
		if code != 0 {
			t.Fatalf("recovered reply placement failed with code %d:\n%s", code, diagnostics)
		}
		if gotID != recoveredID {
			t.Fatalf("recovered reply ID = %q, want %q", gotID, recoveredID)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history", "thread")
		assertContainsNone(t, diagnostics, "Retry with:", "revise the same named body source", "safe-merge")
	})

	t.Run("moved tail without exact reply revises same custom source", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: "transport dropped after write", exit: 1},
			providerStep{stdout: historyResponse(original, followup)},
			providerStep{stdout: threadResponse(threadID, false, original, followup)},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body, originalID, customPath,
			)
			return postCode
		})
		if code == 0 {
			t.Fatalf("moved-tail ambiguity succeeded:\n%s", diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history", "thread")
		assertContainsAll(t, diagnostics,
			"revise the same named body source in place",
			`reply_file='/private/tmp/custom reply'"'"'s body'`,
			`--body-file "$reply_file"`,
		)
		assertContainsNone(t, diagnostics,
			"Keep the exact same named reply-body source and do not edit",
			"create a task-owned reply file",
		)
	})

	t.Run("unchanged tail selects source-aware exact retry", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: "transport dropped before acknowledgement", exit: 1},
			providerStep{stdout: historyResponse(original)},
			providerStep{stdout: threadResponse(threadID, false, original)},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body, originalID, canonical,
			)
			return postCode
		})
		if code == 0 {
			t.Fatalf("unconfirmed post succeeded:\n%s", diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history", "thread")
		assertContainsAll(t, diagnostics,
			"Keep the exact same named reply-body source and do not edit or replace it",
			"reply_file='/tmp/resolve-review-thread.ABC123'",
			"Retain that file unchanged",
		)
		assertContainsNone(t, diagnostics,
			"revise the same named body source in place",
			"create a task-owned reply file",
		)
	})

	t.Run("unreadable history requires inspection and retains stdin", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: "transport dropped after write", exit: 1},
			providerStep{stderr: "history unavailable", exit: 1},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body, originalID, "-",
			)
			return postCode
		})
		if code == 0 {
			t.Fatalf("unreadable provider state succeeded:\n%s", diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history")
		assertContainsAll(t, diagnostics,
			"Retain the exact standard-input bytes while provider state is unverified",
			"Do not revise or discard them",
			"do not continue to another thread or safe-merge",
			"Inspect the live thread before any retry",
		)
		assertContainsNone(t, diagnostics,
			"rm -f",
			"Replay the exact same retained standard-input bytes without revising them",
			"Replay the revised retained bytes",
		)
	})
}

func TestResolvedPlacementMismatchReopensBeforeRevisedGuidance(t *testing.T) {
	const (
		threadID   = "PRRT_resolved_race"
		originalID = "PRRC_original"
		replyID    = "PRRC_reply"
		bodyFile   = "/private/tmp/custom-recovery-body"
	)
	original := providerComment{id: originalID, login: "reviewer", body: "P2: close the race"}
	reply := providerComment{id: replyID, login: "author", body: "Fixed."}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "Not yet."}

	tests := []struct {
		name     string
		bodyFile string
		comments []providerComment
		marker   string
	}{
		{
			name:     "buried",
			bodyFile: bodyFile,
			comments: []providerComment{original, reply, followup},
			marker:   "posted reply is not last",
		},
		{
			name:     "jumped",
			bodyFile: "-",
			comments: []providerComment{original, followup, reply},
			marker:   "arrived between the original read",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, true, tc.comments...)},
				providerStep{stdout: unresolveResponse(threadID)},
			)
			code, _, diagnostics := provider.capture(func() int {
				return verifyReplyPlacement(
					context.Background(), threadID, originalID, replyID, tc.bodyFile,
				)
			})
			if code == 0 {
				t.Fatalf("resolved %s placement mismatch succeeded:\n%s", tc.name, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "unresolve")
			assertContainsAll(t, diagnostics, tc.marker, "revised")
			if tc.bodyFile == "-" {
				assertContainsAll(t, diagnostics,
					"revise the retained standard-input bytes",
					"--body-file -",
				)
			} else {
				assertContainsAll(t, diagnostics,
					"revise the same named body source in place",
					"reply_file='/private/tmp/custom-recovery-body'",
				)
			}
		})
	}
}

func TestResolutionMutationRecoveryDistinguishesUnverifiableFromSuperseded(t *testing.T) {
	const (
		threadID = "PRRT_resolution_recovery"
		anchorID = "PRRC_reply"
		bodyFile = "/tmp/resolve-review-thread.RESOLVE"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P2: prove it"}
	reply := providerComment{id: anchorID, login: "author", body: "Proved."}

	tests := []struct {
		name         string
		mutationTail string
		want         []string
		forbidden    []string
	}{
		{
			name:         "empty mutation anchor keeps unchanged source",
			mutationTail: "",
			want: []string{
				"did not confirm its evidence anchor",
				"do not revise the reply source",
				"Keep the exact same named reply-body source",
				"reply_file='/tmp/resolve-review-thread.RESOLVE'",
			},
			forbidden: []string{"revise the same named body source in place"},
		},
		{
			name:         "changed nonempty mutation anchor needs new answer",
			mutationTail: "PRRC_newer",
			want: []string{
				"newer comment",
				"revised-answer lifecycle",
				"revise the same named body source in place",
				"reply_file='/tmp/resolve-review-thread.RESOLVE'",
			},
			forbidden: []string{"do not revise the reply source"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, reply)},
				providerStep{stdout: resolveResponse(threadID, tc.mutationTail)},
				providerStep{stdout: unresolveResponse(threadID)},
			)
			var recovery string
			code, _, diagnostics := provider.capture(func() int {
				_, mutated, err := resolveWithEvidence(context.Background(), threadID, false, anchorID)
				if err == nil || mutated {
					t.Fatalf("resolveWithEvidence = (mutated=%t, err=%v), want reopened failure", mutated, err)
				}
				var handled bool
				recovery, handled = replyResolutionEvidenceFailure(
					threadID,
					err,
					revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
				)
				if !handled {
					t.Fatalf("recovery error was not classified: %T: %v", err, err)
				}
				return 0
			})
			if code != 0 || diagnostics != "" {
				t.Fatalf("recovery classification emitted unexpected command diagnostics (code=%d): %s", code, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "unresolve")
			assertContainsAll(t, recovery, tc.want...)
			assertContainsNone(t, recovery, tc.forbidden...)
		})
	}
}

func TestUnavailableOrEqualAuthorEvidenceInspectsAndRetains(t *testing.T) {
	const (
		threadID = "PRRT_author_evidence"
		anchorID = "PRRC_reply"
		bodyFile = "/tmp/resolve-review-thread.AUTHOR"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P2: prove it"}
	tests := []struct {
		name      string
		lastLogin string
	}{
		{name: "unavailable latest author", lastLogin: ""},
		{name: "latest author equals opener", lastLogin: "reviewer"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			last := providerComment{id: anchorID, login: tc.lastLogin, body: "Attempted reply."}
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, last)},
			)
			var recovery string
			code, _, diagnostics := provider.capture(func() int {
				_, mutated, err := resolveWithEvidence(context.Background(), threadID, false, anchorID)
				if err == nil || mutated {
					t.Fatalf("resolveWithEvidence = (mutated=%t, err=%v), want evidence refusal", mutated, err)
				}
				var handled bool
				recovery, handled = replyResolutionEvidenceFailure(
					threadID,
					err,
					revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
				)
				if !handled {
					t.Fatalf("author-evidence error was not classified: %T: %v", err, err)
				}
				return 0
			})
			if code != 0 || diagnostics != "" {
				t.Fatalf("author-evidence classification emitted unexpected diagnostics (code=%d): %s", code, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread")
			assertContainsAll(t, recovery,
				"author evidence is insufficient",
				"Retain the exact named reply-body source unchanged",
				"reply_file='/tmp/resolve-review-thread.AUTHOR'",
				"Inspect the live thread before any retry",
			)
			assertContainsNone(t, recovery,
				"revise the same named body source in place",
				"Replay the revised retained bytes",
			)
		})
	}

	t.Run("externally resolved exact tail still requires independent answer evidence", func(t *testing.T) {
		const body = "The same opening identity supplied this attempted answer."
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		reply := providerComment{id: anchorID, login: original.login, body: body}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: historyResponse(original)},
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: replyResponse(anchorID)},
			providerStep{stdout: threadResponse(threadID, false, original, reply)},
			providerStep{stdout: threadResponse(threadID, true, original, reply)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("resolved exact tail bypassed independent-answer evidence:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("evidence refusal printed terminal success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "thread", "thread")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"author evidence is insufficient",
			"Retain the exact named reply-body source unchanged",
			"Inspect the live thread before any retry",
		)
		assertContainsNone(t, diagnostics,
			"skipped "+threadID+" (already resolved)",
			"Only then continue",
			"rm -f",
		)
	})
}

func TestMutationResponsesRequireRequestedIdentityAndState(t *testing.T) {
	const (
		threadID = "PRRT_postcondition"
		anchorID = "PRRC_answer"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: prove the state"}
	answer := providerComment{id: anchorID, login: "author", body: "The state is proved."}

	t.Run("nominal resolve response remains unresolved", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original, answer)},
			providerStep{stdout: resolveResponseWithState(threadID, anchorID, false)},
			providerStep{stdout: threadResponse(threadID, false, original, answer)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"resolve", threadID})
		})
		if code == 0 {
			t.Fatalf("resolve accepted isResolved=false:\nstdout:\n%s\nstderr:\n%s", stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("failed resolve printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "resolve", "thread")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, strings.ToLower(diagnostics), "resolved")
		assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
	})

	t.Run("nominal unresolve response remains resolved", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: unresolveResponseWithState(threadID, true)},
			providerStep{stdout: threadResponse(threadID, true, original, answer)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"unresolve", threadID})
		})
		if code == 0 {
			t.Fatalf("unresolve accepted isResolved=true:\nstdout:\n%s\nstderr:\n%s", stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("failed unresolve printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "unresolve", "thread")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, strings.ToLower(diagnostics), "resolved")
		assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
	})

	t.Run("forced resolve still requires a nonempty mutation anchor", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: resolveResponse(threadID, "")},
			providerStep{stdout: unresolveResponse(threadID)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"resolve", threadID, "--force"})
		})
		if code == 0 {
			t.Fatalf("forced resolve accepted an empty mutation anchor:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("unverifiable forced resolve printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "resolve", "unresolve")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"did not confirm its evidence anchor",
			"automatic reopen restored a safe retry state",
			"Retry the unchanged resolution operation",
			"resolve-review-threads resolve "+threadID+" --force",
		)
	})

	t.Run("identity payloads", testMutationThreadIdentityRequiresExactTargetOrRereadProof)
}

func testMutationThreadIdentityRequiresExactTargetOrRereadProof(t *testing.T) {
	const (
		threadID = "PRRT_identity"
		anchorID = "PRRC_answer"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: verify identity"}
	answer := providerComment{id: anchorID, login: "author", body: "Identity verified."}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "This changed."}

	for _, variant := range []string{"missing", "null", "mismatched", "state-missing", "state-null"} {
		t.Run("resolve "+variant+" identity does not prove resolution", func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, answer)},
				providerStep{stdout: malformedMutationResponse(
					"resolveReviewThread", variant, threadID, anchorID, true,
				)},
				providerStep{stdout: threadResponse(threadID, false, original, answer)},
			)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"resolve", threadID})
			})
			if code == 0 {
				t.Fatalf("resolve accepted %s identity without a proving reread:\nstdout:\n%s\nstderr:\n%s",
					variant, stdout, diagnostics)
			}
			if stdout != "" {
				t.Errorf("failed resolve printed success output:\n%s", stdout)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
		})

		t.Run("unresolve "+variant+" identity does not prove reopening", func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: malformedMutationResponse(
					"unresolveReviewThread", variant, threadID, "", false,
				)},
				providerStep{stdout: threadResponse(threadID, true, original, answer)},
			)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"unresolve", threadID})
			})
			if code == 0 {
				t.Fatalf("unresolve accepted %s identity while target stayed resolved:\nstdout:\n%s\nstderr:\n%s",
					variant, stdout, diagnostics)
			}
			if stdout != "" {
				t.Errorf("failed unresolve printed success output:\n%s", stdout)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "unresolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
		})

		t.Run("automatic reopen "+variant+" identity is reconciled before guidance", func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, answer)},
				providerStep{stdout: resolveResponse(threadID, followup.id)},
				providerStep{stdout: malformedMutationResponse(
					"unresolveReviewThread", variant, threadID, "", false,
				)},
				providerStep{stdout: threadResponse(threadID, false, original, answer, followup)},
			)
			code, _, diagnostics := provider.capture(func() int {
				return run([]string{"resolve", threadID})
			})
			if code == 0 {
				t.Fatalf("stale resolution unexpectedly succeeded after %s reopen identity:\n%s",
					variant, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "unresolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, diagnostics, "reopened", "requires a new answer")
			assertContainsNone(t, diagnostics, "STILL RESOLVED")
		})
	}
}

func TestIncompleteCommentIdentityRequiresInspection(t *testing.T) {
	const (
		threadID   = "PRRT_missing_ids"
		originalID = "PRRC_original"
		body       = "A descriptor-bound read now closes the race."
		bodyFile   = "/tmp/resolve-review-thread.MISSINGIDS"
	)
	original := providerComment{id: originalID, login: "reviewer", body: "P2: close the race"}

	tests := []struct {
		name  string
		steps []providerStep
		want  []string
	}{
		{
			name: "history predecessor id missing",
			steps: []providerStep{
				{stdout: replyResponse("")},
				{stdout: historyResponse(providerComment{id: "", login: original.login, body: original.body})},
			},
			want: []string{"provider state is unverified", "Inspect the live thread before any retry"},
		},
		{
			name: "matching recovered reply id missing",
			steps: []providerStep{
				{stderr: "connection closed after write", exit: 1},
				{stdout: historyResponse(original, providerComment{id: "", login: "author", body: body})},
			},
			want: []string{"provider state is unverified", "Inspect the live thread before any retry"},
		},
		{
			name: "current tail id missing",
			steps: []providerStep{
				{stderr: "connection closed before acknowledgement", exit: 1},
				{stdout: historyResponse(original)},
				{stdout: threadResponse(threadID, false, providerComment{id: "", login: original.login, body: original.body})},
			},
			want: []string{"current thread state could not be matched", "Inspect the live thread before any retry"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t, tc.steps...)
			code, _, diagnostics := provider.capture(func() int {
				_, postCode := postReplyOrExit(
					context.Background(), threadID, body, originalID, bodyFile,
				)
				return postCode
			})
			if code == 0 {
				t.Fatalf("missing comment identity selected a recovery branch:\n%s", diagnostics)
			}
			provider.assertExhausted()
			wantKinds := []string{"reply", "history"}
			if len(tc.steps) == 3 {
				wantKinds = append(wantKinds, "thread")
			}
			assertQueryKinds(t, provider, wantKinds...)
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, diagnostics, tc.want...)
			assertContainsAll(t, diagnostics,
				"Retain the exact named reply-body source unchanged",
				"do not continue to another thread or safe-merge",
			)
			assertContainsNone(t, diagnostics,
				"an exact-body retry remains applicable",
				"use this revised-answer lifecycle",
				"rm -f",
			)
		})
	}
}

func TestResolveErrorRecoveryClassifiesFreshState(t *testing.T) {
	const (
		threadID = "PRRT_resolve_transport"
		anchorID = "PRRC_reply"
		body     = "The descriptor now supplies the verified bytes."
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P2: prove it"}
	reply := providerComment{id: anchorID, login: "author", body: body}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "The evidence changed."}

	tests := []struct {
		name      string
		after     string
		want      []string
		forbidden []string
		inspect   bool
	}{
		{
			name:  "unchanged nonempty tail permits exact-body retry",
			after: threadResponse(threadID, false, original, reply),
			want: []string{
				"Keep the exact same named reply-body source",
				"Retain that file unchanged",
			},
			forbidden: []string{"revise the same named body source in place"},
		},
		{
			name:  "changed nonempty tail requires revision",
			after: threadResponse(threadID, false, original, reply, followup),
			want: []string{
				"newer comment",
				"revise the same named body source in place",
			},
			forbidden: []string{"Keep the exact same named reply-body source"},
		},
		{
			name: "empty tail id requires inspection",
			after: threadResponse(threadID, false,
				original,
				providerComment{id: "", login: "author", body: body},
			),
			want: []string{
				"provider state could not be verified",
				"Inspect the live thread before any retry",
				"do not continue to another thread or safe-merge",
			},
			forbidden: []string{
				"Keep the exact same named reply-body source",
				"revise the same named body source in place",
				"rm -f",
			},
			inspect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := filepath.Join(t.TempDir(), "reply body")
			if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
				t.Fatalf("write reply body: %v", err)
			}
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, reply)},
				providerStep{stdout: historyResponse(original, reply)},
				providerStep{stdout: threadResponse(threadID, false, original, reply)},
				providerStep{stdout: threadResponse(threadID, false, original, reply)},
				providerStep{stdout: threadResponse(threadID, false, original, reply)},
				providerStep{stderr: "connection reset after resolve", exit: 1},
				providerStep{stdout: tc.after},
			)
			code, _, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatalf("ambiguous resolve with unresolved reread succeeded:\n%s", diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "history", "thread", "thread", "thread", "resolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, diagnostics, tc.want...)
			assertContainsNone(t, diagnostics, tc.forbidden...)
			if tc.inspect {
				assertContainsNone(t, diagnostics, "Only then continue")
			}
		})
	}
}

func TestUnresolveErrorRecoveryReconcilesFreshState(t *testing.T) {
	const threadID = "PRRT_unresolve_transport"
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reopen this"}

	tests := []struct {
		name       string
		reread     providerStep
		wantCode   int
		wantStdout []string
		wantStderr []string
		unverified bool
	}{
		{
			name:       "transport failed after reopen applied",
			reread:     providerStep{stdout: threadResponse(threadID, false, original)},
			wantCode:   0,
			wantStdout: []string{"unresolved", "isResolved=false", "recovered"},
		},
		{
			name:       "transport failed and reopen did not apply",
			reread:     providerStep{stdout: threadResponse(threadID, true, original)},
			wantCode:   1,
			wantStderr: []string{"still resolved"},
		},
		{
			name:       "transport failed and reread failed",
			reread:     providerStep{stderr: "thread state unavailable", exit: 1},
			wantCode:   1,
			wantStderr: []string{"could not be re-read"},
			unverified: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stderr: "connection reset after unresolve", exit: 1},
				tc.reread,
			)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"unresolve", threadID})
			})
			if tc.wantCode == 0 && code != 0 {
				t.Fatalf("applied unresolve was not recovered (code=%d):\n%s", code, diagnostics)
			}
			if tc.wantCode != 0 && code == 0 {
				t.Fatalf("unverified/unapplied unresolve succeeded:\n%s", stdout)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "unresolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, stdout, tc.wantStdout...)
			assertContainsAll(t, strings.ToLower(diagnostics), tc.wantStderr...)
			if tc.wantCode != 0 && stdout != "" {
				t.Errorf("failed unresolve printed success output:\n%s", stdout)
			}
			if tc.unverified {
				assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
			}
		})
	}
}

func TestMovedResolvedTailReopensBeforeGuidance(t *testing.T) {
	t.Run("buried reply with unavailable latest author uses neutral supersession wording", func(t *testing.T) {
		const (
			threadID = "PRRT_buried_unknown_author"
			body     = "The earlier answer addressed only the original state."
		)
		original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
		oldReply := providerComment{id: "PRRC_reply", login: "author", body: body}
		followup := providerComment{id: "PRRC_followup", login: "", body: "There is newer commentary."}
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original, oldReply, followup)},
			providerStep{stdout: historyResponse(original, oldReply, followup)},
			providerStep{stdout: threadResponse(threadID, false, original, oldReply, followup)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("buried reply with unknown latest author succeeded:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread")
		assertContainsAll(t, diagnostics,
			"newer commentary follows it",
			"read the comment(s) after your reply",
			"revise the same named body source in place",
		)
		assertContainsNone(t, diagnostics,
			"the reviewer has commented since",
			"reviewer follow-up",
		)
	})

	t.Run("buried prior reply is classified before resolved skip", func(t *testing.T) {
		const (
			threadID = "PRRT_buried_before_skip"
			body     = "The earlier answer addressed only the original review state."
		)
		original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
		oldReply := providerComment{id: "PRRC_reply", login: "author", body: body}
		followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "That answer is incomplete."}
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original, oldReply, followup)},
			providerStep{stdout: historyResponse(original, oldReply, followup)},
			providerStep{stdout: threadResponse(threadID, true, original, oldReply, followup)},
			providerStep{stdout: unresolveResponse(threadID)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("buried prior reply was skipped as terminal:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("buried-reply refusal printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"earlier matching reply is followed by newer commentary",
			"automatically reopened",
			"revise the same named body source in place",
		)
		assertContainsNone(t, diagnostics,
			"skipped "+threadID+" (already resolved)",
			"Keep the exact same named reply-body source",
		)
	})

	t.Run("history reread compares the moved tail before resolved skip", func(t *testing.T) {
		const (
			threadID = "PRRT_history_tail_race"
			body     = "The reply was prepared for the original review state."
		)
		original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
		followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "The state changed."}
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: historyResponse(original)},
			providerStep{stdout: threadResponse(threadID, true, original, followup)},
			providerStep{stdout: unresolveResponse(threadID)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("moved resolved history tail was skipped as terminal:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("history-tail refusal printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"history tail changed",
			"automatically reopened",
			"revised-answer lifecycle",
			"revise the same named body source in place",
		)
		assertContainsNone(t, diagnostics,
			"skipped "+threadID+" (already resolved)",
			"Keep the exact same named reply-body source",
		)
	})

	t.Run("ambiguous reply observes a moved resolved tail", testResolvedMovedAmbiguousReplyOutcome)
}

func testResolvedMovedAmbiguousReplyOutcome(t *testing.T) {
	const (
		threadID = "PRRT_full_ambiguous_reply"
		body     = "The reply addresses the state observed before posting."
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "Provider state moved."}
	bodyFile := filepath.Join(t.TempDir(), "reply source")
	if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
		t.Fatalf("write reply body: %v", err)
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, original)},
		providerStep{stdout: historyResponse(original)},
		providerStep{stdout: threadResponse(threadID, false, original)},
		providerStep{stderr: "connection closed after reply write", exit: 1},
		providerStep{stdout: historyResponse(original, followup)},
		providerStep{stdout: threadResponse(threadID, true, original, followup)},
		providerStep{stdout: unresolveResponse(threadID)},
	)
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatalf("resolved moved ambiguous reply outcome succeeded:\nstdout:\n%s\nstderr:\n%s",
			stdout, diagnostics)
	}
	if stdout != "" {
		t.Errorf("ambiguous reply path printed success output:\n%s", stdout)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider,
		"thread", "history", "thread", "reply", "history", "thread", "unresolve",
	)
	assertQueryTargets(t, provider, threadID)
	assertContainsAll(t, diagnostics,
		"thread tail moved",
		"revised-answer lifecycle",
		"revise the same named body source in place",
	)
	assertContainsNone(t, diagnostics,
		"Keep the exact same named reply-body source",
		"provider state is unverified",
	)
}

func TestResolveAllRefusalSummaryIsTruthful(t *testing.T) {
	const (
		unansweredID = "PRRT_unanswered"
		supersededID = "PRRT_superseded"
		answerID     = "PRRC_answer"
	)
	unansweredOpening := providerComment{
		id: "PRRC_unanswered_opening", login: "reviewer", body: "P1: answer this",
	}
	supersededOpening := providerComment{
		id: "PRRC_superseded_opening", login: "reviewer", body: "P2: prove this",
	}
	answer := providerComment{id: answerID, login: "author", body: "Initially proved."}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "The proof is stale."}

	provider := installSequencedProvider(t,
		providerStep{stdout: listResponse(
			threadNodeResponse(unansweredID, false, unansweredOpening),
			threadNodeResponse(supersededID, false, supersededOpening, answer),
		)},
		providerStep{stdout: threadResponse(unansweredID, false, unansweredOpening)},
		providerStep{stdout: threadResponse(supersededID, false, supersededOpening, answer)},
		providerStep{stdout: resolveResponse(supersededID, followup.id)},
		providerStep{stdout: unresolveResponse(supersededID)},
	)
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"resolve-all", "owner", "repo", "42"})
	})
	if code == 0 {
		t.Fatalf("resolve-all succeeded despite two evidence refusals:\nstdout:\n%s\nstderr:\n%s",
			stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "list", "thread", "thread", "resolve", "unresolve")
	assertContainsAll(t, stdout, "resolved 0 thread(s), refused 2, skipped 0")
	assertContainsAll(t, diagnostics,
		"REFUSED",
		unansweredID,
		supersededID,
		"may have no independent reply yet",
		"superseded by newer commentary",
	)
	assertContainsNone(t, diagnostics,
		"nobody replied",
		"all refused threads are unanswered",
	)
}
