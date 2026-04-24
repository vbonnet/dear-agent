#!/usr/bin/env node

/**
 * Multi-persona Review CLI
 * Command-line interface for running code reviews
 */

import { Command } from 'commander';
import { readFile } from 'fs/promises';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { exec } from 'child_process';
import { promisify } from 'util';
import {
  loadCrossCheckConfig,
  loadPersonas,
  resolvePersonaPaths,
  reviewFiles,
  createAnthropicReviewer,
  createVertexAIReviewer,
  createVertexAIClaudeReviewer,
  formatReviewResult,
  createCostSink,
} from './index.js';
import { formatReviewResultJSON } from './formatters/json-formatter.js';
import { formatReviewResultGitHub } from './formatters/github-formatter.js';
import type { ReviewMode, FileScanMode, CostSinkConfig } from './types.js';
import type { CostSink, CostMetadata } from './cost-sink.js';

const execAsync = promisify(exec);

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * Auto-detect provider from environment variables
 * @internal - exported for testing only
 */
export function detectProvider(options: {
  provider?: string;
  apiKey?: string;
  vertexProject?: string;
  vertexLocation?: string;
}, verbose = false): {
  provider: string;
  vertexProject?: string;
  vertexLocation?: string;
} {
  // Auto-detect Claude Code VertexAI credentials
  // Pattern source: skills/review-spec/review_spec.py:230-232
  const claudeCodeUseVertex = process.env.CLAUDE_CODE_USE_VERTEX === '1';
  const claudeCodeVertexProject = process.env.ANTHROPIC_VERTEX_PROJECT_ID;
  const claudeCodeVertexRegion = process.env.CLOUD_ML_REGION;

  // Determine provider (auto-detect or explicit)
  let provider = options.provider || 'anthropic';
  let vertexProject = options.vertexProject;
  let vertexLocation = options.vertexLocation;

  // If running in Claude Code with VertexAI, auto-switch to vertexai-claude provider
  if (claudeCodeUseVertex && claudeCodeVertexProject && !options.provider) {
    if (verbose) {
      console.error('[VERBOSE] Detected Claude Code VertexAI session');
      console.error(`[VERBOSE] Project: ${claudeCodeVertexProject}`);
      console.error(`[VERBOSE] Region: ${claudeCodeVertexRegion || 'us-east5'}`);
      console.error('[VERBOSE] Auto-selecting provider: vertexai-claude');
    }
    provider = 'vertexai-claude';

    // Map Claude Code env vars to standard format
    if (!vertexProject) {
      vertexProject = claudeCodeVertexProject;
    }
    if (!vertexLocation && claudeCodeVertexRegion) {
      vertexLocation = claudeCodeVertexRegion;
    }
  }

  // Auto-detect provider based on available credentials (fallback behavior)
  if (provider === 'anthropic') {
    const apiKey = options.apiKey || process.env.ANTHROPIC_API_KEY;
    if (!apiKey) {
      // Check if VertexAI credentials are available
      const fallbackVertexProject = vertexProject || process.env.VERTEX_PROJECT_ID;
      if (fallbackVertexProject) {
        if (verbose) {
          console.error('[VERBOSE] Anthropic API key not found, switching to VertexAI');
        }
        provider = 'vertexai';
      }
    }
  }

  return { provider, vertexProject, vertexLocation };
}

/**
 * Load package.json for version
 */
async function getVersion(): Promise<string> {
  try {
    const pkgPath = join(__dirname, '../package.json');
    const pkgJson = await readFile(pkgPath, 'utf-8');
    const pkg = JSON.parse(pkgJson);
    return pkg.version || '0.1.0';
  } catch {
    return '0.1.0';
  }
}

/**
 * Main CLI function
 */
