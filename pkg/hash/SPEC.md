# Hash package requirements specification

<!-- Last audited at: 2026-07-18 -->

## Executable EARS requirements

**HASHR-01** When a file hash is requested, the package shall compute SHA-256 over the exact file contents and return `sha256:<lowercase hexadecimal digest>`.

**HASHR-02** When a relative or absolute non-tilde path is expanded, the package shall return its absolute path.

**HASHR-03** When `~` or `~/...` is expanded, the package shall replace the tilde prefix with the current user's home directory and clean the resulting path. Expansion is not a containment boundary; traversal segments may resolve outside the home directory.

**HASHR-04** If a path begins with unsupported `~user` syntax, then the package shall return an error.

**HASHR-05** If a path cannot be expanded, opened, or read, then the package shall return an error without a partial digest.

**HASHR-06** When identical file bytes are hashed repeatedly, the package shall return the same canonical digest.

**HASHR-07** When file bytes differ, the package shall derive the digest from the changed byte stream rather than file metadata.

## BDD traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

## Verification

```sh
go test -race ./pkg/hash -count=1
```

The normative implementation surface is `hash.go`; `hash_test.go` and
`property_test.go` provide executable evidence.
