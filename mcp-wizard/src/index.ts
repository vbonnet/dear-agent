#!/usr/bin/env node
import { Command } from 'commander';
import { setupCommand } from './commands/setup';
import { statusCommand } from './commands/status';
import { authCommand } from './commands/auth';
import { validateCommand } from './commands/validate';
import { repairCommand } from './commands/repair';
import { health } from './commands/health';
import { doctor } from './commands/doctor';
import { sessionStart } from './commands/session-start';
import { serve } from './commands/serve'; // NEW
import { logoutCommand } from './lib/logout';
import { getConfigValue, setConfigValue, loadConfig, saveConfig, getUserConfigPath } from './lib/user-config';
import { TraceLogger } from './lib/trace-logger';
import inquirer from 'inquirer';

// Export Intent Analyzer for Context Broker (Phase 3-v2)
export {
  analyzeIntent,
  analyzeIntentBatch,
  isConfident,
  type RequirementEnvelope,
  type Action,
  type Service,
  PATTERNS,
} from './lib/intent-analyzer';

// Export Token Injection Layer for Context Broker (Phase 3-v2)
export {
  spawnMCPWithToken,
  getValidOktaToken,
  refreshOktaToken,
  checkTokenHealth,
  needsTokenRefresh,
  type TokenInjectionConfig,
  type TokenHealth,
} from './lib/token-injection';

const program = new Command();

program
  .name('mcp-wizard')
  .description('Automated MCP setup tool')
  .version('0.1.0')
  .option('--trace', 'Enable trace logging for debugging (logs to stderr)')
  .option('--log-file <path>', 'Write trace logs to file (requires --trace)')
  .hook('preAction', (thisCommand) => {
    // Initialize TraceLogger before any command runs
    const opts = thisCommand.opts();
    if (opts.trace) {
      TraceLogger.getInstance().configure({
        enabled: true,
        logFile: opts.logFile,
      });
    }
  })
  .action(async (options, command) => {
    // Default action when no command is specified - launch interactive setup
    const opts = command.optsWithGlobals();
    if (opts.trace) {
      await TraceLogger.getInstance().withCorrelationId(() => setupCommand({}));
    } else {
      await setupCommand({});
    }
  })
  .addHelpText('after', `
Examples:
  # Interactive setup (default - no command needed)
  $ mcp-wizard
  $ mcp-wizard setup

  # Setup specific MCPs via CLI
  $ mcp-wizard setup --mcps=googledocs,atlassian

  # Resume from interrupted setup
  $ mcp-wizard setup --resume

  # Setup for specific agents
  $ mcp-wizard setup --agents=claude-code,cursor

  # Check current setup status
  $ mcp-wizard status

Debugging:
  Use --trace for detailed logging (OAuth flows, MCP spawning, timing)
  Use --log-file to persist logs (JSON Lines format, machine-readable)

  $ mcp-wizard setup --trace                      # Trace to stderr
  $ mcp-wizard setup --trace --log-file debug.log # Trace to file
  $ cat debug.log | jq 'select(.level == "ERROR")' # Filter errors

Documentation:
  Troubleshooting: See TROUBLESHOOTING.md
  Atlassian OAuth: See docs/ATLASSIAN-MCP.md
  Support: https://github.com/your-org/mcp-wizard/issues
`);

program
  .command('serve')
  .description('Start mcp-wizard as MCP server (meta-server mode)')
  .addHelpText('after', `
This command runs mcp-wizard as an MCP server that proxies to downstream MCPs.
It reads MCP JSON-RPC requests from stdin and writes responses to stdout.

Configuration:
  Set MCP_WIZARD_TRACE=1 for debug logging (to stderr)
  Set MCP_WIZARD_CONFIG_PATH to override config location

Example Claude config:
  {
    "mcpServers": {
      "mcp-wizard": {
        "command": "npx",
        "args": ["-y", "mcp-wizard", "serve"]
      }
    }
  }

Note: Requires ~/.config/mcp-wizard/downstream.json with downstream MCP configs.
      Use 'mcp-wizard setup --mode=meta-server' to generate this config.
`)
  .action(async () => {
    await serve();
  });

