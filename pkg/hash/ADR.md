# Hash package decisions

Status: Accepted

## Context

Phase artifacts and file verification need one stable digest representation.
Callers frequently supply user-facing paths, including home-relative paths.

## Decisions

1. **One algorithm and format.** File hashes use SHA-256 and return
   `sha256:<lowercase hex>`. Algorithm agility is not exposed until a caller
   needs it.
2. **Streaming input.** Files are hashed through an `io.Reader` path rather
   than loaded into memory.
3. **Constrained path expansion.** The package accepts `~` and `~/...`,
   resolves ordinary paths to absolute paths, and rejects `~user` syntax.

Resource cleanup and package statelessness follow ordinary Go practice and are
not separate architecture decisions.

## Consequences

- Hashes compare consistently across callers.
- File size does not determine memory use.
- The format is intentionally incompatible with bare hexadecimal digests.

## Evidence

- `hash.go`
- `hash_test.go` and `property_test.go`
