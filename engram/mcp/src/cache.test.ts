/**
 * Tests for TTLCache.
 *
 * Run: npx tsx --test src/cache.test.ts
 */

import { describe, it, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { writeFileSync, unlinkSync, mkdtempSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';
import { TTLCache } from './cache.js';

describe('TTLCache', () => {
  it('get returns undefined for missing key', () => {
    const cache = new TTLCache();
    assert.equal(cache.get('missing'), undefined);
  });

  it('set and get round-trips', () => {
    const cache = new TTLCache();
    cache.set('k', 'v');
    assert.equal(cache.get('k'), 'v');
  });

  it('expired entries return undefined', async () => {
    const cache = new TTLCache({ defaultTTLMs: 50 });
    cache.set('k', 'v');
    assert.equal(cache.get('k'), 'v');

    await new Promise((r) => setTimeout(r, 80));
    assert.equal(cache.get('k'), undefined);
  });

  it('custom TTL per entry', async () => {
    const cache = new TTLCache({ defaultTTLMs: 5000 });
    cache.set('short', 'v', 50);
    cache.set('long', 'v', 5000);

    await new Promise((r) => setTimeout(r, 80));
    assert.equal(cache.get('short'), undefined);
    assert.equal(cache.get('long'), 'v');
    cache.destroy();
  });

  it('invalidate removes specific key', () => {
    const cache = new TTLCache();
    cache.set('a', '1');
    cache.set('b', '2');

    assert.equal(cache.invalidate('a'), true);
    assert.equal(cache.get('a'), undefined);
    assert.equal(cache.get('b'), '2');
  });

  it('invalidateByPrefix removes matching keys', () => {
    const cache = new TTLCache();
    cache.set('retrieve:foo', '1');
    cache.set('retrieve:bar', '2');
    cache.set('plugins:list', '3');

    assert.equal(cache.invalidateByPrefix('retrieve:'), 2);
    assert.equal(cache.get('retrieve:foo'), undefined);
    assert.equal(cache.get('plugins:list'), '3');
  });

  it('clear removes all entries', () => {
    const cache = new TTLCache();
    cache.set('a', '1');
    cache.set('b', '2');
    cache.clear();

    assert.equal(cache.get('a'), undefined);
    assert.equal(cache.get('b'), undefined);
    assert.deepEqual(cache.stats(), { size: 0, watcherCount: 0 });
  });

  it('maxEntries evicts oldest on overflow', () => {
    const cache = new TTLCache({ maxEntries: 3 });
    cache.set('a', '1');
    cache.set('b', '2');
    cache.set('c', '3');
    cache.set('d', '4'); // should evict 'a'

    assert.equal(cache.get('a'), undefined);
    assert.equal(cache.get('d'), '4');
    assert.equal(cache.stats().size, 3);
  });

  it('stats returns correct counts', () => {
    const cache = new TTLCache();
    cache.set('a', '1');
    cache.set('b', '2');

    const stats = cache.stats();
    assert.equal(stats.size, 2);
    assert.equal(stats.watcherCount, 0);
    cache.destroy();
  });

  it('watchFile returns false for nonexistent path', () => {
    const cache = new TTLCache();
    assert.equal(cache.watchFile('/nonexistent/path', 'prefix:'), false);
    cache.destroy();
  });

  it('watchFile invalidates cache on file change', async () => {
    const cache = new TTLCache({ defaultTTLMs: 60_000 });
    const tmpDir = mkdtempSync(join(tmpdir(), 'cache-test-'));
    const filePath = join(tmpDir, 'watched.txt');

    writeFileSync(filePath, 'initial');

    cache.set('wayfinder:/proj', 'cached-status');
    assert.equal(cache.watchFile(filePath, 'wayfinder:'), true);

    // Modify the file
    writeFileSync(filePath, 'changed');

    // Wait for fs.watch to fire
    await new Promise((r) => setTimeout(r, 200));

    assert.equal(cache.get('wayfinder:/proj'), undefined);

    cache.destroy();
    try { unlinkSync(filePath); } catch {}
  });

  it('destroy cleans up watchers', () => {
    const cache = new TTLCache();
    const tmpDir = mkdtempSync(join(tmpdir(), 'cache-test-'));
    const filePath = join(tmpDir, 'watched.txt');
    writeFileSync(filePath, 'test');

    cache.watchFile(filePath, 'prefix:');
    assert.equal(cache.stats().watcherCount, 1);

    cache.destroy();
    assert.equal(cache.stats().watcherCount, 0);
    assert.equal(cache.stats().size, 0);

    try { unlinkSync(filePath); } catch {}
  });
});
