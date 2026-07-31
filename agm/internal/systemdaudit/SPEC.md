# Systemd audit installer contract

**SDA-01** When the Linux override-audit installer is invoked through the approved privileged bootstrap, the system shall require root-owned non-writable non-symlink destination ancestry, stage and hash all three artifacts, back up the complete live set, atomically activate the staged files, reload systemd, and restore the prior set on any failure or cancellation before completion.

**SDA-02** When the final executable destination directory already exists, the system shall reject a symlink or non-directory before changing its mode or ownership.

## BDD traceability

- Feature: `agm/test/bdd/features/dangerous_override_governance.feature`

## Test traceability

- Unit package: `agm/internal/systemdaudit`
- Command package: `agm/cmd/override-audit-systemd-installer`
