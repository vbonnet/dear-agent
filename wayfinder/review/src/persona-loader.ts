/**
 * Persona loader for multi-persona-review plugin
 * Handles loading and validating persona definitions from YAML files
 */

import { readFile, readdir, stat } from 'fs/promises';
import { parse as parseYaml } from 'yaml';
import { join, resolve, isAbsolute } from 'path';
import { homedir } from 'os';
import { createHash } from 'crypto';
import type { Persona, Severity } from './types.js';

/**
 * Error codes for persona loading
 */
export const PERSONA_ERROR_CODES = {
  FILE_NOT_FOUND: 'PERSONA_2001',
  PARSE_ERROR: 'PERSONA_2002',
  VALIDATION_ERROR: 'PERSONA_2003',
  INVALID_YAML: 'PERSONA_2004',
  DUPLICATE_PERSONA: 'PERSONA_2005',
} as const;

/**
 * Configuration for cache eligibility
 */
export interface CacheConfig {
  /** Enable caching features (default: true) */
  enableCaching: boolean;

  /** Minimum token count for cache eligibility (default: 1024) */
  minTokens: number;
}

const DEFAULT_CACHE_CONFIG: CacheConfig = {
  enableCaching: true,
  minTokens: 1024,
};

/**
 * Persona loading error
 */
export class PersonaError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'PersonaError';
  }
}

/**
 * Persona search path resolution order:
 * 1. User: ~/.wayfinder/personas
 * 2. Project: .wayfinder/personas
 * 3. Company: {company}/personas
 * 4. Core: {core}/personas
 */
export interface PersonaSearchPaths {
  user: string;
  project: string;
  company?: string;
  core?: string;
}

/**
 * Estimates token count for a given text string
 * Uses a simple approximation: ~4 characters per token
 *
 * For accurate counting, integrate tiktoken (js-tiktoken package)
 * This approximation is calibrated to match Claude's tokenization behavior
 *
 * @param text - Text to count tokens for
 * @returns Estimated token count
 */
export function countTokens(text: string): number {
  if (!text || text.length === 0) {
    return 0;
  }

  // Simple approximation: 4 characters per token
  // This is a reasonable average for English text and code
  // Based on empirical observations:
  // - English prose: ~4-5 chars/token
  // - Code: ~3-4 chars/token
  // - Markdown: ~4 chars/token

  return Math.ceil(text.length / 4);
}

/**
 * Validates whether a persona is eligible for prompt caching
 *
 * @param persona - Persona to validate
 * @param config - Cache configuration
 * @returns true if persona meets caching threshold
 */
export function validateCacheEligibility(
  persona: Persona,
  config: CacheConfig = DEFAULT_CACHE_CONFIG
): boolean {
  if (!config.enableCaching) {
    return false;
  }

  const tokens = countTokens(persona.prompt);

  if (tokens < config.minTokens) {
    console.warn(
      `Persona '${persona.name}' below cache threshold: ${tokens} tokens (min: ${config.minTokens})`
    );
    return false;
  }

  return true;
}

/**
 * Generates a stable cache key for a persona
 *
 * Cache key format: persona:{name}:{version}:{hash}
 * Hash includes: version, prompt, focus areas
 *
 * The hash ensures cache invalidation when:
 * - Persona prompt changes
 * - Focus areas change
 * - Version changes
 *
 * @param persona - Persona to generate cache key for
 * @returns Cache key string
 */
export function generateCacheKey(persona: Persona): string {
  const hashInput = [
    persona.version,
    persona.prompt,
    persona.focusAreas.join(','),
  ].join('|');

  const hash = createHash('sha256')
    .update(hashInput)
    .digest('hex')
    .substring(0, 8);

  return `persona:${persona.name}:${persona.version}:${hash}`;
}

/**
 * Enriches a persona with cache metadata
 *
 * @param persona - Persona to enrich
 * @param config - Cache configuration
 * @returns Persona with cache metadata
 */
