# AGM Backend Adapter Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/backend` defines the legacy backend interface used to abstract
session operations across tmux and future backends. It also adapts that backend
contract into `agm/internal/session.TmuxInterface` so older session code can
move toward backend selection without losing attachment and client metadata.

## EARS Requirements

**BEND-01** When a backend implementation is used, the system shall expose session existence, listing, creation, attachment, client listing, and key delivery through one backend interface.

**BEND-02** When backend session information is returned through the adapter, the system shall preserve session name, attached-client count, and attached-client list.

**BEND-03** When backend client information is returned through the adapter, the system shall preserve session name, TTY, and process ID.

**BEND-04** When the default backend adapter is requested, the system shall select the configured backend and return an adapter compatible with `session.TmuxInterface`.

**BEND-05** When a wrapped backend operation fails, the system shall return the backend error instead of hiding or translating the failure into a success.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
