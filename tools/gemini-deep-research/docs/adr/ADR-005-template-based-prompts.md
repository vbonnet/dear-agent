# ADR-005: Template-Based Prompts and Variable Substitution

**Status**: Accepted
**Date**: 2025-01-20 (backfilled 2025-02-11)
**Deciders**: Engineering Team
**Tags**: customization, prompts, templates, variables

## Context

The gemini-deep-research tool uses prompts at three pipeline stages:
1. **Extraction**: Guiding content extraction (optional)
2. **Analysis**: Identifying research topics with Gemini
3. **Research**: Formulating Deep Research queries

Initially, prompts were:
- **Hardcoded**: Default prompts embedded in source code
- **Static**: No runtime customization
- **Context-free**: No access to runtime data (URL, topics, content type)

### User Requirements

Users need to customize prompts for:
- **Domain-specific research**: Security analysis, architecture review, performance optimization
- **Custom workflows**: Competitive analysis, technical documentation, code review
- **Template reuse**: Share prompts across team members
- **Context awareness**: Reference URL, topics, or content type in prompts

### Technical Requirements

1. **Per-Stage Customization**: Different prompts for extract, analyze, research stages
2. **File-Based Templates**: Load prompts from external files
3. **Variable Substitution**: Inject runtime data into prompts
4. **Validation**: Fail fast on unknown variables
5. **Azure CLI Compatibility**: Support @file syntax (`@prompts/security.txt`)

## Decision

Implement a **three-layer prompt system**:

```
ConfigParser → FileResolver → VariableSubstitutor
```

### Layer 1: ConfigParser

Parse CLI flags and identify prompt source:

```go
type PromptConfig struct {
    ExtractPrompt   string // --extract-prompt flag
    AnalyzePrompt   string // --analyze-prompt flag
    ResearchPrompt  string // --research-prompt flag
}

// Determine if prompt is inline text or @file reference
func ParsePromptSource(prompt string) (source PromptSource, value string) {
    if strings.HasPrefix(prompt, "@") {
        return SourceFile, strings.TrimPrefix(prompt, "@")
    }
    return SourceInline, prompt
}
```

**Usage**:
```bash
# Inline prompt
gemini-deep-research --analyze-prompt "Focus on security topics" https://example.com

# File reference
gemini-deep-research --analyze-prompt "@prompts/security.txt" https://example.com
```

### Layer 2: FileResolver

Load prompt content from files (Azure @file syntax):

```go
func ResolvePromptFromFile(path string) (string, error) {
    // Expand ~ and relative paths
    expandedPath := expandPath(path)

    // Validate file exists
    if _, err := os.Stat(expandedPath); err != nil {
        return "", fmt.Errorf("prompt file not found: %s", path)
    }

    // Read file content
    content, err := os.ReadFile(expandedPath)
    if err != nil {
        return "", fmt.Errorf("failed to read prompt file: %w", err)
    }

    // Validate file size (prevent accidental large files)
    if len(content) > 1_000_000 { // 1MB limit
        return "", fmt.Errorf("prompt file too large: %s (max 1MB)", path)
    }

    return string(content), nil
}
```

**Features**:
- Cross-platform path resolution (Windows/Unix)
- UTF-8 encoding support
- File size limit (1MB)
- Helpful error messages

**Example prompt file** (`prompts/security.txt`):
```
Extract security vulnerabilities and CVEs from the content.

Focus on:
- Authentication/authorization flaws
- Input validation issues
- Cryptographic weaknesses
- Dependency vulnerabilities
- Configuration errors

For each vulnerability:
- Severity (Critical/High/Medium/Low)
- Description
- Mitigation steps
```

### Layer 3: VariableSubstitutor

Replace template variables with runtime values:

```go
type Variables struct {
    URL         string   // Source URL
    Topics      []string // Identified topics (available after analysis)
    ContentType string   // Detected content type
}

func SubstituteVariables(prompt string, vars Variables) (string, error) {
    // Define available variables
    availableVars := map[string]string{
        "url":          vars.URL,
        "topics":       strings.Join(vars.Topics, ", "),
        "content_type": vars.ContentType,
    }

    // Find all template variables in prompt
    varPattern := regexp.MustCompile(`\{([a-z_]+)\}`)
    matches := varPattern.FindAllStringSubmatch(prompt, -1)

    // Validate all variables are known
    for _, match := range matches {
        varName := match[1]
        if _, exists := availableVars[varName]; !exists {
            knownVars := []string{"url", "topics", "content_type"}
            return "", fmt.Errorf(
                "unknown variable: {%s} (available: %s)",
                varName,
                strings.Join(knownVars, ", "),
            )
        }
    }

    // Substitute variables
    result := prompt
    for varName, varValue := range availableVars {
        placeholder := fmt.Sprintf("{%s}", varName)
        result = strings.ReplaceAll(result, placeholder, varValue)
    }

    return result, nil
}
```

