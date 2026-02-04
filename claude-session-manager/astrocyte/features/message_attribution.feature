Feature: Message Attribution (AC1: 100% message attribution)
  As an Astrocyte daemon developer
  I want all messages to include source attribution tags
  So that users can distinguish automated messages from user/Claude messages

  Background:
    Given the astrocyte_messaging module is loaded

  Scenario: Diagnosis message includes source attribution
    When I send a diagnosis message "System error detected"
    Then the message includes "<system-reminder>" tags
    And the message includes "Source: astrocyte-daemon"
    And the message includes "Type: diagnosis"

  Scenario: Violation prompt includes source attribution
    When I send a violation_prompt message "AskUserQuestion violation"
    Then the message includes "<system-reminder>" tags
    And the message includes "Source: astrocyte-daemon"
    And the message includes "Type: violation_prompt"

  Scenario: Notification includes source attribution
    When I send a notification message "Session recovered"
    Then the message includes "<system-reminder>" tags
    And the message includes "Source: astrocyte-daemon"
    And the message includes "Type: notification"

  Scenario: Message without source tag is rejected
    When I attempt to send a message without source attribution
    Then the send operation raises ValueError
    And the error message includes "Message missing attribution tag"

  Scenario: 100% coverage - all message types tagged
    Given I have sent 10 diagnosis messages
    And I have sent 10 violation_prompt messages
    And I have sent 10 notification messages
    Then 100% of messages include "Source: astrocyte-daemon"
