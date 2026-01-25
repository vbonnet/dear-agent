import { PlatformAdapter } from './PlatformAdapter';
import { ClaudeCodeAdapter } from './ClaudeCodeAdapter';
import { GeminiAdapter } from './GeminiAdapter';

/**
 * Factory for creating platform adapters
 *
 * Usage:
 *   const adapter = AdapterFactory.create('claude');
 *   await adapter.configure(servers);
 */
export class AdapterFactory {
  /**
   * Create platform adapter for specified platform
   *
   * @param platform - Platform name ('claude', 'gemini')
   * @returns PlatformAdapter instance
   * @throws {Error} - If platform not supported
   */
  static create(platform: string): PlatformAdapter {
    switch (platform.toLowerCase()) {
      case 'claude':
      case 'claude-code':
        return new ClaudeCodeAdapter();

      case 'gemini':
        return new GeminiAdapter();

      default:
        throw new Error(
          `Unsupported platform: ${platform}. Supported: claude, gemini`
        );
    }
  }
}
