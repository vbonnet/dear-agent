# calc CLI - Architecture

## Overview

calc is a minimalist CLI calculator implemented as a single Go package with clear separation of concerns between operation logic, validation, and CLI interface.

## System Architecture

### Component Diagram

```
┌─────────────────────────────────────┐
│         calc CLI                     │
│                                      │
│  ┌──────────────────────────────┐  │
│  │   main()                      │  │
│  │   - Flag parsing              │  │
│  │   - Orchestration             │  │
│  └──────────────────────────────┘  │
│           │                          │
│           ├──────────┬──────────┐   │
│           ▼          ▼          ▼   │
│  ┌─────────────┐  ┌──────────────┐ │
│  │  validate() │  │  Operations  │ │
│  │             │  │  - add()     │ │
│  │  - Op count │  │  - subtract()│ │
│  │  - Arg count│  │  - multiply()│ │
│  │  - Numeric  │  │  - divide()  │ │
│  └─────────────┘  └──────────────┘ │
│                                      │
└─────────────────────────────────────┘
```

## Components

### 1. Main Function

**File**: `main.go:74-115`

**Responsibilities**:
- Define and parse command-line flags
- Extract positional arguments
- Invoke validation
- Dispatch to appropriate operation function
- Handle errors and output formatting

**Dependencies**:
- `flag` package (standard library)
- `fmt` package (standard library)
- `os` package (standard library)
- `strconv` package (standard library)

### 2. Operation Functions

**File**: `main.go:10-29`

**Functions**:
```go
func add(a, b float64) float64
func subtract(a, b float64) float64
func multiply(a, b float64) float64
func divide(a, b float64) (float64, error)
```

**Characteristics**:
- Pure functions (no side effects)
- Simple arithmetic implementations
- Only `divide()` returns error (division by zero)

### 3. Validation Function

**File**: `main.go:33-70`

**Function**:
```go
func validate(add, subtract, multiply, divide bool, args []string) error
```

**Validation Rules**:
1. Exactly one operation flag must be set
2. Exactly two arguments must be provided
3. Both arguments must be valid float64 numbers

**Design Pattern**: Fail-fast validation with descriptive error messages

## Data Flow

```
User Input
    │
    ▼
Flag Parser (stdlib flag package)
    │
    ├──> Operation flags (bool)
    └──> Positional args ([]string)
    │
    ▼
validate()
    │
    ├──> ✗ Invalid → stderr + exit(1)
    └──> ✓ Valid
         │
         ▼
    strconv.ParseFloat() × 2
         │
         ▼
    Operation function
         │
         ├──> divide() error → stderr + exit(1)
         └──> result
              │
              ▼
         fmt.Println(result)
              │
              ▼
         stdout
```

## Build and Deployment

### Build Process

```bash
go build -o calc main.go
```

**Output**: Single static binary `calc`

### Dependencies

**Runtime**: None (standard library only)

**Build-time**:
- Go toolchain (tested with Go 1.x)

### Testing

**File**: `main_test.go`

**Test Coverage**:
- Operation tests (5 tests): TestAdd, TestSubtract, TestMultiply, TestDivide, TestDivideByZero
- Validation tests (5 tests): TestValidateNoOperation, TestValidateMultipleOperations, TestValidateWrongArgCount, TestValidateInvalidNumber, TestValidateSuccess

**Test Execution**:
```bash
go test -v
```

## Design Decisions

### Language Choice: Go

**Rationale**:
- Single static binary compilation
- Fast startup time
- Built-in flag parsing
- Strong standard library
- Easy cross-compilation

### Flag-Based Operations

**Alternative Considered**: Infix notation (e.g., `calc "5 + 3"`)

**Decision**: Use explicit operation flags (`--add`, `--subtract`, etc.)

**Rationale**:
- Simpler parsing (leverage stdlib `flag` package)
- No expression evaluation complexity
- Clear, unambiguous syntax
- Better error reporting

### Float64 for All Operations

**Rationale**:
- Handles both integers and decimals
- Consistent numeric type
- Sufficient precision for basic calculations
- Standard Go numeric type

### Single-File Implementation

**Rationale**:
- Tool is simple enough (~115 lines)
- No need for package separation
- Easy to understand and maintain
- Quick compilation

## Error Handling Strategy

1. **Input Validation Errors**: Caught early in `validate()` function
2. **Runtime Errors**: Only division by zero, checked in `divide()` function
3. **All Errors**: Printed to stderr with `fmt.Fprintf(os.Stderr, ...)`
4. **Exit Codes**: Non-zero (1) for all error cases

## Future Considerations

### Scalability

Current design is intentionally simple. If extending with more operations:
- Consider operation registry pattern
- Move to package-based structure
- Add operation interface

### Extensibility

To add new operations:
1. Add operation function
2. Add flag definition in `main()`
3. Add case to switch statement
4. Add validation check
5. Add unit tests

Not recommended for this tool's scope - keep it simple.
