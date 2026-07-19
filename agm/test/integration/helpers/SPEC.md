# AGM Integration Helpers Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**IHELP-01** When integration helpers create session fixtures, the helpers shall isolate manifests, archives, tmux state, and working directories under the test environment.

**IHELP-02** When tmux helpers execute commands, the helpers shall consistently target the configured test socket and avoid the user's default server.

**IHELP-03** When command-order or literal-send behavior is asserted, the helpers shall report the missing safety invariant with the observed command stream.

**IHELP-04** When integration cleanup runs, the helpers shall tolerate already-removed resources without masking other failures.

**IHELP-05** When a real AGM integration environment is created, the helper shall own a temporary HOME, state directory, SQLite database, manifest directory, short unique tmux socket, session prefix, working directory, and AGM binary built from the checkout under test.

**IHELP-06** When isolated integration cleanup runs, the helper shall target only exact registered session names and its owned tmux server and paths; it shall not scan or mutate the user's default tmux server.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/test/integration/helpers/*_test.go`