export function enrichPersonaWithCacheMetadata(
  persona: Persona,
  config: CacheConfig = DEFAULT_CACHE_CONFIG
): Persona {
  if (!config.enableCaching) {
    // Skip cache metadata if caching is disabled
    return persona;
  }

  const tokenCount = countTokens(persona.prompt);
  const cacheEligible = tokenCount >= config.minTokens;
  const cacheKey = generateCacheKey(persona);

  return {
    ...persona,
    cacheMetadata: {
      cacheEligible,
      tokenCount,
      cacheKey,
    },
  };
}

/**
 * Resolves persona search paths with tilde expansion and placeholders
 *
 * @param paths - Array of path templates
 * @param cwd - Current working directory
 * @param companyPath - Optional company plugin path
 * @param corePath - Optional core plugin path
 * @param personasPath - Optional personas plugin path
 * @returns Resolved search paths
 */
export function resolvePersonaPaths(
  paths: string[],
  cwd: string = process.cwd(),
  companyPath?: string,
  corePath?: string,
  personasPath?: string
): string[] {
  return paths
    .map(path => {
      // Expand tilde
      if (path.startsWith('~/')) {
        path = join(homedir(), path.slice(2));
      }

      // Replace placeholders
      if (path.includes('{company}') && companyPath) {
        path = path.replace('{company}', companyPath);
      } else if (path.includes('{company}')) {
        return null; // Skip if company path not available
      }

      if (path.includes('{core}') && corePath) {
        path = path.replace('{core}', corePath);
      } else if (path.includes('{core}')) {
        return null; // Skip if core path not available
      }

      if (path.includes('{personas}') && personasPath) {
        path = path.replace('{personas}', personasPath);
      } else if (path.includes('{personas}')) {
        return null; // Skip if personas path not available
      }

      // Resolve relative paths
      if (!isAbsolute(path)) {
        path = join(cwd, path);
      }

      return resolve(path);
    })
    .filter((path): path is string => path !== null);
}

/**
 * Loads a single persona from a YAML or .ai.md file
 *
 * Supports two formats:
 * 1. Legacy .yaml files (Multi-Persona Review format)
 * 2. New .ai.md files (Personas plugin format with YAML frontmatter)
 *
 * @param personaPath - Path to the persona file (.yaml, .yml, or .ai.md)
 * @param cacheConfig - Optional cache configuration
 * @returns Parsed persona with cache metadata
 * @throws PersonaError if file cannot be read or parsed
 */
export async function loadPersonaFile(
  personaPath: string,
  cacheConfig: CacheConfig = DEFAULT_CACHE_CONFIG
): Promise<Persona> {
  try {
    const content = await readFile(personaPath, 'utf-8');

    try {
      if (personaPath.endsWith('.ai.md')) {
        // New Personas plugin format: YAML frontmatter + markdown body
        const parts = content.split('---', 3);
        if (parts.length < 3) {
          throw new PersonaError(
            PERSONA_ERROR_CODES.INVALID_YAML,
            `Invalid .ai.md format (expected YAML frontmatter): ${personaPath}`
          );
        }

        const frontmatter = parseYaml(parts[1]);
        const body = parts[2].trim();

        const persona: Persona = {
          name: frontmatter.name,
          displayName: frontmatter.displayName,
          version: frontmatter.version,
          description: frontmatter.description,
          focusAreas: frontmatter.focusAreas || [],
          severityLevels: frontmatter.severityLevels,
          gitHistoryAccess: frontmatter.gitHistoryAccess || false,
          prompt: body,
        };

        validatePersona(persona, personaPath);
        return enrichPersonaWithCacheMetadata(persona, cacheConfig);
      } else {
        // Legacy Multi-Persona Review format: Pure YAML
        const persona = parseYaml(content) as Persona;
        validatePersona(persona, personaPath);
        return enrichPersonaWithCacheMetadata(persona, cacheConfig);
      }
    } catch (parseError) {
      throw new PersonaError(
        PERSONA_ERROR_CODES.INVALID_YAML,
        `Failed to parse persona file ${personaPath}: ${parseError instanceof Error ? parseError.message : String(parseError)}`,
        parseError
      );
    }
  } catch (error) {
    if (error instanceof PersonaError) {
      throw error;
    }

    if (error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT') {
      throw new PersonaError(
        PERSONA_ERROR_CODES.FILE_NOT_FOUND,
        `Persona file not found: ${personaPath}`,
        error
      );
    }

    throw new PersonaError(
      PERSONA_ERROR_CODES.PARSE_ERROR,
      `Failed to load persona from ${personaPath}: ${error instanceof Error ? error.message : String(error)}`,
      error
    );
  }
}

