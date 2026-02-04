Feature: Send-Time Logging (AC2: Complete audit trail)
  As an Astrocyte daemon developer
  I want all message sends to be logged
  So that I have a complete audit trail for debugging and compliance

  Background:
    Given the astrocyte_messaging module is loaded
    And the log directory exists at "~/.csm/astrocyte/logs/"

  Scenario: Message send creates log entry
    Given the messages.log file is empty or missing
    When I send a diagnosis message "Test message"
    Then a log entry is created in messages.log
    And the log entry includes the session name
    And the log entry includes the message type "diagnosis"
    And the log entry includes the message length
    And the log entry includes a message hash

  Scenario: Log entry includes all required fields
    When I send a notification message "Session recovered"
    Then the latest log entry includes "SEND session="
    And the log entry includes "type=notification"
    And the log entry includes "length="
    And the log entry includes "hash="

  Scenario: Multiple messages create multiple log entries
    When I send 5 diagnosis messages
    Then messages.log contains exactly 5 new log entries
    And each entry has a unique message hash

  Scenario: Log failure does not block send (fail-safe)
    Given the log directory has no write permissions
    When I send a diagnosis message "Test message"
    Then the message is sent successfully
    And a warning is written to stderr
    And the warning includes "Failed to log message"

  Scenario: Log file rotation at 10MB
    Given messages.log size is approaching 10MB
    When I send enough messages to exceed 10MB
    Then messages.log is rotated to messages.log.1
    And a new messages.log is created
    And up to 5 backup files are kept

  Scenario: Log file permissions are 0600 (security)
    When a new messages.log file is created
    Then the file permissions are 0600 (owner read/write only)
    And other users cannot read the log file
