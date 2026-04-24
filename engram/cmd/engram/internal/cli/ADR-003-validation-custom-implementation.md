# ADR-003: Input Validation Custom Implementation

**Status**: Accepted
**Date**: 2026-03-20
**Phase**: Language Audit Phase 6 - Go Library Modernization
**Task**: 6.1 - Input Validation Standardization Evaluation
**Decision Maker**: Language Audit Team

---

## Context

The engram CLI has custom input validation functions in `core/cmd/engram/internal/cli/validation.go` (~278 lines). Phase 6 aims to evaluate replacing custom implementations with industry-standard libraries where beneficial.

**Current Implementation**: Security-focused validation library with:
- Path traversal prevention (`ValidateNoTraversal`, `ValidateSafePath`)
- Command injection prevention (`ValidateNoShellMetacharacters`)
- DoS protection (`ValidateMaxLength`, `ValidateNamespaceComponents`)
- Standard validation (`ValidateEnum`, `ValidateRange`, `ValidatePathExists`)
- Comprehensive test coverage (20+ tests, security scenarios)

**Evaluation Trigger**: Language Audit Phase 6 task to evaluate migrating to `go-playground/validator` struct tag-based validation.

**Key Finding**: Custom validation functions are **not currently used** in command implementations (zero imports found), suggesting this is infrastructure ready for future use rather than actively deployed code.

---

## Decision

**We will KEEP the custom validation implementation** rather than migrate to `go-playground/validator`. The code will be enhanced with usage examples and documentation to encourage adoption in new commands.

---

## Alternatives Considered

### 1. go-playground/validator (14,000+ ⭐, Industry Standard)

**Pros**:
- **Widely adopted**: 14,000+ stars, used by Gin, Echo, many frameworks
- **Declarative validation**: Struct tags reduce boilerplate
- **Extensive built-ins**: 100+ validators (email, URL, UUID, etc.)
- **Custom validators**: Can register custom validation functions
- **Error messages**: Internationalization support (i18n)
- **Active maintenance**: Weekly releases, 3,000+ commits

**Cons**:
- **Security gap**: No built-in path traversal, command injection, or DoS prevention
- **Generic approach**: Validators are general-purpose, not security-focused
- **Struct-centric**: Designed for struct validation, not ad-hoc CLI flag validation
- **Migration effort**: 3-5 days to create custom security validators + migrate existing code
- **Loss of specificity**: Custom error messages (EngramError with suggestions) harder to preserve
- **Learning curve**: Struct tags + custom validator registration adds complexity

**Rejected**: While excellent for general validation, it lacks the security-specific focus of our custom implementation and would require significant work to replicate security features.

---

### 2. ozzo-validation (3,700+ ⭐, Programmatic Validation)

**Pros**:
- **Programmatic approach**: Validation rules in code, not struct tags
- **Composable**: Build complex validation rules from simple ones
- **No reflection overhead**: Direct function calls
- **Custom errors**: Easy to customize error messages

**Cons**:
- **Security gap**: Same as go-playground/validator (no security validators)
- **Less popular**: Smaller ecosystem than go-playground/validator
- **Migration effort**: Similar 3-5 day timeline
- **Still generic**: Not designed for CLI security scenarios

**Rejected**: Doesn't solve the security validator problem and has less community adoption.

---

### 3. asaskevich/govalidator (6,000+ ⭐, String Validation)

**Pros**:
- **String-focused**: Good for validating string inputs (CLI flags)
- **Built-in validators**: Email, URL, IP, credit card, etc.
- **Simple API**: Easy to use for basic validation

**Cons**:
- **Security gap**: No path traversal, command injection, or DoS validators
- **Limited scope**: Focused on format validation, not security
- **Less maintained**: Fewer recent updates than go-playground/validator

**Rejected**: Too limited for security-critical CLI validation.

---

## Rationale

### 1. Security-First Design

The custom validation library is purpose-built for CLI security:

