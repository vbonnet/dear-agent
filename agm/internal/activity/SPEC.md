# AGM Activity Tracking Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/activity` reads harness-specific history stores and exposes a
shared last-activity contract for AGM session health, stale-session detection,
and lifecycle reporting. Claude and Gemini use different history layouts, but
callers should receive the same timestamp/error semantics.

## EARS Requirements

**ACT-01** When Claude activity is requested for a session, the system shall read the centralized Claude history file and return the latest timestamp for the matching session ID.

**ACT-02** When Gemini activity is requested for a session, the system shall read that session's Gemini history file and return the latest message timestamp in UTC.

**ACT-03** When a requested history file is missing, unreadable, corrupted, or empty, the system shall return the corresponding typed activity error.

**ACT-04** When batch activity is requested, the system shall return timestamps only for sessions whose activity can be resolved.

**ACT-05** When a harness has a more efficient shared history format, the system shall avoid rereading the same history file once per session during batch lookup.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
