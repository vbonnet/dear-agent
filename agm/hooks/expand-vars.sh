#!/bin/bash
# PreToolUse:Bash hook — expand safe shell variables to literal values
#
# Problem: Claude Code rejects Bash commands with shell variable expansions
# like $HOME, $USER with "Contains simple_expansion" errors.
#
# Solution: This hook reads the tool input JSON from stdin, expands safe
# variables to their literal values, and outputs the modified JSON so Claude
# Code never sees the variable syntax.
#
# Safe variables: $HOME, ${HOME}, $USER, ${USER}, $TMPDIR, ${TMPDIR}
#
# Usage: Configure in ~/.claude/settings.json as a PreToolUse hook for Bash

# Capture current values
HOME_VAL="${HOME}"
USER_VAL="${USER}"
TMPDIR_VAL="${TMPDIR:-/tmp}"

# Read JSON from stdin, replace safe variable patterns, output result
sed \
  "s|\\\$HOME|$HOME_VAL|g; \
   s|\\\${HOME}|$HOME_VAL|g; \
   s|\\\$USER|$USER_VAL|g; \
   s|\\\${USER}|$USER_VAL|g; \
   s|\\\$TMPDIR|$TMPDIR_VAL|g; \
   s|\\\${TMPDIR}|$TMPDIR_VAL|g"

exit 0
