# SPEC: internal/markdownvisible/SPEC.md
Feature: Provider-neutral visible Markdown classification
  Policy tools classify one complete CommonMark document while preserving
  source alignment and excluding non-normative examples from semantic checks.

  Scenario: Hidden examples preserve visible source alignment
    Given a SPEC document containing visible requirements and hidden CommonMark examples
    When AGM selects normative Markdown prose
    Then hidden examples should be excluded without changing visible source line alignment
