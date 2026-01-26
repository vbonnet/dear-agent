import inquirer from 'inquirer';

/**
 * Run Sequential Thinking MCP setup guide (informational only)
 */
export async function runSequentialThinkingSetupGuide(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Sequential Thinking MCP Setup                                ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('The Sequential Thinking MCP enhances AI reasoning by breaking down');
  console.log('complex problems into structured steps.\n');

  console.log('Features:');
  console.log('  ✓ No authentication required');
  console.log('  ✓ Thought process logging enabled by default');
  console.log('  ✓ Automatic installation via npx\n');

  console.log('Note: Thought logging may add ~50-100ms latency per AI response.');
  console.log('To disable: Set DISABLE_THOUGHT_LOGGING=true environment variable.\n');

  console.log('✓ Sequential Thinking MCP configured\n');
}

/**
 * Show Sequential Thinking setup summary
 */
export async function showSequentialThinkingSetupSummary(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Sequential Thinking Setup Complete                           ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('What was set up:');
  console.log('  ✓ Sequential Thinking MCP configured');
  console.log('  ✓ Thought logging enabled');
  console.log('  ℹ  MCP server will download on first use\n');

  console.log('Next steps:');
  console.log('  1. Restart Claude Code (or reload window)');
  console.log('  2. Try: "Break down this problem step-by-step"');
  console.log('  3. Try: "Plan the implementation for [feature]"\n');

  console.log('Advanced:');
  console.log('  • To disable thought logging: export DISABLE_THOUGHT_LOGGING=true\n');
}
