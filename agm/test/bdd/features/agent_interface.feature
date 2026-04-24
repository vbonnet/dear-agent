Feature: Agent Interface Implementation
  All agent adapters must implement the Agent interface correctly

  Scenario Outline: Adapter implements required methods
    Given I have a mock <agent> adapter configured
    Then the adapter should have method CreateSession
    And the adapter should have method SendMessage
    And the adapter should have method GetHistory
    And the adapter should have method PauseSession
    And the adapter should have method ResumeSession
    And the adapter should have method ArchiveSession
    And the adapter should have method GetSession
    And the adapter should return name "<agent>"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |

  Scenario Outline: Session state transitions correctly
    Given I have a mock <agent> adapter configured
    When I create a session "state-test" with agent "<agent>"
    Then the session state should be "active"
    When I pause the session "state-test"
    Then the session state should be "paused"
    When I resume the session "state-test"
    Then the session state should be "active"
    When I archive the session "state-test"
    Then the session state should be "archived"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |
