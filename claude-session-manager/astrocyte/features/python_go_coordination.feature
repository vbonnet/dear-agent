Feature: Python + Go Coordination (AC5: Format alignment, pass-through)
  As an Astrocyte daemon developer
  I want Python and Go to coordinate message delivery correctly
  So that messages are delivered reliably to tmux sessions

  Background:
    Given the astrocyte_messaging module is loaded
    And the csm send command is available
    And a test tmux session exists

  Scenario: Small message uses csm send --prompt
    When I send a message smaller than 10KB
    Then the wrapper calls csm send with --prompt flag
    And the message is passed as a command-line argument
    And the message is delivered to the tmux session

  Scenario: Large message uses csm send --prompt-file
    When I send a message larger than or equal to 10KB
    Then the wrapper calls csm send with --prompt-file flag
    And the message is written to a temporary file
    And the temporary file path is passed to csm
    And the message is delivered to the tmux session
    And the temporary file is deleted after send

  Scenario: Multi-line message preservation
    Given a multi-line message with 10 lines
    When I send the message via send_tagged_message
    Then all 10 lines are delivered to the tmux session
    And the line breaks are preserved
    And no lines are truncated

  Scenario: Special characters preservation
    Given a message with special characters: quotes, brackets, symbols
    When I send the message via send_tagged_message
    Then all special characters are preserved
    And the message content matches exactly in the tmux session

  Scenario: csm send failure propagates error
    Given a non-existent tmux session name
    When I attempt to send a message to the non-existent session
    Then csm send returns non-zero exit code
    And subprocess.CalledProcessError is raised
    And the error message indicates the session was not found

  Scenario: Pass-through coordination (no transformation)
    When I send a tagged message via Python wrapper
    Then the message is passed to csm without transformation
    And csm delivers the exact message to tmux
    And no characters are escaped or modified
    And the message in tmux matches the formatted message exactly

  Scenario: Temporary file cleanup on error
    Given a message large enough to require --prompt-file
    When csm send fails during delivery
    Then the temporary file is still deleted
    And no temporary files are left behind

  Scenario: Format alignment between Python and Go
    When I send a message with <system-reminder> tags
    Then csm send accepts the tags without modification
    And tmux receives the tags as-is
    And Claude Code recognizes the <system-reminder> block
