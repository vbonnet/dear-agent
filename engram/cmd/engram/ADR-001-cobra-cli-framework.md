# ADR-001: Use Cobra for CLI Framework

**Status**: Accepted

**Date**: 2024-01-15

**Context**: The Engram CLI requires a robust command-line interface framework to handle multiple commands, subcommands, flags, and help text. The framework should provide:
- Command hierarchy and organization
- Flag parsing and validation
- Auto-generated help text
- Shell completion generation
- Extensibility for adding new commands

**Decision**: Use `github.com/spf13/cobra` as the CLI framework.

**Rationale**:

1. **Industry Standard**: Cobra is used by major projects (kubectl, Hugo, GitHub CLI)
2. **Rich Features**:
   - Nested subcommand support
   - Persistent and local flags
   - Auto-generated help and usage
   - Shell completion for bash, zsh, fish, powershell
3. **Active Maintenance**: Well-maintained with strong community support
4. **Go Integration**: Idiomatic Go API, easy to integrate
5. **Documentation**: Extensive documentation and examples

**Alternatives Considered**:

1. **urfave/cli**: Simpler but less flexible for complex command hierarchies
2. **flag package**: Go standard library, too basic for our needs
3. **Custom parser**: High maintenance burden, reinventing the wheel

**Consequences**:

**Positive**:
- Consistent command structure across all operations
- Auto-generated help reduces documentation burden
- Shell completion improves UX
- Easy to add new commands and subcommands

**Negative**:
- External dependency (mitigated by stability)
- Learning curve for contributors (mitigated by ubiquity)
- Opinionated structure (generally a positive)

**Implementation Notes**:
- Root command in `cmd/root.go`
- Each command in separate file: `cmd/{command}.go`
- Subcommands as nested cobra.Command structs
- Global flags on root command
- Command-specific flags on each command

**Related Decisions**:
- ADR-002: Error Handling Strategy
- ADR-003: Output Formatting Standards
