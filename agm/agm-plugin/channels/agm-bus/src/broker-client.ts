// BrokerClient is the agm-bus channel adapter's side of the broker wire
// protocol. It owns a single unix-socket connection, sends/receives Frames,
// and surfaces inbound frames to listeners. The MCP server layer wraps
// this to translate between MCP notifications/tool-calls and broker frames.

import { createConnection, Socket } from "node:net";
import { EventEmitter } from "node:events";
import { homedir } from "node:os";
import { resolve } from "node:path";

import { Frame, FrameReader, encodeFrame } from "./wire.js";

/**
 * resolveSocketPath honors AGM_BUS_SOCKET and expands ~/ to the current
 * user's home. Mirrors agm/internal/bus/server.go's SocketPath.
 */
export function resolveSocketPath(env: NodeJS.ProcessEnv = process.env): string {
  const raw = env.AGM_BUS_SOCKET ?? "~/.agm/bus.sock";
  if (raw.startsWith("~/")) {
    return resolve(homedir(), raw.slice(2));
  }
  return raw;
}

export interface BrokerClientOptions {
  sessionID: string;
  socketPath?: string;
  /**
   * reconnectDelayMs controls how long we wait before reconnecting after a
   * socket error or EOF. The broker is expected to be colocated on the
   * same machine and ~always available; short delays are appropriate.
   */
  reconnectDelayMs?: number;
}

/**
 * BrokerClient events:
 *   - "frame": (frame: Frame) => void   # any non-ack, non-welcome frame
 *   - "connected": () => void           # Welcome received
 *   - "disconnected": (err?: Error)     # socket closed or errored
 */
export declare interface BrokerClient {
  on(event: "frame", listener: (frame: Frame) => void): this;
  on(event: "connected", listener: () => void): this;
  on(event: "disconnected", listener: (err?: Error) => void): this;
  emit(event: "frame", frame: Frame): boolean;
  emit(event: "connected"): boolean;
  emit(event: "disconnected", err?: Error): boolean;
}

export class BrokerClient extends EventEmitter {
  private readonly sessionID: string;
  private readonly socketPath: string;
  private readonly reconnectDelayMs: number;

  private socket: Socket | null = null;
  private reader = new FrameReader();
  private closedByCaller = false;
  private connected = false;

  constructor(opts: BrokerClientOptions) {
    super();
    if (!opts.sessionID) {
      throw new Error("BrokerClient: sessionID is required");
    }
    this.sessionID = opts.sessionID;
    this.socketPath = opts.socketPath ?? resolveSocketPath();
    this.reconnectDelayMs = opts.reconnectDelayMs ?? 500;
  }

  /** connect opens the socket and sends Hello. Subsequent reconnects are
   *  automatic unless close() was called. */
  connect(): void {
    this.closedByCaller = false;
    this.openSocket();
  }

  /** close stops auto-reconnect and terminates the current connection. */
  close(): void {
    this.closedByCaller = true;
    if (this.socket) {
      this.socket.end();
      this.socket = null;
    }
  }

  /**
   * sendRaw writes an arbitrary frame. Caller is responsible for setting
   * Type; From will be overwritten by the server so there's no need to
   * set it. Returns false if the socket isn't connected (frame dropped —
   * caller should buffer if durability matters; the broker's own queue
   * will pick up the slack for offline targets, but can't help if the
   * local socket is down).
   */
  sendRaw(frame: Frame): boolean {
    if (!this.socket || !this.connected) {
      return false;
    }
    this.socket.write(encodeFrame(frame));
    return true;
  }

  /** send is the convenience helper for FrameSend — the A2A outbound path. */
  send(to: string, text: string, extra?: Record<string, string>): boolean {
    return this.sendRaw({
      type: "send",
      to,
      text,
      extra,
      id: generateFrameID(),
    });
  }

  /** sendPermissionVerdict relays an allow/deny back to a blocked worker. */
  sendPermissionVerdict(id: string, to: string, verdict: "allow" | "deny"): boolean {
    return this.sendRaw({
      type: "permission_verdict",
      id,
      to,
      verdict,
    });
  }

  private openSocket(): void {
    if (this.socket) {
      return;
    }
    const sock = createConnection(this.socketPath);
    this.socket = sock;
    this.connected = false;
    this.reader = new FrameReader();

    sock.on("connect", () => {
      // Hello. The server validates From matches the authenticated session,
      // which for unix-socket + no auth is just whatever we claim here.
      sock.write(encodeFrame({ type: "hello", from: this.sessionID }));
    });

    sock.on("data", (chunk) => {
      let frames: Frame[];
      try {
        frames = this.reader.push(chunk);
      } catch (err) {
        // Corrupt stream — drop the connection and retry.
        sock.destroy(err instanceof Error ? err : new Error(String(err)));
        return;
      }
      for (const f of frames) {
        this.handleFrame(f);
      }
    });

    sock.on("error", (err) => {
      this.connected = false;
      this.emit("disconnected", err);
    });
    sock.on("close", () => {
      this.connected = false;
      this.socket = null;
      this.emit("disconnected");
      if (!this.closedByCaller) {
        setTimeout(() => this.openSocket(), this.reconnectDelayMs);
      }
    });
  }

  private handleFrame(f: Frame): void {
    switch (f.type) {
      case "welcome":
        this.connected = true;
        this.emit("connected");
        return;
      case "ack":
      case "error":
        // Acks and errors could carry correlation ids we care about later;
        // for now, expose them as normal frames so callers can inspect.
        this.emit("frame", f);
        return;
      default:
        this.emit("frame", f);
    }
  }
}

/**
 * generateFrameID produces a short-but-unique-enough id. Crypto RNG for
 * uniqueness; time prefix keeps them roughly ordered in logs.
 */
export function generateFrameID(): string {
  const now = Date.now().toString(36);
  const rnd = Math.random().toString(36).slice(2, 8);
  return `${now}-${rnd}`;
}
