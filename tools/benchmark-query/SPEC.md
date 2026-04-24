# benchmark-query — Specification

## Overview

benchmark-query provides a CLI for querying benchmark metrics collected from
ai-tools test runs and session infrastructure. It reads metric files from a
local directory and supports filtering by time window.

## Functional Requirements

- Query individual metrics by name
- List all available metrics
- Filter results by time window (`-since`)
- Output in human-readable or JSON format
- Read metrics from configurable directory

## Non-Functional Requirements

- Query latency: < 100ms for local metric files
- No external dependencies (reads local files only)
- Exit code 0 on success, non-zero on error
