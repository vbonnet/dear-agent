Feature: Conversation Persistence
  As an AGM user
  I want conversations to persist across sessions
  So that context is maintained when I resume work

  Scenario Outline: Messages persist across sessions
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "persist-test" exists with agent "<agent>"
    When I send message "Hello, world" to session "persist-test"
    And I pause the session "persist-test"
    And I resume the session "persist-test"
    Then the message "Hello, world" should be in the conversation history
    And the agent should be "<agent>"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |

  Scenario Outline: Context maintained after resume
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "context-test" exists with agent "<agent>"
    When I send message "My name is Alice" to session "context-test"
    And I send message "What is my name?" to session "context-test"
    Then the response should contain "Alice"
    And the context should be maintained

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |

  Scenario Outline: Multi-turn conversation maintains context across 5 exchanges
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "multi-turn-test" exists with agent "<agent>"
    When I send message "My favorite color is blue" to session "multi-turn-test"
    And I send message "My age is 30" to session "multi-turn-test"
    And I send message "I live in Paris" to session "multi-turn-test"
    And I send message "I work as a developer" to session "multi-turn-test"
    And I send message "What is my favorite color?" to session "multi-turn-test"
    Then the response should contain "blue"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |

  Scenario Outline: Session handles large message history
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "large-history-test" exists with agent "<agent>"
    When I send 25 sequential messages
    And I send message "recall first" to session "large-history-test"
    Then the session history should contain 52 messages
    And the response should reference the first message

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |
