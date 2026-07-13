# AGM Installer Image Fixture Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**FIX-INSTALL-IMAGE-01** When installer end-to-end tests build distribution images, the system shall use the declared Debian and Ubuntu Dockerfiles.

**FIX-INSTALL-IMAGE-02** If an installation cannot complete in a declared image, the system shall fail the distribution scenario.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_fixture_guardrails.feature`
