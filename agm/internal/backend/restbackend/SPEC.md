# REST Process Backend Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/backend/restbackend`.

## Overview

`restbackend` implements the legacy `backend.Backend` contract using managed
subprocesses and a JSON REST API instead of tmux. It keeps process-backed
sessions in memory, streams subprocess output through bounded buffers, and
exposes session lifecycle, message delivery, and output retrieval through
HTTP handlers with JSON responses.

## EARS Requirements

**RESTBACKEND-01** When the backend is constructed without a Claude binary path, the system shall default to `claude`.

**RESTBACKEND-02** When a session exists and its process is alive, the system shall report that the session exists.

**RESTBACKEND-03** When listing sessions, the system shall include only sessions whose managed processes are alive.

**RESTBACKEND-04** When creating a session with a duplicate live name, the system shall reject the request.

**RESTBACKEND-05** When creating a session succeeds, the system shall store the managed process by session name.

**RESTBACKEND-06** When terminal attachment is requested, the system shall return an unsupported-operation error.

**RESTBACKEND-07** When sending input to an unknown or stopped session, the system shall return an error instead of writing to a process pipe.

**RESTBACKEND-08** When reading output for a known session, the system shall return the requested number of recent buffered output lines.

**RESTBACKEND-09** When terminating a session, the system shall remove the session from the in-memory registry before stopping the process.

**RESTBACKEND-10** When the REST API receives an invalid create-session body or missing name, the system shall return HTTP 400 with a JSON error.

**RESTBACKEND-11** When the REST API cannot create a session because the backend rejects it, the system shall return HTTP 409 with a JSON error.

**RESTBACKEND-12** When the REST API receives an unknown session route, the system shall return HTTP 404 with a JSON error.

**RESTBACKEND-13** When REST output line count is absent or invalid, the system shall use the default output line count.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/internal/backend/restbackend`
