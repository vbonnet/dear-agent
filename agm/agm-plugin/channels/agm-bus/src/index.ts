#!/usr/bin/env node
// agm-bus channel MCP adapter — the Claude Code side of the Layer 3 bus.
//
// Responsibilities:
//   1. Open a unix-socket connection to the agm-bus broker.
//   2. Identify ourselves via Hello using AGM_SESSION_NAME.
//   3. Declare MCP capabilities `claude/channel` and `claude/channel/permission`
//      so Claude Code registers us as a channel and forwards permission
//      prompts for relay.
//   4. Bridge:
//        inbound broker frame  → MCP notification `notifications/claude/channel`
//        Claude tool call `send(target, text)` → outbound broker FrameSend
//        Claude Code permission prompt → outbound broker FramePermissionRequest
//        inbound permission_verdict → MCP notification `notifications/claude/channel/permission`
//
// The adapter is ~150 LoC because the broker does the heavy lifting: ACL
// enforcement, routing, offline queueing. This file is a thin translator.

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";

import { BrokerClient, generateFrameID } from "./broker-client.js";
import type { Frame } from "./wire.js";

// Session id comes from AGM's env var so the user doesn't have to thread it
// through MCP config. Fallback to a diagnostic id so errors are obvious.
const sessionID = process.env.AGM_SESSION_NAME ?? "agm-bus-unnamed";

// --- MCP server setup -------------------------------------------------------

const mcp = new Server(
  { name: "agm-bus", version: "0.1.0" },
  {
    capabilities: {
      experimental: {
        "claude/channel": {},
        "claude/channel/permission": {},
      },
      tools: {},
    },
    instructions:
      `Messages between AGM sessions arrive as <channel source="agm-bus" from="..." id="..."> events. ` +
      `Use the "send" tool with target=<peer session id> and text=<message> to reply or initiate. ` +
      `Permission prompts from Claude Code are forwarded as <channel source="agm-bus" kind="permission_request" id="..."> — ` +
      `wait for the peer supervisor to reply "yes <id>" or "no <id>" via normal chat, and Claude Code will apply the verdict.`,
  },
);

// --- Broker client ----------------------------------------------------------

const broker = new BrokerClient({ sessionID });

broker.on("connected", () => {
  void mcp.notification({
    method: "notifications/claude/channel",
    params: {
      content: `agm-bus connected as ${sessionID}`,
      meta: { kind: "status" },
    },
  });
});

broker.on("disconnected", () => {
  // Claude Code may or may not pick this up depending on how it surfaces
  // channel errors; log to stderr so the terminal shows it.
  process.stderr.write(`agm-bus: disconnected from broker (will reconnect)\n`);
});

// Inbound broker frame → MCP notification. We translate all non-ack/non-
// welcome frames into <channel> events; each frame's Type becomes the
// "kind" metadata so Claude can branch on it.
broker.on("frame", (f: Frame) => {
  switch (f.type) {
    case "deliver":
      void mcp.notification({
        method: "notifications/claude/channel",
        params: {
          content: f.text ?? "",
          meta: buildMeta({
            kind: "deliver",
            from: f.from,
            id: f.id,
            ...(f.extra ?? {}),
          }),
        },
      });
      return;
    case "permission_request":
      void mcp.notification({
        method: "notifications/claude/channel",
        params: {
          content: describePermissionRequest(f),
          meta: buildMeta({
            kind: "permission_request",
            from: f.from,
            id: f.id,
            tool: f.tool_name,
          }),
        },
      });
      return;
    case "permission_verdict":
      void mcp.notification({
        method: "notifications/claude/channel/permission",
        params: {
          request_id: f.id,
          behavior: f.verdict,
        },
      });
      return;
    case "error":
      process.stderr.write(
        `agm-bus: broker error: ${f.code ?? "?"} ${f.message ?? ""}\n`,
      );
      return;
    default:
    // ack and others: ignore for now.
  }
});

// --- Tools ------------------------------------------------------------------

