# ADR 0002: Flag-Based Operation Selection

**Date**: 2024 (inferred from codebase)

**Status**: Accepted

## Context

The calculator needs a way for users to specify which arithmetic operation to perform. Several interface patterns were considered:

1. **Flag-based**: `calc --add 5 3`
2. **Positional operator**: `calc + 5 3` or `calc 5 + 3`
3. **Infix expression**: `calc "5 + 3"`
4. **Subcommands**: `calc add 5 3`
5. **Interactive mode**: `calc` then `> 5 + 3`

## Decision

We will use flag-based operation selection (`--add`, `--subtract`, `--multiply`, `--divide`).

## Rationale

### Why Flag-Based

1. **Standard Library Support**: Go's `flag` package handles this pattern natively
2. **Explicit and Clear**: No ambiguity about which operation is being requested
3. **Self-Documenting**: `--help` automatically lists all available operations
4. **No Parsing Complexity**: No need to implement expression parser
5. **Error Handling**: Flag package provides built-in validation and error messages
6. **Consistency**: Follows Unix/POSIX CLI conventions

### Why Not Other Options

**Positional operator** (`calc + 5 3`):
- Requires manual parsing of operator symbol
- Shell escaping issues with `*` (multiply) - would need quoting
- Less obvious to new users

**Infix expression** (`calc "5 + 3"`):
- Requires expression parser implementation
- Significantly more complex (operator precedence, parentheses, etc.)
- Overkill for two-operand calculator
- Would expand scope beyond "simple calculator"

**Subcommands** (`calc add 5 3`):
- More verbose than flags
- Requires subcommand framework (cobra, etc.) or manual implementation
- Less idiomatic for simple operations

**Interactive mode**:
- Not suitable for scripting or piping
- Requires REPL implementation
- Slower for single calculations

## Consequences

### Positive

- Simple implementation (~40 lines for flag setup and dispatch)
- Built-in `--help` generation
- Clear error messages from flag package
- Easy to extend (add new flag for new operation)
- Scriptable and pipeable

### Negative

- Slightly more verbose than operator symbols
- Users must remember flag names (mitigated by `--help`)
- Only one operation per invocation (intentional simplicity)

### Neutral

- Enforces exactly one operation (mutual exclusivity validated in code)
- Follows flag pattern used by tools like `grep`, `ls`, etc.

## Implementation Details

```go
addFlag := flag.Bool("add", false, "Add two numbers")
subtractFlag := flag.Bool("subtract", false, "Subtract two numbers")
multiplyFlag := flag.Bool("multiply", false, "Multiply two numbers")
divideFlag := flag.Bool("divide", false, "Divide two numbers")
flag.Parse()
```

Validation ensures exactly one flag is set:
```go
if opCount == 0 {
    return fmt.Errorf("Must specify exactly one operation...")
}
if opCount > 1 {
    return fmt.Errorf("Cannot specify multiple operations")
}
```

## Related Decisions

- See ADR-0001 for language choice (Go's flag package influenced this decision)
- Future: If expression evaluation is needed, create separate tool (e.g., `eval` or `expr`)
