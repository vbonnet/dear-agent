---
name: code-health-budget
description: Use BEFORE opening a pull request that adds or changes Go functions, and whenever you are about to ship a new package, a branchy function, or code you have not written a test for. Gives the repository's complexity-against-coverage budget and which gate already owns which check, so you do not duplicate one or skip one. Trigger on "open a PR", "ready to push this up", "add a new package", "this function is getting complicated", "do I need a test for this", or any diff that adds control flow.
---

# Complexity you do not test is the debt

Measure the branch before opening the PR:

```bash
go run ./tools/crap-lint -base origin/main -head HEAD
```

It scores every function your diff changed and prints nothing when the diff is
clean. Run it from a checkout that is at your head commit; it refuses to score
otherwise rather than report numbers from the wrong source.

## The budget

**CRAP = complexity^2 * (1 - coverage)^3 + complexity.**

At full coverage the score is just the complexity, so a score of 6 means six
fully-tested paths. Two numbers matter:

- **6 is the target** for agent-written code (Uncle Bob's figure: 4 for humans,
  raised to 6 for agents because they hold more branching context reliably).
- **30 is where CI starts naming your function individually.** That is Crap4j's
  own default and exists to keep the comment short enough to read. It is a
  ceiling, not a target. A function at 29 is five times over budget and one
  point under the alarm.

For intuition: a function with complexity 8 and no tests scores 72. The same
function at 70% coverage scores 14. At full coverage, 8.

## What each gate already owns

Do not hand-roll a check that already has an owner. One rule, one home.

| Concern | Owner | Posture |
|---|---|---|
| Discarded error returns | `errcheck` in `.golangci.yml`, `check-blank: true` | Hard fail on new occurrences |
| Raw cyclomatic complexity over 15 | `gocyclo` in `.golangci.yml` | Hard fail on new occurrences |
| Result of `append` never used | `staticcheck` SA4010 | Hard fail |
| Packages shipping no `_test.go` at all | `zero-test` scan in `cmd/structural-health` | Baselined ratchet |
| Complexity that no test exercises | `tools/crap-lint` | Advisory comment only |

Both golangci-lint checks run with `new-from-merge-base: origin/main`, so they
are already scoped to your diff.

## Why this is not cosmetic

Evidence this budget exists, from the CRAP audit in engram-research PR #354:

`agm/cmd/agm-bus` measured **0.0% coverage** with `cmdServe` at **CRAP 2162**,
more than seven times the next-worst function in the repository. The package
was not untested. It had four tests. They built the binary and exec'd it, so
every statement they exercised ran in a subprocess and counted for nothing, and
the zero-test scan saw test files and stayed quiet. Nothing in CI could see it.

The same audit found `staticcheck` SA4010 disabled in `.golangci.yml` behind the
comment "Disable unused append check", suppressed years earlier while trimming
the linter set "to a passing baseline" with a stated plan to re-enable
incrementally. The one violation it was hiding was a real bug: an adapter parsed
an imported conversation and dropped it.

Both are the same failure. A gate that cannot see a thing reports safety that
does not exist, and that reads exactly like safety that does.

## Reading the CI comment

The signal posts into the existing size-and-scope comment. It is **advisory**:
it never fails a check and never blocks a merge.

When it names a function, you have three honest answers:

1. **Add tests for the uncovered branches.** Usually right, usually cheap.
2. **Split the function** until each part is simple enough to be obviously
   right. Right when the complexity is the actual problem.
3. **Say the code is covered by a harness the signal cannot run** and move on.
   Legitimate here: packages needing a live tmux socket, a Dolt server, or a
   container runtime genuinely cannot be measured in that job.

Answer 3 is legitimate and also the easy way out. If you reach for it, say
which harness covers the code. If nothing does, it is answer 1.

## Making a subprocess-tested command countable

If your command's only tests exec the built binary, coverage will read 0% no
matter how thorough they are. Split the entry point so the work is reachable
in-process:

- Parse flags into an options struct.
- Build the object graph in one function that starts nothing.
- Take a `context.Context` for the run loop instead of installing signal
  handlers inside it.
- Keep the subprocess tests. They are the only cover for `main`'s dispatch and
  for real signal delivery.

`agm/cmd/agm-bus` went 0.0% to 75.3% this way, with the entry point's
complexity dropping from 46 to 4 and no behavior change.

## Verify before you open the PR

1. Run the lens from a checkout at your head commit:

   ```bash
   go run ./tools/crap-lint -base origin/main -head HEAD
   ```

2. If it prints `clean:`, you are done. Check the second number it reports:
   how many of your changed functions are at or under 6. Raising it is the
   goal even when nothing is over 30.

3. If it names a function, pick one of the three answers above and act on it.
   Re-run until it is clean, or until the remaining entries are ones you can
   name a covering harness for.

4. If it says coverage could not be collected for a package, that is a
   measurement gap and not a pass. Say in the PR description which harness
   covers that code.

5. For anything you newly covered, confirm the test actually bites: gut the
   function body, re-run the package tests, and check they fail. A test that
   passes against an empty body raised the coverage number and nothing else.
   Restore the body afterwards.

The signal is advisory and cannot block a merge, so nothing forces step 5.
That is exactly why it is on you.

## References

- `internal/craplens`: the analysis, and `internal/craplens/SPEC.md` for its contract.
- `tools/crap-lint`: the command wrapper, and `--help` for its flags.
- `.github/workflows/pr-size-scope.yml`: the job that runs it and posts the comment.
- `REVIEW.md` section 3: why this is advisory and not an escalation trigger.
- `.golangci.yml`: the `errcheck`, `gocyclo`, and `staticcheck` settings this
  skill defers to.
- Alberto Savoia, Crap4j (2007): the origin of the formula and the default of 30.
