# AGM Event Bus TUI Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/tui` adapts AGM event-bus subscriptions and connection state into
Bubble Tea commands and messages.

## Requirements

**TUI-01** When an event-bus client connects, the system shall subscribe to configured events and deliver decoded records through a bounded channel.

**TUI-02** When the broker connection drops, the system shall reconnect and restore subscriptions without duplicating the active client state.

**TUI-03** When a received record is invalid, the system shall report or skip it without corrupting subsequent event delivery.

**TUI-04** When WebSocket delivery is unavailable, the system shall support the maintained HTTP fallback behavior.

**TUI-05** When Bubble Tea requests the next event or connection check, the system shall return typed messages without blocking the update loop.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/internal/tui/*_test.go`
