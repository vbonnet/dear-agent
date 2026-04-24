/**
 * Tests for persona loader
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mkdir, writeFile, rm } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';
import {
  resolvePersonaPaths,
  loadPersonaFile,
  validatePersona,
  loadPersonasFromDir,
  loadPersonas,
  loadSpecificPersonas,
  PersonaError,
  PERSONA_ERROR_CODES,
  countTokens,
  validateCacheEligibility,
  generateCacheKey,
  enrichPersonaWithCacheMetadata,
} from '../src/persona-loader.js';
import type { Persona } from '../src/types.js';

describe('persona-loader', () => {
  let testDir: string;

  beforeEach(async () => {
    // Create temporary test directory
    testDir = join(tmpdir(), `multi-persona-review-test-${Date.now()}`);
    await mkdir(testDir, { recursive: true });
  });

  afterEach(async () => {
    // Clean up test directory
    try {
      await rm(testDir, { recursive: true, force: true });
    } catch (error) {
      // Ignore cleanup errors
    }
  });

  describe('resolvePersonaPaths', () => {
    it('should resolve tilde paths', () => {
      const paths = resolvePersonaPaths(['~/.wayfinder/personas']);
      expect(paths[0]).toContain('.wayfinder/personas');
      expect(paths[0]).not.toContain('~');
    });

    it('should resolve relative paths', () => {
      const paths = resolvePersonaPaths(['.wayfinder/personas'], '/tmp/test/project');
      expect(paths[0]).toBe('/tmp/test/project/.wayfinder/personas');
    });

    it('should replace {company} placeholder', () => {
      const paths = resolvePersonaPaths(
        ['{company}/personas'],
        '/tmp/test/project',
        '/opt/company'
      );
      expect(paths[0]).toBe('/opt/company/personas');
    });

    it('should replace {core} placeholder', () => {
      const paths = resolvePersonaPaths(
        ['{core}/personas'],
        '/tmp/test/project',
        undefined,
        '/opt/wayfinder/core'
      );
      expect(paths[0]).toBe('/opt/wayfinder/core/personas');
    });

    it('should skip paths with missing placeholders', () => {
      const paths = resolvePersonaPaths(['{company}/personas', '.wayfinder/personas']);
      expect(paths).toHaveLength(1);
      expect(paths[0]).toContain('.wayfinder/personas');
    });
  });

  describe('validatePersona', () => {
    const validPersona: Persona = {
      name: 'test-persona',
      displayName: 'Test Persona',
      version: '1.0.0',
      description: 'A test persona',
      focusAreas: ['testing'],
      prompt: 'You are a test persona',
    };

    it('should accept valid persona', () => {
      expect(() => validatePersona(validPersona)).not.toThrow();
    });

    it('should reject non-object', () => {
      expect(() => validatePersona('not an object')).toThrow(PersonaError);
      expect(() => validatePersona('not an object')).toThrow(/must be an object/);
    });

    it('should reject missing name', () => {
      const persona = { ...validPersona, name: undefined };
      expect(() => validatePersona(persona as any)).toThrow(PersonaError);
      expect(() => validatePersona(persona as any)).toThrow(/must have a 'name'/);
    });

    it('should reject invalid name format', () => {
      const persona = { ...validPersona, name: 'TestPersona' };
      expect(() => validatePersona(persona)).toThrow(PersonaError);
      expect(() => validatePersona(persona)).toThrow(/must be lowercase kebab-case/);
    });

    it('should reject invalid version format', () => {
      const persona = { ...validPersona, version: '1.0' };
      expect(() => validatePersona(persona)).toThrow(PersonaError);
      expect(() => validatePersona(persona)).toThrow(/must be semver format/);
    });

    it('should reject invalid severity levels', () => {
      const persona = {
        ...validPersona,
        severityLevels: ['invalid' as any],
      };
      expect(() => validatePersona(persona)).toThrow(PersonaError);
      expect(() => validatePersona(persona)).toThrow(/invalid severity/);
    });

    it('should accept valid severity levels', () => {
      const persona = {
        ...validPersona,
        severityLevels: ['critical', 'high', 'medium'] as const,
      };
      expect(() => validatePersona(persona)).not.toThrow();
    });
  });

  describe('loadPersonaFile', () => {
    it('should load valid persona file', async () => {
      const personaPath = join(testDir, 'test-persona.yaml');
      const personaData = {
        name: 'test-persona',
        displayName: 'Test Persona',
        version: '1.0.0',
        description: 'A test persona',
        focusAreas: ['testing'],
        prompt: 'You are a test persona',
      };

      await writeFile(personaPath, JSON.stringify(personaData));

      const persona = await loadPersonaFile(personaPath);
      expect(persona.name).toBe('test-persona');
      expect(persona.displayName).toBe('Test Persona');
    });

    it('should throw on missing file', async () => {
      const personaPath = join(testDir, 'missing.yaml');

      await expect(loadPersonaFile(personaPath)).rejects.toThrow(PersonaError);
      await expect(loadPersonaFile(personaPath)).rejects.toThrow(/not found/);
    });

    it('should throw on invalid YAML', async () => {
      const personaPath = join(testDir, 'invalid.yaml');
      await writeFile(personaPath, 'invalid: yaml: content:');

      await expect(loadPersonaFile(personaPath)).rejects.toThrow(PersonaError);
    });

    it('should throw on invalid persona data', async () => {
      const personaPath = join(testDir, 'invalid-persona.yaml');
      await writeFile(personaPath, JSON.stringify({ name: 'invalid' }));

      await expect(loadPersonaFile(personaPath)).rejects.toThrow(PersonaError);
      await expect(loadPersonaFile(personaPath)).rejects.toThrow(/must have a 'displayName'/);
    });
  });

  describe('loadPersonasFromDir', () => {
    it('should load all personas from directory', async () => {
      const persona1 = {
        name: 'persona-1',
        displayName: 'Persona 1',
        version: '1.0.0',
        description: 'First persona',
        focusAreas: ['testing'],
        prompt: 'You are persona 1',
      };

      const persona2 = {
        name: 'persona-2',
        displayName: 'Persona 2',
        version: '1.0.0',
        description: 'Second persona',
        focusAreas: ['testing'],
        prompt: 'You are persona 2',
      };

      await writeFile(join(testDir, 'persona-1.yaml'), JSON.stringify(persona1));
      await writeFile(join(testDir, 'persona-2.yaml'), JSON.stringify(persona2));

      const personas = await loadPersonasFromDir(testDir);

      expect(personas.size).toBe(2);
      expect(personas.has('persona-1')).toBe(true);
      expect(personas.has('persona-2')).toBe(true);
    });

    it('should ignore non-YAML files', async () => {
      const persona = {
        name: 'test-persona',
        displayName: 'Test Persona',
        version: '1.0.0',
        description: 'A test persona',
        focusAreas: ['testing'],
        prompt: 'You are a test persona',
      };

      await writeFile(join(testDir, 'persona.yaml'), JSON.stringify(persona));
      await writeFile(join(testDir, 'readme.txt'), 'This is not a persona');
      await writeFile(join(testDir, 'data.json'), '{"not": "a persona"}');

      const personas = await loadPersonasFromDir(testDir);

      expect(personas.size).toBe(1);
      expect(personas.has('test-persona')).toBe(true);
    });

    it('should return empty map for non-existent directory', async () => {
      const personas = await loadPersonasFromDir(join(testDir, 'non-existent'));
      expect(personas.size).toBe(0);
    });

    it('should continue loading on individual file errors', async () => {
      const validPersona = {
        name: 'valid-persona',
        displayName: 'Valid Persona',
        version: '1.0.0',
        description: 'A valid persona',
        focusAreas: ['testing'],
        prompt: 'You are a valid persona',
      };

      await writeFile(join(testDir, 'valid.yaml'), JSON.stringify(validPersona));
      await writeFile(join(testDir, 'invalid.yaml'), 'invalid yaml content');

      const personas = await loadPersonasFromDir(testDir);

      expect(personas.size).toBe(1);
      expect(personas.has('valid-persona')).toBe(true);
    });
  });

  describe('loadPersonas', () => {
    it('should override personas from later search paths', async () => {
      const dir1 = join(testDir, 'dir1');
      const dir2 = join(testDir, 'dir2');

      await mkdir(dir1, { recursive: true });
      await mkdir(dir2, { recursive: true });

      // Same persona in both directories with different versions
      const persona1 = {
        name: 'test-persona',
        displayName: 'Test Persona',
        version: '1.0.0',
        description: 'Version 1',
        focusAreas: ['testing'],
        prompt: 'Version 1 prompt',
      };

      const persona2 = {
        name: 'test-persona',
        displayName: 'Test Persona Updated',
        version: '2.0.0',
        description: 'Version 2',
        focusAreas: ['testing', 'review'],
        prompt: 'Version 2 prompt',
      };

      await writeFile(join(dir1, 'test.yaml'), JSON.stringify(persona1));
      await writeFile(join(dir2, 'test.yaml'), JSON.stringify(persona2));

      const personas = await loadPersonas([dir1, dir2]);

      expect(personas.size).toBe(1);
      expect(personas.get('test-persona')?.version).toBe('2.0.0');
      expect(personas.get('test-persona')?.description).toBe('Version 2');
      expect(personas.get('test-persona')?.displayName).toBe('Test Persona Updated');
    });

    it('should not mutate input array', async () => {
      const dir1 = join(testDir, 'dir1');
      await mkdir(dir1, { recursive: true });

      const originalPaths = [dir1];
      const pathsCopy = [...originalPaths];

      await loadPersonas(originalPaths);

      expect(originalPaths).toEqual(pathsCopy);
    });
  });

  describe('loadSpecificPersonas', () => {
    it('should load requested personas', async () => {
      const persona1 = {
        name: 'persona-1',
        displayName: 'Persona 1',
        version: '1.0.0',
        description: 'First persona',
        focusAreas: ['testing'],
        prompt: 'You are persona 1',
      };

      const persona2 = {
        name: 'persona-2',
        displayName: 'Persona 2',
        version: '1.0.0',
        description: 'Second persona',
        focusAreas: ['testing'],
        prompt: 'You are persona 2',
      };

      await writeFile(join(testDir, 'persona-1.yaml'), JSON.stringify(persona1));
      await writeFile(join(testDir, 'persona-2.yaml'), JSON.stringify(persona2));

      const personas = await loadSpecificPersonas(['persona-1'], [testDir]);

      expect(personas).toHaveLength(1);
      expect(personas[0].name).toBe('persona-1');
    });

    it('should throw on missing persona', async () => {
      await expect(
        loadSpecificPersonas(['non-existent'], [testDir])
      ).rejects.toThrow(PersonaError);
      await expect(
        loadSpecificPersonas(['non-existent'], [testDir])
      ).rejects.toThrow(/not found/);
    });
  });

  describe('loadPersonaFile - .ai.md format', () => {
    it('should load .ai.md persona with frontmatter', async () => {
      const personaPath = join(testDir, 'test-persona.ai.md');
      const personaContent = `---
name: test-persona
displayName: Test Persona
version: 1.0.0
description: A test persona for .ai.md format
focusAreas:
  - testing
  - validation
severityLevels:
  - critical
  - high
---

# Test Persona

You are a test persona. Review code carefully.

## Focus Areas
- Testing
- Validation
`;

      await writeFile(personaPath, personaContent);

      const persona = await loadPersonaFile(personaPath);
      expect(persona.name).toBe('test-persona');
      expect(persona.displayName).toBe('Test Persona');
      expect(persona.version).toBe('1.0.0');
      expect(persona.focusAreas).toEqual(['testing', 'validation']);
      expect(persona.severityLevels).toEqual(['critical', 'high']);
      expect(persona.prompt).toContain('You are a test persona');
    });

    it('should throw on .ai.md without frontmatter delimiters', async () => {
      const personaPath = join(testDir, 'no-frontmatter.ai.md');
      const personaContent = `# Test Persona
No frontmatter here!`;

      await writeFile(personaPath, personaContent);

      await expect(loadPersonaFile(personaPath)).rejects.toThrow(PersonaError);
      await expect(loadPersonaFile(personaPath)).rejects.toThrow(/Invalid .ai.md format/);
    });

    it('should throw on .ai.md with malformed YAML frontmatter', async () => {
      const personaPath = join(testDir, 'bad-yaml.ai.md');
      const personaContent = `---
name: test
invalid yaml syntax here
---

# Content
`;

      await writeFile(personaPath, personaContent);

      await expect(loadPersonaFile(personaPath)).rejects.toThrow(PersonaError);
    });

    it('should handle .ai.md with minimal frontmatter', async () => {
      const personaPath = join(testDir, 'minimal.ai.md');
      const personaContent = `---
name: minimal-persona
displayName: Minimal Persona
version: 1.0.0
description: Minimal valid persona
focusAreas: []
---

# Minimal Persona
Basic prompt content.
`;

      await writeFile(personaPath, personaContent);

      const persona = await loadPersonaFile(personaPath);
      expect(persona.name).toBe('minimal-persona');
      expect(persona.focusAreas).toEqual([]);
      expect(persona.severityLevels).toBeUndefined();
    });
  });

  describe('loadPersonasFromDir - mixed formats and special directories', () => {
    it('should load both .yaml and .ai.md personas', async () => {
      const yamlPersona = {
        name: 'yaml-persona',
        displayName: 'YAML Persona',
        version: '1.0.0',
        description: 'YAML format persona',
        focusAreas: ['yaml'],
        prompt: 'YAML prompt',
      };

      const aiMdPersona = `---
name: aimd-persona
displayName: AI.md Persona
version: 1.0.0
description: AI.md format persona
focusAreas: [aimd]
---

# AI.md Prompt
`;

      await writeFile(join(testDir, 'yaml-persona.yaml'), JSON.stringify(yamlPersona));
      await writeFile(join(testDir, 'aimd-persona.ai.md'), aiMdPersona);

      const personas = await loadPersonasFromDir(testDir);

      expect(personas.size).toBe(2);
      expect(personas.has('yaml-persona')).toBe(true);
      expect(personas.has('aimd-persona')).toBe(true);
    });

    it('should skip .why.md files', async () => {
      const aiMdPersona = `---
name: test-persona
displayName: Test Persona
version: 1.0.0
description: Test
focusAreas: [test]
---

# Prompt
`;

      const whyMd = `# Why Test Persona

This explains why the persona exists.
`;

      await writeFile(join(testDir, 'test-persona.ai.md'), aiMdPersona);
      await writeFile(join(testDir, 'test-persona.why.md'), whyMd);

      const personas = await loadPersonasFromDir(testDir);

      expect(personas.size).toBe(1);
      expect(personas.has('test-persona')).toBe(true);
    });

    it('should skip _meta directories', async () => {
      const metaDir = join(testDir, '_meta');
      await mkdir(metaDir, { recursive: true });

      const metaFile = {
        version: '1.0.0',
        count: 0,
      };

      const validPersona = `---
name: valid-persona
displayName: Valid Persona
version: 1.0.0
description: Valid
focusAreas: [test]
---

# Prompt
`;

      await writeFile(join(metaDir, 'index.yaml'), JSON.stringify(metaFile));
      await writeFile(join(testDir, 'valid-persona.ai.md'), validPersona);

      const personas = await loadPersonasFromDir(testDir);

      expect(personas.size).toBe(1);
      expect(personas.has('valid-persona')).toBe(true);
    });

    it('should skip hidden directories', async () => {
      const hiddenDir = join(testDir, '.hidden');
      await mkdir(hiddenDir, { recursive: true });

      const hiddenPersona = {
        name: 'hidden-persona',
        displayName: 'Hidden',
        version: '1.0.0',
        description: 'Should not load',
        focusAreas: ['test'],
        prompt: 'Hidden',
      };

      await writeFile(join(hiddenDir, 'persona.yaml'), JSON.stringify(hiddenPersona));

      const personas = await loadPersonasFromDir(testDir);

      expect(personas.size).toBe(0);
    });
  });

  describe('Cache functionality', () => {
    describe('countTokens', () => {
      it('should estimate tokens for short text', () => {
        const text = 'Hello world';
        const tokens = countTokens(text);
        expect(tokens).toBeGreaterThan(0);
        expect(tokens).toBeLessThan(10);
      });

      it('should estimate tokens for multi-line text', () => {
        const text = 'Line 1\nLine 2\nLine 3';
        const tokens = countTokens(text);
        expect(tokens).toBeGreaterThanOrEqual(5);
      });

      it('should handle empty text', () => {
        const tokens = countTokens('');
        expect(tokens).toBe(0);
      });

      it('should produce consistent results', () => {
        const text = 'Consistent token counting test';
        const tokens1 = countTokens(text);
        const tokens2 = countTokens(text);
        expect(tokens1).toBe(tokens2);
      });

      it('should estimate more tokens for longer text', () => {
        const shortText = 'Short';
        const longText = 'This is a much longer text with many more words and characters';
        expect(countTokens(longText)).toBeGreaterThan(countTokens(shortText));
      });
    });

    describe('validateCacheEligibility', () => {
      it('should mark large persona as cache-eligible', () => {
        // Need 1024 tokens * 4 chars/token = 4096+ chars
        const largePrompt = 'word '.repeat(1100); // ~5500 chars -> ~1375 tokens
        const persona: Persona = {
          name: 'large-persona',
          displayName: 'Large Persona',
          version: '1.0.0',
          description: 'Large persona',
          focusAreas: ['testing'],
          prompt: largePrompt,
        };

        const eligible = validateCacheEligibility(persona);
        expect(eligible).toBe(true);
      });

      it('should mark small persona as not cache-eligible', () => {
        const persona: Persona = {
          name: 'small-persona',
          displayName: 'Small Persona',
          version: '1.0.0',
          description: 'Small persona',
          focusAreas: ['testing'],
          prompt: 'Very short prompt',
        };

        const eligible = validateCacheEligibility(persona);
        expect(eligible).toBe(false);
      });

      it('should respect custom token threshold', () => {
        const persona: Persona = {
          name: 'medium-persona',
          displayName: 'Medium Persona',
          version: '1.0.0',
          description: 'Medium persona',
          focusAreas: ['testing'],
          prompt: 'word '.repeat(150), // ~750 chars -> ~188 tokens
        };

        const notEligible = validateCacheEligibility(persona, { enableCaching: true, minTokens: 1024 });
        expect(notEligible).toBe(false);

        const eligible = validateCacheEligibility(persona, { enableCaching: true, minTokens: 100 });
        expect(eligible).toBe(true);
      });

      it('should return false when caching is disabled', () => {
        const largePrompt = 'word '.repeat(1100); // ~1375 tokens
        const persona: Persona = {
          name: 'large-persona',
          displayName: 'Large Persona',
          version: '1.0.0',
          description: 'Large persona',
          focusAreas: ['testing'],
          prompt: largePrompt,
        };

        const eligible = validateCacheEligibility(persona, { enableCaching: false, minTokens: 1024 });
        expect(eligible).toBe(false);
      });
    });

    describe('generateCacheKey', () => {
      const basePersona: Persona = {
        name: 'test-persona',
        displayName: 'Test Persona',
        version: '1.0.0',
        description: 'Test',
        focusAreas: ['testing'],
        prompt: 'Test prompt',
      };

      it('should generate stable cache keys', () => {
        const key1 = generateCacheKey(basePersona);
        const key2 = generateCacheKey(basePersona);
        expect(key1).toBe(key2);
      });

      it('should include persona name and version', () => {
        const key = generateCacheKey(basePersona);
        expect(key).toContain('test-persona');
        expect(key).toContain('1.0.0');
      });

      it('should change when prompt changes', () => {
        const persona2 = { ...basePersona, prompt: 'Different prompt' };
        const key1 = generateCacheKey(basePersona);
        const key2 = generateCacheKey(persona2);
        expect(key1).not.toBe(key2);
      });

      it('should change when version changes', () => {
        const persona2 = { ...basePersona, version: '2.0.0' };
        const key1 = generateCacheKey(basePersona);
        const key2 = generateCacheKey(persona2);
        expect(key1).not.toBe(key2);
      });

      it('should change when focus areas change', () => {
        const persona2 = { ...basePersona, focusAreas: ['different'] };
        const key1 = generateCacheKey(basePersona);
        const key2 = generateCacheKey(persona2);
        expect(key1).not.toBe(key2);
      });

      it('should not change when description changes', () => {
        const persona2 = { ...basePersona, description: 'Different description' };
        const key1 = generateCacheKey(basePersona);
        const key2 = generateCacheKey(persona2);
        expect(key1).toBe(key2);
      });
    });

    describe('enrichPersonaWithCacheMetadata', () => {
      it('should add cache metadata to large persona', () => {
        const largePrompt = 'word '.repeat(1100); // ~1375 tokens
        const persona: Persona = {
          name: 'large-persona',
          displayName: 'Large Persona',
          version: '1.0.0',
          description: 'Large',
          focusAreas: ['testing'],
          prompt: largePrompt,
        };

        const enriched = enrichPersonaWithCacheMetadata(persona);

        expect(enriched.cacheMetadata).toBeDefined();
        expect(enriched.cacheMetadata?.cacheEligible).toBe(true);
        expect(enriched.cacheMetadata?.tokenCount).toBeGreaterThan(1024);
        expect(enriched.cacheMetadata?.cacheKey).toContain('large-persona');
      });

      it('should mark small persona as not cache-eligible', () => {
        const persona: Persona = {
          name: 'small-persona',
          displayName: 'Small Persona',
          version: '1.0.0',
          description: 'Small',
          focusAreas: ['testing'],
          prompt: 'Short',
        };

        const enriched = enrichPersonaWithCacheMetadata(persona);

        expect(enriched.cacheMetadata).toBeDefined();
        expect(enriched.cacheMetadata?.cacheEligible).toBe(false);
        expect(enriched.cacheMetadata?.tokenCount).toBeLessThan(1024);
      });

      it('should skip cache metadata when caching disabled', () => {
        const persona: Persona = {
          name: 'test-persona',
          displayName: 'Test',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: 'word '.repeat(1100), // ~1375 tokens
        };

        const enriched = enrichPersonaWithCacheMetadata(persona, { enableCaching: false, minTokens: 1024 });

        expect(enriched.cacheMetadata).toBeUndefined();
      });

      it('should not mutate original persona', () => {
        const persona: Persona = {
          name: 'test-persona',
          displayName: 'Test',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: 'word '.repeat(1100), // ~1375 tokens
        };

        const original = { ...persona };
        enrichPersonaWithCacheMetadata(persona);

        expect(persona).toEqual(original);
      });
    });

    describe('loadPersonaFile with cache metadata', () => {
      it('should enrich loaded persona with cache metadata', async () => {
        const largePrompt = 'word '.repeat(1100); // ~1375 tokens
        const personaPath = join(testDir, 'large-persona.yaml');
        const personaData = {
          name: 'large-persona',
          displayName: 'Large Persona',
          version: '1.0.0',
          description: 'Large persona',
          focusAreas: ['testing'],
          prompt: largePrompt,
        };

        await writeFile(personaPath, JSON.stringify(personaData));

        const persona = await loadPersonaFile(personaPath);

        expect(persona.cacheMetadata).toBeDefined();
        expect(persona.cacheMetadata?.cacheEligible).toBe(true);
        expect(persona.cacheMetadata?.cacheKey).toBeDefined();
      });

      it('should support loading without cache metadata', async () => {
        const personaPath = join(testDir, 'test-persona.yaml');
        const personaData = {
          name: 'test-persona',
          displayName: 'Test Persona',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: 'Short prompt',
        };

        await writeFile(personaPath, JSON.stringify(personaData));

        const persona = await loadPersonaFile(personaPath, { enableCaching: false, minTokens: 1024 });

        expect(persona.cacheMetadata).toBeUndefined();
      });

      it('should enrich .ai.md personas with cache metadata', async () => {
        const personaPath = join(testDir, 'large.ai.md');
        const largeBody = 'word '.repeat(1100); // ~1375 tokens
        const personaContent = `---
name: large-aimd
displayName: Large AI.md
version: 1.0.0
description: Large .ai.md persona
focusAreas: [testing]
---

${largeBody}`;

        await writeFile(personaPath, personaContent);

        const persona = await loadPersonaFile(personaPath);

        expect(persona.cacheMetadata).toBeDefined();
        expect(persona.cacheMetadata?.cacheEligible).toBe(true);
      });
    });

    describe('loadPersonasFromDir with cache config', () => {
      it('should propagate cache config to all personas', async () => {
        const largePrompt = 'word '.repeat(1100); // ~1375 tokens
        const persona1 = {
          name: 'persona-1',
          displayName: 'Persona 1',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: largePrompt,
        };

        const persona2 = {
          name: 'persona-2',
          displayName: 'Persona 2',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: 'Short',
        };

        await writeFile(join(testDir, 'persona-1.yaml'), JSON.stringify(persona1));
        await writeFile(join(testDir, 'persona-2.yaml'), JSON.stringify(persona2));

        const personas = await loadPersonasFromDir(testDir);

        expect(personas.get('persona-1')?.cacheMetadata?.cacheEligible).toBe(true);
        expect(personas.get('persona-2')?.cacheMetadata?.cacheEligible).toBe(false);
      });

      it('should respect custom cache config', async () => {
        const mediumPrompt = 'word '.repeat(150); // ~750 chars -> ~188 tokens
        const persona = {
          name: 'medium-persona',
          displayName: 'Medium',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: mediumPrompt,
        };

        await writeFile(join(testDir, 'medium.yaml'), JSON.stringify(persona));

        const personas = await loadPersonasFromDir(testDir, { enableCaching: true, minTokens: 100 });

        expect(personas.get('medium-persona')?.cacheMetadata?.cacheEligible).toBe(true);
      });
    });

    describe('backward compatibility', () => {
      it('should load personas without breaking existing code', async () => {
        const personaPath = join(testDir, 'legacy.yaml');
        const personaData = {
          name: 'legacy-persona',
          displayName: 'Legacy',
          version: '1.0.0',
          description: 'Legacy persona',
          focusAreas: ['testing'],
          prompt: 'Legacy prompt',
        };

        await writeFile(personaPath, JSON.stringify(personaData));

        const persona = await loadPersonaFile(personaPath);

        // Should still have all required fields
        expect(persona.name).toBe('legacy-persona');
        expect(persona.displayName).toBe('Legacy');
        expect(persona.version).toBe('1.0.0');
        expect(persona.prompt).toBe('Legacy prompt');

        // Cache metadata is optional
        expect(persona.cacheMetadata).toBeDefined();
      });

      it('should work with loadPersonas function', async () => {
        const persona = {
          name: 'test-persona',
          displayName: 'Test',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: 'Test prompt',
        };

        await writeFile(join(testDir, 'test.yaml'), JSON.stringify(persona));

        const personas = await loadPersonas([testDir]);

        expect(personas.size).toBe(1);
        expect(personas.has('test-persona')).toBe(true);
      });

      it('should work with loadSpecificPersonas function', async () => {
        const persona = {
          name: 'specific-persona',
          displayName: 'Specific',
          version: '1.0.0',
          description: 'Test',
          focusAreas: ['testing'],
          prompt: 'Test prompt',
        };

        await writeFile(join(testDir, 'specific.yaml'), JSON.stringify(persona));

        const personas = await loadSpecificPersonas(['specific-persona'], [testDir]);

        expect(personas).toHaveLength(1);
        expect(personas[0].name).toBe('specific-persona');
      });
    });
  });
});
