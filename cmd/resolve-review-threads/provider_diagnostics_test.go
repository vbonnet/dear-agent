package main

import (
	"context"
	"fmt"
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
			providerStep{stdout: historyResponse(opening)},
			providerStep{stdout: threadResponse(threadID, false, opening)},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body, original, bodyFile,
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
		providerStep{stdout: historyResponse(opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
	)
	code, _, diagnostics := provider.capture(func() int {
		_, postCode := postReplyOrExit(
			context.Background(), threadID, body, original, bodyFile,
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
			_, err := ghGraphQL(context.Background(), listQuery, map[string]any{
				"owner": "owner",
				"repo":  "repo",
				"pr":    37,
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
