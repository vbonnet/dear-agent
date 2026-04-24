Feature: Error Handling
  The system must handle errors gracefully

  Scenario Outline: Invalid session ID returns error
    Given I have a mock <agent> adapter configured
    When I try to send message "test" to session "nonexistent-session-id"
    Then an error should occur
    And the error message should contain "not found"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |

  Scenario Outline: Cannot resume archived session
    Given I have a mock <agent> adapter configured
    And a session "archived-test" exists with agent "<agent>"
    When I archive the session "archived-test"
    And I try to resume session "archived-test"
    Then an error should occur
    And the error message should contain "cannot resume archived"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |

  Scenario Outline: Cannot send to archived session
    Given I have a mock <agent> adapter configured
    And a session "archived-msg-test" exists with agent "<agent>"
    When I archive the session "archived-msg-test"
    And I try to send message "test" to session "archived-msg-test"
    Then an error should occur
    And the error message should contain "is archived"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |
