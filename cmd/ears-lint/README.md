# ears-lint

`ears-lint` validates requirements written in [EARS](https://alistairmavin.com/ears/)
(Easy Approach to Requirements Syntax) inside `SPEC.md` files. It is a fast,
deterministic, regex-based check — the replacement for the previous LLM
"quality rubric" used by the Wayfinder `SPEC` phase gate.

## Usage

```sh
# Lint ./SPEC.md
ears-lint

# Lint specific files and/or directories (directories are searched
# recursively for SPEC.md)
ears-lint internal/sandbox/SPEC.md ./pkg

# Fail on any non-conforming requirement, not just zero-requirement files
ears-lint --strict ./...

# Machine-readable output
ears-lint --json SPEC.md

# Use custom patterns
ears-lint --config .earslint.yml SPEC.md
```

Or via make:

```sh
make lint-specs                                   # whole repo, non-strict
make lint-specs PATHS=internal/sandbox/SPEC.md STRICT=1
```

### Exit codes

| code | meaning |
|------|---------|
| 0    | all linted files passed |
| 1    | a file had zero valid requirements, or (with `--strict`) a non-conforming requirement |
| 2    | usage or I/O error |

## EARS templates

A line is treated as a *candidate requirement* when it contains the requirement
keyword (`shall` by default). Each candidate must match one of these templates:

| name           | template |
|----------------|----------|
| event-driven   | `When <trigger>, the <system> shall <response>` |
| state-driven   | `While <state>, the <system> shall <behavior>` |
| feature-driven | `Where <feature>, the <system> shall <behavior>` |
| option         | `If <condition>, then the <system> shall <behavior>` |
| unwanted       | `The <system> shall not <behavior>` |
| ubiquitous     | `The <system> shall <behavior>` |

Lines inside fenced code blocks (```` ``` ```` / `~~~`) are ignored.

## Configuration

Patterns are configurable. Pass `--config <file>` (the wayfinder gate also picks
up a project-local `.earslint.yml` automatically). Omitted fields fall back to
the built-in defaults.

```yaml
# .earslint.yml
requirement_keyword: shall
patterns:
  - name: ubiquitous
    regex: '(?i)^the\s+.+\s+shall\s+.+'
    description: The <system> shall <behavior>
```

## Library

```go
import "github.com/vbonnet/dear-agent/spec-governance/earslint"

linter, _ := earslint.New(earslint.Config{}) // empty == defaults
res, _ := linter.LintFile("SPEC.md")
if res.Failed(true /* strict */) {
    for _, f := range res.Findings {
        fmt.Println(f)
    }
}
```