```go
// Path traversal prevention (OWASP #1)
func ValidateNoTraversal(field string, path string) error {
    if strings.Contains(path, "..") {
        return &EngramError{...} // Custom error with security context
    }
    return nil
}

// Command injection prevention (OWASP #3)
func ValidateNoShellMetacharacters(field string, value string) error {
    dangerous := []string{";", "|", "&", "$", "`", "\n", "\x00"}
    for _, char := range dangerous {
        if strings.Contains(value, char) {
            return &EngramError{...} // Security-focused error message
        }
    }
    return nil
}

// DoS prevention (resource exhaustion)
func ValidateMaxLength(field string, value string, max int) error {
    if len(value) > max {
        return fmt.Errorf("%s exceeds maximum length %d (got %d)", field, max, len(value))
    }
    return nil
}
```

**Third-party alternatives** don't provide these out-of-the-box. We'd need to:
1. Create custom validators for path traversal (10-15 LOC)
2. Create custom validators for command injection (10-15 LOC)
3. Create custom validators for DoS prevention (10-15 LOC)
4. Test all custom validators (20+ test cases)
5. Document security validators for team
6. **Result**: ~50 LOC + testing + docs = similar effort to keeping custom code

---

### 2. CLI-Specific Error Messages

Custom `EngramError` type provides actionable suggestions:

```go
// Before (custom implementation)
return &EngramError{
    Symbol:  "✗",
    Message: "Path does not exist: /invalid/path",
    Cause:   nil,
    Suggestions: []string{
        "Check if path exists: ls -la /invalid",
        "Verify the path is correct",
    },
    RelatedCommands: []string{},
}