program
  .command('setup')
  .description('Interactive setup wizard for MCP servers')
  .option('--dry-run', 'Show what would be done without doing it')
  .option('--verbose', 'Show detailed logs')
  .option('--skip-install', 'Skip MCP installation')
  .option('--skip-auth', 'Skip OAuth authentication')
  .option('--resume', 'Resume from saved state')
  .option('--mcps <mcps>', 'Comma-separated list of MCPs (googledocs,atlassian)')
  .option('--agents <agents>', 'Comma-separated list of agents (claude-code,cursor,cline,windsurf)')
  .option('--platform <name>', 'Target platform (claude, gemini)', 'claude')
  .option('--yes', 'Skip interactive prompts and use defaults (non-interactive mode)')
  .addHelpText('after', `
Examples:
  $ mcp-wizard setup                           # Interactive MCP selection (defaults to Claude Code)
  $ mcp-wizard setup --mcps=googledocs         # GoogleDocs only
  $ mcp-wizard setup --mcps=googledocs,atlassian --agents=claude-code
  $ mcp-wizard setup --platform=gemini         # Configure for Gemini CLI
  $ mcp-wizard setup --mcps=atlassian --yes    # Non-interactive mode
  $ mcp-wizard setup --resume                  # Resume interrupted setup
  $ mcp-wizard setup --skip-auth               # Skip OAuth (complete later)
`)
  .action(async (options, command) => {
    const opts = command.optsWithGlobals();
    if (opts.trace) {
      await TraceLogger.getInstance().withCorrelationId(() => setupCommand(options));
    } else {
      await setupCommand(options);
    }
  });

program
  .command('status')
  .description('Show current MCP setup status')
  .addHelpText('after', `
Example:
  $ mcp-wizard status    # Check setup state and MCP configuration
`)
  .action(statusCommand);

program
  .command('auth <mcp>')
  .description('Re-authenticate MCP (coming soon)')
  .option('--verbose', 'Show detailed logs')
  .addHelpText('after', `
Note: This command is not yet implemented.
      Use 'mcp-wizard setup --resume' to re-authenticate.
`)
  .action(authCommand);

program
  .command('validate')
  .description('Validate current MCP setup')
  .option('--verbose', 'Show detailed logs')
  .option('--fix', 'Attempt automatic fixes (coming soon)')
  .addHelpText('after', `
Examples:
  $ mcp-wizard validate           # Check setup health
  $ mcp-wizard validate --verbose # Show detailed validation results
`)
  .action(validateCommand);

program
  .command('repair [mcp]')
  .description('Repair broken setup (coming soon)')
  .option('--verbose', 'Show detailed logs')
  .option('--dry-run', 'Show what would be done without doing it')
  .option('--force', 'Force repair even if checks pass')
  .addHelpText('after', `
Note: This command is not yet implemented.
      See TROUBLESHOOTING.md for manual repair instructions.
`)
  .action(repairCommand);

program
  .command('logout')
  .description('Logout and revoke OAuth tokens')
  .option('--silent', 'Suppress success messages (for automation)')
  .addHelpText('after', `
Examples:
  $ mcp-wizard logout           # Interactive mode with success messages
  $ mcp-wizard logout --silent  # Silent mode (no output on success)

Note: This command revokes OAuth tokens at Google's server and clears
      local credentials from the OS keychain. If network is unavailable,
      local credentials will still be cleared (best-effort revocation).
`)
  .action(async (options) => {
    try {
      await logoutCommand(options);
    } catch (error: any) {
      console.error(`\n✗ Logout failed: ${error.message}`);
      if (options.verbose && error.stack) {
        console.error('\nStack trace:');
        console.error(error.stack);
      }
      process.exit(1);
    }
  });

program
  .command('health')
  .description('Fast health check for MCP system')
  .option('--silent', 'Exit code only, no output')
  .option('--json', 'Output JSON format')
  .option('--force', 'Bypass cache, run fresh checks')
  .addHelpText('after', `
Examples:
  $ mcp-wizard health           # Quick health status
  $ mcp-wizard health --json    # JSON output for scripting
  $ mcp-wizard health --force   # Force fresh check (bypass cache)

Exit codes:
  0 - All checks healthy
  1 - One or more warnings (degraded)
  2 - One or more errors (unhealthy)
`)
  .action(async (options, command) => {
    const opts = command.optsWithGlobals();
    if (opts.trace) {
      await TraceLogger.getInstance().withCorrelationId(() => health(options));
    } else {
      await health(options);
    }
  });

