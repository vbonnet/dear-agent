# Migration Guide: Bash → Go

This guide helps users migrate from the old Bash script (`gemini-deep-research.sh`) to the new Go binary.

## Overview

The Go rewrite provides the same functionality with several improvements:

- Cross-platform support (Linux, macOS, Windows)
- Better error handling and recovery
- Type safety and compile-time validation
- Modular, testable architecture
- Support for multiple content types (not just YouTube)
- Improved configuration management

## Quick Migration

### Before (Bash)

```bash
VIDEO_URL="https://www.youtube.com/watch?v=VIDEO_ID" \
OUTPUT_DIR="./my-output" \
GOOGLE_CLOUD_PROJECT="my-project" \
./gemini-deep-research.sh
```

### After (Go)

```bash
gemini-deep-research \
  --output-dir ./my-output \
  --project my-project \
  https://www.youtube.com/watch?v=VIDEO_ID
```

## Breaking Changes

### 1. Positional Arguments

**Before**: URL passed via `VIDEO_URL` environment variable
```bash
VIDEO_URL="https://example.com" ./gemini-deep-research.sh
```

**After**: URL is a positional argument
```bash
gemini-deep-research https://example.com
```

### 2. Configuration Format

**Before**: Environment variables only
```bash
OUTPUT_DIR="./output"
TIMEOUT=90
```

**After**: Command-line flags (with environment variable fallbacks)
```bash
gemini-deep-research --output-dir ./output --timeout 90 https://example.com
```

Or still use environment variables:
```bash
export GEMINI_DR_OUTPUT_DIR="./output"
export GEMINI_DR_TIMEOUT=90
gemini-deep-research https://example.com
```

### 3. API Authentication

**Before**: Used ADC (Application Default Credentials) only
```bash
gcloud auth application-default login
```

**After**: Uses direct API key
```bash
export GEMINI_API_KEY="your-api-key"
```

**Migration Note**: If you prefer ADC, you can still use it by extracting the token:
```bash
export GEMINI_API_KEY=$(gcloud auth application-default print-access-token)
```

However, API keys are recommended for simplicity.

### 4. Output Structure

**Before**: Files written directly to output directory
```
output/
├── transcript.txt
├── topics.json
└── research-report.md
```

**After**: Files written to timestamped subdirectory
```
output/
└── 20240203-150405-example-com/
    ├── metadata.json
    ├── topics.json
    ├── report.md
    └── content.txt
```

**Migration Note**: Each run creates a new timestamped directory, preserving history.

### 5. Exit Codes

**Before**: Generic exit codes (0 or 1)

**After**: Specific exit codes for different errors
- 0: Success
- 1: Invalid arguments
- 2: Extraction failed
- 3: Analysis failed
- 4: Research failed
- 5: Output failed
- 6: Unexpected error

## Feature Mapping

### Environment Variables

| Bash Variable | Go Equivalent | Notes |
|---------------|---------------|-------|
| `VIDEO_URL` | Positional argument | Now required as CLI arg |
| `OUTPUT_DIR` | `--output-dir` or `GEMINI_DR_OUTPUT_DIR` | Flag preferred |
| `GOOGLE_CLOUD_PROJECT` | `--project` or `GOOGLE_CLOUD_PROJECT` | Same env var name |
| N/A | `GEMINI_API_KEY` | New: API key required |
| N/A | `--timeout` or `GEMINI_DR_TIMEOUT` | New: configurable timeout |

### Custom Prompts

**Before**: Not supported directly (had to modify script)

**After**: Built-in support
```bash
# Short prompt
gemini-deep-research --input "Focus on security" https://example.com

# File-based prompt
gemini-deep-research --input-file ./prompt.txt https://example.com
```

### Content Type Override

**Before**: Not supported (always detected)

**After**: Manual override available
```bash
gemini-deep-research --type article https://youtube.com/watch?v=VIDEO_ID
```

## New Features

### 1. Multiple Content Types

The Go version supports more than just YouTube videos:

```bash
# YouTube video
gemini-deep-research https://www.youtube.com/watch?v=VIDEO_ID

# arXiv paper
gemini-deep-research https://arxiv.org/abs/2501.12345

# HuggingFace paper
gemini-deep-research https://huggingface.co/papers/2501.12345

# Generic web article
gemini-deep-research https://blog.example.com/article
```

### 2. Better Error Messages

The Go version provides clear, actionable error messages:

```
Error detecting content type: invalid URL scheme: ftp (must be http or https)

Examples of valid URLs:
  https://www.youtube.com/watch?v=VIDEO_ID
  https://arxiv.org/abs/2601.20802
  https://huggingface.co/papers/2501.12345
  https://example.com/article
```

### 3. Structured Output

The `metadata.json` file contains complete run metadata:

```json
{
  "url": "https://example.com",
  "content_type": "GenericArticle",
  "topics": ["Topic 1", "Topic 2"],
  "timestamp": "2024-02-03T15:04:05Z",
  "metadata": {
    "title": "Article Title",
    "authors": ["Author"]
  }
}
```

### 4. Testing Support

The Go version includes comprehensive tests:

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...
```

## Common Migration Scenarios

### Scenario 1: Basic YouTube Analysis

**Before**:
```bash
#!/bin/bash
VIDEO_URL="https://www.youtube.com/watch?v=dQw4w9WgXcQ"
./gemini-deep-research.sh
```

**After**:
```bash
#!/bin/bash
gemini-deep-research https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

