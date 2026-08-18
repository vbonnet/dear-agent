# Working in `.github/workflows`

Scoped notes for CI definitions. Mined from the automated-review corpus: this
directory is where both of the repository's most-repeated review findings land.

## A skipped required check satisfies branch protection

Required contexts that are conditional on change detection have repeatedly been
made to *skip* rather than fail when something went wrong — and a skipped
required check satisfies protection just as well as a passing one. The gate
looks green while nothing ran.

Caught here by hand, repeatedly:

- A detector job failing meant dependent required jobs were skipped, because an
  `if:` without a status function inherits an implicit `success()`.
- A path selector missed `//go:embed` assets, so a PR changing compiled-in data
  skipped both `Build & Test` jobs *and* CodeQL.
- A linter's own implementation changed without its applicability condition
  including that path, so the check never ran against the code defining it.
- A rename to a non-Markdown name reported only the destination path, so a docs
  gate concluded nothing changed.

The rule: **when detection fails or is uncertain, run the check.** Put the
detector's result inside an `always()`-bearing condition, and when you add a
selector, include the implementation and embedded inputs of whatever it gates.

## Report failures; do not exit on a tally

Steps have reported success while the operation failed — a rejected
`git push --delete` ignored because the script had no `errexit` and exited on a
review count instead of the command status, labelling the branch "Auto-deleted"
while it still existed.

Record only outcomes you actually observed, and return a distinct non-zero
status for the failure. Bash here is capped at 20 non-comment lines; put real
logic in Go under `cmd/` or `tools/` and call it.