program
  .command('doctor')
  .description('Comprehensive diagnostics and recommendations')
  .option('--silent', 'Exit code only, no output')
  .option('--json', 'Output JSON format')
  .option('--force', 'Bypass cache, run fresh checks')
  .addHelpText('after', `
Examples:
  $ mcp-wizard doctor           # Full system diagnostics
  $ mcp-wizard doctor --json    # JSON output for scripting
  $ mcp-wizard doctor --force   # Force fresh diagnostics

Exit codes:
  0 - All checks healthy
  1 - One or more warnings
  2 - One or more errors
`)
  .action(async (options) => {
    await doctor(options);
  });

program
  .command('session-start')
  .description('Check MCP authentication status at session startup')
  .option('--verbose', 'Show detailed health check results')
  .option('--auto-refresh', 'Automatically refresh expired tokens')
  .addHelpText('after', `
Examples:
  $ mcp-wizard session-start              # Quick health check
  $ mcp-wizard session-start --verbose    # Detailed status
  $ mcp-wizard session-start --auto-refresh  # Auto-refresh expired tokens

Shell Integration:
  Add to ~/.bashrc or ~/.zshrc:
    mcp-wizard session-start

  Or for silent mode (suppress warnings):
    mcp-wizard session-start 2>/dev/null || true

See docs/SESSIONSTART-HOOK.md for detailed documentation.
`)
  .action(async (options) => {
    await sessionStart(options);
  });

// Config command with subcommands
const configCmd = program.command('config').description('Manage configuration');

configCmd
  .command('get <key>')
  .description('Get configuration value')
  .addHelpText('after', `
Examples:
  $ mcp-wizard config get company.glean_instance
  $ mcp-wizard config get company.okta_domain
`)
  .action((key: string) => {
    const value = getConfigValue(key);
    console.log(value);
  });

configCmd
  .command('set <key> <value>')
  .description('Set configuration value')
  .addHelpText('after', `
Examples:
  $ mcp-wizard config set company.glean_instance acme
  $ mcp-wizard config set company.okta_domain acme.okta.com
  $ mcp-wizard config set company.name "Acme Corp"
`)
  .action((key: string, value: string) => {
    try {
      setConfigValue(key, value);
      console.log(`✓ Set ${key} = ${value}`);
    } catch (error) {
      console.error(`Error: ${(error as Error).message}`);
      process.exit(1);
    }
  });

configCmd
  .command('list')
  .description('List all configuration values')
  .addHelpText('after', `
Example:
  $ mcp-wizard config list    # Show full config as JSON
`)
  .action(() => {
    const config = loadConfig();
    console.log(JSON.stringify(config, null, 2));
  });

configCmd
  .command('init')
  .description('Interactive configuration setup')
  .addHelpText('after', `
Example:
  $ mcp-wizard config init    # Prompts for company name, glean instance, okta domain
`)
  .action(async () => {
    try {
      const answers = await inquirer.prompt([
        {
          type: 'input',
          name: 'company.name',
          message: 'Company name:',
          validate: (input: string) => {
            if (!input.trim()) return 'Company name is required';
            return true;
          },
        },
        {
          type: 'input',
          name: 'company.glean_instance',
          message: 'Glean instance (lowercase, no spaces):',
          validate: (input: string) => {
            if (!input.trim()) return 'Glean instance is required';
            if (input.includes(' ')) return 'No spaces allowed';
            if (input !== input.toLowerCase()) return 'Must be lowercase';
            return true;
          },
        },
        {
          type: 'input',
          name: 'company.okta_domain',
          message: 'Okta domain (e.g., company.okta.com):',
          validate: (input: string) => {
            if (!input.trim()) return 'Okta domain is required';
            if (!input.includes('.')) return 'Must be a valid domain (e.g., company.okta.com)';
            return true;
          },
        },
      ]);

      // Convert flat keys to nested object
      const config = {
        company: {
          name: answers['company.name'],
          glean_instance: answers['company.glean_instance'],
          okta_domain: answers['company.okta_domain'],
        },
      };

      saveConfig(config);
      console.log(`✓ Configuration saved to ${getUserConfigPath()}`);
    } catch (error) {
      console.error(`Error: ${(error as Error).message}`);
      process.exit(1);
    }
  });

program.parse(process.argv);
