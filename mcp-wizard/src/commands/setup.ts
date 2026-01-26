import inquirer from 'inquirer';
import { detectEnvironment } from '../lib/detect';
import { installMcpServers } from '../lib/install';
import { runGcpSetupGuide, showGcpSetupSummary } from '../guides/gcp-setup';
import { runGitHubSetupGuide, showGitHubSetupSummary } from '../guides/github-setup';
// Atlassian uses mcp-remote which handles OAuth automatically - no setup needed
// import { runAtlassianSetupGuide, showAtlassianSetupSummary } from '../guides/atlassian-setup';
// Slack requires workspace admin to create official app
// import { runSlackSetupGuide, showSlackSetupSummary } from '../guides/slack-setup';
import { oauthFlow } from '../lib/oauth';
import { generateMcpConfig, writeMcpConfig, showChezmoiSnippet, promptMcpSelection, SUPPORTED_MCPS } from '../lib/config';
import { automateChezmoiSetup } from '../lib/chezmoi-manager';
import { loadState, saveState, clearState, createNewState, updateState, SETUP_STATES } from '../lib/state';
import { sanitizeError } from '../lib/errors';
import { PrerequisitesValidator } from '../validators/prerequisites';
import { SetupVerifier } from '../verifiers/setup-verifier';
import { ConfigLocationDetector } from '../config/location-detector';
import { ProgressTracker } from '../ui/progress-tracker';
import { AdapterFactory } from '../adapters/AdapterFactory';
import { MCPServer } from '../adapters/PlatformAdapter';

interface SetupOptions {
  dryRun?: boolean;
  verbose?: boolean;
  skipInstall?: boolean;
  skipAuth?: boolean;
  resume?: boolean;
  agents?: string;
  mcps?: string;
  platform?: string;
  yes?: boolean;
}

/**
 * Convert legacy McpConfig to platform-agnostic MCPServer[] format
 */
function convertToMCPServers(config: any): MCPServer[] {
  const servers: MCPServer[] = [];

  for (const [name, server] of Object.entries(config.mcpServers)) {
    const s = server as any;

    // Determine server type from config structure
    if (s.command) {
      // stdio server
      servers.push({
        name,
        type: 'stdio',
        command: s.command,
        args: s.args || [],
        env: s.env || {},
      });
    } else if (s.url) {
      // HTTP or SSE server (detect based on URL or explicit type)
      const type = s.type === 'sse' ? 'sse' : 'http';
      servers.push({
        name,
        type,
        url: s.url,
        headers: s.headers || {},
      });
    }
  }

  return servers;
}

