# Retrospective: impl-send-commands

**Date:** 2026-04-14
**Session:** impl-send-commands
**Branch:** impl-trust-scheduling
**Duration:** ~30 minutes

## Objective

Implement three new `agm send` subcommands (`enter`, `clear`, `stash`) to address the #1 time-waster: the ENTER bug and message delivery failures.

## What Was Built

### agm send enter <session>
- Sends C-m (Enter) to submit content in input line
- Pre-flight: captures pane to verify input line has content
- Refuses on empty input to prevent accidental blank submissions
- `--force` flag skips the content check

### agm send clear <session>
- Clears input prompt without submitting (C-c + C-u)
- Captures pane before and after to verify clearing worked
- `--force` sends additional C-a C-k if first attempt fails
- Does NOT trigger "human input detected" refusal

### agm send stash <session>
- Sends Ctrl+S to stash current message in Claude Code
- Preserves human text while clearing input for AGM delivery
- Verifies stash via capture-pane after sending
- Notes that unstash happens automatically on next send

### send_group.go
- Updated help text to list all three new commands with examples

## Technical Approach

- Followed existing patterns from `send_clear_input.go` and `send_approve.go`
- Used `tmux.CapturePaneOutput()` for all pre-flight verification
- Used `tmux.SendKeys()` for key delivery (inherits tmux lock + normalization)
- Used `tmux.ClassifyQueuedInput()` and `tmux.InputLineHasContent()` for input detection
- All commands use cobra `ExactArgs(1)` with session name as positional arg

## What Went Well

- Clean implementation — each command is self-contained (~70-100 lines)
- Reused existing tmux package functions, no new internal APIs needed
- Build verified clean on first attempt
- All 4 commits pushed to remote

## What Could Be Improved

- No tests written (unit tests for the command logic would be valuable)
- The `clear` command's fallback (C-a C-k) is untested in practice
- No integration test to verify actual tmux behavior
- Branch not merged to main — left for orchestrator

## Commits

1. `7f335d75` — agm send enter: submit input line content with safety check
2. `ddf0d214` — agm send clear: clear input prompt without submitting
3. `8e85bd01` — agm send stash: preserve input text via Ctrl+S before delivery
4. `57bca5e5` — agm send: add enter/clear/stash to group help text
