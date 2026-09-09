# SPEC: cmd/resolve-review-threads/SPEC.md
Feature: Review thread reply safety
  Review reply content must remain exact data throughout thread resolution.

  Scenario: Review reply bodies and thread mutations remain safely verifiable
    When AGM runs the review reply data-only regressions
    Then reply-resolve should require a body-file source
    And invalid body sources should fail before GitHub mutation
    And oversized and endless body sources should be bounded before GitHub mutation
    And a replaced named source should fail without blocking
    And GitHub should receive the exact reply bytes without shell evaluation
    And failed provider diagnostics should not echo the reply body
    And body-bearing access denials should remain redacted and require credential repair
    And body-free provider diagnostics should remain available
    And retry guidance should reuse the body-file source without rendering its content
    And posted-reply continuation receipts should authenticate host-bound body, identity, time, and edit evidence without reposting
    And continuation issuance should accept a safe shared state namespace while keeping command-owned issuer state private and preflighted
    And unsupported continuation platforms should fail before reply mutation and before continuation body-source or provider access
    And continuation environment failures should retain the exact receipt and source for issuer-state restoration
    And continuation replay should require the unchanged current provider-visible extant boundary
    And unreceipted existing replies should not be rebound into fresh temporal evidence
    And changed retry bodies should not bypass provider-history deduplication while reviewer hand-back permits revision
    And resolved reviewer hand-backs should never be skipped as terminal
    And generated reply-file guidance should prescribe per-thread external creation, retry retention, and confirmed-terminal cleanup
    And superseded reply guidance should revise the same source without losing its cleanup identity
    And ambiguous reply outcomes should preserve the full predecessor identity, time, edit, and author boundary before choosing unchanged retry or same-source revision
    And stale resolved continuation mismatches should reopen before recovery guidance
    And resolution-anchor recovery should distinguish unverifiable absence from confirmed change
    And resolved buried or jumped replies should reopen before recovery guidance
    And uncertain thread evidence should retain the actual source pending inspection
    And unchanged and revised guidance should preserve the caller's named path or standard-input form
    And every mutation response should prove the requested thread identity, state, and answer-author boundary
    And moved resolved tails should reopen before already-resolved guidance
    And incomplete comment identities should require inspection
    And resolve transport errors should be classified from a fresh state read
    And unresolve transport errors should reconcile fresh state before claims
    And aggregate refusal summaries should report only confirmed outcomes
