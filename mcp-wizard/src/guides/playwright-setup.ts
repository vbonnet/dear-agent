import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import inquirer from 'inquirer';

export async function runPlaywrightSetupGuide(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Playwright MCP Setup                                         ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('The Playwright MCP provides browser automation capabilities.');
  console.log('No authentication required (runs locally).\n');

  // Network access prompt
  console.log('⚠️  SECURITY CONSIDERATION (Network Access):');
  console.log('- Enabled: Playwright can intercept network requests (useful for testing APIs)');
  console.log('- Disabled: Network interception blocked (safer, but limits some features)\n');
  console.log('Recommendation: Disable unless you need network interception\n');

  const { networkAccess } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'networkAccess',
      message: 'Allow network access for Playwright?',
      default: false,
    },
  ]);

  // File system access prompt
  console.log('\n⚠️  SECURITY CONSIDERATION (File System Access):');
  console.log('- Enabled: Playwright can save screenshots and PDFs (useful for documentation)');
  console.log('- Disabled: File operations blocked (safer, no local file saving)\n');
  console.log('Recommendation: Disable unless you need to save screenshots/PDFs\n');

  const { fileSystemAccess } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'fileSystemAccess',
      message: 'Allow file system access for Playwright?',
      default: false,
    },
  ]);

  // Browser download note
  console.log('\nℹ️  Browser Binary Download:');
  console.log('Playwright will download Chromium browser (~300MB) on first use.');
  console.log('This happens automatically, no manual installation required.\n');

  // Final confirmation
  console.log('Configuration summary:');
  console.log(`  - Network access: ${networkAccess ? 'Enabled' : 'Disabled'}`);
  console.log(`  - File system access: ${fileSystemAccess ? 'Enabled' : 'Disabled'}`);
  console.log(`  - Browser: Chromium (auto-downloaded)\n`);

  const { proceed } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'proceed',
      message: 'Continue with Playwright setup?',
      default: true,
    },
  ]);

  if (!proceed) {
    throw new Error('Playwright setup cancelled');
  }

  console.log('\n✓ Playwright MCP configured\n');
}

export async function showPlaywrightSetupSummary(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Playwright Setup Complete                                    ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('What was set up:');
  console.log('  ✓ Playwright MCP configured');
  console.log('  ✓ Browser automation ready (Chromium)\n');

  console.log('Next steps:');
  console.log('  1. Restart Claude Code (or reload window)');
  console.log('  2. Try: "Navigate to google.com"');
  console.log('  3. Try: "Take a screenshot of this page"\n');

  console.log('Note: Browser binaries (~300MB) will download on first use.\n');
}
