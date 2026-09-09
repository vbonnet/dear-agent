// Human and machine output helpers for the review-thread CLI.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// printThreads emits one compact JSON object per line, matching the original
// shell wrapper's output contract.
func printThreads(threads []thread) error {
	encoder := json.NewEncoder(os.Stdout)
	for _, current := range threads {
		if err := encoder.Encode(current); err != nil {
			return err
		}
	}
	return nil
}

// cleanBody collapses runs of whitespace to single spaces and truncates to
// bodyPreviewLen runes (rune-safe so multibyte bodies aren't split mid-char).
func cleanBody(body string) string {
	fields := bytes.Fields([]byte(body))
	collapsed := string(bytes.Join(fields, []byte(" ")))
	runes := []rune(collapsed)
	if len(runes) > bodyPreviewLen {
		return string(runes[:bodyPreviewLen])
	}
	return collapsed
}

// sameReplyBody compares reply bodies losslessly apart from surrounding
// whitespace. Preview normalization must never participate in this decision.
func sameReplyBody(lastBody, want string) bool {
	return strings.TrimSpace(lastBody) == strings.TrimSpace(want)
}

func fail(format string, arguments ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", arguments...)
	return 1
}

func usage() {
	fmt.Fprint(os.Stderr, `resolve-review-threads — list and resolve GitHub PR review threads

usage:
  resolve-review-threads list          <owner> <repo> <pr>          unresolved threads (JSON lines)
  resolve-review-threads list-all      <owner> <repo> <pr>          every thread
  resolve-review-threads resolve       <threadId> [--force]          resolve one thread by ID
                                                                    (same evidence rule as resolve-all)
  resolve-review-threads unresolve     <threadId>                   re-open one thread by ID
  resolve-review-threads reply-resolve <threadId> --body-file <path|->
                                                                    reply with exact file/stdin data,
                                                                    then resolve (the normal path)
  resolve-review-threads continue-resolve <receipt> --body-file <path|->
                                                                    validate an already-posted reply
                                                                    and resolve without reposting
  resolve-review-threads resolve-all   <owner> <repo> <pr> [author] [--force]
                                                                    resolve ANSWERED threads only;
                                                                    refuses unanswered ones by name
                                                                    and exits non-zero

A thread counts as answered when someone other than its opening author had the
last word. Resolving asserts the point was handled, so it needs that evidence;
--force overrides it for threads you are deliberately dismissing. Outdated is
reported but never sufficient: the hunk moving is not the point being fixed.

Resolution is GraphQL-only; all calls go through an authenticated gh CLI.
Use a named body file when durable retry may matter; standard input cannot be
replayed unless the caller retains the same bytes. Reply bodies are limited to
65,536 Unicode characters and 262,144 UTF-8 bytes.

Continuation receipts are authenticated by a long-lived local key under the
selected XDG state home and are bound to the effective GH_HOST. Preserve the
exact receipt, body source, host, state home, and key until the thread reaches a
confirmed terminal state; regenerating the key invalidates outstanding receipts.
Continuation issuance and replay currently require Unix. Non-Unix platforms fail
before reply mutation. For continue-resolve, failure precedes both
body-source and provider access because private durable key state is not yet proven
there. A receipt applies only while the currently provider-visible extant comment
boundary is unchanged. It is not a durable record of later comments that GitHub
no longer returns after deletion and does not prove reviewer acceptance.

If the process stops after GitHub accepts a reply but before a receipt is
printed, the reply remains unresolved and is not rebound automatically. Inspect
the exact live thread and use the bare resolve operation only if that current
answer is accepted.
`)
}
