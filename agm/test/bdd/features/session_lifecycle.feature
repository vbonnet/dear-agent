Feature: Session Lifecycle
  As an AGM user
  I want to manage session lifecycle (create, resume, archive)
  So that I can organize my work across multiple agents

  Scenario Outline: Create new session
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    When I run "agm new --harness <harness> test-session"
    Then a session "test-session" should be created
    And the manifest should show agent "<agent>"
    And the session state should be "active"

    Examples:
      | agent    | harness      |
      | claude   | claude-code  |
      | gemini   | gemini-cli   |
      | codex    | codex-cli    |
      | opencode | opencode-cli |

  Scenario Outline: Resume existing session
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "existing-session" exists with agent "<agent>"
    And I pause the session "existing-session"
    When I run "agm resume existing-session"
    Then the session "existing-session" should be active
    And the agent should be "<agent>"

    Examples:
      | agent    |
      | claude   |
      | gemini   |
      | codex    |
      | opencode |

  Scenario Outline: Archive session
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "temp-session" exists with agent "<agent>"
    When I run "agm archive temp-session"
    Then the session "temp-session" should be archived
    And the session state should be "archived"

    Examples:
      | agent    |
      | claude   |
      | gemini   |
      | codex    |
      | opencode |

  Scenario Outline: Graceful error when sending to non-existent session
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    When I try to send a message to session "invalid-session-id"
    Then no history should be created for "invalid-session-id"

    Examples:
      | agent    |
      | claude   |
      | gemini   |
      | codex    |
      | opencode |

  Scenario Outline: Concurrent sessions with same agent are isolated
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "session-A" exists with agent "<agent>"
    And a session "session-B" exists with agent "<agent>"
    When I send message "Message A" to session "session-A"
    And I send message "Message B" to session "session-B"
    Then session "session-A" history should contain only "Message A"
    And session "session-B" history should contain only "Message B"

    Examples:
      | agent    |
      | claude   |
      | gemini   |
      | codex    |
      | opencode |
