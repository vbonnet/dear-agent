# SPEC: agm/internal/db/SPEC.md
Feature: AGM database persistence guardrails
  AGM database persistence should keep session, message, escalation, hierarchy,
  and search storage harness-neutral while exposing executable schema and FTS
  invariants.

  Scenario: Database schema exposes persistence and search objects
    Given an AGM in-memory database is open
    When AGM inspects the database schema
    Then the database should expose table "sessions"
    And the database should expose table "messages"
    And the database should expose table "escalations"
    And the database should expose table "sessions_fts"
    And the database should expose view "active_sessions"
    And the database should expose view "unresolved_escalations"

  Scenario: Session persistence preserves harness-neutral metadata
    Given an AGM in-memory database is open
    And an AGM session manifest with harness "codex-cli" and model "gpt-5-codex"
    When AGM stores and retrieves the session manifest
    Then the retrieved session should preserve harness-neutral metadata

  Scenario: FTS search returns harness-filtered results
    Given an AGM in-memory database is open
    And AGM has stored searchable sessions across harnesses
    When AGM searches sessions for "temporal" with harness "codex-cli"
    Then the search results should include only session "db-search-codex"
