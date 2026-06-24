# AGM Slash Command Skills

> **DEPRECATED**: Legacy shell skills have been removed. All AGM skills are now
> markdown-based and live in `agm-plugin/commands/`.

## New Location

Skills are now defined as markdown files in:

```
agm-plugin/commands/
  agm-assoc.md    # Associate session
  agm-exit.md     # Exit and archive session
  agm-list.md     # List sessions
  agm-status.md   # Session status
  agm-new.md      # Create session
  agm-resume.md   # Resume session
  agm-send.md     # Send message
  agm-search.md   # Search sessions
```

## Usage

In the Claude plugin surface, skills are invoked as slash commands:

```
/agm:list
/agm:new my-project
/agm:send my-session --prompt "Run tests"
/agm:status my-session
/agm:resume my-session
/agm:search research
```

## Format

Each skill is a markdown file with YAML frontmatter (description,
allowed-tools) and step-by-step instructions for the invoking harness. All CLI
calls use `--output json` for reliable parsing.
