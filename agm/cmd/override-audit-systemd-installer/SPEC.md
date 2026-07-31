# Override-audit systemd installer specification

## EARS requirements

**SDA-CMD-01** When the privileged systemd installer receives an invalid argument count, root group ID, or installer configuration, the system shall exit with command-usage status before starting the installation transaction.

**SDA-CMD-02** When the installer receives HUP, INT, or TERM, the system shall cancel the shared systemd installation transaction, wait for rollback, and return the corresponding signal exit status.

**SDA-CMD-03** When the shared installation transaction fails without a handled signal, the system shall report the failure and return a nonzero status.

**SDA-CMD-04** When the shared installation transaction completes, the system shall return success only after the complete executable, service, and timer artifact set is active.

## BDD traceability

- Feature: `agm/test/bdd/features/dangerous_override_governance.feature`

## Test traceability

- Unit package: `agm/cmd/override-audit-systemd-installer`
- Transaction package: `agm/internal/systemdaudit`
