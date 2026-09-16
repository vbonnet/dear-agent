package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBodyBearingAccessDenialIsRedactedAndRequiresCredentialRepair(t *testing.T) {
	const (
		threadID = "PRRT_access_denied"
		original = "PRRC_original"
		bodyFile = "/tmp/resolve-review-thread.ACCESS01"
		body     = "SECRET-REPLY-BODY must never appear in diagnostics"
		denial   = "gh: Resource not accessible by integration (HTTP 403)"
	)
	bodyEcho := fmt.Sprintf(`{"query":"mutation","variables":{"body":%q}}`, body)

	t.Run("typed provider boundary", func(t *testing.T) {
		provider := installSequencedProvider(t, providerStep{
			stderr: bodyEcho + "\n" + denial + "\n",
			exit:   1,
		})
		_, err := postReply(context.Background(), threadID, body)
		if err == nil {
			t.Fatal("access-denied reply succeeded")
		}
		if !isAccessDenied(err) {
			t.Fatalf("body-bearing denial lost its typed category: %v", err)
		}
		assertContainsNone(t, err.Error(), body, "SECRET-REPLY-BODY", denial)
		assertContainsAll(t, err.Error(), "provider access denied", "provider diagnostics suppressed")
		provider.assertExhausted()
	})

	t.Run("reconciled unchanged tail", func(t *testing.T) {
		opening := providerComment{id: original, login: "reviewer", body: "P2: permission handling"}
		provider := installSequencedProvider(t,
			providerStep{stderr: bodyEcho + "\ngh: Must have push permission to resolve\n", exit: 1},
			providerStep{stdout: historyResponse(threadID, opening)},
			providerStep{stdout: threadResponse(threadID, false, opening)},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body,
				testReplyIssuancePredecessor(opening, body), bodyFile,
			)
			return postCode
		})
		if code == 0 {
			t.Fatalf("access-denied reply recovery succeeded:\n%s", diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history", "thread")
		assertContainsAll(t, diagnostics,
			"This is an access problem",
			"gh auth status",
			"fix credentials before retrying",
			"re-running with the same credentials will be denied again",
			"Retain the exact reply-body source unchanged",
			"reply_file='/tmp/resolve-review-thread.ACCESS01'",
			"After credential repair",
		)
		assertContainsNone(t, diagnostics,
			body,
			"SECRET-REPLY-BODY",
			denial,
			"an exact-body retry remains applicable",
		)
	})
}

func TestReplyBodyAccessMarkersDoNotForgeAccessDenial(t *testing.T) {
	const (
		threadID = "PRRT_marker_forgery"
		original = "PRRC_original"
		bodyFile = "/tmp/resolve-review-thread.MARKERS"
	)
	body := strings.Join(providerAccessDeniedMarkers, " | ") + " | SECRET-MARKER"
	bodyEcho := fmt.Sprintf(`{"query":"mutation","variables":{"body":%q}}`, body)
	opening := providerComment{id: original, login: "reviewer", body: "P2: classify safely"}
	provider := installSequencedProvider(t,
		providerStep{stderr: bodyEcho + "\ngh: connection reset by peer (HTTP 403 rate limit)\n", exit: 1},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
	)
	code, _, diagnostics := provider.capture(func() int {
		_, postCode := postReplyOrExit(
			context.Background(), threadID, body,
			testReplyIssuancePredecessor(opening, body), bodyFile,
		)
		return postCode
	})
	if code == 0 {
		t.Fatalf("ambiguous reply recovery succeeded:\n%s", diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "reply", "history", "thread")
	assertContainsAll(t, diagnostics,
		"an exact-body retry remains applicable",
		"reply_file='/tmp/resolve-review-thread.MARKERS'",
	)
	assertContainsNone(t, diagnostics,
		"This is an access problem",
		"fix credentials before retrying",
		"SECRET-MARKER",
	)
}

func TestRedactedProviderDiagnosticClassifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name: "trusted denial split across bytes and CRLF",
			input: "{\"variables\":{\"body\":\"permission denied\"}}\r\n" +
				"gh: Resource not accessible by integration (HTTP 403)\r\n",
			want: true,
		},
		{
			name:  "exact GraphQL provider prefix is accepted",
			input: "gh: GraphQL:\tBad credentials\n",
			want:  true,
		},
		{
			name:  "fine-grained token denial is accepted",
			input: "gh: GraphQL: Resource not accessible by personal access token\n",
			want:  true,
		},
		{
			name:  "debug body markers are untrusted",
			input: "{\"body\":\"access denied bad credentials unauthorized\"}\n",
		},
		{
			name:  "bare permission and rate-limit 403 are not authorization proof",
			input: "gh: permission metadata unavailable\ngh: HTTP 403 rate limit exceeded\nGH: access denied\n",
		},
		{
			name:  "quoted denial after validation preamble is not authorization proof",
			input: "gh: validation failed: reply body \"access denied\" is not acceptable\n",
		},
		{
			name: "candidate bytes beyond cap are ignored",
			input: "gh:" + strings.Repeat(" ", maxClassifiedProviderDiagnosticBytes) +
				"access denied\n" + strings.Repeat("gh:\n", 100),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			classifier := newRedactedProviderDiagnosticClassifier()
			for i := range len(tc.input) {
				if _, err := classifier.Write([]byte{tc.input[i]}); err != nil {
					t.Fatalf("classify diagnostic byte %d: %v", i, err)
				}
			}
			if got := classifier.AccessDenied(); got != tc.want {
				t.Fatalf("access-denied category = %t, want %t", got, tc.want)
			}
			if classifier.inspected > maxClassifiedProviderDiagnosticBytes {
				t.Fatalf("inspected %d diagnostic bytes, cap is %d",
					classifier.inspected, maxClassifiedProviderDiagnosticBytes)
			}
		})
	}
}

