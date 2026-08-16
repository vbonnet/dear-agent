# bumblebee — developer-endpoint supply-chain scanning

[Bumblebee](https://github.com/perplexityai/bumblebee) is a read-only
inventory collector for developer endpoints. It reads lockfiles
(`package-lock.json`, `pnpm-lock.yaml`, `*.dist-info/METADATA`, `go.sum`,
…), MCP config files, and IDE/browser extension manifests, and emits an
NDJSON inventory of what's installed on this machine. With an exposure
catalog it cross-references that inventory against known-bad versions.

It complements DeepSec, not replaces it:

| Layer           | DeepSec     | Bumblebee   |
| --------------- | ----------- | ----------- |
| Reads           | source code | on-disk packages + MCP/extension configs |
| Surface         | PR / push   | the developer's laptop |
| LLM cost        | yes         | none (Go binary, no LLM) |
| Triggers install hooks? | no  | no — reads metadata only |

The motivating audit is archived in
[`engram-research`](https://github.com/vbonnet/engram-research/blob/main/archive/dear-agent-temporal-artifacts-2026-06-30/docs/design/2026-05-23-security-pipeline-bumblebee-deepsec.md);
see `docs/adr/ADR-027` for the decision record.

## Cost model

| Surface | Cost |
| ------- | ---- |
| Local   | $0 — single Go binary, no LLM. Catalog fetches (if configured) are HTTP only. |
| CI      | N/A — Bumblebee scans the endpoint, not the repo. Running it in CI would inventory the GitHub Actions runner, which is meaningless. |

## Install

The binary is pinned to a specific release with an SHA-256 checksum
verified before the tarball is opened. The pin and the digests live in
`cmd/dear-agent-bumblebee/install.go` (`BumblebeeVersion` + `pinnedDigests`);
bumping them is a deliberate, reviewed change. Re-uploaded or MITM'd
releases fail closed.

```bash
make bumblebee-install
# default prefix is ~/.local/bin
# override: BUMBLEBEE_PREFIX=/usr/local/bin make bumblebee-install
```

Add `~/.local/bin` to your `PATH` if it isn't already.

## Run on demand

```bash
make bumblebee-scan
```

Output lands in:

- macOS: `~/Library/Application Support/dear-agent/bumblebee/<YYYY-MM-DD>.ndjson`
- Linux: `${XDG_DATA_HOME:-~/.local/share}/dear-agent/bumblebee/<YYYY-MM-DD>.ndjson`

The wrapper writes atomically (temp + rename) and prints a one-line
record-count summary to stderr.

Profile, root, and extra args pass through:

```bash
dear-agent-bumblebee scan --profile baseline
dear-agent-bumblebee scan --profile deep --root "$HOME"
```

## Run on a schedule (macOS)

```bash
make install-bumblebee-launchagent      # daily at 04:00 local
make uninstall-bumblebee-launchagent
dear-agent-bumblebee install-launchagent --status
```

LaunchAgent (per-user), **not** LaunchDaemon (root). Bumblebee inventories
user-scoped state — running it as root would broaden scope without
gaining anything. Logs at `~/Library/Logs/dear-agent/bumblebee.log`.

`RunAtLoad` is off — the scan does not fire on login. Trigger an
out-of-band run with:

```bash
launchctl kickstart gui/$UID/com.dear-agent.bumblebee
```

On Linux, set up an equivalent `systemd --user` timer or a cron entry that
runs `dear-agent-bumblebee scan`. We don't ship one — macOS is the only
endpoint scheduling surface `cmd/dear-agent-bumblebee` implements and tests.

## Exposure catalog (optional, recommended later)

Without a catalog, Bumblebee runs in pure-inventory mode — useful on its
own because the day-over-day diff reveals new MCP servers, IDE
extensions, or browser extensions appearing on your machine. With a
catalog, matches against known-bad versions are surfaced inline.

When you want to add one, drop it at `etc/bumblebee/catalog.json` (or
point `BUMBLEBEE_CATALOG` at any JSON file). The scan wrapper auto-detects
the repo-local path.

The catalog format is Bumblebee's own
([see upstream README](https://github.com/perplexityai/bumblebee#exposure-catalogs)).
The dear-agent posture is to maintain a *small, narrow* catalog seeded
from incidents the `weekly-security-audit` task or other intel actually
flags — one PR per added entry. Premature today (Bumblebee is v0.1.x and
community catalogs haven't coalesced); the bare scheduler is still useful
without it.

## Output, briefly

Each NDJSON line is one record: hostname, OS/arch, ecosystem, package
name and version, source file, confidence band, severity (only set when
matched against a catalog), and catalog ID. Inspect with `jq`:

```bash
# how many records?
wc -l < ~/Library/Application\ Support/dear-agent/bumblebee/$(date +%F).ndjson

# what ecosystems showed up?
jq -r '.ecosystem' < ~/Library/Application\ Support/dear-agent/bumblebee/$(date +%F).ndjson | sort -u

# diff today vs yesterday — what got installed since the last run?
diff <(jq -c '{ecosystem,name,version}' < <yesterday>.ndjson | sort -u) \
     <(jq -c '{ecosystem,name,version}' < <today>.ndjson    | sort -u)
```

## Limits / known gaps

- **No catalog shipped yet.** Inventory-only on first run. See above.
- **macOS scheduler only.** Linux users currently run on demand or wire
  their own systemd timer.
- **Bumblebee v0.1.x.** CLI flags and catalog format may shift; the
  pinned-version install protects us from breaking under our feet.
