# ADR-006: Error Handling Strategy and Exit Codes

**Status**: Accepted
**Date**: 2025-01-05 (backfilled 2025-02-11)
**Deciders**: Engineering Team
**Tags**: error-handling, user-experience, debugging, cli

## Context

As a CLI tool, gemini-deep-research must provide:
1. **Clear feedback**: Users understand what went wrong
2. **Actionable guidance**: Users know how to fix the problem
3. **Exit codes**: Scripts can detect failure types
4. **Debugging support**: Developers can troubleshoot issues
5. **Graceful degradation**: Failures don't cascade

The bash version had:
- Generic error messages ("failed")
- No exit code differentiation (all failures = exit 1)
- Stack traces exposed to users
- Silent failures (errors suppressed)

### Requirements

1. **Specific Exit Codes**: Different codes for different failure types
2. **Structured Errors**: Error types with context
3. **Helpful Messages**: Include troubleshooting steps
4. **No Secret Leakage**: Never log API keys or sensitive data
5. **Developer Context**: Stack traces available for debugging

## Decision

Implement a **tiered error handling strategy** with:

### Exit Code System

```go
const (
    ExitSuccess           = 0 // Pipeline completed successfully
    ExitInvalidArgs       = 1 // Invalid arguments or configuration
    ExitExtractionFailed  = 2 // Content extraction failed
    ExitAnalysisFailed    = 3 // Topic analysis failed
    ExitResearchFailed    = 4 // Deep Research failed
    ExitOutputFailed      = 5 // Output writing failed
    ExitUnexpected        = 6 // Unexpected/panic errors
)
```

**Exit Code Usage**:
```bash
gemini-deep-research https://example.com
echo $?  # Check exit code

# Exit codes enable scripted error handling
if ! gemini-deep-research $URL; then
    case $? in
        1) echo "Configuration error" ;;
        2) echo "Extraction failed" ;;
        3) echo "Analysis failed" ;;
        4) echo "Research failed" ;;
        5) echo "Output failed" ;;
        *) echo "Unexpected error" ;;
    esac
fi
```

### Structured Error Types

**Error Hierarchy**:
```go
// Base error interface
type Error interface {
    error
    ExitCode() int
    Context() map[string]interface{}
}

// Configuration errors (exit 1)
type ConfigError struct {
    Field   string
    Value   string
    Reason  string
}

// Extraction errors (exit 2)
type ExtractionError struct {
    URL         string
    ContentType string
    Cause       error
}

// Analysis errors (exit 3)
type AnalysisError struct {
    Content string // Truncated, not full content
    Cause   error
}

// Research errors (exit 4)
type ResearchError struct {
    Topics []string
    Cause  error
}

// Output errors (exit 5)
type OutputError struct {
    Path  string
    Cause error
}
```

**Usage**:
```go
func extractContent(url string) (*Content, error) {
    content, err := extractor.Extract(ctx, url)
    if err != nil {
        return nil, &ExtractionError{
            URL:         url,
            ContentType: contentType.String(),
            Cause:       err,
        }
    }
    return content, nil
}
```

### Error Message Format

**Template**:
```
Error: <Short description>

<Context section>

<Troubleshooting section>
```

**Example 1: Invalid URL**
```
Error: Invalid URL

URL: not-a-url
Reason: Missing scheme (http:// or https://)

Examples of valid URLs:
  https://www.youtube.com/watch?v=VIDEO_ID
  https://arxiv.org/abs/2601.20802
  https://huggingface.co/papers/2501.12345
  https://example.com/article

Or try competitive analysis:
  gemini-deep-research "competitive analysis of Tool X vs Tool Y"
```

**Example 2: Extraction Failure**
```
Error: Failed to extract transcript from video

URL: https://youtube.com/watch?v=abc123
Cause: No subtitles available

Possible causes:
1. Video has no subtitles → Try different video
2. Video is private/restricted → Use public video
3. yt-dlp needs update → Run: pip install --upgrade yt-dlp

Troubleshooting:
- Check video has subtitles: https://youtube.com/watch?v=abc123
- Verify yt-dlp installation: yt-dlp --version
- Test extraction manually: yt-dlp --write-auto-sub --skip-download <URL>
```

**Example 3: API Error**
```
Error: Deep Research timeout

Research timeout: 60 minutes
Topics: artificial intelligence, machine learning, neural networks

Solution: Increase timeout
  gemini-deep-research --timeout 120 https://example.com

Or simplify research scope (reduce topic count)
```

