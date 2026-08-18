# Working in `agm/internal/ops`

Scoped notes for this package. Mined from the automated-review corpus: these
two mistakes were caught here by hand, repeatedly, across many PRs.

## Never classify a failure as a domain state

The most-repeated defect in this package is not an unchecked error — `errcheck`
and `nilerr` already catch those. It is an error that *is* checked and then
mapped onto a legitimate business outcome, so the caller receives a confident
wrong answer instead of a failure.

Real examples the reviewer had to catch:

- A `gh pr list` failure (auth, rate limit, transient) became `nil`, which the
  caller read as **"this branch never had a PR"** and reported as a finding.
- An unreadable relay-target file became **"no override configured"**, silently
  routing to the stale fallback while reporting `source: fallback`.
- A failed heartbeat write became **"reaped successfully"**, so the watchdog
  raised a false dead-reaper alarm six hours later with no trace of the cause.

The rule: **"absent" and "could not determine" are different answers.** When a
lookup, read, or subprocess fails, either propagate the error or return an
explicit third state. Never collapse it into the empty/zero/negative case.

Concretely: distinguish `os.ErrNotExist` from every other read error; give
classifiers a `lookup-failed` bucket rather than reusing `none`; and if the
function's signature has nowhere to report the failure, change the signature.

## One owner per behaviour

The second-most-repeated defect here: adding a second path beside the canonical
one instead of using it — a parallel status computation, a second tmux-name
derivation, a private copy of a shared parser.

Before adding a helper, search for the canonical one and call it. When you
replace a path, delete the retired one — along with its flags, tests, and
documentation — in the same change. A replacement that leaves the old path
alive is the defect, not a follow-up.
