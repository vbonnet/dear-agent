package main

import (
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
	id            string
	login         string
	body          string
	updatedAt     string
	omitUpdatedAt bool
	editCount     int
	omitEditCount bool
	lastEditID    string
	omitEditNodes bool
}

func (c providerComment) providerEditEvidence() string {
	if c.omitEditCount {
		return ""
	}
	nodes := ""
	if !c.omitEditNodes {
		nodes = `,"nodes":[]`
		if c.lastEditID != "" {
			nodes = fmt.Sprintf(`,"nodes":[{"id":%q}]`, c.lastEditID)
		}
	}
	return fmt.Sprintf(`,"userContentEdits":{"totalCount":%d%s}`, c.editCount, nodes)
}

const providerFixtureUpdatedAt = "2026-09-09T00:00:00Z"

func (c providerComment) providerUpdatedAt() string {
	if c.omitUpdatedAt {
		return ""
	}
	if c.updatedAt != "" {
		return c.updatedAt
	}
	return providerFixtureUpdatedAt
}

func testReplyIssuancePredecessor(comment providerComment, replyBody string) replyIssuancePredecessor {
	editRevisionPresent := !comment.omitEditCount && !comment.omitEditNodes &&
		validContinuationEditRevision(comment.editCount, comment.lastEditID)
	return replyIssuancePredecessor{
		ID:                comment.id,
		BodySHA256:        exactBodySHA256([]byte(comment.body)),
		UpdatedAt:         comment.providerUpdatedAt(),
		EditCount:         comment.editCount,
		EditCountPresent:  editRevisionPresent,
		LastEditID:        comment.lastEditID,
		LastEditIDPresent: editRevisionPresent,
		OpeningAuthor:     comment.login,
		Author:            comment.login,
		ReplyBodySHA256:   exactBodySHA256([]byte(replyBody)),
	}
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
		recent = append(recent, fmt.Sprintf(`{"id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}`,
			comment.id, comment.login, comment.body, comment.providerUpdatedAt(), comment.providerEditEvidence()))
	}
	return fmt.Sprintf(`{"id":%q,"isResolved":%t,"isOutdated":false,"path":"review.go","opening":{"totalCount":%d,"nodes":[%s]},"recent":{"nodes":[%s]}}`,
		threadID, resolved, len(comments), opening, strings.Join(recent, ","))
}

func listResponse(nodes ...string) string {
	return fmt.Sprintf(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[%s]}}}}}`, strings.Join(nodes, ","))
}

func historyResponse(threadID string, comments ...providerComment) string {
	nodes := make([]string, 0, len(comments))
	for _, comment := range comments {
		nodes = append(nodes, fmt.Sprintf(`{"id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}`,
			comment.id, comment.login, comment.body, comment.providerUpdatedAt(), comment.providerEditEvidence()))
	}
	return fmt.Sprintf(`{"data":{"node":{"id":%q,"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[%s]}}}}`, threadID, strings.Join(nodes, ","))
}

func replyResponse(comment providerComment) string {
	return fmt.Sprintf(`{"data":{"addPullRequestReviewThreadReply":{"comment":{"id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}}}}`,
		comment.id, comment.login, comment.body, comment.providerUpdatedAt(), comment.providerEditEvidence())
}

func resolveResponse(threadID, lastCommentID string) string {
	return resolveResponseWithState(threadID, lastCommentID, true)
}

func resolveResponseWithState(threadID, lastCommentID string, resolved bool) string {
	opening := ""
	recent := ""
	if lastCommentID != "" {
		opening = `{"author":{"login":"reviewer"}}`
		recent = fmt.Sprintf(`{"id":%q,"author":{"login":"author"}}`, lastCommentID)
	}
	return fmt.Sprintf(`{"data":{"resolveReviewThread":{"thread":{"id":%q,"isResolved":%t,"opening":{"nodes":[%s]},"recent":{"nodes":[%s]}}}}}`,
		threadID, resolved, opening, recent)
}

func resolveResponseWithExactComments(threadID string, resolved bool, comments ...providerComment) string {
	opening := ""
	if len(comments) > 0 {
		opening = fmt.Sprintf(`{"author":{"login":%q}}`, comments[0].login)
	}
	start := 0
	if len(comments) > 2 {
		start = len(comments) - 2
	}
	recent := make([]string, 0, len(comments)-start)
	for _, comment := range comments[start:] {
		recent = append(recent, fmt.Sprintf(`{"id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}`,
			comment.id, comment.login, comment.body, comment.providerUpdatedAt(), comment.providerEditEvidence()))
	}
	return fmt.Sprintf(`{"data":{"resolveReviewThread":{"thread":{"id":%q,"isResolved":%t,"opening":{"nodes":[%s]},"recent":{"nodes":[%s]}}}}}`,
		threadID, resolved, opening, strings.Join(recent, ","))
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
