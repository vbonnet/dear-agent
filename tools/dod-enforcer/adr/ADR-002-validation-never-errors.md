# ADR-002: Validation Returns Results, Not Errors

## Status

Accepted

## Context

The `Validate()` method runs DoD checks and must communicate outcomes to callers. In Go, there are two common patterns for reporting failures:

1. **Error Returns**: Return an error when validation fails
   ```go
   err := dod.Validate()
   if err != nil {
       log.Fatalf("Validation failed: %v", err)
   }
   ```

2. **Result Objects**: Return a result object with success/failure details
   ```go
   result, err := dod.Validate()
   if err != nil {
       log.Fatalf("System error: %v", err)
   }
   if !result.Success {
       log.Printf("Validation failed: %s", result.Error)
   }
   ```

We need to decide which pattern better represents validation outcomes.

## Decision

We will use **Result Objects with No Errors** pattern.

The `Validate()` method signature:
```go
func (d *BeadDoD) Validate() (*ValidationResult, error)
```

**Behavior**:
- Always returns `(ValidationResult, nil)` for normal execution
- `ValidationResult.Success` indicates pass/fail
- Only returns error for catastrophic system failures (file system errors, internal bugs)
- In practice, error is always nil for current implementation

**Rationale**:
- **Validation failures are expected outcomes, not exceptional errors**
- DoD checks failing is a normal part of development workflow
- Errors in Go should represent unexpected, exceptional conditions

## Consequences

### Positive

1. **Semantic Clarity**: Validation failures vs system errors are distinct
   - `Success: false` = Expected outcome (tests failed, files missing)
   - `error != nil` = Unexpected outcome (file system crash, panic)

2. **Detailed Feedback**: Result object provides rich information
   - Overall success status
   - Individual check results
   - Error messages for each failure
   - Validation duration

3. **Simpler Caller Logic**: One check instead of two
   ```go
   result, _ := dod.Validate()
   if !result.Success {
       // Handle validation failure
   }
   ```

4. **Better for Automation**: Structured results are machine-parseable
   - CI/CD systems can extract specific check failures
   - Dashboards can visualize validation trends
   - No need to parse error strings

5. **Extensibility**: Easy to add new result fields
   - Additional metrics (coverage, performance)
   - Warnings (non-fatal issues)
   - Suggestions (how to fix failures)

### Negative

1. **Less Idiomatic Go**: Go convention favors error returns
   - Many Go APIs return errors for failures
   - Tools like errcheck flag ignored errors
   - May confuse developers expecting error-based APIs

2. **Two Checks Required**: Caller must check both error and success
   ```go
   result, err := dod.Validate()
   if err != nil {
       return fmt.Errorf("system error: %w", err)
   }
   if !result.Success {
       return fmt.Errorf("validation failed: %s", result.Error)
   }
   ```

3. **No Error Wrapping**: Cannot use `%w` verb for validation failures
   - Error chains work only for exceptional errors
   - Validation failures in error messages require manual formatting

4. **Nil Result Risk**: If error is returned, result may be nil
   - Caller must check error before accessing result fields
   - Risk of nil pointer dereference if not careful

## Alternatives Considered

### 1. Error-Only Pattern

**Description**: Return error on validation failure, nil on success

```go
func (d *BeadDoD) Validate() error
```

**Usage**:
```go
if err := dod.Validate(); err != nil {
    log.Fatalf("Validation failed: %v", err)
}
```

**Rejected because**:
- Loses detailed check information (which checks failed?)
- Error messages are strings (hard to parse programmatically)
- No easy way to report validation duration
- Cannot distinguish validation failures from system errors

### 2. Result with Errors

**Description**: Return error for validation failures

```go
func (d *BeadDoD) Validate() (*ValidationResult, error)
```

**Behavior**: `error != nil` when validation fails

**Usage**:
```go
result, err := dod.Validate()
if err != nil {
    log.Fatalf("Validation failed: %v", err)
}
```

**Rejected because**:
- Confuses validation failures (expected) with system errors (unexpected)
- Forces callers to treat normal workflow as error case
- Error should be exceptional, not routine

### 3. Multiple Return Values

**Description**: Return (passed bool, details string, error)

```go
func (d *BeadDoD) Validate() (bool, string, error)
```

**Rejected because**:
- Too many return values
- Details as string (not structured)
- Less extensible than result object

### 4. Callback Pattern

**Description**: Accept callback for each check result

```go
func (d *BeadDoD) Validate(onCheck func(CheckResult)) error
```

**Rejected because**:
- More complex API
- Harder to aggregate results
- Less flexible for callers

### 5. Panic on Failure

**Description**: Panic when validation fails

```go
func (d *BeadDoD) Validate() {
    // Panics on failure
}
```

**Rejected because**:
- Not idiomatic Go (panics for bugs, not validation)
- Harder to handle gracefully
- Poor for CI/CD integration

## Implementation Notes

**Current Signature**:
```go
func (d *BeadDoD) Validate() (*ValidationResult, error)
```

**Current Behavior**:
- Always returns `(result, nil)`
- Error parameter reserved for future system errors
- All validation failures captured in `ValidationResult`

**Caller Pattern**:
```go
result, err := dod.Validate()
if err != nil {
    // This should never happen in current implementation
    return fmt.Errorf("system error during validation: %w", err)
}

if !result.Success {
    log.Printf("DoD validation failed: %s", result.Error)
    for _, check := range result.Checks {
        if !check.Success {
            log.Printf("  [%s] %s: %s", check.Type, check.Name, check.Error)
        }
    }
    os.Exit(1)
}
```

**Future Error Cases**:
- File system errors during check execution
- Internal panics recovered and wrapped as errors
- Resource exhaustion (e.g., cannot spawn goroutines)

## Design Philosophy

**Errors Are Exceptional**: In Go, errors should represent unexpected conditions that prevent normal operation.

**Validation Failures Are Expected**: A DoD check failing is not exceptional—it's a normal part of the development process.

**Structured Results Over Strings**: Rich result objects enable better automation and debugging than error strings.

**Explicit Over Implicit**: Caller explicitly checks `Success` field, making intent clear.

## References

- [Go Blog: Error Handling](https://go.dev/blog/error-handling-and-go)
- [Go Proverbs: Errors are values](https://go-proverbs.github.io/)
- [Effective Go: Errors](https://go.dev/doc/effective_go#errors)
- [Dave Cheney: Don't just check errors, handle them gracefully](https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully)
