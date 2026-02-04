Feature: Architectural Enforcement (AC3: Impossible to bypass tagging)
  As an Astrocyte daemon developer
  I want message tagging to be architecturally enforced
  So that it's impossible to send untagged messages

  Background:
    Given the astrocyte_messaging module is loaded

  Scenario: Direct csm send is replaced by wrapper
    Given the existing send_diagnosis_prompt_via_csm function
    When I call send_diagnosis_prompt_via_csm("session", "message")
    Then the function delegates to send_tagged_message
    And the message is tagged before sending

  Scenario: Violation prompt uses wrapper
    Given the existing send_violation_prompt function
    When I call send_violation_prompt("session")
    Then the function delegates to send_tagged_message
    And the message type is "violation_prompt"

  Scenario: Permission rejection uses wrapper
    Given the existing reject_permission_prompt function
    When I call reject_permission_prompt("session")
    Then the function delegates to send_tagged_message
    And the message type is "violation_prompt"

  Scenario: Wrapper is the single entry point
    Given all send functions in astrocyte.py
    When I analyze the code paths
    Then all paths route through send_tagged_message
    And no path bypasses the wrapper

  Scenario: Validation blocks untagged messages
    When I attempt to call _send_via_csm directly with an untagged message
    Then the validation layer blocks the send
    And raises ValueError "Message missing attribution tag"

  Scenario: Backward compatibility maintained
    Given existing code calling send_diagnosis_prompt_via_csm
    When I upgrade to the new wrapper implementation
    Then all existing callers continue to work
    And all messages are now tagged automatically
