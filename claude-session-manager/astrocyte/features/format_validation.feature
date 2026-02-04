Feature: Format Validation (AC4: Malformed messages rejected)
  As an Astrocyte daemon developer
  I want malformed messages to be rejected with clear errors
  So that I catch configuration errors early (fail-fast)

  Background:
    Given the astrocyte_messaging module is loaded

  Scenario: Empty message is rejected
    When I attempt to send an empty message ""
    Then the send operation raises ValueError
    And the error message includes "Message cannot be empty"

  Scenario: Whitespace-only message is rejected
    When I attempt to send a whitespace-only message "   \n  "
    Then the send operation raises ValueError
    And the error message includes "Message cannot be empty"

  Scenario: Invalid message type is rejected
    When I attempt to send a message with type "invalid_type"
    Then the send operation raises ValueError
    And the error message includes "Invalid message type: invalid_type"
    And the error message lists valid types: "violation_prompt, diagnosis, notification"

  Scenario: Empty session name is rejected
    When I attempt to send a message to session ""
    Then the send operation raises ValueError
    And the error message includes "Session name cannot be empty"

  Scenario: Valid message passes validation
    When I send a well-formed message
    Then the send operation succeeds
    And no ValueError is raised

  Scenario Outline: All message types are validated
    When I send a message with type "<message_type>"
    Then the message type is validated as "<validity>"
    And the send operation <result>

    Examples:
      | message_type      | validity | result   |
      | diagnosis         | valid    | succeeds |
      | violation_prompt  | valid    | succeeds |
      | notification      | valid    | succeeds |
      | invalid           | invalid  | raises   |
      | warning           | invalid  | raises   |
      | error             | invalid  | raises   |
