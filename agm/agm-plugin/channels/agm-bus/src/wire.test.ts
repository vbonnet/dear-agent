// Pure-TypeScript tests for the wire protocol. Run with `node --test` after
// `npm run build`. Covers the frame encoder/decoder round trip and the
// streaming FrameReader's handling of partial chunks — the most likely
// source of subtle bugs (and the part the broker-client depends on).

import { strict as assert } from "node:assert";
import { test } from "node:test";

import { FrameReader, encodeFrame } from "./wire.js";

test("encodeFrame emits one JSON line with trailing newline", () => {
  const s = encodeFrame({ type: "hello", from: "s1" });
  assert.equal(s.endsWith("\n"), true);
  assert.equal(s.split("\n").length, 2);
  const parsed = JSON.parse(s.trim());
  assert.equal(parsed.type, "hello");
  assert.equal(parsed.from, "s1");
});

test("FrameReader surfaces whole frames across multiple chunks", () => {
  const reader = new FrameReader();
  // Split a two-frame payload at awkward boundaries.
  const payload = encodeFrame({ type: "hello", from: "s1" }) +
    encodeFrame({ type: "deliver", to: "s2", text: "hi" });
  const mid = Math.floor(payload.length / 2);
  const first = reader.push(payload.slice(0, mid));
  // The boundary might land mid-frame → first should hold 0 or 1 frames.
  const second = reader.push(payload.slice(mid));
  const all = [...first, ...second];
  assert.equal(all.length, 2);
  assert.equal(all[0].type, "hello");
  assert.equal(all[1].type, "deliver");
  assert.equal(all[1].text, "hi");
});

test("FrameReader ignores blank lines between frames", () => {
  const reader = new FrameReader();
  const frames = reader.push("\n\n" + encodeFrame({ type: "welcome" }) + "\n\n");
  assert.equal(frames.length, 1);
  assert.equal(frames[0].type, "welcome");
});

test("FrameReader throws on malformed JSON", () => {
  const reader = new FrameReader();
  assert.throws(() => reader.push("not-json\n"));
});
