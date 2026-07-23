# AGM Integration Tests

The integration-tagged graph exercises current production boundaries without
using the user's AGM state or hiding packages behind a global skip.

## Test boundaries

- The root package covers component interactions that own their filesystem,
  database, process, or service fixtures and scopes unavailable prerequisites
  to the affected test.
- `portable/` verifies the shared production adapter contract for every active
  harness without credentials, installed harnesses, or remote services.
- `isolated/` builds AGM from the checkout and exercises the real Codex CLI
  lifecycle through a fake `codex` executable, private HOME and SQLite state,
  a unique tmux socket, and exact resource cleanup.
- `helpers/` owns the isolated runtime builder used by real lifecycle tests.

Retired Ginkgo scenarios that wrote legacy YAML manifests, invoked installed
AGM binaries, used the default tmux server, or asserted removed CLI flags are
not part of this graph. Equivalent current behavior is enforced in production
package tests, strict BDD features, portable adapter parity, and the isolated
source-built lifecycle.

## Running

From the repository root:

```bash
go test -race -count=1 -timeout=20m \
  -tags=integration ./agm/test/integration/...
```

Run only the credential-free active-harness contract:

```bash
go test -race -count=1 -tags=integration \
  ./agm/test/integration/portable \
  -run 'TestActiveHarnessParityContract|TestHarnessPrerequisitesAreScoped'
```

Run the source-built Codex lifecycle:

```bash
go test -race -count=1 -tags=integration \
  ./agm/test/integration/isolated \
  -run '^TestCodexLifecycleUsesIsolatedSourceEnvironment$'
```

The isolated lifecycle skips only when process-table or tmux access is missing
or explicitly denied. Other setup failures fail the test.

## CI

Every pull request runs portable parity, the isolated Codex lifecycle, and the
critical lifecycle coverage ratchet. Scheduled and manually dispatched CI also
runs the complete contract- and integration-tagged graphs.
