// Wire protocol types mirror agm/internal/bus/wire.go. Keep in sync when
// changing fields — the broker rejects unknown types and silently ignores
// unknown fields, so adding a field is safe but renaming is breaking.

export type FrameType =
  | "hello"
  | "welcome"
  | "send"
  | "deliver"
  | "ack"
  | "error"
  | "permission_request"
  | "permission_verdict"
  | "bye";

export type ErrorCode =
  | "unknown_target"
  | "not_allowed"
  | "bad_frame"
  | "internal";

export interface Frame {
  type: FrameType;
  id?: string;
  from?: string;
  to?: string;
  text?: string;
  ts?: string;

  // Permission relay.
  tool_name?: string;
  description?: string;
  input_preview?: string;
  verdict?: "allow" | "deny";

  // Error frame.
  code?: ErrorCode;
  message?: string;

  // Forwarded channel metadata (chat_id, severity, etc.).
  extra?: Record<string, string>;
}

/**
 * encodeFrame serializes a frame to a newline-terminated UTF-8 buffer.
 * Callers writing to a stream must write the returned Buffer as a unit —
 * interleaving partial frames from concurrent goroutines would desync the
 * broker's line-oriented parser.
 */
export function encodeFrame(f: Frame): string {
  return JSON.stringify(f) + "\n";
}

/**
 * FrameReader consumes newline-delimited frames from an incremental byte
 * stream. The socket's 'data' event fires with arbitrary-sized chunks, so
 * we buffer until we see a newline. Malformed JSON throws; the caller
 * decides whether to close the connection or skip.
 */
export class FrameReader {
  private buf = "";

  /**
   * push a chunk and yield any complete frames it revealed. Returns an
   * array (possibly empty) — never a single frame — so the caller can
   * iterate without special-casing "nothing yet".
   */
  push(chunk: string | Buffer): Frame[] {
    this.buf += typeof chunk === "string" ? chunk : chunk.toString("utf8");
    const out: Frame[] = [];
    let nl = this.buf.indexOf("\n");
    while (nl !== -1) {
      const line = this.buf.slice(0, nl).trim();
      this.buf = this.buf.slice(nl + 1);
      if (line.length > 0) {
        out.push(JSON.parse(line) as Frame);
      }
      nl = this.buf.indexOf("\n");
    }
    return out;
  }
}
