/**
 * Tests for VertexAI Claude client
 */

import { describe, it, expect } from 'vitest';
import {
  createVertexAIClaudeReviewer,
  VertexAIClaudeClientError,
  VERTEX_CLAUDE_ERROR_CODES,
} from '../../src/vertex-ai-claude-client.js';
import type { Persona } from '../../src/types.js';

describe('vertex-ai-claude-client', () => {
  const mockPersona: Persona = {
    name: 'test-persona',
    displayName: 'Test Persona',
    version: '1.0.0',
    description: 'Test persona',
    focusAreas: ['testing'],
    prompt: 'You are a test reviewer',
  };

  describe('createVertexAIClaudeReviewer', () => {
    it('should throw without project ID', () => {
      expect(() => createVertexAIClaudeReviewer({ projectId: '' })).toThrow(
        VertexAIClaudeClientError
      );
      expect(() => createVertexAIClaudeReviewer({ projectId: '' })).toThrow(
        /project ID is required/
      );
    });

    it('should create reviewer with valid config', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
        location: 'us-east5',
        model: 'claude-sonnet-4-5@20250929',
      });

      expect(reviewer).toBeDefined();
      expect(typeof reviewer).toBe('function');
    });

    it('should use default model if not specified', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
      });

      expect(reviewer).toBeDefined();
    });

    it('should use default location (us-east5) if not specified', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
      });

      expect(reviewer).toBeDefined();
    });

    it('should accept custom location', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
        location: 'europe-west4',
      });

      expect(reviewer).toBeDefined();
    });

    it('should accept all supported Claude models', () => {
      const models = [
        'claude-sonnet-4-5@20250929',
        'claude-haiku-4-5@20251001',
        'claude-opus-4-6@20260205',
      ];

      models.forEach(model => {
        const reviewer = createVertexAIClaudeReviewer({
          projectId: 'test-project',
          model,
        });
        expect(reviewer).toBeDefined();
      });
    });

    it('should accept custom temperature', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
        temperature: 0.5,
      });

      expect(reviewer).toBeDefined();
    });

    it('should accept custom maxTokens', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
        maxTokens: 8192,
      });

      expect(reviewer).toBeDefined();
    });

    it('should accept custom timeout', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
        timeout: 120000,
      });

      expect(reviewer).toBeDefined();
    });

    // New tests for keyFilename (credentials) support
    it('should accept keyFilename parameter (backward compatible)', () => {
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
        keyFilename: undefined,
      });

      expect(reviewer).toBeDefined();
    });

    it('should throw error for non-existent credential file', () => {
      expect(() =>
        createVertexAIClaudeReviewer({
          projectId: 'test-project',
          keyFilename: '/nonexistent/path/to/credentials.json',
        })
      ).toThrow(VertexAIClaudeClientError);

      expect(() =>
        createVertexAIClaudeReviewer({
          projectId: 'test-project',
          keyFilename: '/nonexistent/path/to/credentials.json',
        })
      ).toThrow(/Credential file not found/);
    });

    it('should throw error for malformed JSON credential file', () => {
      // Create a temporary file with invalid JSON
      const fs = require('fs');
      const path = require('path');
      const tmpDir = require('os').tmpdir();
      const invalidJsonFile = path.join(tmpDir, 'invalid-credentials.json');

      try {
        fs.writeFileSync(invalidJsonFile, '{invalid json content');

        expect(() =>
          createVertexAIClaudeReviewer({
            projectId: 'test-project',
            keyFilename: invalidJsonFile,
          })
        ).toThrow(VertexAIClaudeClientError);

        expect(() =>
          createVertexAIClaudeReviewer({
            projectId: 'test-project',
            keyFilename: invalidJsonFile,
          })
        ).toThrow(/Invalid credential file format/);
      } finally {
        // Cleanup
        if (fs.existsSync(invalidJsonFile)) {
          fs.unlinkSync(invalidJsonFile);
        }
      }
    });

    it('should accept valid JSON credential file', () => {
      // Create a temporary file with valid JSON
      const fs = require('fs');
      const path = require('path');
      const tmpDir = require('os').tmpdir();
      const validJsonFile = path.join(tmpDir, 'valid-credentials.json');

      try {
        fs.writeFileSync(
          validJsonFile,
          JSON.stringify({
            type: 'service_account',
            project_id: 'test-project',
            private_key_id: 'key-id',
            private_key: '-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n',
            client_email: 'test@test-project.iam.gserviceaccount.com',
            client_id: '123456789',
            auth_uri: 'https://accounts.google.com/o/oauth2/auth',
            token_uri: 'https://oauth2.googleapis.com/token',
          })
        );

        const reviewer = createVertexAIClaudeReviewer({
          projectId: 'test-project',
          keyFilename: validJsonFile,
        });

        expect(reviewer).toBeDefined();
      } finally {
        // Cleanup
        if (fs.existsSync(validJsonFile)) {
          fs.unlinkSync(validJsonFile);
        }
      }
    });

    it('should work without keyFilename (backward compatibility with ADC)', () => {
      // Should not throw when keyFilename is not provided
      const reviewer = createVertexAIClaudeReviewer({
        projectId: 'test-project',
      });

      expect(reviewer).toBeDefined();
    });
  });

  describe('error codes', () => {
    it('should have all expected error codes', () => {
      expect(VERTEX_CLAUDE_ERROR_CODES.CREDENTIALS_MISSING).toBe('VERTEX_CLAUDE_5001');
      expect(VERTEX_CLAUDE_ERROR_CODES.API_ERROR).toBe('VERTEX_CLAUDE_5002');
      expect(VERTEX_CLAUDE_ERROR_CODES.RATE_LIMIT).toBe('VERTEX_CLAUDE_5003');
      expect(VERTEX_CLAUDE_ERROR_CODES.PARSE_ERROR).toBe('VERTEX_CLAUDE_5004');
      expect(VERTEX_CLAUDE_ERROR_CODES.TIMEOUT).toBe('VERTEX_CLAUDE_5005');
      expect(VERTEX_CLAUDE_ERROR_CODES.REGION_ERROR).toBe('VERTEX_CLAUDE_5006');
    });
  });

  describe('VertexAIClaudeClientError', () => {
    it('should create error with code and message', () => {
      const error = new VertexAIClaudeClientError(
        VERTEX_CLAUDE_ERROR_CODES.API_ERROR,
        'Test error'
      );

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(VertexAIClaudeClientError);
      expect(error.code).toBe('VERTEX_CLAUDE_5002');
      expect(error.message).toBe('Test error');
      expect(error.name).toBe('VertexAIClaudeClientError');
    });

    it('should accept optional details', () => {
      const details = { foo: 'bar' };
      const error = new VertexAIClaudeClientError(
        VERTEX_CLAUDE_ERROR_CODES.PARSE_ERROR,
        'Parse failed',
        details
      );

      expect(error.details).toEqual(details);
    });
  });

  // Note: Integration tests that require real VertexAI API calls are skipped
  // The reviewer function is tested via integration tests with actual API access
  // when VERTEX_PROJECT_ID environment variable is set
});
