# ADR-002: Structured Error Handling with User Guidance

**Status**: Accepted

**Date**: 2024-01-20

**Context**: CLI error messages often lack context and actionable guidance. Users encountering errors need:
- Clear indication of what went wrong
- Context about why it failed
- Actionable suggestions to fix the issue
- Consistent error formatting across all commands

Traditional Go errors (`fmt.Errorf`) provide stack traces useful for developers but unhelpful for end users.

**Decision**: Implement custom `EngramError` type with structured error information and actionable suggestions.

**Error Structure**:
```go
type EngramError struct {
    Symbol      string   // Visual indicator (✗, !, etc.)
    Message     string   // User-facing message
    Cause       error    // Underlying error
    Suggestions []string // Actionable suggestions
}
```

**Rationale**:

1. **User Experience**: Non-technical users need guidance, not stack traces
2. **Consistency**: All commands use same error format
3. **Actionable**: Suggestions tell users what to do next
4. **Debuggable**: Underlying cause preserved for debugging
5. **Visual**: Icons (✗, !) provide quick visual feedback

**Error Factories**:
- `InvalidInputError(field, value, constraint)` - Invalid user input
- `ConfigNotFoundError(path, cause)` - Missing configuration
- `PluginLoadError(path, cause)` - Plugin loading failure
- Plus command-specific errors

**Alternatives Considered**:

1. **Standard errors**: Insufficient context for users
2. **Error codes**: Not user-friendly, requires lookup
3. **pkg/errors**: Stack traces unhelpful for CLI users
4. **Structured logging**: Logs != user-facing errors

**Consequences**:

**Positive**:
- Improved user experience with clear guidance
- Reduced support burden (users can self-help)
- Consistent error formatting
- Easy to add suggestions for new error cases

**Negative**:
- Additional code for each error type (mitigated by factories)
- Requires discipline to use consistently

**Implementation Guidelines**:

1. **Use factories**: Always use error factories, never construct EngramError directly
2. **Be specific**: Tailor suggestions to the exact error
3. **Be actionable**: Suggestions should be concrete steps
4. **Be concise**: 1-3 suggestions max, clear and brief

**Examples**:

```go
// Good: Specific, actionable
InvalidInputError("limit", "150", "must be between 1 and 100")
// Suggestions:
// - Use a value between 1 and 100
// - Example: engram retrieve --limit 10

// Bad: Vague, not actionable
fmt.Errorf("invalid limit")
```

**Related Decisions**:
- ADR-001: Cobra CLI Framework
- ADR-003: Output Formatting Standards
- ADR-006: Input Validation Strategy