export async function setupCommand(options: SetupOptions = {}): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   MCP Setup Wizard                                             ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  try {
    // Prerequisites check (runs before any other checks)
    const progress = new ProgressTracker();
    progress.startStep(1, 5, 'Checking prerequisites', '~5s');

    const validator = new PrerequisitesValidator();
    const results = await validator.validateAll();

    const failures = results.filter((r) => !r.passed);

    if (failures.length > 0) {
      progress.failStep('Prerequisites check failed');

      console.error('\n✗ Prerequisites check failed:\n');
      for (const failure of failures) {
        console.error(`  ✗ ${failure.name}: ${failure.error}`);
        console.error(`    Fix: ${failure.fix}\n`);
      }
      console.error('Please fix the above issues and restart the wizard.');
      process.exit(1);
    }

    progress.completeStep();
    console.log('✓ Prerequisites validated\n');

    // Check for sudo
    if (detectSudo()) {
      console.error('✗ Error: Do not run as root (no sudo needed)');
      console.error('  This tool operates on your user files only.');
      process.exit(1);
    }

    // Load or create state
    let state = options.resume ? await loadState() : null;
    if (state) {
      const { shouldResume } = await inquirer.prompt([
        {
          type: 'confirm',
          name: 'shouldResume',
          message: `Resume from previous setup (${state.currentState})?`,
          default: true,
        },
      ]);

      if (!shouldResume) {
        state = createNewState();
      }
    } else {
      state = createNewState();
    }

    // 1. Environment detection
    if (!state.completedSteps.includes('DETECT_ENVIRONMENT')) {
      console.log('─── Detecting environment ───\n');
      const env = await detectEnvironment();

      if (!env.isWorkMachine) {
        console.error(`✗ Error: Not a work machine (hostname: ${env.hostname})`);
        console.error('  Work machines must have hostnames ending with "-w"');
        process.exit(1);
      }

      if (!env.nodeVersionValid) {
        console.error(`✗ Error: Node.js version ${env.nodeVersion} is too old`);
        console.error('  Require Node.js >= 18.0.0');
        process.exit(1);
      }

      console.log(`✓ Work machine detected: ${env.hostname}`);
      console.log(`✓ Node.js: ${env.nodeVersion}`);
      if (env.chezmoiDetected) {
        console.log('✓ Chezmoi detected - config is managed');
      }

      state = updateState(state, SETUP_STATES.CHECK_MCP_INSTALLATION, 'DETECT_ENVIRONMENT');
      state.context.chezmoiDetected = env.chezmoiDetected;
      state.context.workMachine = true;
      await saveState(state);
    }

    // 2. MCP Selection
    let selectedMcps: string[] = [];
    if (!state.context.selectedMcps) {
      if (options.mcps) {
        // Parse MCP selection from CLI args
        const mcpInput = options.mcps.split(',').map(m => m.trim().toLowerCase());
        const validMcpIds = SUPPORTED_MCPS.map(m => m.id);
        selectedMcps = mcpInput.filter(id => validMcpIds.includes(id));

        if (selectedMcps.length === 0) {
          console.error('✗ Error: Invalid MCP names. Valid options: googledocs, atlassian');
          process.exit(1);
        }
      } else {
        // Interactive MCP selection
        selectedMcps = await promptMcpSelection();
      }

      state.context.selectedMcps = selectedMcps;
      await saveState(state);
    } else {
      // Resume from saved state
      selectedMcps = state.context.selectedMcps;
    }

    console.log(`\n✓ Selected MCPs: ${selectedMcps.join(', ')}\n`);

    // 3. MCP installation (only GoogleDocs needs installation)
    if (!state.completedSteps.includes('INSTALL_MCP') && !options.skipInstall) {
      if (selectedMcps.includes('googledocs')) {
        console.log('\n─── Installing MCP servers ───\n');

        if (options.dryRun) {
          console.log('[DRY RUN] Would install Google Docs MCP to ~/mcp-servers/google-docs-mcp');
        } else {
          await installMcpServers();
          state = updateState(state, SETUP_STATES.CHECK_CREDENTIALS, 'INSTALL_MCP');
          await saveState(state);
        }
      } else {
        // Skip installation if GoogleDocs not selected
        state = updateState(state, SETUP_STATES.CHECK_CREDENTIALS, 'INSTALL_MCP');
        await saveState(state);
      }
    }

    // 4. GoogleDocs OAuth (if selected)
    if (selectedMcps.includes('googledocs')) {
      if (!state.completedSteps.includes('UPLOAD_CREDENTIALS')) {
        console.log('');
        if (options.dryRun) {
          console.log('[DRY RUN] Would run GCP Console setup guide');
        } else {
          const credentialsPath = await runGcpSetupGuide();
          await showGcpSetupSummary();

          state = updateState(state, SETUP_STATES.OAUTH_FLOW, 'UPLOAD_CREDENTIALS');
          state.context.credentialsPath = credentialsPath;
          await saveState(state);
        }
      }

      if (!state.completedSteps.includes('OAUTH_FLOW') && !options.skipAuth) {
        console.log('');
        if (options.dryRun) {
          console.log('[DRY RUN] Would run Google OAuth flow');
        } else {
          const credentialsPath = state.context.credentialsPath!;
          const tokenPath = credentialsPath.replace('credentials.json', 'token.json');

          await oauthFlow(credentialsPath, tokenPath);

          state = updateState(state, SETUP_STATES.OAUTH_FLOW, 'OAUTH_FLOW');
          state.context.tokenPath = tokenPath;
          await saveState(state);
        }
      }
    }

    // 5. GitHub Setup (if selected)
    if (selectedMcps.includes('github')) {
      if (!state.completedSteps.includes('GITHUB_SETUP')) {
        console.log('');
        if (options.dryRun) {
          console.log('[DRY RUN] Would run GitHub setup guide');
        } else {
          await runGitHubSetupGuide();
          await showGitHubSetupSummary();

          state = updateState(state, SETUP_STATES.WRITE_CONFIG, 'GITHUB_SETUP');
          await saveState(state);
        }
      }
    }

    // 6. Atlassian OAuth - HANDLED AUTOMATICALLY by mcp-remote
    // Check if we're in SSH and warn about port forwarding requirement
    if (selectedMcps.includes('atlassian')) {
      const isSSH = !!(process.env.SSH_CLIENT || process.env.SSH_TTY || process.env.SSH_CONNECTION);

      if (isSSH) {
        console.log('\n╔════════════════════════════════════════════════════════════════╗');
        console.log('║   ⚠️  SSH Environment Detected                                 ║');
        console.log('╚════════════════════════════════════════════════════════════════╝\n');
        console.log('Atlassian MCP requires OAuth with browser-based authentication.');
        console.log('This won\'t work over SSH without port forwarding.\n');

        const env = await detectEnvironment();
        const hostname = env.hostname;

        console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
        console.log('OPTION 1: SSH Port Forwarding (Recommended)');
        console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');
        console.log('On your LOCAL machine (Mac/laptop), add this to ~/.ssh/config:\n');
        console.log('Host ' + hostname);
        console.log('    RemoteForward 5598 localhost:5598\n');
        console.log('Or for all work machines:\n');
        console.log('Host *-w');
        console.log('    RemoteForward 5598 localhost:5598\n');
        console.log('Then reconnect your SSH session.');
        console.log('');
        console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
        console.log('OPTION 2: Use API Token-Based Alternative');
        console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');
        console.log('Use community MCP server instead: @sooperset/mcp-atlassian');
        console.log('Supports API tokens (no OAuth required)');
        console.log('See: https://github.com/sooperset/mcp-atlassian\n');

        if (!options.yes) {
          await inquirer.prompt([
            {
              type: 'input',
              name: 'confirm',
              message: 'Press Enter when you\'ve set up port forwarding (or chosen alternative)...',
              default: '',
            },
          ]);
        }
      }
    }

    // 6. Slack token setup (if selected)
    // NOTE: Commented out - requires workspace admin to create Slack app
    // if (selectedMcps.includes('slack') && !options.skipAuth) {
    //   if (!state.completedSteps.includes('SLACK_TOKEN')) {
    //     console.log('');
    //     if (options.dryRun) {
    //       console.log('[DRY RUN] Would run Slack token setup');
    //     } else {
    //       const { botToken, teamId } = await runSlackSetupGuide();
    //       await showSlackSetupSummary();
    //
    //       state = updateState(state, SETUP_STATES.UPDATE_CONFIG, 'SLACK_TOKEN');
    //       state.context.slackBotToken = botToken;
    //       state.context.slackTeamId = teamId;
    //       await saveState(state);
    //     }
    //   }
    // }

    // 7. Config generation
    if (!state.completedSteps.includes('UPDATE_CONFIG')) {
      console.log('\n─── Configuring MCP servers ───\n');

      if (options.dryRun) {
        console.log('[DRY RUN] Would generate and write MCP config');
      } else {
        const config = await generateMcpConfig(selectedMcps);

        // Parse agent selection if provided via CLI
        let selectedAgents: string[] | undefined;
        if (options.agents) {
          const agentInput = options.agents.split(',').map(a => a.trim().toLowerCase());
          const agentMap: Record<string, string> = {
            'claude-code': 'Claude Code',
            'cursor': 'Cursor',
            'cline': 'Cline',
            'windsurf': 'Windsurf',
          };
          selectedAgents = agentInput.map(a => agentMap[a]).filter(Boolean);

          if (selectedAgents.length === 0) {
            console.error('✗ Error: Invalid agent names. Valid options: claude-code, cursor, cline, windsurf');
            process.exit(1);
          }
        } else if (state.context.selectedAgents) {
          // Resume from saved state
          selectedAgents = state.context.selectedAgents;
        }

        if (state.context.chezmoiDetected) {
          // Ask user preference for automated apply (or use default if --yes flag is set)
          let shouldAutoApply = true;
          let wantDiff = false;

          if (!options.yes) {
            const response = await inquirer.prompt([{
              type: 'confirm',
              name: 'shouldAutoApply',
              message: 'Apply via chezmoi?',
              default: true,
            }]);
            shouldAutoApply = response.shouldAutoApply;

            if (shouldAutoApply) {
              // Optional diff preview
              const diffResponse = await inquirer.prompt([{
                type: 'confirm',
                name: 'wantDiff',
                message: 'Show diff before applying?',
                default: false,
              }]);
              wantDiff = diffResponse.wantDiff;
            }
          } else {
            console.log('✓ Auto-applying via chezmoi (--yes flag)');
          }

          if (shouldAutoApply) {

            // Attempt automated setup
            console.log('\n─── Applying configuration via chezmoi ───\n');
            const result = await automateChezmoiSetup(config, 'claude-code', { showDiff: wantDiff });

            if (result.method === 'automated') {
              console.log('\n✓ Applied MCP config via chezmoi');
              if (result.result?.output) {
                console.log(`  ${result.result.output}`);
              }
            } else {
              // Fall back to manual instructions
              console.error(`\n✗ Chezmoi apply failed: ${result.error}`);
              console.log('\nFalling back to manual instructions...\n');
              await showChezmoiSnippet(config, options.yes);
            }
          } else {
            // User declined automation, show manual instructions
            await showChezmoiSnippet(config, options.yes);
          }

          state = updateState(state, SETUP_STATES.VERIFY_SETUP, 'UPDATE_CONFIG');
        } else {
          // Use platform adapter for all platforms (fixes Claude Code bug)
          const platform = options.platform || 'claude';

          // Check if user explicitly requested --agents flag (legacy behavior)
          // If not, use platform adapter for Claude Code (fixes ~/.claude.json bug)
          const usePlatformAdapter = !selectedAgents || platform !== 'claude';

          if (usePlatformAdapter) {
            console.log(`\n─── Configuring for ${platform} platform ───\n`);

            try {
              const adapter = AdapterFactory.create(platform);
              const servers = convertToMCPServers(config);
              await adapter.configure(servers);
              console.log(`\n✓ Configured ${servers.length} MCP server(s) for ${platform}`);
            } catch (error: any) {
              console.error(`\n✗ Platform adapter error: ${error.message}`);
              throw error;
            }
          } else {
            // Use existing writeMcpConfig only if --agents explicitly provided
            // (backward compatibility for multi-agent setup)
            await writeMcpConfig(config, selectedAgents);
          }

          state = updateState(state, SETUP_STATES.VERIFY_SETUP, 'UPDATE_CONFIG');
        }

        // Save selected agents for resume capability
        if (selectedAgents && !state.context.selectedAgents) {
          state.context.selectedAgents = selectedAgents;
        }

        await saveState(state);
      }
    }

    // 8. MCP Verification (optional, non-blocking)
    if (!options.dryRun) {
      console.log('\n─── Verifying MCP connection ───\n');
      progress.startStep(5, 5, 'Verifying MCP connections', '~10s');

      const verifier = new SetupVerifier();
      const verifiedMcps: string[] = [];
      const failedMcps: string[] = [];

      for (const mcpId of selectedMcps) {
        const mcpName = mcpId === 'googledocs' ? 'googledocs' : mcpId;
        const verifyResult = await verifier.verifyMcpConnection(mcpName);

        if (verifyResult.success) {
          verifiedMcps.push(mcpId);
        } else {
          failedMcps.push(mcpId);
        }
      }

      if (failedMcps.length === 0) {
        progress.completeStep();
        console.log(`✓ Verified ${verifiedMcps.length} MCP(s): ${verifiedMcps.join(', ')}`);
        console.log(`  (Verified with: claude mcp list)\n`);
      } else {
        progress.failStep('Some MCPs not verified (non-blocking)');
        console.warn('\n⚠️  Warning: Some MCPs not found in MCP list');
        console.warn(`  Verified: ${verifiedMcps.join(', ') || 'none'}`);
        console.warn(`  Not found: ${failedMcps.join(', ')}`);
        console.warn('  Try restarting Claude Code to load the new config\n');
        // Non-blocking - continue to success message
      }
    }

    // 9. Success!
    await clearState();

    console.log('\n╔════════════════════════════════════════════════════════════════╗');
    console.log('║   Setup Complete!                                              ║');
    console.log('╚════════════════════════════════════════════════════════════════╝\n');

    // Show configured MCPs
    const mcpStatusMap: Record<string, string> = {
      googledocs: 'Google Docs MCP installed and configured',
      atlassian: 'Atlassian MCP configured',
      // glean: 'Glean MCP configured',  // Not available - requires Glean admin token
      // slack: 'Slack MCP configured',  // Not available - requires workspace admin
    };

    for (const mcpId of selectedMcps) {
      console.log(`✓ ${mcpStatusMap[mcpId]}`);
    }
    console.log('');

    console.log('Next steps:');
    console.log('  1. Restart Claude Code (or reload window)');
    console.log('  2. Use Claude Code to interact with your configured MCPs');
    if (selectedMcps.includes('googledocs')) {
      console.log('  3. Try: "List my recent Google Docs"');
    }
    // if (selectedMcps.includes('slack')) {
    //   console.log('  3. Try: "Search Slack for messages about [topic]"');
    // }
    if (selectedMcps.includes('atlassian')) {
      console.log('  3. Try: "Show recent Jira issues"');
      console.log('');
      console.log('Note: Atlassian will prompt for OAuth on first use:');
      console.log('  - Browser will open to mcp.atlassian.com');
      console.log('  - Select your Atlassian site and authorize');
      console.log('  - This is normal - mcp-remote handles OAuth automatically');
    }
    console.log('');

    if (state.context.chezmoiDetected) {
      console.log('Note: Remember to run "chezmoi apply" if you haven\'t already!\n');
    }
  } catch (error) {
    const sanitized = sanitizeError(error as Error);
    console.error(`\n✗ Setup failed: ${sanitized.message}\n`);

    if (options.verbose) {
      console.error('Stack trace:');
      console.error((error as Error).stack);
    }

    console.error('Recovery options:');
    console.error('  • Run: mcp-wizard setup --resume  (continue from last step)');
    console.error('  • Run: mcp-wizard status  (check current state)');
    console.error('  • See: TROUBLESHOOTING.md for manual fixes\n');

    process.exit(1);
  }
}

function detectSudo(): boolean {
  return process.env.SUDO_USER !== undefined || process.getuid?.() === 0;
}