/**
 * Validates a persona definition
 *
 * @param persona - Persona to validate
 * @param filePath - File path for error messages
 * @throws PersonaError if persona is invalid
 */
export function validatePersona(persona: unknown, filePath: string = 'unknown'): asserts persona is Persona {
  if (!persona || typeof persona !== 'object') {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona must be an object in ${filePath}`
    );
  }

  const p = persona as Partial<Persona>;

  // Required fields
  if (!p.name || typeof p.name !== 'string') {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona must have a 'name' string field in ${filePath}`
    );
  }

  if (!p.displayName || typeof p.displayName !== 'string') {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona '${p.name}' must have a 'displayName' string field in ${filePath}`
    );
  }

  if (!p.version || typeof p.version !== 'string') {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona '${p.name}' must have a 'version' string field in ${filePath}`
    );
  }

  if (!p.description || typeof p.description !== 'string') {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona '${p.name}' must have a 'description' string field in ${filePath}`
    );
  }

  if (!p.focusAreas || !Array.isArray(p.focusAreas)) {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona '${p.name}' must have a 'focusAreas' array field in ${filePath}`
    );
  }

  if (!p.prompt || typeof p.prompt !== 'string') {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona '${p.name}' must have a 'prompt' string field in ${filePath}`
    );
  }

  // Validate name format (lowercase-kebab-case)
  if (!/^[a-z][a-z0-9-]*$/.test(p.name)) {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona name '${p.name}' must be lowercase kebab-case (a-z, 0-9, hyphen) in ${filePath}`
    );
  }

  // Validate version format (semver-like)
  if (!/^\d+\.\d+\.\d+$/.test(p.version)) {
    throw new PersonaError(
      PERSONA_ERROR_CODES.VALIDATION_ERROR,
      `Persona '${p.name}' version '${p.version}' must be semver format (x.y.z) in ${filePath}`
    );
  }

  // Validate severity levels if provided
  if (p.severityLevels) {
    if (!Array.isArray(p.severityLevels)) {
      throw new PersonaError(
        PERSONA_ERROR_CODES.VALIDATION_ERROR,
        `Persona '${p.name}' severityLevels must be an array in ${filePath}`
      );
    }

    const validSeverities: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];
    for (const severity of p.severityLevels) {
      if (!validSeverities.includes(severity)) {
        throw new PersonaError(
          PERSONA_ERROR_CODES.VALIDATION_ERROR,
          `Persona '${p.name}' has invalid severity '${severity}'. Must be one of: ${validSeverities.join(', ')} in ${filePath}`
        );
      }
    }
  }
}

/**
 * Loads all personas from a directory
 *
 * Supports subdirectories (personas/core/, personas/domain/)
 * and both .yaml and .ai.md file formats
 *
 * @param dirPath - Path to directory containing persona files
 * @param cacheConfig - Optional cache configuration
 * @returns Map of persona name to persona object
 */
