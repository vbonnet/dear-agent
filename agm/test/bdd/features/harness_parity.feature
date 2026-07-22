# SPEC: agm/internal/agent/SPEC.md
# RELATED-SPEC: agm/internal/launchparity/SPEC.md
# RELATED-SPEC: agm/internal/session/SPEC.md
# RELATED-SPEC: agm/internal/activity/SPEC.md
# RELATED-SPEC: agm/internal/agysession/SPEC.md
# RELATED-SPEC: agm/internal/codexsession/SPEC.md
# RELATED-SPEC: agm/internal/agent/openai/SPEC.md
# RELATED-SPEC: agm/internal/command/SPEC.md
# RELATED-SPEC: agm/internal/monitor/opencode/SPEC.md
# RELATED-SPEC: agm/internal/monitor/tmux/SPEC.md
# RELATED-SPEC: agm/cmd/agm/SPEC.md
# RELATED-SPEC: agm/cmd/agm-mcp-server/SPEC.md
# RELATED-SPEC: agm/cmd/agm/hooks/SPEC.md
# RELATED-SPEC: agm/cmd/agm/parity/SPEC.md
# RELATED-SPEC: cmd/vroom-dispatch/SPEC.md
# RELATED-SPEC: agm/internal/tmux/SPEC.md
# RELATED-SPEC: agm/internal/safety/SPEC.md
# RELATED-SPEC: agm/internal/cleanup/SPEC.md
# RELATED-SPEC: agm/internal/procguard/SPEC.md
# RELATED-SPEC: agm/internal/procreaper/SPEC.md
# RELATED-SPEC: agm/internal/recovery/SPEC.md
# RELATED-SPEC: agm/internal/sweeper/SPEC.md
# RELATED-SPEC: agm/internal/bus/SPEC.md
# RELATED-SPEC: agm/internal/messages/SPEC.md
# RELATED-SPEC: agm/internal/config/SPEC.md
# RELATED-SPEC: agm/internal/daemon/SPEC.md
# RELATED-SPEC: agm/internal/eventbus/SPEC.md
# RELATED-SPEC: agm/internal/interrupt/SPEC.md
# RELATED-SPEC: agm/internal/lifecycle/SPEC.md
# RELATED-SPEC: agm/internal/backend/SPEC.md
# RELATED-SPEC: agm/internal/backend/restbackend/SPEC.md
# RELATED-SPEC: agm/internal/manager/SPEC.md
# RELATED-SPEC: agm/internal/manager/tmuxbackend/SPEC.md
# RELATED-SPEC: agm/internal/manager/dockerbackend/SPEC.md
# RELATED-SPEC: agm/internal/readiness/SPEC.md
# RELATED-SPEC: agm/internal/send/SPEC.md
# RELATED-SPEC: agm/internal/manifest/SPEC.md
# RELATED-SPEC: agm/internal/dolt/SPEC.md
# RELATED-SPEC: agm/internal/dolt/migrations/SPEC.md
# RELATED-SPEC: agm/internal/statusline/SPEC.md
# RELATED-SPEC: agm/cmd/agm-bus/SPEC.md
# RELATED-SPEC: agm/cmd/agm-aware-reaper/SPEC.md
# RELATED-SPEC: agm/cmd/agm-reaper/SPEC.md
# RELATED-SPEC: agm/cmd/agm-statusline/SPEC.md
# RELATED-SPEC: agm/cmd/agm-statusline-capture/SPEC.md
# RELATED-SPEC: agm/internal/a2a/SPEC.md
# RELATED-SPEC: agm/internal/a2a/artifacts/SPEC.md
# RELATED-SPEC: agm/internal/a2a/broker/SPEC.md
# RELATED-SPEC: agm/internal/a2a/channel/SPEC.md
# RELATED-SPEC: agm/internal/a2a/beads/SPEC.md
# RELATED-SPEC: agm/internal/a2a/discovery/SPEC.md
# RELATED-SPEC: agm/internal/a2a/messaging/SPEC.md
# RELATED-SPEC: agm/internal/a2a/metrics/SPEC.md
# RELATED-SPEC: agm/internal/a2a/modelcard/SPEC.md
# RELATED-SPEC: agm/internal/a2a/protocol/SPEC.md
# RELATED-SPEC: agm/internal/a2a/review/SPEC.md
# RELATED-SPEC: agm/internal/a2a/tasks/SPEC.md
# RELATED-SPEC: agm/internal/a2a/token/SPEC.md
Feature: Harness parity
  AGM should use one harness-neutral delivery contract for interactive CLI
  harnesses. Claude Code is the reference implementation. Codex CLI, AGY, and
  OpenCode and Pi have different terminal chrome and control surfaces than Claude
  Code, but their idle prompts must still be sendable and their trust/menu
  prompts must not be treated as ready. Gemini CLI is deprecated compatibility
  and is not part of active parity enforcement.

  Scenario: Removed A2A helpers do not remain normative
    Given the retained A2A coordination implementation
    When AGM validates A2A coordination specification drift
    Then A2A coordination specifications should describe only retained behavior

  Scenario Outline: Active parity harnesses are canonical
    Given harness "<harness>" is configured
    When AGM validates active parity support
    Then harness "<harness>" should be active for parity
    And harness "<harness>" should not be deprecated

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario: Gemini CLI is deprecated compatibility
    Given harness "gemini-cli" is configured
    When AGM validates active parity support
    Then harness "gemini-cli" should be deprecated

  Scenario: AGY doctor health uses the native installation surfaces
    Given harness "agy" is configured
    When AGM resolves doctor health for the configured harness
    Then doctor should recognize CLI binary "agy"
    And doctor should recognize config directory suffix ".gemini/antigravity-cli"

  Scenario Outline: AGY doctor health normalizes legacy manifest spellings
    Given harness "<harness>" is configured
    When AGM resolves doctor health for the configured harness
    Then doctor should recognize CLI binary "agy"
    And doctor should recognize config directory suffix ".gemini/antigravity-cli"

    Examples:
      | harness     |
      | agy-cli     |
      | antigravity |

  Scenario: Active harness adapters satisfy shared conformance
    Given AGM active harnesses are configured
    When AGM validates active harness adapter conformance
    Then every active harness adapter should satisfy the shared conformance suite

  Scenario Outline: Active harness launch commands preserve startup mode and persistence
    Given active harness "<harness>" uses startup mode "<mode>"
    When AGM builds the harness launch command with persistence enabled
    Then the launch command should use the native interactive startup contract
    And the launch command should not exit the tmux pane shell

    Examples:
      | harness      | mode |
      | claude-code  | auto |
      | codex-cli    | auto |
      | agy          | auto |
      | opencode-cli | plan |
      | pi-cli       | plan |

  Scenario Outline: Active harness startup is transactional
    Given active harness "<harness>" uses startup mode "default"
    When AGM validates final startup liveness
    Then startup should require a live tmux session and harness process

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario Outline: Active harness recovery requires process-state evidence
    Given harness "<harness>" is configured
    When AGM validates session recovery parity
    Then recovery should require process-state confirmation
    And recovery waits should respect context cancellation
    And harness "<harness>" should have a safe recovery fallback policy

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario Outline: Active harness capture uses the canonical AGM socket
    Given harness "<harness>" is configured
    When AGM validates the pane capture invocation
    Then pane capture should use the canonical AGM tmux socket
    And pane capture should normalize the session target
    And pane capture should be bounded and process-group isolated

    Examples:
      | harness      |
      | claude-code  |
      | codex-cli    |
      | agy          |
      | opencode-cli |
      | pi-cli       |

  Scenario: Every tmux-facing AGM command declares active harness parity
    Given AGM tmux-facing command sources
    When AGM validates tmux command parity contracts
    Then every tmux-facing command should declare all active harness strategies
    And every tmux-facing Cobra command source should have a parity contract

  Scenario Outline: Model-independent tmux commands cross model families
    Given model family "<family>" is configured
    When AGM validates model-independent tmux command parity
    Then model-independent tmux commands should support model family "<family>"

    Examples:
      | family    |
      | anthropic |
      | openai    |
      | gemini    |
      | glm       |
      | deepseek  |
      | nemotron  |
      | qwen      |

  Scenario Outline: AGM runtime helper commands declare SPEC coverage
    Given AGM runtime helper command "<command>" is configured
    When AGM validates runtime helper command coverage
    Then runtime helper command "<command>" should have a co-located SPEC

    Examples:
      | command                |
      | agm-bus                |
      | agm-aware-reaper       |
      | agm-reaper             |
      | agm-statusline         |
      | agm-statusline-capture |

  Scenario Outline: AGM backend implementations declare SPEC coverage
    Given AGM backend implementation "<backend>" is configured
    When AGM validates backend implementation coverage
    Then backend implementation "<backend>" should have a co-located SPEC

    Examples:
      | backend                         |
      | backend/restbackend             |
      | manager/tmuxbackend             |
      | manager/dockerbackend           |

  Scenario Outline: AGM cleanup and process support packages declare SPEC coverage
    Given AGM cleanup support package "<package>" is configured
    When AGM validates cleanup support package coverage
    Then cleanup support package "<package>" should have a co-located SPEC

    Examples:
      | package    |
      | cleanup    |
      | procguard  |
      | procreaper |
      | sweeper    |

  Scenario: AGM archive cleanup preserves a repository primary checkout
    Given AGM archive cleanup targets a repository checkout
    When AGM validates primary checkout cleanup safety
    Then the primary checkout and session-named branch should remain
    And a linked session worktree should still be removed
    And an unclassified worktree should not authorize branch deletion
    And a context-only checkout should not authorize branch deletion
    And branch deletion should require attributed worktree ownership

  Scenario Outline: Supported model families have default routes
    Given model family "<family>" is configured
    When AGM validates model family parity support
    Then model family "<family>" should be supported
    And model family "<family>" should have a default model route

    Examples:
      | family    |
      | anthropic |
      | openai    |
      | gemini    |
      | glm       |
      | deepseek  |
      | nemotron  |
      | qwen      |

  Scenario Outline: Active harness model changes use the shared registry
    Given harness "<harness>" is configured
    When AGM resolves a model change for harness "<harness>" with model "<model>"
    Then the model change should use tmux command "/model"
    And the resolved model should not be empty

    Examples:
      | harness      | model     |
      | claude-code  | sonnet    |
      | codex-cli    | 5.4-mini  |
      | agy          | 3.5-flash |
      | opencode-cli | glm-5.2   |
      | opencode-cli | deepseek-v4 |
      | opencode-cli | nemotron  |
      | opencode-cli | qwen      |
      | pi-cli       | sonnet    |
      | pi-cli       | gpt       |
      | pi-cli       | gemini-flash |
      | pi-cli       | glm-5.2   |
      | pi-cli       | deepseek-v4 |
      | pi-cli       | nemotron  |
      | pi-cli       | qwen      |

  Scenario: Codex composer is ready to receive input
    Given a Codex CLI composer pane
    When AGM checks whether the session can receive input
    Then delivery should be allowed
    And the detected session state should be "ready"

  Scenario: Codex trust prompt is not treated as ready
    Given a Codex CLI trust prompt
    When AGM checks whether the session can receive input
    Then delivery should be queued

  Scenario: Stale Codex composer above shell output is not treated as ready
    Given a stale Codex CLI composer followed by shell output
    When AGM checks whether the session can receive input
    Then delivery should be queued

  Scenario: AGY prompt is ready to receive input
    Given an AGY ready prompt
    When AGM checks whether the session can receive input
    Then delivery should be allowed
    And the detected session state should be "ready"

  Scenario: AGY trust prompt is not treated as ready
    Given an AGY trust prompt
    When AGM checks whether the session can receive input
    Then delivery should be queued

  Scenario: AGY feedback survey owns input focus
    Given an AGY feedback survey over a ready prompt
    When AGM checks whether the session can receive input
    Then delivery should require dismissing an overlay

  Scenario: Codex detached session receives startup prompt
    Given Codex CLI is available
    When AGM creates a detached Codex session with a startup prompt
    Then AGM should wait for the Codex composer
    And AGM should deliver the startup prompt even though the session is detached

  Scenario: Runtime-backed shared Codex creation rolls back when the composer is absent
    Given shared Codex creation cannot observe the composer
    When AGM creates Codex through a surface runtime
    Then shared creation should fail before registration and prompt delivery
    And shared creation should remove its newly created tmux session

  Scenario: Shared readiness requires current process and composer ownership
    When AGM validates slow harness startup readiness
    Then shared startup readiness should honor the total deadline
    And shared input readiness should serialize exact-pane delivery and preserve rendered composer ownership without treating resolved prompts as live
    And CLI message and startup prompt sends should use shared atomic readiness for exact-pane delivery
    And forced CLI message sends should preserve the measured queued AGM anchor across prompt-like payload lines
    And autonomous CLI message sends should preserve only positively identified queued AGM recovery
    And API delivery should restore persisted configuration, linearize archive and deletion with bounded completed turns, renew fan-out deadlines with separate preflight and full provider budgets, honor request cancellation during reconstruction and readiness, preserve large JSONL records, batch imports, require adapter readiness without tmux, and document its compatibility-only control plane
    And shared Gemini readiness should advance first-run trust on the verified pane
    And legacy AGY names should reach canonical shared send readiness
    And the Pi alias should reach canonical shared send readiness

  Scenario: Codex detached startup clears first-run trust before delivery
    Given Codex CLI is available
    And a Codex CLI trust prompt
    When AGM creates a detached Codex session with a startup prompt
    Then AGM should auto-accept the Codex trust prompt before prompt delivery
    And AGM should wait for the Codex composer

  Scenario: Codex current-tmux creation launches before registration
    Given current-tmux creation selects Codex CLI
    When AGM validates current-tmux Codex launch wiring
    Then Codex credential validation should precede the canonical launcher
    And the top-level new command should route into current tmux
    And Codex current-tmux launch should require the executable without waiting behind its own AGM process
    And every queued current-tmux harness should defer readiness until AGM exits
    And current-tmux Claude should associate its UUID on SessionStart
    And Codex queue failures should propagate to shared creation rollback

  Scenario: AGY current-tmux creation refuses unsafe deferred identity
    Given current-tmux creation selects AGY
    When AGM validates current-tmux AGY safety
    Then current-tmux AGY creation should fail before launch with detached guidance

  Scenario: Codex send safety is harness-specific
    Given Codex CLI is available
    And a Codex CLI composer pane
    When AGM runs send safety for the configured harness
    Then send safety should not require a Claude process

  Scenario Outline: Shared send gates delivery on pane readiness
    Given a shared Codex send target with readiness "<readiness>"
    When AGM sends a message through shared operations
    Then the shared send result should be "<outcome>"
    And shared send should emit <commands> tmux commands

    Examples:
      | readiness | outcome       | commands |
      | YES       | delivered     | 1        |
      | NO        | not_delivered | 0        |
      | QUEUE     | not_delivered | 0        |
      | OVERLAY   | not_delivered | 0        |
      | NOT_FOUND | not_delivered | 0        |
      | WRONG_HARNESS | not_delivered | 0     |
      | ONBOARDING    | not_delivered | 0     |
      | PERMISSION    | not_delivered | 0     |

  Scenario: Cancelled shared send emits no input
    Given a shared Codex send target with readiness "YES"
    And the shared send request is cancelled
    When AGM sends a message through shared operations
    Then the shared send result should be "cancelled"
    And shared send should emit 0 tmux commands

  Scenario: AGY detached session receives startup prompt
    Given AGY is available
    When AGM creates a detached AGY session with a startup prompt
    Then AGM should wait for the AGY prompt
    And AGM should deliver the startup prompt even though the session is detached

  Scenario: AGY detached startup clears first-run trust before delivery
    Given AGY is available
    And an AGY trust prompt
    When AGM creates a detached AGY session with a startup prompt
    Then AGM should auto-accept the AGY trust prompt before prompt delivery
    And AGM should wait for the AGY prompt

  Scenario: AGY send safety is harness-specific
    Given AGY is available
    And an AGY ready prompt
    When AGM runs send safety for the configured harness
    Then send safety should not require a Claude process

  Scenario: AGY attributed multiline delivery remains one native request
    Given AGY is available
    When AGM validates AGY multiline delivery
    Then every AGY message surface should preserve one bracketed multiline submission

  Scenario Outline: Pi and OpenCode send safety is harness-specific
    Given harness "<harness>" is configured
    When AGM runs send safety for the configured harness
    Then send safety should not require a Claude process

    Examples:
      | harness      |
      | opencode-cli |
      | pi-cli       |

  Scenario: AGY adapter uses safe concurrent native lifecycle truth
    Given AGY is available
    When AGM validates the AGY adapter lifecycle
    Then the AGY adapter should preserve canonical launch and resume policy
    And the AGY adapter should require AGY process and transcript truth

  Scenario: Current harness session can be associated with AGM
    Given an existing tmux session running Codex CLI
    When /agm:agm-assoc runs in that session
    Then AGM should create or update a Dolt session record with harness "codex-cli"
    And AGM should create the ready-file signal

  Scenario: Current AGY session can be associated with AGM
    Given an existing tmux session running AGY
    When /agm:agm-assoc runs in that session
    Then AGM should create or update a Dolt session record with harness "agy"
    And AGM should create the ready-file signal

  Scenario: Orphaned Codex conversation can be imported and resumed
    Given a Codex saved session exists outside AGM
    When AGM imports the Codex session UUID with harness "codex-cli"
    Then AGM should create or update a Dolt session record with harness "codex-cli"
    And the record should preserve the Codex session UUID
    And AGM should launch a tmux pane that resumes the Codex conversation

  Scenario: Failed Codex resume is rolled back before success effects
    Given a stopped Codex CLI session without a tmux pane
    When AGM validates the Codex resume transaction
    Then Codex resume success should require process and composer readiness
    And a failed Codex resume should serialize concurrent attempts through every production entry point, release the session lock before attachment, preserve canonical tmux identity from stale full-session updates, reconcile ambiguous metadata commits, compensate owned provisional metadata before removing its creation-specific tmux identity even when tmux ID output is lost, and preserve tmux whenever metadata cleanup is unproven
    And authoritative session renames should serialize with cold resume, fence ambiguous storage writes, preserve both identity names from stale writers, preserve claimed tmux identity across lost replies and server restarts, reject stale identity revisions, and compensate tmux after storage conflicts
    And administrative hierarchy repairs should atomically link parents and inherited names through the observed identity revision
    And successful Codex prompt delivery should remain successful after later caller cancellation
    And ambiguous final Codex prompt submission should preserve work that may have started
    And failed Codex prompt delivery should not suppress a later attach failure
    And Codex activity updates should follow resume readiness

  Scenario: Orphaned AGY conversation can be imported and resumed
    Given an AGY saved conversation exists outside AGM
    When AGM imports the AGY conversation ID with harness "agy"
    Then AGM should create or update a Dolt session record with harness "agy"
    And the record should preserve the AGY conversation ID
    And AGM should launch a tmux pane that resumes the AGY conversation

  Scenario: AGY auto permission mode is preserved on resume
    Given an imported AGY session with permission mode "auto"
    When AGM resumes the session
    Then AGM should launch a tmux pane that resumes the AGY conversation
    And the AGY resume command should include "--dangerously-skip-permissions"

  Scenario: AGY model compatibility survives catalog migrations
    Given AGY is available
    When AGM validates AGY model compatibility
    Then retired AGY manifest models should map to current public labels
    And exact AGY public labels should remain unchanged
    And cross-harness AGY aliases should normalize case-insensitively
    And imported AGY conversations should preserve unknown model provenance
    And AGY runtime model switches should not leave a stale resume override

  Scenario: MCP waits for AGY before delivering its startup prompt
    Given AGY is available
    When AGM validates AGY MCP creation readiness
    Then MCP creation should wait for the AGY composer before prompt delivery
    And shared creation should persist the new AGY identity before registration

  Scenario: AGY startup prompt bootstraps lazy provider identity exactly once
    Given AGY is available
    When AGM validates AGY lazy identity bootstrap
    Then shared creation should deliver the AGY startup prompt before identity discovery
    And every AGY creation surface should avoid duplicate prompt delivery
    And AGY bootstrap failures should preserve transactional rollback

  Scenario: Active-harness creation signals preserve rollback
    Given AGY is available
    When AGM validates AGY root cancellation plumbing
    Then root signal cancellation should reach every command-scoped readiness wait

  Scenario: Session list fields can target session rows
    Given AGM has Codex session records in Dolt
    When an agent lists sessions as JSON with fields "name,status,harness,workspace,tags"
    Then the output should include a "sessions" array
    And each session row should include the requested fields
    And the output should not collapse to an empty object

  Scenario: Codex lifecycle commands work end to end
    Given a Codex CLI session created by AGM
    When AGM sends a message to the session
    And AGM resumes the session
    And AGM kills the session
    And AGM archives the stopped session
    Then Dolt should reflect the expected lifecycle transitions
    And the matching Codex saved session should be archived