**Available Variables**:

| Variable | Description | Example Value | Available Stage |
|----------|-------------|---------------|-----------------|
| `{url}` | Source URL being analyzed | `https://youtube.com/watch?v=abc123` | All stages |
| `{topics}` | Comma-separated list of topics | `AI, machine learning, neural networks` | Research only |
| `{content_type}` | Detected content type | `video`, `article`, `arxiv` | All stages |

**Example template**:
```
Research the following topics from {url}:
{topics}

Focus on practical implementation and security considerations.
Prioritize {content_type}-specific insights.
```

**After substitution** (runtime):
```
Research the following topics from https://example.com/article:
AI security, threat detection, vulnerability scanning

Focus on practical implementation and security considerations.
Prioritize article-specific insights.
```

**Error handling**:
```bash
# Unknown variable
gemini-deep-research --analyze-prompt "Extract {foo} from content" https://example.com

# Error: unknown variable: {foo} (available: url, topics, content_type)
```

### Prompt Loading Pipeline

```
Step 2.5: Loading prompt configuration...

┌──────────────┐
│ CLI Flags    │
│ (user input) │
└──────┬───────┘
       │
       ▼
┌──────────────────────────┐
│ ConfigParser             │
│ - Parse --*-prompt flags │
│ - Detect @file syntax    │
└──────────┬───────────────┘
           │
           ▼
┌──────────────────────────┐
│ FileResolver             │
│ - Load file if @file     │
│ - Validate file exists   │
│ - Check file size        │
└──────────┬───────────────┘
           │
           ▼
┌──────────────────────────┐
│ VariableSubstitutor      │
│ - Find {variables}       │
│ - Validate known vars    │
│ - Substitute values      │
└──────────┬───────────────┘
           │
           ▼
┌──────────────────────────┐
│ Final Prompt             │
│ (ready for Gemini/API)   │
└──────────────────────────┘
```

## Consequences

### Positive

1. **Customization**: Users can adapt prompts to their domain without code changes
2. **Reusability**: Prompt files shareable across team members
3. **Context-Awareness**: Variables enable dynamic prompts based on runtime data
4. **Fail-Fast**: Unknown variables caught early with helpful error messages
5. **Azure CLI Compatibility**: @file syntax familiar to Azure users
6. **Separation of Concerns**: Prompts decoupled from code
7. **Testability**: Easy to test with different prompt variations

### Negative

1. **Complexity**: Three-layer system more complex than hardcoded prompts
2. **File Management**: Users must manage prompt files
3. **Error Surface**: More failure points (file not found, invalid variables)
4. **Learning Curve**: Users must learn variable syntax
5. **Template Maintenance**: Outdated templates may produce poor results

### Neutral

1. **Variable Expansion**: Limited to string substitution (no conditionals/loops)
2. **File Size Limit**: 1MB limit prevents misuse but may restrict legitimate use cases
3. **UTF-8 Only**: No support for other encodings

## Implementation

### File Structure

```
project/
├── prompts/
│   ├── security.txt         # Security-focused analysis
│   ├── architecture.txt     # Architecture review
│   ├── performance.txt      # Performance analysis
│   └── competitive.txt      # Competitive analysis
└── main.go
```

### Usage Examples

**1. Inline Prompt**:
```bash
gemini-deep-research \
  --analyze-prompt "Extract key topics from {url}" \
  https://example.com
```

**2. File-Based Prompt**:
```bash
# Create prompt file
cat > security-prompt.txt <<EOF
Extract security vulnerabilities from {url}.
Focus on {content_type}-specific issues.
EOF

# Use prompt
gemini-deep-research --analyze-prompt "@security-prompt.txt" https://example.com
```

**3. Multi-Stage Prompts**:
```bash
gemini-deep-research \
  --extract-prompt "@prompts/extract-security.txt" \
  --analyze-prompt "@prompts/analyze-security.txt" \
  --research-prompt "@prompts/research-security.txt" \
  https://example.com
```

**4. Variable Substitution**:
```bash
# Prompt file with variables
cat > research-template.txt <<EOF
Research these topics from {url}:
{topics}

Provide {content_type}-specific implementation details.
EOF

# Variables substituted at runtime
gemini-deep-research --research-prompt "@research-template.txt" https://example.com
```

### Testing Strategy

