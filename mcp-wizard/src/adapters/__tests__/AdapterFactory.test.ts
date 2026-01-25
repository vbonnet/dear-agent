import { AdapterFactory } from '../AdapterFactory';
import { ClaudeCodeAdapter } from '../ClaudeCodeAdapter';
import { GeminiAdapter } from '../GeminiAdapter';

describe('AdapterFactory', () => {
  describe('create', () => {
    it('should return ClaudeCodeAdapter for "claude"', () => {
      const adapter = AdapterFactory.create('claude');

      expect(adapter).toBeInstanceOf(ClaudeCodeAdapter);
    });

    it('should return ClaudeCodeAdapter for "claude-code"', () => {
      const adapter = AdapterFactory.create('claude-code');

      expect(adapter).toBeInstanceOf(ClaudeCodeAdapter);
    });

    it('should return ClaudeCodeAdapter for "CLAUDE" (case insensitive)', () => {
      const adapter = AdapterFactory.create('CLAUDE');

      expect(adapter).toBeInstanceOf(ClaudeCodeAdapter);
    });

    it('should return GeminiAdapter for "gemini"', () => {
      const adapter = AdapterFactory.create('gemini');

      expect(adapter).toBeInstanceOf(GeminiAdapter);
    });

    it('should return GeminiAdapter for "GEMINI" (case insensitive)', () => {
      const adapter = AdapterFactory.create('GEMINI');

      expect(adapter).toBeInstanceOf(GeminiAdapter);
    });

    it('should throw error for unsupported platform', () => {
      expect(() => AdapterFactory.create('cursor')).toThrow(
        'Unsupported platform: cursor. Supported: claude, gemini'
      );
    });

    it('should throw error for empty string', () => {
      expect(() => AdapterFactory.create('')).toThrow(
        'Unsupported platform: . Supported: claude, gemini'
      );
    });

    it('should throw error for invalid platform name', () => {
      expect(() => AdapterFactory.create('invalid-platform')).toThrow(
        'Unsupported platform: invalid-platform. Supported: claude, gemini'
      );
    });
  });
});