export async function loadPersonasFromDir(
  dirPath: string,
  cacheConfig: CacheConfig = DEFAULT_CACHE_CONFIG
): Promise<Map<string, Persona>> {
  const personas = new Map<string, Persona>();

  try {
    const entries = await readdir(dirPath);

    for (const entry of entries) {
      const fullPath = join(dirPath, entry);
      const stats = await stat(fullPath);

      // Recursively load from subdirectories (personas/core/, personas/domain/)
      if (stats.isDirectory()) {
        // Skip special directories (hidden and metadata)
        if (entry.startsWith('.') || entry.startsWith('_')) {
          continue;
        }

        const subPersonas = await loadPersonasFromDir(fullPath, cacheConfig);
        for (const [name, persona] of subPersonas) {
          if (personas.has(name)) {
            // Skip duplicates from subdirectories (earlier ones take precedence)
            continue;
          }
          personas.set(name, persona);
        }
        continue;
      }

      // Only process .yaml, .yml, and .ai.md files
      if (!entry.endsWith('.yaml') && !entry.endsWith('.yml') && !entry.endsWith('.ai.md')) {
        continue;
      }

      // Skip .why.md files (they're documentation, not personas)
      if (entry.endsWith('.why.md')) {
        continue;
      }

      if (!stats.isFile()) {
        continue;
      }

      try {
        const persona = await loadPersonaFile(fullPath, cacheConfig);

        // Check for duplicates
        if (personas.has(persona.name)) {
          throw new PersonaError(
            PERSONA_ERROR_CODES.DUPLICATE_PERSONA,
            `Duplicate persona '${persona.name}' found in ${fullPath}. Already loaded from another file.`
          );
        }

        personas.set(persona.name, persona);
      } catch (error) {
        // Log error but continue loading other personas
        console.warn(`Failed to load persona from ${fullPath}:`, error instanceof Error ? error.message : String(error));
      }
    }
  } catch (error) {
    // If directory doesn't exist, return empty map
    if (error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT') {
      return personas;
    }

    throw new PersonaError(
      PERSONA_ERROR_CODES.PARSE_ERROR,
      `Failed to read personas directory ${dirPath}: ${error instanceof Error ? error.message : String(error)}`,
      error
    );
  }

  return personas;
}

/**
 * Loads personas from multiple search paths
 * Later paths override earlier ones (user overrides project, project overrides company, etc.)
 *
 * @param searchPaths - Array of directory paths to search (not mutated)
 * @param cacheConfig - Optional cache configuration
 * @returns Map of persona name to persona object
 */
export async function loadPersonas(
  searchPaths: string[],
  cacheConfig: CacheConfig = DEFAULT_CACHE_CONFIG
): Promise<Map<string, Persona>> {
  const allPersonas = new Map<string, Persona>();

  // Load in order, with later paths overriding earlier ones
  for (const path of searchPaths) {
    const personas = await loadPersonasFromDir(path, cacheConfig);

    // Later paths override by overwriting Map entries
    for (const [name, persona] of personas) {
      allPersonas.set(name, persona);
    }
  }

  return allPersonas;
}

/**
 * Loads specific personas by name from search paths
 *
 * @param personaNames - Array of persona names to load
 * @param searchPaths - Array of directory paths to search
 * @param cacheConfig - Optional cache configuration
 * @returns Array of loaded personas
 * @throws PersonaError if any persona cannot be found
 */
export async function loadSpecificPersonas(
  personaNames: string[],
  searchPaths: string[],
  cacheConfig: CacheConfig = DEFAULT_CACHE_CONFIG
): Promise<Persona[]> {
  const allPersonas = await loadPersonas(searchPaths, cacheConfig);
  const loadedPersonas: Persona[] = [];

  for (const name of personaNames) {
    const persona = allPersonas.get(name);

    if (!persona) {
      throw new PersonaError(
        PERSONA_ERROR_CODES.FILE_NOT_FOUND,
        `Persona '${name}' not found in search paths: ${searchPaths.join(', ')}`
      );
    }

    loadedPersonas.push(persona);
  }

  return loadedPersonas;
}
