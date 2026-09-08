---
name: stacked-prs
description: Use whenever you create, update, rebase, or repair a chain of dependent pull requests, and before you write any "Stack N/M" line into a PR description. Gives the real base-ref wiring, the tip-merge cascade, the keep-the-original-as-tip rule, and how to restack after a rebase. Trigger on "stack these PRs", "stacked PR", "this PR depends on", "rebase the stack", "restack", "the stack is broken", or any PR whose base is another PR's branch.
---

# Stacked pull requests

## Why the wiring matters

A **real** stack is one where each pull request's base is the previous one's
head. Merging the tip then **cascades**: GitHub merges the whole chain in order,
in one all-or-nothing operation, and every pull request keeps its own commits,
description and review history.

A **hand-labelled** stack is one where the descriptions say "stack" but every
base targets `main`. Merging the tip lands it alone against `main`. The siblings
are left orphaned and have to be closed, and closing them **destroys their
descriptions and review threads**. That is the loss this skill exists to
prevent, and it is why `stack-lint` fails such a pull request rather than
warning about it.

Real stacks also cost less CI: the tip is tested as the batch, instead of every
branch re-running the suite against `main`.

A stack is three things at once, and all three must be true:

1. **Base refs chain.** Each base is the previous head. Only the bottom targets
   the trunk.
2. **Branches actually descend.** Each head must be a descendant of its base.
   A correct base ref over a branch that no longer contains its parent looks
   fine in the API and is wrong in the UI.
3. **The stack is registered.** GitHub renders the stack, and `gh stack merge`
   and `gh stack sync` work, only once the chain is registered.

Getting 1 without 2 and 3 is what happened to #1379 through #1383: every base
ref was correct, every description said "Stack N/5", and two links were stale
because a parent was rebased and its children were not.

## Workflow

1. **Wire the base refs bottom to top.** Branch each slice off the one below,
   never off `main`:

   ```sh
   git checkout -b slice-1 main
   git checkout -b slice-2 slice-1
   git checkout -b slice-3 slice-2
   ```

2. **Register the stack** in one call. Existing pull requests are reused;
   branches without one get a pull request created with the correct base.

   ```sh
   gh stack link <bottom-pr> <next-pr> ... <top-pr>
   ```

   Starting from scratch instead: `gh stack init`, `gh stack add <branch>`,
   `gh stack submit`.

3. **Mark each description** with the marker `stack-lint` parses. The declared
   base must equal the actual base ref:

   ```
   Stack 2/5. Base: refactor/ce-1hu9-84-durable-guards.
   ```

4. **Keep the original pull request as the stack tip.** When you split work out
   of an existing pull request, the original keeps its number, review history
   and threads, and becomes the *top*. Extract the lower slices into new pull
   requests beneath it. Never close the original and reopen it lower down: a
   closed pull request loses its threads, and a base branch deleted during a
   squash-merge auto-closes anything still pointing at it. That is how #1304
   was lost and had to be reopened as #1345.

5. **Merge from the tip, not one at a time.** This is the payoff for the
   wiring:

   ```sh
   gh stack merge <pr-number>     # merges everything up to and including it
   ```

   It is atomic: if any member cannot merge, none do. Merging the members
   individually against `main` is what re-creates the orphan problem.

6. **Restack after any parent moves.** Rebasing, amending or squash-merging a
   lower pull request orphans every branch above it. Repair the whole chain
   bottom to top:

   ```sh
   git rebase --onto <new-parent-head> <old-parent-head> <child-branch>
   ```

   With local tracking, `gh stack rebase` and `gh stack sync` do this for you.

7. **Verify the restack moved topology only**, never content. Every line must
   show `=`:

   ```sh
   git range-diff <old-parent>..<old-child> <new-parent>..<new-child>
   ```

8. **Push each branch with a lease**, so a concurrent agent moving the branch
   aborts the push instead of losing work:

   ```sh
   safe-push --force-with-lease=<branch>:<recorded-sha> origin <local>:refs/heads/<branch>
   ```

## Verification

Run the checker on every pull request in the stack. Exit 0 with no findings is
the only passing state:

```sh
go run ./tools/stack-lint -pr <number>          # -strict promotes hygiene findings
```

Then confirm the shape independently:

- Each base ref equals the branch below:
  `gh pr view <n> --json baseRefName,headRefName`.
- Every link descends:
  `git merge-base --is-ancestor origin/<base> origin/<head>` exits 0.
- The stack is registered and ordered: the GraphQL
  `pullRequest{ stack{number size} stackEntry{position} }` fields are non-null
  and the positions run bottom to top.
- Each pull request's diff contains only its own commits.

If `stack-lint` reports STACK-01, the description claims a stack the base ref
cannot support, and a tip merge would orphan the siblings: fix the base ref
before merging anything. If it reports STACK-03 the stack is stale, so return to
step 6. If it reports STACK-04 the description and the base ref disagree, and
you must decide which is telling the truth before changing either.

## References

- `tools/stack-lint/SPEC.md` for the rule table and exit codes.
- `.github/workflows/stack-integrity.yml` for the CI gate.
- `.claude/skills/split-prs/SKILL.md` for deciding *whether* to split at all.
- `.claude/skills/pr-merge-blockers/SKILL.md` when a stacked pull request will
  not merge.
- GitHub's stacked pull requests CLI reference: https://gh.io/stacks
