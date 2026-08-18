# Override-audit LaunchDaemon installer specification

## EARS requirements

**LDA-CMD-01** When the privileged LaunchDaemon installer receives an invalid argument count, root group ID, or installer configuration, the system shall exit with command-usage status before starting the installation transaction.

**LDA-CMD-02** When the installer receives HUP, INT, or TERM, the system shall cancel the shared LaunchDaemon installation transaction, wait for rollback, and return the corresponding signal exit status.

**LDA-CMD-03** When the shared installation transaction fails without a handled signal, the system shall report the failure and return a nonzero status.

**LDA-CMD-04** When the shared installation transaction completes, the system shall return success only after the complete executable and LaunchDaemon artifact set is active.

## BDD traceability

- Feature: `agm/test/bdd/features/dangerous_override_governance.feature`

## Test traceability

- Unit package: `agm/cmd/override-audit-launchdaemon-installer`
- Transaction package: `agm/internal/launchdaudit`