**Example 4: Missing API Key**
```
Error: GEMINI_API_KEY not set

The Gemini API key is required for Deep Research.

Setup instructions:
1. Get your API key: https://ai.google.dev/
2. Export environment variable:
   export GEMINI_API_KEY="your-api-key-here"
3. Run command again

For persistent configuration, add to ~/.bashrc or ~/.zshrc:
  echo 'export GEMINI_API_KEY="your-key"' >> ~/.bashrc
```

### Logging Strategy

**Log Levels**:
```go
log.Printf("info: <message>")     // Normal operation
log.Printf("warning: <message>")  // Non-fatal issues
log.Printf("error: <message>")    // Fatal errors
log.Printf("debug: <message>")    // Debugging only
```

**What to Log**:
- ✅ Content type detection
- ✅ Extraction progress
- ✅ Topic count
- ✅ Research status
- ✅ Cache hits/misses
- ✅ Configuration values
- ✅ Pipeline steps

**What NOT to Log**:
- ❌ API keys
- ❌ Full content (only length)
- ❌ Credentials
- ❌ Sensitive user data

**Example Logging**:
```go
// Good: Contextual without secrets
log.Printf("Starting research with %d topics (timeout: %d min)", len(topics), cfg.Timeout)

// Bad: Exposes API key
log.Printf("API key: %s", apiKey) // NEVER DO THIS

// Good: Sanitized error
log.Printf("API call failed: %v", sanitizeError(err))

// Bad: Full content logged
log.Printf("Content: %s", content.Raw) // Avoid (too verbose)
```

### Error Wrapping

**Use Go 1.13+ error wrapping**:
```go
import "fmt"

// Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to extract content from %s: %w", url, err)
}

// Unwrap errors
if errors.Is(err, context.DeadlineExceeded) {
    return &ResearchError{Cause: errors.New("research timeout")}
}
```

**Error Chain Example**:
```
Error: failed to run pipeline
  → failed to extract content
    → failed to download video
      → yt-dlp: HTTP 403 Forbidden
```

### Panic Recovery

**Top-level panic handler**:
```go
func main() {
    defer func() {
        if r := recover(); r != nil {
            fmt.Fprintf(os.Stderr, "Error: Unexpected panic\n\n")
            fmt.Fprintf(os.Stderr, "Panic: %v\n", r)
            fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", debug.Stack())
            fmt.Fprintf(os.Stderr, "\nPlease report this issue: https://github.com/vbonnet/ai-tools/issues\n")
            os.Exit(ExitUnexpected)
        }
    }()

    // Run application
    exitCode := cmd.Run(url, flags, config)
    os.Exit(exitCode)
}
```

## Consequences

### Positive

1. **User Clarity**: Specific exit codes enable scripting
2. **Troubleshooting**: Actionable error messages reduce support burden
3. **Debugging**: Structured errors with context aid development
4. **Security**: No API key/credential leakage in logs
5. **Reliability**: Panic recovery prevents crashes
6. **Graceful Degradation**: Cache failures don't stop pipeline

### Negative

1. **Complexity**: More error handling code than simple `return err`
2. **Maintenance**: Error messages need updates as tool evolves
3. **Verbosity**: Detailed errors increase output size

### Neutral

1. **Error Types**: Custom error types add abstraction but improve clarity
2. **Exit Codes**: Standard convention but requires documentation

## Implementation

### Error Helper Functions

```go
// Sanitize errors to remove sensitive data
func sanitizeError(err error) error {
    errStr := err.Error()
    // Remove API keys
    apiKeyPattern := regexp.MustCompile(`key=[\w-]+`)
    errStr = apiKeyPattern.ReplaceAllString(errStr, "key=***")
    // Remove auth tokens
    tokenPattern := regexp.MustCompile(`Bearer [\w-]+`)
    errStr = tokenPattern.ReplaceAllString(errStr, "Bearer ***")
    return errors.New(errStr)
}

// Create user-friendly error messages
func formatError(err error, context map[string]interface{}) string {
    var buf bytes.Buffer

    fmt.Fprintf(&buf, "Error: %s\n\n", err.Error())

    if len(context) > 0 {
        fmt.Fprintf(&buf, "Context:\n")
        for key, value := range context {
            fmt.Fprintf(&buf, "  %s: %v\n", key, value)
        }
        fmt.Fprintf(&buf, "\n")
    }

    return buf.String()
}
```

### Testing Strategy

