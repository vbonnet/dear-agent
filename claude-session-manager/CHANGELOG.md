# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Removed

- **`--no-lock` flag**: Removed obsolete workaround flag from all CSM commands
  - **Reason**: Flag was never implemented (defined but unused in code)
  - **Background**: Deadlock between `csm new` and `csm associate` was fixed in commit 262c069 by releasing lock before waiting for ready-file
  - **Impact**: No functional change (flag had no effect)
  - **Migration**: Remove `--no-lock` from any scripts (flag will cause "unknown flag" error if used)
