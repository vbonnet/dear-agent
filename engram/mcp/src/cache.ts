/**
 * TTL-based cache for MCP server tool results.
 *
 * Provides time-based expiry with configurable TTL per entry.
 * Optionally watches files for invalidation using fs.watch.
 */

import { watch, FSWatcher, existsSync, statSync } from 'fs';

interface CacheEntry<T> {
  value: T;
  expiresAt: number; // epoch ms
}

export interface CacheOptions {
  /** Default TTL in milliseconds (default: 30000 = 30s) */
  defaultTTLMs?: number;
  /** Maximum number of entries (default: 100) */
  maxEntries?: number;
}

export class TTLCache<T = string> {
  private entries = new Map<string, CacheEntry<T>>();
  private watchers = new Map<string, FSWatcher>();
  private defaultTTLMs: number;
  private maxEntries: number;

  constructor(options: CacheOptions = {}) {
    this.defaultTTLMs = options.defaultTTLMs ?? 30_000;
    this.maxEntries = options.maxEntries ?? 100;
  }

  /**
   * Get a cached value. Returns undefined if missing or expired.
   */
  get(key: string): T | undefined {
    const entry = this.entries.get(key);
    if (!entry) return undefined;

    if (Date.now() > entry.expiresAt) {
      this.entries.delete(key);
      return undefined;
    }

    return entry.value;
  }

  /**
   * Set a cached value with optional custom TTL.
   */
  set(key: string, value: T, ttlMs?: number): void {
    // Evict expired entries if at capacity
    if (this.entries.size >= this.maxEntries) {
      this.evictExpired();
    }

    // If still at capacity, evict oldest
    if (this.entries.size >= this.maxEntries) {
      const oldest = this.entries.keys().next().value;
      if (oldest !== undefined) {
        this.entries.delete(oldest);
      }
    }

    this.entries.set(key, {
      value,
      expiresAt: Date.now() + (ttlMs ?? this.defaultTTLMs),
    });
  }

  /**
   * Invalidate a specific key.
   */
  invalidate(key: string): boolean {
    return this.entries.delete(key);
  }

  /**
   * Invalidate all entries whose keys start with prefix.
   */
  invalidateByPrefix(prefix: string): number {
    let count = 0;
    for (const key of this.entries.keys()) {
      if (key.startsWith(prefix)) {
        this.entries.delete(key);
        count++;
      }
    }
    return count;
  }

  /**
   * Clear all cached entries.
   */
  clear(): void {
    this.entries.clear();
  }

  /**
   * Watch a file path; invalidate all entries with given prefix when file changes.
   * Returns true if watcher was set up, false if path doesn't exist.
   */
  watchFile(filePath: string, invalidatePrefix: string): boolean {
    if (!existsSync(filePath)) return false;

    // Don't double-watch
    if (this.watchers.has(filePath)) return true;

    try {
      const watcher = watch(filePath, { persistent: false }, (_event) => {
        this.invalidateByPrefix(invalidatePrefix);
      });

      // Handle watcher errors gracefully (file deleted, etc.)
      watcher.on('error', () => {
        this.unwatchFile(filePath);
      });

      this.watchers.set(filePath, watcher);
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Stop watching a file.
   */
  unwatchFile(filePath: string): void {
    const watcher = this.watchers.get(filePath);
    if (watcher) {
      watcher.close();
      this.watchers.delete(filePath);
    }
  }

  /**
   * Close all file watchers and clear cache.
   */
  destroy(): void {
    for (const [path, watcher] of this.watchers) {
      watcher.close();
    }
    this.watchers.clear();
    this.entries.clear();
  }

  /**
   * Get cache statistics.
   */
  stats(): { size: number; watcherCount: number } {
    return {
      size: this.entries.size,
      watcherCount: this.watchers.size,
    };
  }

  /**
   * Remove all expired entries.
   */
  private evictExpired(): void {
    const now = Date.now();
    for (const [key, entry] of this.entries) {
      if (now > entry.expiresAt) {
        this.entries.delete(key);
      }
    }
  }
}
