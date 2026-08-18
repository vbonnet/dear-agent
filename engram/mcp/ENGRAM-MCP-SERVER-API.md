# Engram MCP Server API

The server exposes three tools over MCP stdio. Every successful call returns one
text content item:

```json
{"content":[{"type":"text","text":"..."}]}
```

Unhandled validation or routing failures also set `isError: true`.

## `engram.retrieve`

Retrieves memories through the configured Engram CLI.

Input:

```json
{
  "query": "error handling",
  "tag": "go",
  "limit": 5
}
```

| Field | Required | Constraint |
| --- | --- | --- |
| `query` | yes | non-empty string |
| `tag` | no | string |
| `limit` | no | integer from 1 through 1000; default 5 |

The text is the CLI output, `No results found`, or a diagnostic beginning
`Error retrieving engrams:`. The command is equivalent to:

```text
engram retrieve QUERY [--tag TAG] [--limit LIMIT]
```

The executable may be overridden with `ENGRAM_CLI`.

## `engram.plugins.list`

Lists plugin directories containing `plugin.yaml` beneath:

- `$ENGRAM_ROOT/core/plugins`
- `$ENGRAM_ROOT/user/plugins`

Input:

```json
{}
```

The text contains each plugin's name, type, description, and core/user
location. Missing or malformed manifests are skipped. If none are readable, the
result is `No plugins found`.

## `wayfinder.phase.status`

Reads canonical Wayfinder status for a project.

Input:

```json
{"project":"/absolute/or/relative/project/path"}
```

| Field | Required | Constraint |
| --- | --- | --- |
| `project` | yes | path resolved against the server working directory |

The project must contain `WAYFINDER-STATUS.md` with only strict schema-2.0 YAML
frontmatter. A valid result is a text item whose text is formatted JSON:

```json
{
  "project": "/resolved/project/path",
  "phase": "BUILD",
  "progress": "77%",
  "status": "in-progress"
}
```

The response does not contain `current_waypoint`, `next_waypoint`, deliverables,
or inferred state. Missing files produce text beginning `No Wayfinder status
found for project:`. Parse or read failures produce text beginning `Error
reading Wayfinder status:`.

## Caching

Retrievals are keyed by query, tag, and limit. Plugin listings use one key.
Wayfinder status is keyed by resolved project path. The default TTL is 30
seconds and may be changed with `MCP_CACHE_TTL_MS`; the process holds at most
200 entries.

## Transport

Run the built server with:

```bash
npm run build
npm start
```

The server uses MCP stdio. Do not mix other stdout output into the transport.
