# calc CLI - Specification

## Overview

**calc** is a simple command-line calculator tool that performs basic arithmetic operations on two numeric operands.

## Problem Statement

Users need a lightweight, command-line tool for performing basic arithmetic calculations without opening a full calculator application or interactive REPL.

## Solution

A Go-based CLI tool that accepts two numeric arguments and performs one of four arithmetic operations: addition, subtraction, multiplication, or division.

## Requirements

### Functional Requirements

1. **Operations**: Support four basic arithmetic operations
   - Addition (`--add`)
   - Subtraction (`--subtract`)
   - Multiplication (`--multiply`)
   - Division (`--divide`)

2. **Input Validation**:
   - Accept exactly two numeric arguments
   - Require exactly one operation flag
   - Validate that arguments are parseable as float64
   - Prevent division by zero

3. **Output**:
   - Print the result to stdout
   - Print errors to stderr with exit code 1

### Non-Functional Requirements

1. **Simplicity**: Single-file implementation with no external dependencies
2. **Performance**: Immediate execution (no noticeable latency)
3. **Reliability**: Clear error messages for invalid inputs
4. **Portability**: Compile to a static binary for easy distribution

## Usage

```bash
# Addition
calc --add 5 3
# Output: 8

# Subtraction
calc --subtract 10 3
# Output: 7

# Multiplication
calc --multiply 4 5
# Output: 20

# Division
calc --divide 10 2
# Output: 5
```

## Error Handling

1. **No operation specified**: Exit with error message
2. **Multiple operations specified**: Exit with error message
3. **Wrong number of arguments**: Exit with error message "Expected 2 numeric arguments, got N"
4. **Non-numeric arguments**: Exit with error message "Arguments must be valid numbers"
5. **Division by zero**: Exit with error message "Cannot divide by zero"

## Out of Scope

- Interactive mode
- Command chaining or expression evaluation
- Advanced mathematical functions (trigonometry, logarithms, etc.)
- Arbitrary precision arithmetic
- Multiple operands (limited to two)
- Operation history or memory functions

## Success Criteria

1. All unit tests pass (10 test cases covering operations and validation)
2. Binary compiles successfully with `go build`
3. Tool handles all error cases gracefully
4. Output is accurate for floating-point arithmetic within Go's float64 precision
