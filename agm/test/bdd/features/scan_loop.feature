@implemented
Feature: Scan Loop
  As an AGM orchestrator
  I want the scan loop to detect anomalies across sessions
  So that I can maintain situational awareness of the multi-agent system

  Scenario: Auto-approve only matches RBAC allowlist
    Given a cross-check configuration with default RBAC allowlist
    Then the allowlist should contain "Read"
    And the allowlist should contain "Glob"
    And the allowlist should contain "Grep"
    And the allowlist should not contain "rm"
    And the allowlist should not contain "sudo"

  Scenario: Well-known sessions excluded from unmanaged list
    Given well-known tmux session names
    Then "main" should be excluded from unmanaged checks
    And "default" should be excluded from unmanaged checks

  Scenario: Health status escalation follows severity ordering
    Given a scan with no alerts
    Then the health status should be "healthy"
    When a warning-level alert is added
    Then the health status should escalate to "warning"
    When a critical-level alert is added
    Then the health status should escalate to "critical"

  Scenario: Scan loop uses SLO contract thresholds
    Given the default SLO contracts
    Then the default scan interval should be "5m"
    And the stuck timeout should be "5m"
    And the scan gap timeout should be "10m"
    And the worker commit lookback should be "24h"
    And the metrics window should be "1h"
    And the tmux capture depth should be 30
    And the session list limit should be 1000
