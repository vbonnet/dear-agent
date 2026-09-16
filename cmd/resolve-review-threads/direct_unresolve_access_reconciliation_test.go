package main

import (
	"context"
	"testing"
)

func TestDirectUnresolveAccessDenialUsesNeutralConfirmedStateGuidance(t *testing.T) {
	const (
		threadID = "PRRT_direct_unresolve_denied"
		denial   = "gh: GraphQL: Resource not accessible by personal access token"
	)
	comment := providerComment{
		id:    "PRRC_direct_unresolve_private",
		login: "reviewer",
		body:  "private reviewer body that must not enter unresolve diagnostics",
	}

	t.Run("typed cause survives fresh unresolved state", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: denial + "\n", exit: 1},
			providerStep{stdout: threadResponse(threadID, false, comment)},
		)

		msg, err := unresolveWithEvidence(context.Background(), threadID)
		provider.assertExhausted()
		assertQueryKinds(t, provider, "unresolve", "thread")
		assertQueryTargets(t, provider, threadID)
		if err == nil {
			t.Fatal("access-denied unresolve was recovered as caller-applied success")
		}
		if !matchesErrorType[*accessDeniedMutationWithConfirmedStateError](err) {
			t.Fatalf("reconciled error type = %T, want *accessDeniedMutationWithConfirmedStateError: %v", err, err)
		}
		if !isAccessDenied(err) {
			t.Fatalf("fresh unresolved reconciliation lost its typed access-denial cause: %v", err)
		}
		assertContainsAll(t, msg,
			"unresolved "+threadID,
			"isResolved=false",
			"fresh provider read confirmed the requested state",
		)
		assertContainsAll(t, err.Error(),
			"denied this caller's unresolve mutation",
			"independently confirmed the requested state",
		)
		assertContainsNone(t, msg+"\n"+err.Error(),
			"stale resolution",
			"corrective mutation",
			"corrective reopen",
			"confirmed reopened",
			"had already applied",
			"this caller applied",
			comment.body,
			denial,
		)
	})

	t.Run("command emits neutral credential repair guidance", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: denial + "\n", exit: 1},
			providerStep{stdout: threadResponse(threadID, false, comment)},
		)

		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"unresolve", threadID})
		})
		provider.assertExhausted()
		assertQueryKinds(t, provider, "unresolve", "thread")
		assertQueryTargets(t, provider, threadID)
		if code == 0 {
			t.Fatalf("access-denied direct unresolve claimed success:\nstdout:\n%s\nstderr:\n%s", stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("access-denied direct unresolve printed success output: %q", stdout)
		}
		assertContainsAll(t, diagnostics,
			"requested unresolved state is independently confirmed",
			"cannot claim the mutation",
			"denied this caller's unresolve mutation",
			"repair `gh` credentials",
			"unchanged credentials will be denied again",
			"Inspect the fresh thread",
		)
		assertContainsNone(t, diagnostics,
			"stale resolution",
			"corrective mutation",
			"corrective reopen",
			"confirmed reopened",
			"had already applied",
			"this caller applied",
			comment.body,
			denial,
		)
	})
}
