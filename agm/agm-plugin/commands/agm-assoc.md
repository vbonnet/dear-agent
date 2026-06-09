---
model: haiku
effort: low
content-hash: 55b6fc976c5c0364782390f03a8d6695fe8a22707e5f7563dc6b281c7f5cb571
description: Associate Claude session with AGM (auto-detects tmux session)
argument-hint: "{session-name}"
allowed-tools: Bash(agm session associate *), Bash(tmux display-message *), Bash(tmux -S * display-message *), Bash(pwd)
---

# AGM Session Association

I'll associate this Claude session with AGM.

**Step 1: Determine session name source**

Execute these checks in sequence using separate tool calls. Do NOT use bash if/elif/else conditionals.

**1.1: Check for argument**
- Parse $ARGUMENTS to extract session name
- If $ARGUMENTS contains a non-empty session name:
  - Store as SESSION_NAME
  - Proceed to Step 2
- If $ARGUMENTS is empty or whitespace only:
  - Continue to check 1.2

**1.2: Check TMUX environment**
- Run: `tmux display-message -p '#S'`
- If successful (exit code 0): Capture output as SESSION_NAME, proceed to Step 2
- If failed: Continue to check 1.3

**1.3: No session name available**
- Show error: "Not in tmux session and no session name provided"
- Show message: "Usage: /agm:assoc {session-name}"
- Exit gracefully (do not proceed to Step 2)

**Note**: Make separate bash calls, analyze results in your reasoning layer, then decide next action. Do NOT use conditional logic in bash. Do NOT use echo, printf, test, or command chaining (&&, ||). These are blocked by the PreToolUse hook.

**Step 2: Try association (without --create)**
Run the appropriate command using SESSION_NAME from Step 1.

- Run: `agm session associate "{SESSION_NAME}"`
- Capture exit code and output.

**Step 3: Handle result**
- If exit code is 0:
  - Note the storage location from output (see Step 4)
  - Continue to Step 4
- If output contains "session not found":
  - Session needs to be created with --create flag
  - Run `pwd` and capture output as CURRENT_DIR
  - Run: `agm session associate "{SESSION_NAME}" --create -C "{CURRENT_DIR}"`
    (using SESSION_NAME from Step 1, not from tmux again)
  - If this fails, show error and suggest: "Try running: agm admin doctor", then Exit
  - If successful, continue to Step 4
- If any other error:
  - Show the error output
  - Suggest troubleshooting: "Try running: agm admin doctor"
  - Exit

**Step 4: Show completion message**
Extract the session name and the storage location from the `agm session
associate` output. AGM persists sessions in Dolt, so the output reports the
storage location as either a `Manifest:` line (when a manifest file exists on
disk) or `Session ID:` + `Storage:` lines (Dolt). Use whichever lines are
present, then display:
```
Session associated successfully

Session: {session_name}
Storage: {storage_location}

To rename the Claude session to match: /rename {session_name}
```

**Note:** The skill cannot automatically invoke `/rename` because slash commands can only be executed from user input, not from Claude's responses. Users must manually type the `/rename` command if they want to rename the Claude session.

**Note**: The `agm session associate` command automatically creates a ready-file signal at `~/.agm/ready-{session_name}` to notify AGM that Claude initialization is complete. This enables `agm session new` to detect when Claude is ready without fragile text-matching.

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/dear-agent"
- If tmux not available: Use provided session name only
- If --create fails: Check directory permissions

**Note:** AGM auto-detects Claude UUID from ~/.claude/history.jsonl
