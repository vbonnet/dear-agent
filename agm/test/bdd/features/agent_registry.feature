Feature: Agent Registry
  The agent registry must correctly manage adapter instances

  Scenario: Registry returns correct adapter for each type
    Given I have AGM installed
    And I have a mock claude adapter configured
    And I have a mock gemini adapter configured
    And I have a mock codex adapter configured
    When I request adapter "claude" from environment
    Then the adapter name should be "claude"
    When I request adapter "gemini" from environment
    Then the adapter name should be "gemini"
    When I request adapter "codex" from environment
    Then the adapter name should be "codex"

  Scenario: Registry returns error for unknown adapter
    Given I have AGM installed
    When I request adapter "invalid" from environment
    Then an error should occur
    And the error message should contain "unknown adapter"

  Scenario Outline: Multiple sessions are isolated
    Given I have a mock <agent> adapter configured
    And a session "session-A" exists with agent "<agent>"
    And a session "session-B" exists with agent "<agent>"
    When I send message "Message for A" to session "session-A"
    And I send message "Message for B" to session "session-B"
    Then session "session-A" history should contain "Message for A"
    And session "session-B" history should contain "Message for B"
    And session "session-A" history should not contain "Message for B"
    And session "session-B" history should not contain "Message for A"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex    |