mcp.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [
    {
      name: "send",
      description:
        "Send a message to another AGM session by id. For peer-to-peer supervisor coordination and worker-to-supervisor requests.",
      inputSchema: {
        type: "object",
        properties: {
          target: { type: "string", description: "Recipient session id" },
          text: { type: "string", description: "Message body" },
        },
        required: ["target", "text"],
      },
    },
    {
      name: "permission_verdict",
      description:
        "Respond to a relayed permission_request with allow or deny. Pass the request_id from the original event.",
      inputSchema: {
        type: "object",
        properties: {
          request_id: { type: "string" },
          target: { type: "string", description: "Worker session id the request came from" },
          verdict: { type: "string", enum: ["allow", "deny"] },
        },
        required: ["request_id", "target", "verdict"],
      },
    },
  ],
}));

mcp.setRequestHandler(CallToolRequestSchema, async (req) => {
  if (req.params.name === "send") {
    const args = z
      .object({ target: z.string(), text: z.string() })
      .parse(req.params.arguments ?? {});
    const ok = broker.send(args.target, args.text);
    return {
      content: [
        { type: "text", text: ok ? "sent" : "broker not connected; dropped" },
      ],
    };
  }
  if (req.params.name === "permission_verdict") {
    const args = z
      .object({
        request_id: z.string(),
        target: z.string(),
        verdict: z.union([z.literal("allow"), z.literal("deny")]),
      })
      .parse(req.params.arguments ?? {});
    const ok = broker.sendPermissionVerdict(
      args.request_id,
      args.target,
      args.verdict,
    );
    return {
      content: [
        { type: "text", text: ok ? "verdict sent" : "broker not connected; dropped" },
      ],
    };
  }
  throw new Error(`agm-bus: unknown tool ${req.params.name}`);
});

// --- Permission-request relay ----------------------------------------------
// When Claude Code opens a permission dialog, it notifies us via
// `notifications/claude/channel/permission_request`. We forward that to the
// broker as a FramePermissionRequest; the broker routes it to the session's
// peer supervisor (or a Discord adapter for human approval).

const PermissionRequestSchema = z.object({
  method: z.literal("notifications/claude/channel/permission_request"),
  params: z.object({
    request_id: z.string(),
    tool_name: z.string(),
    description: z.string(),
    input_preview: z.string(),
  }),
});

// MCP's notification-handler API requires the schema object itself.
mcp.setNotificationHandler(PermissionRequestSchema, async ({ params }) => {
  // Target is determined by convention: AGM_SUPERVISOR_PRIMARY_FOR (for
  // worker-to-supervisor) or AGM_SUPERVISOR_ID itself (for supervisor-to-
  // human via Discord). For now, keep the routing simple and always send
  // to an explicit AGM_PERMISSION_RELAY_TARGET env var; the supervisor
  // mesh config will refine this.
  const target = process.env.AGM_PERMISSION_RELAY_TARGET;
  if (!target) {
    process.stderr.write(
      "agm-bus: no AGM_PERMISSION_RELAY_TARGET set; dropping permission_request\n",
    );
    return;
  }
  broker.sendRaw({
    type: "permission_request",
    id: params.request_id,
    to: target,
    tool_name: params.tool_name,
    description: params.description,
    input_preview: params.input_preview,
  });
});

// --- Helpers ----------------------------------------------------------------

function describePermissionRequest(f: Frame): string {
  const id = f.id ?? "?";
  const tool = f.tool_name ?? "?";
  const desc = f.description ?? "";
  const preview = f.input_preview ?? "";
  return (
    `Claude wants to run ${tool} (request ${id}): ${desc}\n` +
    `Input: ${preview}\n` +
    `Reply with permission_verdict tool: request_id=${id} target=${f.from} verdict=allow|deny`
  );
}

/**
 * buildMeta flattens undefined values so JSON.stringify doesn't emit
 * "from": undefined keys, which the broker's meta parsing would reject.
 * Also coerces all values to strings per the <channel> tag contract.
 */
function buildMeta(
  src: Record<string, string | undefined>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(src)) {
    if (v !== undefined && v !== "") {
      out[k] = v;
    }
  }
  return out;
}

// --- Bootstrap --------------------------------------------------------------

async function main(): Promise<void> {
  broker.connect();
  await mcp.connect(new StdioServerTransport());
}

main().catch((err) => {
  process.stderr.write(`agm-bus: fatal: ${err?.stack ?? err}\n`);
  process.exit(1);
});

// Small static use so z and generateFrameID don't become dead in CI's
// unused-import sweep even if a refactor removes their current callers.
export const _internal = { generateFrameID };