async function main() {
  const version = await getVersion();
  const program = new Command();

  program
    .name('multi-persona-review')
    .description('Multi-persona code review using AI')
    .version(version);

  // Main review command
  program
    .argument('[files...]', 'Files or directories to review')
    .option('-m, --mode <mode>', 'Review mode (quick|thorough|custom)', 'quick')
    .option('-p, --personas <personas>', 'Comma-separated persona names')
    .option('-s, --scan <mode>', 'Scan mode (full|diff|changed)', 'changed')
    .option('-f, --format <format>', 'Output format (text|json|github)', 'text')
    .option('--full', 'Review entire files without git diff (same as --scan full)')
    .option('--no-colors', 'Disable colored output')
    .option('--no-cost', 'Hide cost information')
    .option('--no-dedupe', 'Disable deduplication')
    .option('--flat', 'Flat output (don\'t group by file)')
    .option('--provider <provider>', 'AI provider (anthropic|vertexai|vertexai-claude)')
    .option('--api-key <key>', 'Anthropic API key (or use ANTHROPIC_API_KEY env var)')
    .option('--model <model>', 'Model to use (claude-3-5-sonnet-20241022|gemini-2.0-flash-exp)')
    .option('--vertex-project <id>', 'VertexAI project ID (or use VERTEX_PROJECT_ID env var)')
    .option('--vertex-location <region>', 'VertexAI location (or use VERTEX_LOCATION env var)')
    .option('--gcp-credentials <path>', 'Path to GCP service account key JSON file (or use GOOGLE_APPLICATION_CREDENTIALS env var)')
    .option('--cost-sink <type>', 'Cost tracking sink (stdout|file|gcp)')
    .option('--cost-file <path>', 'File path for file cost sink')
    .option('--gcp-project <id>', 'GCP project ID for GCP cost sink')
    .option('--gcp-key-file <path>', 'GCP service account key file')
    .option('--verbose', 'Enable verbose logging')
    .option('--dry-run', 'Show what would be reviewed without actually running')
    .option('--list-personas', 'List available personas and exit')
    .option('--no-subagents', 'Disable sub-agent orchestrator and use legacy reviewer')
    .option('--no-cache', 'Disable prompt caching (implies --no-subagents)')
    .option('--show-cache-metrics', 'Display cache hit rate and cost savings after review')
    .option('--cache-ttl <ttl>', 'Explicit cache TTL (5min|1h) - overrides auto-selection')
    .option('--no-auto-ttl', 'Disable automatic TTL selection (use explicit --cache-ttl or default)')
    .option('--review-count <count>', 'Expected review count for auto-TTL (overrides environment detection)', parseInt)
    .option('--vote-threshold <threshold>', 'Vote threshold for GO decision (0-1, default: 0.5)', parseFloat)
    .option('--min-confidence <confidence>', 'Minimum confidence for findings (0-1)', parseFloat)
    .action(async (files: string[], options: any) => {
      try {
        const cwd = process.cwd();

        // Load configuration first (needed for persona paths)
        const config = await loadCrossCheckConfig(); // Let it use default path discovery

        // Handle --list-personas
        if (options.listPersonas) {
          // Resolve personas plugin path from environment or default location
          // Default searches core/persona/library/ where .ai.md persona files are located
          const personasPluginPath = process.env.WAYFINDER_PERSONAS_PATH ||
            join(dirname(dirname(__dirname)), '..', 'core', 'persona', 'library');

          const personaPaths = resolvePersonaPaths(
            config.personaPaths || [],
            cwd,
            undefined, // companyPath
            undefined, // corePath
            personasPluginPath
          );
          const allPersonas = await loadPersonas(personaPaths);

          // Validation: warn if no personas found
          if (allPersonas.size === 0 && personaPaths.length > 0) {
            console.warn('⚠️  Warning: No personas found in search paths');
            console.warn('   Searched:', personaPaths.join(', '));
            console.warn('   Ensure .ai.md persona files exist in one of these locations');
          }

          console.log('\n📋 Available Personas:\n');
          for (const [name, persona] of allPersonas.entries()) {
            console.log(`  ${name}`);
            console.log(`    ${persona.description}`);
            console.log(`    Focus: ${persona.focusAreas.slice(0, 3).join(', ')}${persona.focusAreas.length > 3 ? '...' : ''}`);
            console.log();
          }
          console.log(`Total: ${allPersonas.size} personas available\n`);
          process.exit(0);
        }

        // Enable verbose logging
        const verbose = options.verbose || false;
        if (verbose) {
          console.error('[VERBOSE] Starting multi-persona review...');
          console.error('[VERBOSE] Working directory:', cwd);
        }

        // Handle --full flag (override scan mode to 'full')
        if (options.full) {
          options.scan = 'full';
          if (verbose) {
            console.error('[VERBOSE] --full flag detected, overriding scan mode to: full');
          }
        }

        // Default to current directory if no files specified
        if (files.length === 0) {
          files = [cwd];
        }

        // Determine personas
        let personaNames: string[];
        if (options.personas) {
          personaNames = options.personas.split(',').map((p: string) => p.trim());
        } else if (config.defaultPersonas) {
          personaNames = config.defaultPersonas;
        } else {
          // Default personas for quick/thorough modes
          personaNames = options.mode === 'quick'
            ? ['security-engineer', 'error-handling-specialist', 'code-health']
            : ['security-engineer', 'performance-engineer', 'code-health',
               'error-handling-specialist', 'testing-advocate'];
        }

        // Load personas
        // Resolve personas plugin path from environment or default location
        // Default searches core/persona/library/ where .ai.md persona files are located
        const personasPluginPath = process.env.WAYFINDER_PERSONAS_PATH ||
          join(dirname(dirname(__dirname)), '..', 'core', 'persona', 'library');

        const personaPaths = resolvePersonaPaths(
          config.personaPaths || [],
          cwd,
          undefined, // companyPath
          undefined, // corePath
          personasPluginPath
        );
        if (verbose) {
          console.error('[VERBOSE] Personas plugin path:', personasPluginPath);
          console.error('[VERBOSE] Persona search paths:', personaPaths);
          console.error('[VERBOSE] Environment override:', process.env.WAYFINDER_PERSONAS_PATH || 'not set');
        }

        const allPersonas = await loadPersonas(personaPaths);
        if (verbose) {
          console.error(`[VERBOSE] Loaded ${allPersonas.size} personas:`, Array.from(allPersonas.keys()).join(', '));
        }

        // Validation: warn if no personas found and exit with error
        if (allPersonas.size === 0 && personaPaths.length > 0) {
          console.warn('⚠️  Warning: No personas found in search paths');
          console.warn('   Searched:', personaPaths.join(', '));
          console.warn('   Ensure .ai.md persona files exist in one of these locations');
          console.error('Error: Cannot run review without personas');
          process.exit(1);
        }

        const personas = personaNames.map(name => {
          const persona = allPersonas.get(name);
          if (!persona) {
            console.error(`Error: Persona '${name}' not found`);
            console.error(`Available personas: ${Array.from(allPersonas.keys()).join(', ')}`);
            process.exit(1);
          }
          return persona;
        });

        if (verbose) {
          console.error('[VERBOSE] Selected personas:', personaNames.join(', '));
        }

        // Auto-detect provider from environment variables (before dry-run so it shows in preview)
        const detected = detectProvider(options, verbose);
        const provider = detected.provider;
        if (detected.vertexProject) {
          options.vertexProject = detected.vertexProject;
        }
        if (detected.vertexLocation) {
          options.vertexLocation = detected.vertexLocation;
        }

        // Handle --dry-run
        if (options.dryRun) {
          console.log('\n🔍 Dry Run Mode - Preview\n');
          console.log(`Mode: ${options.mode}`);
          console.log(`Provider: ${provider}`);
          console.log(`Scan: ${options.scan}`);
          console.log(`Format: ${options.format}`);
          console.log(`Personas: ${personaNames.join(', ')}`);
          console.log(`Files to review: ${files.join(', ')}`);
          console.log(`Deduplication: ${options.dedupe !== false ? 'enabled' : 'disabled'}`);
          if (options.costSink) {
            console.log(`Cost sink: ${options.costSink}`);
          }
          console.log('\nNo review will be performed in dry-run mode.\n');
          process.exit(0);
        }

        // Create reviewer based on provider
        let reviewer: (input: any) => Promise<any>;

        if (provider === 'vertexai') {
          // VertexAI provider
          const vertexProject = options.vertexProject || process.env.VERTEX_PROJECT_ID;
          const vertexLocation = options.vertexLocation || process.env.VERTEX_LOCATION || 'us-central1';

          if (!vertexProject) {
            console.error('Error: VertexAI project ID required');
            console.error('Set VERTEX_PROJECT_ID environment variable or use --vertex-project option');
            process.exit(1);
          }

          if (verbose) {
            console.error('[VERBOSE] Using VertexAI provider');
            console.error('[VERBOSE] Project:', vertexProject);
            console.error('[VERBOSE] Location:', vertexLocation);
            console.error('[VERBOSE] Model:', options.model || 'gemini-2.0-flash-exp');
          }

          reviewer = createVertexAIReviewer({
            projectId: vertexProject,
            location: vertexLocation,
            model: options.model,
          });
        } else if (provider === 'vertexai-claude') {
          // VertexAI Claude provider
          const vertexProject =
            options.vertexProject ||
            process.env.VERTEX_PROJECT_ID ||
            process.env.ANTHROPIC_VERTEX_PROJECT_ID;

          const vertexLocation =
            options.vertexLocation ||
            process.env.VERTEX_LOCATION ||
            process.env.CLOUD_ML_REGION ||
            'us-east5';

          const gcpCredentials = options.gcpCredentials || process.env.GOOGLE_APPLICATION_CREDENTIALS;

          if (!vertexProject) {
            console.error('Error: VertexAI project ID required');
            console.error('Set VERTEX_PROJECT_ID environment variable or use --vertex-project option');
            process.exit(1);
          }

          if (verbose) {
            console.error('[VERBOSE] Using VertexAI Claude provider');
            console.error('[VERBOSE] Project:', vertexProject);
            console.error('[VERBOSE] Location:', vertexLocation);
            console.error('[VERBOSE] Model:', options.model || 'claude-sonnet-4-5@20250929');
            if (gcpCredentials) {
              console.error('[VERBOSE] Credentials:', gcpCredentials);
            }
          }

          reviewer = createVertexAIClaudeReviewer({
            projectId: vertexProject,
            location: vertexLocation,
            model: options.model,
            keyFilename: gcpCredentials,
            timeout: 180000, // 3 minutes for large files
            maxTokens: 8192, // Increased for Agency-Agents voting responses with alternatives
          });
        } else {
          // Anthropic provider (default)
          const apiKey = options.apiKey || process.env.ANTHROPIC_API_KEY;

          if (!apiKey) {
            console.error('Error: Anthropic API key required');
            console.error('Set ANTHROPIC_API_KEY environment variable or use --api-key option');
            console.error('');
            console.error('Alternatively, use VertexAI with:');
            console.error('  --provider vertexai --vertex-project YOUR_PROJECT_ID');
            console.error('  or set VERTEX_PROJECT_ID environment variable');
            process.exit(1);
          }

          if (verbose) {
            console.error('[VERBOSE] Using Anthropic provider');
            console.error('[VERBOSE] API key configured');
            console.error('[VERBOSE] Model:', options.model || 'claude-3-5-sonnet-20241022');
          }

          reviewer = createAnthropicReviewer({
            apiKey,
            model: options.model,
          });
        }

        console.error(`\n🔍 Reviewing ${files.length} file(s) with ${personas.length} persona(s)...\n`);

        // Create cost sink if configured
        let costSink: CostSink | undefined;
        let costMetadata: CostMetadata | undefined;

        const costSinkConfig = options.costSink
          ? ({
              type: options.costSink,
              config: options.costSink === 'file'
                ? { filePath: options.costFile || './multi-persona-review-costs.jsonl' }
                : options.costSink === 'gcp'
                ? {
                    projectId: options.gcpProject,
                    keyFilePath: options.gcpKeyFile,
                  }
                : {},
            } as CostSinkConfig)
          : config.costTracking;

        if (costSinkConfig) {
          try {
            if (verbose) {
              console.error('[VERBOSE] Creating cost sink:', costSinkConfig.type);
            }
            costSink = await createCostSink(costSinkConfig);

            // Get git metadata for cost tracking
            try {
              const { stdout: repoName } = await execAsync('git remote get-url origin', { cwd });
              const { stdout: branch } = await execAsync('git rev-parse --abbrev-ref HEAD', { cwd });
              const { stdout: commit } = await execAsync('git rev-parse HEAD', { cwd });

              costMetadata = {
                repository: repoName.trim().replace(/\.git$/, '').split('/').pop() || undefined,
                branch: branch.trim(),
                commit: commit.trim(),
              };

              if (verbose) {
                console.error('[VERBOSE] Git metadata:', costMetadata);
              }
            } catch (error) {
              // Git metadata is optional - continue without it
              if (verbose) {
                console.error('[VERBOSE] Git metadata not available (not a git repository or git not installed)');
              }
              // costMetadata remains undefined - this is acceptable
            }
          } catch (error) {
            console.error('Warning: Failed to create cost sink:', error instanceof Error ? error.message : String(error));
          }
        }

        // Run review
        // Set review count for auto-TTL detection
        if (options.reviewCount) {
          process.env.MULTI_PERSONA_REVIEW_COUNT = String(options.reviewCount);
        }

        if (verbose) {
          console.error('[VERBOSE] Starting review with configuration:');
          console.error('[VERBOSE]  - Mode:', options.mode);
          console.error('[VERBOSE]  - Scan mode:', options.scan);
          console.error('[VERBOSE]  - Deduplication:', options.dedupe !== false);
          console.error('[VERBOSE]  - Personas:', personas.length);
          console.error('[VERBOSE]  - Sub-agents:', options.subagents !== false ? 'enabled' : 'disabled');
          if (provider === 'anthropic' && options.subagents !== false) {
            console.error('[VERBOSE]  - Prompt caching: enabled (Anthropic sub-agents)');
            if (options.cacheTtl) {
              console.error('[VERBOSE]  - Cache TTL: explicit', options.cacheTtl);
            } else if (options.autoTtl !== false) {
              console.error('[VERBOSE]  - Cache TTL: auto-selected (based on session context)');
            }
          }
        }

        const startTime = Date.now();
        const result = await reviewFiles(
          {
            files,
            personas,
            mode: options.mode as ReviewMode,
            fileScanMode: options.scan as FileScanMode,
            options: {
              deduplicate: options.dedupe !== false,
              voteThreshold: options.voteThreshold,
              minConfidence: options.minConfidence,
              apiKey: provider === 'anthropic' ? (options.apiKey || process.env.ANTHROPIC_API_KEY) : undefined,
              model: options.model,
            },
          },
          cwd,
          reviewer,
          {
            costSink,
            costMetadata,
            useSubAgents: options.subagents !== false, // Defaults to true unless --no-subagents is used
            apiKey: provider === 'anthropic' ? (options.apiKey || process.env.ANTHROPIC_API_KEY) : undefined,
            model: options.model,
            cacheTtl: options.cacheTtl as '5min' | '1h' | undefined,
            autoTtl: options.autoTtl,
          }
        );
        const duration = ((Date.now() - startTime) / 1000).toFixed(2);

        if (verbose) {
          console.error(`[VERBOSE] Review completed in ${duration}s`);
          console.error('[VERBOSE] Found', result.summary.totalFindings, 'findings');
          console.error('[VERBOSE] Cost: $' + result.cost.totalCost.toFixed(4));
          if (result.summary.deduplicated) {
            console.error('[VERBOSE] Removed', result.summary.deduplicated, 'duplicate findings');
          }
        }

        // Format and output based on format option
        let formatted: string;

        switch (options.format) {
          case 'json':
            formatted = formatReviewResultJSON(result);
            break;

          case 'github':
            formatted = formatReviewResultGitHub(result);
            break;

          case 'text':
          default:
            formatted = formatReviewResult(result, {
              colors: options.colors !== false,
              groupByFile: !options.flat,
              showCost: options.cost !== false,
              showSummary: true,
            });
            break;
        }

        console.log(formatted);

        // Display cache alerts if --show-cache-metrics is enabled
        if (options.showCacheMetrics && costSink && costSink.getCacheAlerts) {
          const alerts = costSink.getCacheAlerts();
          if (alerts.length > 0) {
            const { formatCacheAlerts } = await import('./formatters/text-formatter.js');
            const alertsFormatted = formatCacheAlerts(alerts, options.colors !== false);
            console.log(alertsFormatted);
          }
        }

        // Exit with code 1 if any persona voted NO-GO
        // Agency-Agents voting: GO = approve (exit 0), NO-GO = block (exit 1)
        const hasNoGoDecisions = result.findings.some(f => f.decision === 'NO-GO');
        if (hasNoGoDecisions) {
          process.exit(1);
        }

        // Legacy behavior: Also exit 1 for critical/high findings without decisions
        // (for backward compatibility with pre-voting reviews)
        const hasCriticalWithoutDecision = result.findings.some(
          f => (f.severity === 'critical' || f.severity === 'high') && !f.decision
        );
        if (hasCriticalWithoutDecision) {
          process.exit(1);
        }
      } catch (error) {
        console.error('Error:', error instanceof Error ? error.message : String(error));

        // If ReviewEngineError with details, show detailed error information
        if (error && typeof error === 'object' && 'details' in error) {
          const details = (error as any).details;
          if (Array.isArray(details) && details.length > 0) {
            console.error('\nDetailed errors:');
            for (const detail of details) {
              if (detail instanceof Error) {
                console.error('  -', detail.message);
                if (detail.stack && options.verbose) {
                  console.error('    Stack:', detail.stack);
                }
              } else if (typeof detail === 'object' && detail !== null) {
                console.error('  -', JSON.stringify(detail, null, 2));
              } else {
                console.error('  -', String(detail));
              }
            }
          }
        }

        process.exit(1);
      }
    });

  // Init command - generate configuration file
  program
    .command('init')
    .description('Initialize multi-persona review configuration')
    .option('--force', 'Overwrite existing configuration')
    .action(async (options: any) => {
      try {
        const cwd = process.cwd();
        const configDir = join(cwd, '.wayfinder');
        const configPath = join(configDir, 'config.yml');

        // Check if config already exists
        try {
          await readFile(configPath, 'utf-8');
          if (!options.force) {
            console.error('Error: Configuration file already exists at .wayfinder/config.yml');
            console.error('Use --force to overwrite');
            process.exit(1);
          }
        } catch {
          // File doesn't exist, which is what we want
        }

        // Create .wayfinder directory
        const { mkdir, writeFile } = await import('fs/promises');
        await mkdir(configDir, { recursive: true });

        // Generate default config
        const defaultConfig = `# Multi-Persona Review Configuration
# See https://github.com/vbonnet/engram for documentation

crossCheck:
  # Default review mode (quick|thorough|custom)
  defaultMode: quick

  # Default personas to use
  defaultPersonas:
    - security-engineer
    - code-health
    - error-handling-specialist

  # Persona search paths (checked in order)
  # personaPaths:
  #   - .wayfinder/personas        # Project-specific
  #   - ~/.wayfinder/personas      # User-level

  # Review options
  options:
    # Enable finding deduplication
    deduplicate: true

    # Similarity threshold for deduplication (0-1)
    similarityThreshold: 0.8

    # Maximum number of files to review
    # maxFiles: 100

  # Cost tracking configuration
  # costTracking:
  #   type: stdout  # Options: stdout, file, gcp
  #   config:
  #     # For file sink:
  #     # filePath: ./multi-persona-review-costs.jsonl
  #
  #     # For GCP sink:
  #     # projectId: my-gcp-project
  #     # keyFilePath: /path/to/service-account-key.json

  # GitHub integration (for CI/CD)
  # github:
  #   enabled: true
  #   changedFilesOnly: true
  #   skipDrafts: true
  #   concurrency: 3
`;

        await writeFile(configPath, defaultConfig);

        console.log('✅ Created .wayfinder/config.yml');
        console.log('\nNext steps:');
        console.log('1. Edit .wayfinder/config.yml to customize your setup');
        console.log('2. Run: multi-persona-review --list-personas to see available personas');
        console.log('3. Run: multi-persona-review src/ to review your code\n');
      } catch (error) {
        console.error('Error:', error instanceof Error ? error.message : String(error));
        process.exit(1);
      }
    });

  await program.parseAsync(process.argv);
}

main().catch(error => {
  console.error('Fatal error:', error);
  process.exit(1);
});
