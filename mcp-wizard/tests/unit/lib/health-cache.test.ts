/**
 * Unit Tests for Health Cache
 *
 * Tests caching functionality for health check results
 *
 * Requirements:
 * - Cache health check results for 5 minutes
 * - Return cached results when not expired
 * - Support force bypass of cache
 * - Handle cache expiration correctly
 *
 * Coverage Target: 90%+ statement coverage
 */

import { getCached, setCached, clearCache } from '../../../src/lib/health-cache';
import { HealthCheckResult } from '../../../src/lib/health-checks';

describe('Health Cache', () => {
  beforeEach(() => {
    // Clear cache before each test
    clearCache();
  });

  afterEach(() => {
    // Ensure clean state after each test
    clearCache();
  });

  describe('setCached() and getCached()', () => {
    it('should store and retrieve cached results', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', result);
      const cached = getCached('Token Health');

      // Assert
      expect(cached).toBeDefined();
      expect(cached?.name).toBe('Token Health');
      expect(cached?.status).toBe('healthy');
      expect(cached?.message).toBe('Token valid');
    });

    it('should return null for non-existent cache entry', () => {
      // Execute
      const cached = getCached('Nonexistent Check');

      // Assert
      expect(cached).toBeNull();
    });

    it('should allow multiple cache entries', () => {
      // Setup
      const tokenResult: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      const mcpResult: HealthCheckResult = {
        name: 'MCP Processes',
        status: 'healthy',
        message: 'All processes running',
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', tokenResult);
      setCached('MCP Processes', mcpResult);

      // Assert
      expect(getCached('Token Health')?.name).toBe('Token Health');
      expect(getCached('MCP Processes')?.name).toBe('MCP Processes');
    });

    it('should overwrite existing cache entry with new data', () => {
      // Setup
      const originalResult: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Original message',
        last_check: new Date(),
      };

      const updatedResult: HealthCheckResult = {
        name: 'Token Health',
        status: 'degraded',
        message: 'Updated message',
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', originalResult);
      setCached('Token Health', updatedResult);
      const cached = getCached('Token Health');

      // Assert
      expect(cached?.status).toBe('degraded');
      expect(cached?.message).toBe('Updated message');
    });
  });

  describe('Cache expiration (TTL)', () => {
    it('should return cached result within TTL', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', result);
      const cached = getCached('Token Health');

      // Assert
      expect(cached).not.toBeNull();
    });

    it('should return null for expired cache entry', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      // Execute: Set cache and manually expire it
      setCached('Token Health', result);

      // Mock Date.now() to simulate time passing (>5 minutes = 300000ms)
      const originalDateNow = Date.now;
      Date.now = jest.fn(() => originalDateNow() + 310000); // 5min 10sec later

      const cached = getCached('Token Health');

      // Cleanup
      Date.now = originalDateNow;

      // Assert
      expect(cached).toBeNull();
    });

    it('should clean up expired cache entries on access', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', result);

      // Fast-forward time
      const originalDateNow = Date.now;
      Date.now = jest.fn(() => originalDateNow() + 310000);

      getCached('Token Health'); // Should delete expired entry

      // Restore time and try again
      Date.now = originalDateNow;
      const cached = getCached('Token Health');

      // Cleanup
      Date.now = originalDateNow;

      // Assert
      expect(cached).toBeNull();
    });
  });

  describe('Force bypass', () => {
    it('should return null when force=true', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', result);
      const cached = getCached('Token Health', true); // force=true

      // Assert
      expect(cached).toBeNull();
    });

    it('should return cached result when force=false', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', result);
      const cached = getCached('Token Health', false); // force=false

      // Assert
      expect(cached).not.toBeNull();
      expect(cached?.name).toBe('Token Health');
    });
  });

  describe('clearCache()', () => {
    it('should clear all cache entries', () => {
      // Setup
      const tokenResult: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      const mcpResult: HealthCheckResult = {
        name: 'MCP Processes',
        status: 'healthy',
        message: 'All running',
        last_check: new Date(),
      };

      setCached('Token Health', tokenResult);
      setCached('MCP Processes', mcpResult);

      // Execute
      clearCache();

      // Assert
      expect(getCached('Token Health')).toBeNull();
      expect(getCached('MCP Processes')).toBeNull();
    });

    it('should allow adding entries after clear', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        last_check: new Date(),
      };

      // Execute
      clearCache();
      setCached('Token Health', result);
      const cached = getCached('Token Health');

      // Assert
      expect(cached).not.toBeNull();
    });
  });

  describe('Edge cases', () => {
    it('should handle undefined check names gracefully', () => {
      // Execute & Assert
      expect(getCached('')).toBeNull();
    });

    it('should handle results with details field', () => {
      // Setup
      const result: HealthCheckResult = {
        name: 'Token Health',
        status: 'degraded',
        message: 'Token expires soon',
        details: { ttlMinutes: 3, expiresAt: new Date().toISOString() },
        last_check: new Date(),
      };

      // Execute
      setCached('Token Health', result);
      const cached = getCached('Token Health');

      // Assert
      expect(cached?.details).toBeDefined();
      expect(cached?.details?.ttlMinutes).toBe(3);
    });
  });
});
