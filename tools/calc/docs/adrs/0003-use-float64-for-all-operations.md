# ADR 0003: Use float64 for All Operations

**Date**: 2024 (inferred from codebase)

**Status**: Accepted

## Context

The calculator needs to determine what numeric types to support for operands and results. Options include:

1. **Integer-only**: `int` or `int64`
2. **Float-only**: `float64`
3. **Separate modes**: Detect type from input (integer vs decimal)
4. **Arbitrary precision**: Use `big.Int` and `big.Float`
5. **Mixed types**: Accept both, return appropriate type

## Decision

We will use `float64` for all operands and results, regardless of whether inputs contain decimals.

## Rationale

### Why float64

1. **Universality**: Handles both integer and decimal inputs
   - `calc --add 5 3` → 8.0
   - `calc --add 5.5 3.2` → 8.7

2. **Division Compatibility**: Division naturally produces decimals
   - `calc --divide 10 3` → 3.333... (not 3)
   - Avoids integer division confusion

3. **Consistency**: Single numeric type throughout codebase
   - Simpler function signatures
   - No type conversion logic needed
   - Predictable output format

4. **Sufficient Precision**: float64 provides:
   - ~15-17 decimal digits of precision
   - Range: ±1.7 × 10^308
   - More than adequate for typical calculator use

5. **Go Standard**: float64 is Go's standard floating-point type
   - Used by `math` package
   - Good performance on modern CPUs

### Why Not Other Options

**Integer-only**:
- Cannot represent division results accurately
- Forces truncation (5/2 = 2 instead of 2.5)
- User confusion on decimal inputs

**Separate modes**:
- Complex: requires type detection, conversion, and mode switching
- Unclear behavior: what if one input is int, other is float?
- Over-engineering for simple calculator

**Arbitrary precision** (`big.Float`):
- Unnecessary for basic arithmetic
- Slower performance
- More complex API
- Overkill for this tool's scope

**Mixed types**:
- Complex type system
- Inconsistent output formats
- Harder to test and maintain

## Consequences

### Positive

- Simple, uniform implementation
- No surprises in division behavior
- Handles all realistic calculator inputs
- Floating-point literals parsed with `strconv.ParseFloat()`

### Negative

- Floating-point representation limitations:
  - `0.1 + 0.2` may not exactly equal `0.3` (IEEE 754 behavior)
  - Very large integers may lose precision beyond 2^53
- Integer results displayed with decimal point: `8.0` instead of `8`

### Neutral

- Consistent with most calculators and programming REPLs
- Users familiar with calculators expect float behavior

## Mitigation of Limitations

For the scope of this tool (basic arithmetic), float64 precision is sufficient. If precision becomes an issue:

1. **Document behavior** in README
2. **Use decimal library** (future enhancement if needed)
3. **Recommend alternative tools** for exact arithmetic (e.g., `bc`, `python -m decimal`)

## Implementation

All operation functions use `float64`:

```go
func add(a, b float64) float64 { return a + b }
func subtract(a, b float64) float64 { return a - b }
func multiply(a, b float64) float64 { return a * b }
func divide(a, b float64) (float64, error) { ... }
```

Parsing:
```go
a, _ := strconv.ParseFloat(args[0], 64)
b, _ := strconv.ParseFloat(args[1], 64)
```

Output:
```go
fmt.Println(result)  // Go's default float64 formatting
```

## Trade-offs Accepted

We accept standard floating-point limitations in exchange for:
- Simplicity
- Consistency
- Division correctness
- Universal input handling

For users needing exact decimal arithmetic, we recommend specialized tools. This calculator prioritizes simplicity and typical use cases.