**Unit Tests**:
```go
// Test @file detection
func TestPromptSourceDetection(t *testing.T) {
    tests := []struct {
        input  string
        source PromptSource
        value  string
    }{
        {"@prompts/security.txt", SourceFile, "prompts/security.txt"},
        {"inline prompt text", SourceInline, "inline prompt text"},
    }

    for _, tt := range tests {
        source, value := ParsePromptSource(tt.input)
        assert.Equal(t, tt.source, source)
        assert.Equal(t, tt.value, value)
    }
}

// Test variable substitution
func TestVariableSubstitution(t *testing.T) {
    vars := Variables{
        URL:         "https://example.com",
        Topics:      []string{"topic1", "topic2"},
        ContentType: "article",
    }

    prompt := "Research {topics} from {url} ({content_type})"
    result, err := SubstituteVariables(prompt, vars)

    require.NoError(t, err)
    assert.Contains(t, result, "https://example.com")
    assert.Contains(t, result, "topic1, topic2")
    assert.Contains(t, result, "article")
}

// Test unknown variable detection
func TestUnknownVariableError(t *testing.T) {
    vars := Variables{}
    prompt := "Extract {unknown_var} from content"

    _, err := SubstituteVariables(prompt, vars)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unknown variable: {unknown_var}")
    assert.Contains(t, err.Error(), "available: url, topics, content_type")
}
```

**Integration Tests**:
```go
// Test E2E with file-based prompt
func TestFilePromptE2E(t *testing.T) {
    // Create temporary prompt file
    tmpFile := filepath.Join(t.TempDir(), "prompt.txt")
    os.WriteFile(tmpFile, []byte("Extract topics from {url}"), 0644)

    flags := &types.Flags{
        AnalyzePrompt: "@" + tmpFile,
    }

    exitCode := Run("https://example.com", flags, config)
    assert.Equal(t, 0, exitCode)
}
```

## Alternatives Considered

### 1. Go text/template Package

**Approach**: Use Go's built-in template engine

```go
import "text/template"

tmpl := template.Must(template.New("prompt").Parse("Research {{.Topics}} from {{.URL}}"))
tmpl.Execute(buf, vars)
```

**Pros**:
- Standard library (no dependencies)
- Rich features (conditionals, loops, functions)
- Well-documented

**Cons**:
- Syntax unfamiliar to non-Go users (`{{.Topics}}` vs `{topics}`)
- Overkill for simple variable substitution
- Error messages less user-friendly
- Template compilation overhead

**Decision**: Rejected due to complexity and syntax

### 2. Environment Variables for Prompts

**Approach**: Load prompts from environment variables

```bash
export ANALYZE_PROMPT="Extract topics from the content"
gemini-deep-research https://example.com
```

**Pros**:
- Simple implementation
- No file management

**Cons**:
- Difficult to manage multi-line prompts
- Shell escaping issues
- Not shareable (no version control)
- Limited to environment size
- Poor user experience

**Decision**: Rejected due to poor UX

### 3. JSON Configuration Files

**Approach**: Define prompts in JSON config

```json
{
  "prompts": {
    "analyze": "Extract topics from {url}",
    "research": "Research {topics}"
  }
}
```

**Pros**:
- Structured configuration
- Easy to parse
- Supports complex configs

**Cons**:
- JSON escaping for multi-line prompts
- Heavyweight for simple use case
- Requires config file even for simple customization
- Difficult to edit (JSON syntax)

**Decision**: Rejected due to complexity

### 4. YAML Frontmatter in Prompts

**Approach**: Embed metadata in prompt files

```yaml
---
stage: analyze
variables: [url, topics]
---
Extract topics from {url}
```

**Pros**:
- Self-documenting prompts
- Metadata validation

**Cons**:
- Parsing overhead
- Complexity for simple prompts
- Frontmatter unfamiliar to non-technical users

**Decision**: Rejected as premature optimization

### 5. No Variable Substitution

**Approach**: Only support static prompts

**Pros**:
- Simplest implementation
- No validation needed

**Cons**:
- Prompts not context-aware
- Users must manually include URL/topics
- Poor user experience
- Limited customization

**Decision**: Rejected due to poor UX

## Related Decisions

- ADR-004: Competitive Analysis Mode (uses template system)
- ADR-001: Go Rewrite (enables structured prompt system)
- ADR-006: Configuration Management (prompt loading fits config pattern)

## References

- [Azure CLI @file Syntax](https://docs.microsoft.com/en-us/cli/azure/use-cli-effectively#use-files-for-arguments)
- [String Template Variables](https://en.wikipedia.org/wiki/String_interpolation)
- [Configuration Parser](../../config/parser.go)
- [File Resolver](../../config/file_resolver.go)
- [Variable Substitutor](../../config/variable-substitutor.go)

## Notes

The three-layer architecture provides flexibility while maintaining simplicity:
- **ConfigParser**: Determines prompt source (inline vs file)
- **FileResolver**: Loads content from files with validation
- **VariableSubstitutor**: Injects runtime data with fail-fast validation

The @file syntax provides familiar UX for Azure CLI users while avoiding shell escaping issues with multi-line prompts.

Variable substitution is intentionally limited to string replacement (no conditionals/loops) to maintain simplicity and predictability. Future enhancements could explore template functions if needed.

Error messages prioritize clarity ("unknown variable: {foo} (available: url, topics, content_type)") to guide users toward correct usage.
