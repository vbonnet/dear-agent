# Docker Manager Backend Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/manager/dockerbackend`.

## Overview

`dockerbackend` implements `manager.Backend` with one container per AGM session.
It labels containers for discovery, keeps an in-memory session-to-container
mapping, exposes structured I/O through `docker exec`, and treats termination as
idempotent cleanup. The package is testable through the `ContainerClient`
abstraction and registers a Docker backend implementation with the manager
registry.

## EARS Requirements

**DOCKERBACKEND-01** When the Docker backend reports its name, the system shall return `docker`.

**DOCKERBACKEND-02** When the Docker backend reports capabilities, the system shall advertise structured I/O and interrupt support without attach support.

**DOCKERBACKEND-03** When creating a session without a name, the system shall reject the request.

**DOCKERBACKEND-04** When creating a session succeeds, the system shall create a managed container with session and harness labels.

**DOCKERBACKEND-05** When a working directory is configured, the system shall mount it into the container workspace.

**DOCKERBACKEND-06** When container start fails after creation, the system shall remove the created container before returning an error.

**DOCKERBACKEND-07** When terminating an unknown session, the system shall return success without contacting Docker.

**DOCKERBACKEND-08** When terminating a known session, the system shall stop and remove the mapped container.

**DOCKERBACKEND-09** When listing sessions, the system shall apply name and limit filters to the in-memory session registry.

**DOCKERBACKEND-10** When sending a message, the system shall execute a container command with the message on stdin.

**DOCKERBACKEND-11** When reading output with a non-positive line count, the system shall default to thirty lines.

**DOCKERBACKEND-12** When interrupting a session, the system shall execute an interrupt command in the mapped container.

**DOCKERBACKEND-13** When inspecting a missing session state, the system shall return offline state with full confidence.

**DOCKERBACKEND-14** When container inspection fails, the system shall return error state evidence without surfacing a hard error.

**DOCKERBACKEND-15** When delivery readiness is checked for a running container, the system shall return ready-to-receive status.

**DOCKERBACKEND-16** When checking backend health, the system shall require Docker list access for managed containers.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/internal/manager/dockerbackend`