func TestBodyFreeAmbiguousDiagnosticsStayTransportErrors(t *testing.T) {
	for _, diagnostic := range []string{
		"gh: HTTP 403 rate limit exceeded",
		"gh: permission metadata unavailable",
		"gh: validation failed: reply body \"access denied\" is not acceptable",
	} {
		t.Run(diagnostic, func(t *testing.T) {
			provider := installSequencedProvider(t, providerStep{stderr: diagnostic + "\n", exit: 1})
			_, err := ghGraphQL(context.Background(), unresolveMutation, map[string]any{
				"threadId": "PRRT_body_free",
			})
			if err == nil {
				t.Fatal("failing body-free provider call succeeded")
			}
			provider.assertExhausted()
			if isAccessDenied(err) {
				t.Fatalf("ambiguous diagnostic became an access denial: %v", err)
			}
			if !strings.Contains(err.Error(), diagnostic) {
				t.Fatalf("body-free diagnostic was not retained: %v", err)
			}
		})
	}
}

func TestBodySelectingProviderFailureSuppressesPayloadAndKeepsAccessCategory(t *testing.T) {
	const (
		secret = "SECRET-PROVIDER-COMMENT-BODY"
		denial = "gh: GraphQL: Resource not accessible by personal access token"
	)
	dir := t.TempDir()
	script := `#!/bin/sh
set -eu
cat >/dev/null
if [ "${GH_DEBUG+x}" = x ] || [ "${DEBUG+x}" = x ]; then
  printf '%s\n' 'gh: debug environment reached body-selecting operation' >&2
  exit 2
fi
printf '%s' '{"data":{"resolveReviewThread":{"thread":{"comments":{"nodes":[{"body":"SECRET-PROVIDER-COMMENT-BODY"}]}}}}}'
printf '%s\n' 'provider response contained SECRET-PROVIDER-COMMENT-BODY' >&2
printf '%s\n' 'gh: GraphQL: Resource not accessible by personal access token' >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write failing fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GH_DEBUG", "api")
	t.Setenv("DEBUG", "true")

	raw, err := ghGraphQL(context.Background(), resolveMutation, map[string]any{
		"threadId": "PRRT_sensitive_response",
	})
	if err == nil {
		t.Fatal("failing body-selecting provider call succeeded")
	}
	if len(raw) != 0 {
		t.Fatalf("failing body-selecting provider call returned raw payload: %q", raw)
	}
	if !isAccessDenied(err) {
		t.Fatalf("body-selecting denial lost its typed category: %v", err)
	}
	assertContainsAll(t, err.Error(), "provider access denied", "provider diagnostics suppressed")
	assertContainsNone(t, err.Error(), secret, denial, "debug environment reached")
}

func TestGraphQLOperationSensitiveBodyDetection(t *testing.T) {
	pagedSnapshotQuery := buildPagedHistorySnapshotOperation(
		"PRRT_sensitive_query",
		[]string{"PRRC_sensitive_query"},
	).Query
	tests := []struct {
		name      string
		query     string
		variables map[string]any
		want      bool
	}{
		{name: "reply request body", query: replyMutation, variables: map[string]any{"body": "secret"}, want: true},
		{name: "list response bodies", query: listQuery, want: true},
		{name: "single thread response bodies", query: threadByIDQuery, want: true},
		{name: "history response bodies", query: threadCommentsQuery, want: true},
		{name: "paged history snapshot bodies", query: pagedSnapshotQuery, want: true},
		{name: "resolve response bodies without body variable", query: resolveMutation, variables: map[string]any{"threadId": "PRRT_exact"}, want: true},
		{name: "body alias", query: `query { node { comments { nodes { answer: body } } } }`, want: true},
		{name: "body-free unresolve", query: unresolveMutation},
		{name: "larger name is not body", query: `query { node { somebody bodyText } }`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := graphQLOperationCarriesSensitiveBody(tc.query, tc.variables); got != tc.want {
				t.Fatalf("sensitive operation = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestBodyBearingProviderDropsDebugEnvironment(t *testing.T) {
	input := []string{
		"PATH=/bin",
		"GH_DEBUG=api",
		"debug=true",
		"Gh_DeBuG=verbose",
		"SAFE=value",
	}
	want := []string{"PATH=/bin", "SAFE=value"}
	if got := withoutProviderDebugEnvironment(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered provider environment = %q, want %q", got, want)
	}
}
