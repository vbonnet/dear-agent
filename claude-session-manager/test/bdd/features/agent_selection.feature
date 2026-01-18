Feature: Agent Selection
  As an AGM user
  I want to use different agents for different tasks
  So that I can leverage each agent's strengths

  Scenario: Use different agents for different tasks
    Given I have AGM installed
    And I have a mock claude adapter configured
    And I have a mock gemini adapter configured
    When I run "csm new --agent=claude code-session"
    And I run "csm new --agent=gemini research-session"
    Then session "code-session" should have agent "claude"
    And session "research-session" should have agent "gemini"

  Scenario Outline: Agent selection persists across resume
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "agent-test" exists with agent "<agent>"
    When I pause the session "agent-test"
    And I resume the session "agent-test"
    Then the session should still use agent "<agent>"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | gpt    |

  Scenario Outline: Agent persists across full lifecycle
    Given I have AGM installed
    And I have a mock <agent> adapter configured
    And a session "lifecycle-test" exists with agent "<agent>"
    When I pause the session "lifecycle-test"
    And I resume the session "lifecycle-test"
    And I send message "test" to session "lifecycle-test"
    Then the session should still use agent "<agent>"
    And the response should come from "<agent>"

    Examples:
      | agent  |
      | claude |
      | gemini |
      | gpt    |