### Scenario 2: Custom Output Directory

**Before**:
```bash
#!/bin/bash
VIDEO_URL="https://www.youtube.com/watch?v=VIDEO_ID"
OUTPUT_DIR="/data/research"
./gemini-deep-research.sh
```

**After**:
```bash
#!/bin/bash
gemini-deep-research \
  --output-dir /data/research \
  https://www.youtube.com/watch?v=VIDEO_ID
```

### Scenario 3: Automated Batch Processing

**Before**:
```bash
#!/bin/bash
for url in $(cat urls.txt); do
  VIDEO_URL="$url" OUTPUT_DIR="./output-$(date +%s)" ./gemini-deep-research.sh
done
```

**After**:
```bash
#!/bin/bash
# Go version automatically creates timestamped directories
while read -r url; do
  gemini-deep-research "$url"
done < urls.txt
```

### Scenario 4: CI/CD Integration

**Before**:
```bash
#!/bin/bash
set -e

export GOOGLE_CLOUD_PROJECT="my-project"
export VIDEO_URL="https://www.youtube.com/watch?v=VIDEO_ID"

./gemini-deep-research.sh || exit 1
```

**After**:
```bash
#!/bin/bash
set -e

export GEMINI_API_KEY="${GEMINI_API_KEY}"
export GOOGLE_CLOUD_PROJECT="my-project"

gemini-deep-research \
  --project my-project \
  https://www.youtube.com/watch?v=VIDEO_ID || exit $?
```

## Compatibility Layer

If you need to maintain compatibility with old scripts, create a wrapper:

```bash
#!/bin/bash
# gemini-deep-research-compat.sh
# Wrapper for backward compatibility with bash version

set -euo pipefail

# Check required variables
if [ -z "${VIDEO_URL:-}" ]; then
  echo "Error: VIDEO_URL not set" >&2
  exit 1
fi

# Build command
CMD="gemini-deep-research"

# Add optional flags
if [ -n "${OUTPUT_DIR:-}" ]; then
  CMD="$CMD --output-dir $OUTPUT_DIR"
fi

if [ -n "${TIMEOUT:-}" ]; then
  CMD="$CMD --timeout $TIMEOUT"
fi

if [ -n "${GOOGLE_CLOUD_PROJECT:-}" ]; then
  CMD="$CMD --project $GOOGLE_CLOUD_PROJECT"
fi

# Execute
$CMD "$VIDEO_URL"
```

Usage:
```bash
VIDEO_URL="https://youtube.com/watch?v=VIDEO_ID" ./gemini-deep-research-compat.sh
```

## Troubleshooting Migration

### Issue: "GEMINI_API_KEY not set"

**Cause**: Go version requires API key instead of ADC

**Solution**: Set API key
```bash
export GEMINI_API_KEY="your-api-key"
```

Or use ADC token:
```bash
export GEMINI_API_KEY=$(gcloud auth application-default print-access-token)
```

### Issue: "Can't find output files"

**Cause**: Output structure changed to timestamped directories

**Solution**: Look in subdirectory
```bash
ls -la output/
# output/20240203-150405-example-com/
```

Or get latest:
```bash
ls -t output/ | head -1
```

### Issue: "Script expects VIDEO_URL variable"

**Cause**: URL is now a positional argument

**Solution**: Update script
```bash
# Before
VIDEO_URL="$url" ./tool

# After
./tool "$url"
```

### Issue: "Exit codes changed"

**Cause**: Go version uses specific exit codes

**Solution**: Update error handling
```bash
gemini-deep-research "$url"
case $? in
  0) echo "Success" ;;
  1) echo "Invalid arguments" ;;
  2) echo "Extraction failed" ;;
  3) echo "Analysis failed" ;;
  4) echo "Research failed" ;;
  5) echo "Output failed" ;;
  *) echo "Unexpected error" ;;
esac
```

## Rollback Plan

If you need to rollback to the Bash version:

1. The old script is archived at `.archived/gemini-deep-research-v1.sh`
2. Restore it:
   ```bash
   cp .archived/gemini-deep-research-v1.sh ./gemini-deep-research.sh
   chmod +x ./gemini-deep-research.sh
   ```

3. Revert to ADC authentication:
   ```bash
   gcloud auth application-default login
   ```

4. Use old environment variable format:
   ```bash
   VIDEO_URL="https://example.com" ./gemini-deep-research.sh
   ```

## Benefits of Migration

### 1. Type Safety

Go's type system catches errors at compile time:
- Invalid URLs
- Missing configuration
- Incorrect data types

### 2. Better Error Handling

- Specific error messages
- Proper error propagation
- Retry logic with backoff

### 3. Cross-Platform

Works on Linux, macOS, and Windows without modification.

### 4. Performance

- Faster startup time
- Lower memory usage
- Better concurrency support

### 5. Maintainability

- Modular code structure
- Comprehensive tests
- Clear separation of concerns

## Getting Help

If you encounter issues during migration:

1. Check this migration guide
2. Review the [README.md](README.md) for usage examples
3. Check [ARCHITECTURE.md](ARCHITECTURE.md) for technical details
4. Open an issue on GitHub with:
   - Your bash command/script
   - Expected behavior
   - Actual behavior
   - Error messages

## Timeline

- **v1.x**: Bash script (deprecated)
- **v2.0**: Go rewrite (current)
- **v2.1+**: Additional features, no breaking changes planned

The Bash script is now deprecated and will not receive updates. All new features will be added to the Go version.
