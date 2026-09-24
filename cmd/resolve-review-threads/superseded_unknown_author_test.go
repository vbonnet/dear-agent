package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvedSupersededReplyWithUnknownAuthorIsReopened(t *testing.T) {
	const (
		threadID = "PRRT_resolved_buried_unknown_author"
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
		providerStep{stdout: historyResponse(threadID, original, oldReply, followup)},
		providerStep{stdout: threadResponse(threadID, true, original, oldReply, followup)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatalf("resolved buried reply with unknown latest author succeeded:\nstdout:\n%s\nstderr:\n%s",
			stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
	assertNoReplyOrResolveMutation(t, provider)
	assertContainsAll(t, diagnostics,
		"earlier matching reply is followed by newer commentary",
		"automatically reopened",
		"revise the same named body source in place",
	)
	assertContainsNone(t, stdout+diagnostics,
		"skipped "+threadID,
		original.body,
		oldReply.body,
		followup.body,
	)
}
