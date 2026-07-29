# AGM Shell Quoting Specification

<!-- Last audited at: 2026-07-28 -->

## Overview

`agm/internal/shellquote` owns AGM's dependency-neutral POSIX shell-word
quoting primitive. Callers use it when a value must be embedded in a command
string that a POSIX shell will parse.

## Requirements

**AGM-SHELLQUOTE-01** When `Quote` receives a string containing no NUL byte, the system shall produce one single-quoted POSIX shell word whose parsed value is byte-for-byte identical to the input.

**AGM-SHELLQUOTE-02** When the input contains a single quote, the system shall encode that byte by ending the single-quoted segment, emitting a double-quoted single quote, and reopening the single-quoted segment.

**AGM-SHELLQUOTE-03** When the input is empty, the system shall produce `''` so the shell parses one empty argument rather than no argument.

**AGM-SHELLQUOTE-04** When the input contains whitespace, newlines, command substitutions, control operators, or other shell metacharacters, the system shall keep those bytes inert inside exactly one parsed shell word.

**AGM-SHELLQUOTE-05** When a caller-controlled value contains a NUL byte, the system shall reject it at the terminal-paste validation boundary before calling `Quote`, because neither a POSIX shell command nor an OS argument can represent NUL.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that
  this package keeps co-located SPEC coverage.
