# AGM Integration Helpers Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**IHELP-01** When a real AGM integration environment is created, the helper shall own a temporary HOME, state directory, SQLite database, manifest directory, short unique tmux socket, session prefix, working directory, and AGM binary built from the checkout under test.

**IHELP-02** When isolated integration cleanup runs, the helper shall target only exact registered session names and its owned tmux server and paths; it shall not scan or mutate the user's default tmux server.

**IHELP-03** When a production liveness probe distinguishes harness processes by executable name, the helper shall compile a test-owned executable with the required basename inside the isolated environment.

**IHELP-04** If tmux is missing or permission is denied before server creation, then the helper shall remove owned filesystem state without invoking tmux or converting an intended skip into failure.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Package tests: `agm/test/integration/helpers/*_test.go`
