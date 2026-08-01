# Build Version Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**VERSION-01** When version text is requested, the system shall combine semantic version, commit, and build date in the stable one-line format.

**VERSION-02** When linker values are absent and Go build metadata is present, the system shall populate revision, build time, dirty state, and builder identity.

**VERSION-03** When linker values already identify a commit, the system shall not overwrite them from build metadata.

**VERSION-04** When a short commit is requested, the system shall remove only the trailing dirty marker.

**VERSION-05** When staleness cannot be determined because commit or repository evidence is unavailable, the system shall fail open without a false stale result.

**VERSION-06** When the built commit is not an ancestor of origin main, the system shall report the binary as stale.

**VERSION-07** While binaries serve any supported harness and model family, the system shall expose identical version and staleness semantics.

**VERSION-08** When exact companion-process compatibility is compared, the system shall shorten the commit hash while preserving any dirty-source marker so clean and modified builds cannot be treated as identical.

**VERSION-09** When a governed Make build receives ordinary effective `GOFLAGS`, the system shall preserve those Go settings while injecting Version, GitCommit, BuildDate, and BuiltBy provenance.

**VERSION-10** When effective `GOFLAGS` contains `-ldflags` or `--ldflags`, the governed Make build shall fail before invoking Go and direct linker customization to `EXTRA_GO_LDFLAGS`.

**VERSION-11** When `EXTRA_GO_LDFLAGS` supplies an unpatterned linker argument list beginning with a hyphen, the governed Make build shall treat caller text as opaque data and compose one linker value with protected provenance assignments last; when it supplies Go's package-pattern form, the build shall reject it before invoking Go.

**VERSION-12** When caller provenance contains a Go linker whitespace separator or quote delimiter, the governed Make build shall reject it before invoking Go so metadata cannot reshape or inject linker tokens.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`
- Feature: `agm/test/bdd/features/hook_parity.feature`

## Test Traceability

- Unit package: `pkg/version`
- Integration package: `tests/buildstamp`
