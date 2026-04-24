# CLI Documentation Verification Script

**Script**: `verify-cli-docs.sh`
**Purpose**: Automated verification that CLI tool documentation matches actual implementation
**Author**: cli-docs-sync-audit swarm
**Created**: 2026-02-04

---

## Overview

This script prevents documentation drift by:
1. Extracting documented flags from markdown files
2. Running CLI tools with `--help` to get actual flags
3. Comparing documented vs actual flags
4. Reporting discrepancies

**Exit Codes**:
- `0` - All documentation synced with implementation ✅
- `1` - Drift detected (documented flags don't match actual) ❌
- `2` - Error (missing tool, invalid path, etc.) ⚠️

---

## Installation

```bash
# Make script executable
chmod +x main/scripts/verify-cli-docs.sh

# Add to PATH (optional)
export PATH="$PATH:main/scripts"
```

---

## Usage

### Basic Usage

```bash
# Check all tools
./verify-cli-docs.sh

# Check specific tool
./verify-cli-docs.sh --tool csm

# Verbose mode
./verify-cli-docs.sh --verbose

# Custom docs location
./verify-cli-docs.sh --docs-root ~/my-docs
```

### Options

| Option | Description |
|--------|-------------|
| `-h, --help` | Show help message |
| `-v, --verbose` | Verbose output (shows extraction details) |
| `-d, --docs-root DIR` | Documentation root directory (default: `~/.claude/plugins/cache`) |
| `-t, --tool TOOL` | Check specific tool only (can be repeated) |

---

## Examples

### Example 1: Verify All Tools

```bash
$ ./verify-cli-docs.sh
[INFO] CLI Documentation Verification
[INFO] Docs root: ~/.claude/plugins/cache

[INFO] Checking csm
[✓] csm documentation is synced

[INFO] Checking wayfinder
[✗] Documented but missing in wayfinder:
    --removed-flag

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[✗] Documentation drift detected

[INFO] To fix:
[INFO]   1. Update documentation to match actual flags
[INFO]   2. Or add missing flags to CLI tools
[INFO]   3. Run this script again to verify
```

**Exit code**: `1` (drift detected)

---

### Example 2: Check Specific Tool

```bash
$ ./verify-cli-docs.sh --tool csm
[INFO] CLI Documentation Verification
[INFO] Docs root: ~/.claude/plugins/cache

[INFO] Checking csm
[✓] csm documentation is synced

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[✓] All documentation synced with implementation
```

**Exit code**: `0` (success)

---

### Example 3: Verbose Mode

```bash
$ ./verify-cli-docs.sh --verbose --tool csm
[INFO] CLI Documentation Verification
[INFO] Docs root: ~/.claude/plugins/cache

[INFO] Checking csm
[INFO] Extracting flags from ~/.claude/plugins/cache/.../csm-assoc.md
[INFO] Running csm --help
[✓] csm documentation is synced

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[✓] All documentation synced with implementation
```

---

## CI/CD Integration

### GitHub Actions

Create `.github/workflows/verify-cli-docs.yml`:

```yaml
name: Verify CLI Documentation

on:
  pull_request:
    paths:
      - '**.md'
      - 'commands/**'
  push:
    branches:
      - main

jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install CLI tools
        run: |
          # Install your CLI tools here
          # e.g., go install ./cmd/...

      - name: Verify documentation
        run: |
          chmod +x scripts/verify-cli-docs.sh
          ./scripts/verify-cli-docs.sh

      - name: Fail if drift detected
        if: failure()
        run: |
          echo "Documentation drift detected!"
          echo "Update docs to match implementation or vice versa."
          exit 1
```

---

### Pre-Commit Hook

Create `.git/hooks/pre-commit`:

```bash
#!/usr/bin/env bash

# Run CLI docs verification before commit
if ! ./scripts/verify-cli-docs.sh; then
    echo "❌ Pre-commit hook failed: Documentation drift detected"
    echo "Fix drift and try again, or skip with 'git commit --no-verify'"
    exit 1
fi
```

Make it executable:

```bash
chmod +x .git/hooks/pre-commit
```

---

## Maintenance Protocol

### When Adding a New Flag

1. **Update CLI implementation** (`cmd/*/flags.go`, `src/*.rs`, etc.)
2. **Update documentation** (`commands/*.md`)
3. **Run verification**:
   ```bash
   ./verify-cli-docs.sh --tool <your-tool>
   ```
4. **Fix any drift** reported by script
5. **Commit code + docs together**

---

### When Removing a Flag

1. **Remove from CLI implementation**
2. **Remove from ALL documentation** (grep to find all references)
3. **Add deprecation notice** to CHANGELOG
4. **Run verification**:
   ```bash
   ./verify-cli-docs.sh
   ```
5. **Commit code + docs together**

---

### When Renaming a Flag

1. **Update CLI implementation**
2. **Update ALL documentation references**
3. **Optional**: Keep old flag as alias for backward compatibility
4. **Add migration guide** to docs
5. **Run verification**:
   ```bash
   ./verify-cli-docs.sh
   ```
6. **Commit code + docs together**

---

## Troubleshooting

### Issue: "Tool 'xyz' not found in PATH"

**Cause**: CLI tool not installed or not in PATH

**Fix**:
```bash
# Install the tool
go install ./cmd/xyz
# or
cargo install xyz

# Verify it's in PATH
which xyz

# Or specify PATH
export PATH="$PATH:/path/to/tools"
./verify-cli-docs.sh
```

---

### Issue: "Documentation root not found"

**Cause**: Custom docs location doesn't exist

**Fix**:
```bash
# Check path exists
ls -la ~/.claude/plugins/cache

# Or specify correct path
./verify-cli-docs.sh --docs-root ~/correct/path
```

---

### Issue: "No documentation files found"

**Cause**: Script expects `commands/*.md` structure

**Fix**:
```bash
# Check your docs structure
find ~/.claude/plugins/cache -name "*.md" -path "*/commands/*"

# Or use custom docs root
./verify-cli-docs.sh --docs-root ~/my/docs/location
```

---

### Issue: Script reports drift but documentation looks correct

**Cause**: Flag extraction regex may not match your documentation format

**Fix**:
1. Run with `--verbose` to see extracted flags:
   ```bash
   ./verify-cli-docs.sh --verbose --tool xyz
   ```
2. Check how flags are documented (e.g., `--flag` vs `\`--flag\``)
3. Update regex in `extract_documented_flags()` if needed

---

## How It Works

### 1. Flag Extraction

Script uses regex to find flag patterns in markdown:

```bash
grep -oE -- '--[a-z0-9-]+|-[a-z]' "$doc_file"
```

**Matches**:
- Long flags: `--flag`, `--flag-name`, `--my-flag`
- Short flags: `-f`, `-x`, `-v`

**Example markdown**:
```
csm associate <session-name> --uuid --create
```

**Extracted**: `--uuid`, `--create`

---

### 2. Actual Flag Discovery

Script runs each tool with `--help` and extracts flags:

```bash
csm --help 2>&1 | grep -oE -- '--[a-z0-9-]+|-[a-z]'
```

**Example output**:
```
--uuid
--create
--update-timestamp-only
--auto-detect-only
```

---

### 3. Comparison

Script uses `comm` to find differences:

```bash
# Flags in docs but not in implementation
comm -23 <(documented) <(actual)

# Flags in implementation but not in docs
comm -13 <(documented) <(actual)
```

---

## Limitations

1. **Regex-based extraction**: May miss flags in unusual formats
2. **Requires --help**: Tools must support `--help` flag
3. **No type checking**: Doesn't verify flag types (string vs int vs bool)
4. **No subcommand support**: Doesn't verify subcommand-specific flags
5. **No example execution**: Doesn't actually run documented examples

---

## Future Enhancements

- Auto-fix mode: Automatically update docs when drift detected
- Flag type verification: Check string/int/bool types match
- Example execution: Actually run documented command examples
- Subcommand support: Verify subcommand-specific flags
- Completeness scoring: Report % of flags documented

---

## Related Documentation

- Project: ``
- Sync issues report: `sync-issues-report.md`
- Maintenance protocol: `task-2.3-sync-maintenance-protocol.md`

---

**Version**: 1.0
**Last Updated**: 2026-02-04
