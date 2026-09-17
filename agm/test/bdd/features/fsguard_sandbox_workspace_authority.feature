# SPEC: internal/fsguard/SPEC.md
Feature: FSGUARD sandbox workspace authority
  FSGUARD must replace its parent-wide compatibility allowance with one exact
  configured workspace without breaking valid dot-prefixed workspace paths.

  Scenario Outline: Filesystem write authority is component bounded
    Given FSGUARD uses "<authority>" sandbox workspace authority
    When FSGUARD classifies the "<target>" target
    Then the sandbox workspace decision should be "<decision>"

    Examples:
      | authority  | target                   | decision |
      | default    | default descendant       | allow    |
      | configured | exact workspace          | allow    |
      | configured | workspace descendant     | allow    |
      | configured | workspace dotfile        | allow    |
      | configured | sibling workspace        | deny     |
      | configured | lexical prefix lookalike | deny     |
      | configured | parent traversal escape  | deny     |
      | configured | default descendant       | deny     |

  Scenario: Invalid filesystem authority fails closed
    When AGM validates invalid sandbox workspace authority
    Then invalid sandbox workspace authority should leave no default allowance

  Scenario: Unresolvable symlinks fail closed without blocking ordinary new paths
    When AGM validates writable-root symlink resolution
    Then dangling symlink targets should be denied while ordinary nonexistent descendants remain allowed

  Scenario: Shell path ambiguity and generic writable parents cannot widen exact authority
    When AGM validates exact sandbox workspace escape resistance
    Then symlink parent traversal and unsupported tilde expansion should be denied
    And exact workspace authority should shadow generic writable parents for sibling paths
