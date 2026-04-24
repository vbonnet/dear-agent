/**
 * Tests for Anthropic client
 */

import { describe, it, expect } from 'vitest';
import {
  createAnthropicReviewer,
  AnthropicClientError,
  ANTHROPIC_ERROR_CODES,
} from '../src/anthropic-client.js';
import type { Persona, PersonaReviewInput } from '../src/types.js';

describe('anthropic-client', () => {
  const mockPersona: Persona = {
    name: 'test-persona',
    displayName: 'Test Persona',
    version: '1.0.0',
    description: 'Test persona',
    focusAreas: ['testing'],
    prompt: 'You are a test reviewer',
  };

  describe('createAnthropicReviewer', () => {
    it('should throw without API key', () => {
      expect(() => createAnthropicReviewer({ apiKey: '' })).toThrow(
        AnthropicClientError
      );
      expect(() => createAnthropicReviewer({ apiKey: '' })).toThrow(/API key is required/);
    });

    it('should create reviewer with valid config', () => {
      const reviewer = createAnthropicReviewer({
        apiKey: 'test-key',
        model: 'claude-3-5-sonnet-20241022',
      });

      expect(reviewer).toBeDefined();
      expect(typeof reviewer).toBe('function');
    });

    it('should use default model if not specified', () => {
      const reviewer = createAnthropicReviewer({
        apiKey: 'test-key',
      });

      expect(reviewer).toBeDefined();
    });
  });

  // Note: We skip integration tests that require real API calls
  // The reviewer function is tested via the review-engine tests with mocks
});