**Unit Tests**:
```go
// Test exit codes
func TestExitCodes(t *testing.T) {
    tests := []struct {
        name     string
        setup    func(*testing.T)
        expected int
    }{
        {
            name:     "invalid URL",
            setup:    func(t *testing.T) { /* setup invalid URL */ },
            expected: ExitInvalidArgs,
        },
        {
            name:     "extraction failure",
            setup:    func(t *testing.T) { /* setup extraction failure */ },
            expected: ExitExtractionFailed,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tt.setup(t)
            exitCode := Run(url, flags, config)
            assert.Equal(t, tt.expected, exitCode)
        })
    }
}

// Test error sanitization
func TestSanitizeError(t *testing.T) {
    err := errors.New("API call failed: key=abc123secret456")
    sanitized := sanitizeError(err)

    assert.NotContains(t, sanitized.Error(), "abc123secret456")
    assert.Contains(t, sanitized.Error(), "key=***")
}

// Test error messages
func TestErrorMessages(t *testing.T) {
    err := &ExtractionError{
        URL:         "https://example.com",
        ContentType: "video",
        Cause:       errors.New("no subtitles"),
    }

    message := formatError(err, nil)
    assert.Contains(t, message, "https://example.com")
    assert.Contains(t, message, "no subtitles")
    assert.Contains(t, message, "Possible causes")
}
```

**Integration Tests**:
```go
// Test error scenarios E2E
func TestErrorScenariosE2E(t *testing.T) {
    // Test missing API key
    os.Unsetenv("GEMINI_API_KEY")
    exitCode := Run(url, flags, config)
    assert.Equal(t, ExitInvalidArgs, exitCode)

    // Test invalid URL
    exitCode = Run("not-a-url", flags, config)
    assert.Equal(t, ExitInvalidArgs, exitCode)

    // Test extraction failure (mock extractor)
    exitCode = Run("https://private-video.com", flags, config)
    assert.Equal(t, ExitExtractionFailed, exitCode)
}
```

## Alternatives Considered

### 1. Single Exit Code

**Approach**: All failures exit with code 1

**Pros**:
- Simpler implementation
- Standard Unix convention

**Cons**:
- Scripts cannot differentiate failure types
- Poor debugging experience
- No actionable feedback

**Decision**: Rejected due to poor UX

### 2. HTTP-Style Status Codes

**Approach**: Use HTTP status codes (400, 404, 500, etc.)

**Pros**:
- Familiar to web developers
- Rich taxonomy

**Cons**:
- Not CLI-idiomatic
- Confusing for non-web users
- Exit codes > 255 not supported

**Decision**: Rejected as non-standard for CLI

### 3. Exception-Based Errors (Python-style)

**Approach**: Define exception classes, use panic/recover

**Pros**:
- Familiar to Python/Java developers
- Stack traces included

**Cons**:
- Not Go-idiomatic
- Performance overhead
- Difficult to handle gracefully

**Decision**: Rejected as non-Go-idiomatic

### 4. Verbose Debug Mode

**Approach**: Add --debug flag for detailed logging

**Pros**:
- Optional verbosity
- Useful for troubleshooting

**Cons**:
- Additional complexity
- Users may forget to enable
- Not always needed

**Decision**: Deferred to future enhancement

### 5. Structured JSON Errors

**Approach**: Output errors as JSON

```json
{
  "error": "extraction_failed",
  "message": "No subtitles available",
  "url": "https://youtube.com/watch?v=abc123"
}
```

**Pros**:
- Machine-readable
- Easy to parse in scripts

**Cons**:
- Poor human readability
- Requires --format json flag
- Complicates simple use cases

**Decision**: Rejected for default behavior, could add as `--format json` option

## Related Decisions

- ADR-001: Go Rewrite (enables structured error handling)
- ADR-003: Caching Strategy (graceful cache failure handling)
- ADR-006: Logging Strategy (error logging standards)

## References

- [Go Error Handling](https://go.dev/blog/error-handling-and-go)
- [Go 1.13 Errors](https://go.dev/blog/go1.13-errors)
- [Exit Code Conventions](https://www.gnu.org/software/libc/manual/html_node/Exit-Status.html)
- [CLI Error Handling Best Practices](https://clig.dev/#errors)

## Notes

The tiered error handling strategy balances:
- **User experience**: Clear, actionable error messages
- **Developer experience**: Structured errors with context
- **Security**: No credential leakage
- **Debugging**: Stack traces available without exposing to users

Exit codes follow Unix conventions (0 = success, 1-127 = various failures) while providing CLI-specific differentiation (1 = config, 2 = extraction, etc.).

Error messages prioritize troubleshooting steps over technical details. For example, instead of "HTTP 403", we say "Video is private → Use public video".

Panic recovery ensures the tool never crashes ungracefully, always providing actionable feedback even in unexpected scenarios.

Future enhancement: Consider adding `--format json` for machine-readable errors in automation scenarios.