// After (go-playground/validator)
// Generic validation error: "Key: 'Config.Path' Error:Field validation for 'Path' failed on the 'file' tag"
// No suggestions, no CLI context, no actionable guidance
```

**CLI users need context**: Custom errors provide:
- Visual indicators (✗ symbol)
- Actionable suggestions (commands to run)
- Field-specific context (which flag failed)

**Third-party validators** provide generic errors that require additional code to customize.

---

### 3. Comprehensive Test Coverage

**Current tests** (`validation_test.go`, 20+ tests):
1. **Security tests**:
   - Path traversal attacks (14 test cases)
   - Command injection attempts (14 test cases)
   - DoS prevention (6 test cases)
   - Null byte attacks
   - Unicode handling

2. **Functional tests**:
   - Enum validation
   - Range validation
   - Path existence validation
   - Namespace validation

**Migration would require**:
- Porting all security tests to new validators
- Ensuring same security guarantees
- Risk of security regression during migration

---

### 4. Current Usage: Zero Adoption (Not a Blocker)

**Finding**: Custom validation functions are not currently used in command implementations.

```bash
# Search for imports of cli package
$ grep -r "internal/cli" core/cmd/engram/cmd/*.go
# Result: No matches

# Search for validation function calls
$ grep -r "cli.Validate" core/cmd/engram/cmd/*.go
# Result: No matches
```

**Interpretation**: This is **infrastructure ready for future use**, not actively deployed code.

**Recommendation**: Instead of removing or migrating, **enhance with usage examples**:
- Add godoc examples showing how to use each validator
- Create sample command demonstrating validation
- Update CONTRIBUTING.md encouraging validation usage in new commands
- Add linter rule to encourage validation (future work)

**This is NOT dead code**: It's well-tested security infrastructure waiting to be adopted.

---

### 5. ROI Analysis

**Migration costs**:
- Development: 3-5 days (create custom security validators + migrate)
- Risk: Security regression if migration misses edge cases
- Testing: Port 20+ security test cases, ensure same coverage
- Documentation: Update docs to use new library
- Learning curve: Team learns struct tag validation syntax
- Ongoing: Dependency tracking, updates, version management

**Migration benefits**:
- Struct tag validation (nice-to-have, but CLI uses ad-hoc validation)
- Community validators (email, URL - not needed for engram CLI)
- i18n error messages (not a requirement)
- **Net benefit**: Minimal for this specific use case

**Keep custom costs**:
- Maintenance: ~0 (simple code, comprehensive tests)
- Documentation: 1-2 hours (add godoc examples)
- Adoption effort: Update 5-10 commands to use validators (future work)

**Keep custom benefits**:
- Zero security regression risk
- Custom CLI-focused errors
- No external dependencies (zero supply chain risk)
- Team already familiar with code
- **Net benefit**: High security assurance, low maintenance

**Conclusion**: Poor ROI to migrate. Enhancement + adoption is better investment.

---

## Consequences

### Positive

✅ **Security-first**: Path traversal, command injection, DoS prevention built-in
✅ **Zero dependencies**: No external library, no supply chain risk
✅ **CLI-specific errors**: Custom EngramError with actionable suggestions
✅ **Comprehensive tests**: 20+ tests covering security scenarios
✅ **Simple codebase**: 278 lines, easy to audit and maintain
✅ **Ready for adoption**: Infrastructure available when commands need validation

### Negative

❌ **Not adopted yet**: Zero current usage (but not dead code)
❌ **Manual validation**: Each command must call validation functions (not automatic)
❌ **No struct tags**: Requires programmatic validation calls
❌ **Limited built-ins**: Doesn't have email/URL/UUID validators (not needed for engram)

### Neutral

⚪ **Enhancement opportunity**: Add godoc examples to encourage adoption
⚪ **Linter potential**: Could add linter rule to require validation in new commands
⚪ **Extensibility**: Easy to add new validators as needed (e.g., semver, date range)

---

## Implementation

**No migration required.** This ADR documents the decision to keep the current implementation.

**Enhancements to improve adoption**:

1. **Add godoc examples** (~30-60 minutes):
   ```go
   // Example usage in a command
   // func runMyCommand(cmd *cobra.Command, args []string) error {
   //     if err := cli.ValidatePathExists("file", myFilePath, false); err != nil {
   //         return err
   //     }
   //     if err := cli.ValidateEnum("format", format, []string{"json", "yaml"}); err != nil {
   //         return err
   //     }
   //     // ... rest of command logic
   // }
   ```

2. **Create sample command** (~1-2 hours):
   - Demonstrate all validator types
   - Show security-focused validation
   - Reference from CONTRIBUTING.md

3. **Document in CONTRIBUTING.md** (~30 minutes):
   - Security validation requirements
   - When to use each validator
   - Link to godoc examples

4. **Future**: Add linter rule (~1 day):
   - Detect commands accepting file paths without `ValidatePathExists`
   - Detect commands accepting strings without `ValidateNoShellMetacharacters`
   - Flag for review if validation missing

**Estimated enhancement effort**: 3-4 hours (vs 3-5 days migration)

---

## Future Review Triggers

Reconsider this decision if any of the following occur:

1. **Struct validation need**: Commands shift to struct-based config requiring struct tags
2. **i18n requirement**: Error messages need internationalization support
3. **Generic validators needed**: Email, URL, UUID validation becomes common requirement
4. **Team consensus**: Team strongly prefers third-party library for consistency
5. **Security gap found**: Custom validators miss critical security scenario (unlikely given tests)

**Current status**: None of these triggers apply as of 2026-03-20.

---

## References

**Current Implementation**:
- `core/cmd/engram/internal/cli/validation.go` - 278 lines of validation logic
- `core/cmd/engram/internal/cli/validation_test.go` - 20+ comprehensive tests
- `core/cmd/engram/internal/cli/security.go` - Additional security utilities
- `core/cmd/engram/internal/cli/security_test.go` - Security-focused tests

**Security Standards Addressed**:
- OWASP Top 10 #1: Broken Access Control (path traversal prevention)
- OWASP Top 10 #3: Injection (command injection prevention)
- OWASP Top 10 #4: Insecure Design (DoS prevention, secure defaults)

**Third-Party Libraries Evaluated**:
- go-playground/validator: https://github.com/go-playground/validator
- ozzo-validation: https://github.com/go-ozzo/ozzo-validation
- govalidator: https://github.com/asaskevich/govalidator

**Related ADRs**:
- ADR-001: Circuit Breaker Custom Implementation (keep custom)
- ADR-002: Table Formatting Enhancement (adopt lipgloss)
- (Future) ADR-004: Validation Linter Rules

---

**Approved By**: Language Audit Phase 6 Evaluation
**Review Date**: 2026-03-20
**Next Review**: When review triggers occur or adoption metrics collected (6 months)
