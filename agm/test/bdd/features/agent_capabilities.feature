Feature: Agent Capabilities
  All agents must report their capabilities

  Scenario Outline: All agents support basic operations
    Given I have a mock <agent> adapter configured
    Then the adapter should support session creation
    And the adapter should support message sending
    And the adapter should support history retrieval
    And the adapter should support session lifecycle management

    Examples:
      | agent  |
      | claude |
      | gemini |
      | codex  |
