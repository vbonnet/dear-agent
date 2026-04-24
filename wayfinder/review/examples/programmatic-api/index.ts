/**
 * Multi-Persona Review - Programmatic API Example
 *
 * This example demonstrates how to use multi-persona-review as a library
 * in your TypeScript/JavaScript applications.
 */

import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  createVertexAIReviewer,
  createVertexAIClaudeReviewer,
  formatReviewResult,
  formatReviewResultJSON,
  formatReviewResultGitHub,
  deduplicateFindings,
  createCostSink,
  type ReviewConfig,
  type ReviewResult,
  type ReviewerFunction,
  type Persona,
} from '@wayfinder/multi-persona-review';

/**
 * Example 1: Basic review with Anthropic (Claude)
 */
async function basicReviewExample() {
  console.log('═══════════════════════════════════════════════');
  console.log('Example 1: Basic Review with Anthropic');
  console.log('═══════════════════════════════════════════════\n');

  // 1. Load personas
  const personas = await loadPersonas([
    '.wayfinder/personas',
    process.env.HOME + '/.wayfinder/personas',
  ]);

  console.log(`✓ Loaded ${personas.size} personas`);

  // 2. Select specific personas
  const securityPersona = personas.get('security-engineer');
  const codeHealthPersona = personas.get('code-health');

  if (!securityPersona || !codeHealthPersona) {
    console.error('Required personas not found');
    return;
  }

  // 3. Create Anthropic reviewer
  const reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
    model: 'claude-3-5-sonnet-20241022',
  });

  console.log('✓ Created Anthropic reviewer\n');

  // 4. Create sample code to review
  const testFile = '/tmp/sample-code.ts';
  const sampleCode = `
export class UserService {
  // ISSUE: Hardcoded credentials
  private apiKey = 'sk-1234567890abcdef';

  async fetchUser(id: string) {
    // ISSUE: No error handling
    const response = await fetch(\`https://api.example.com/users/\${id}\`);
    return response.json();
  }
}
`;

  // Write sample code to file
  await import('fs/promises').then(fs => fs.writeFile(testFile, sampleCode, 'utf-8'));

  // 5. Run review
  console.log('Running review...\n');

  const result = await reviewFiles(
    {
      files: [testFile],
      personas: [securityPersona, codeHealthPersona],
      mode: 'thorough',
      fileScanMode: 'full',
    },
    process.cwd(),
    reviewer
  );

  // 6. Format and display results
  const output = formatReviewResult(result, {
    colors: true,
    groupByFile: true,
    showCost: true,
    showSummary: true,
  });

  console.log(output);
  console.log('\n');
}

/**
 * Example 2: Using different AI providers
 */
async function multiProviderExample() {
  console.log('═══════════════════════════════════════════════');
  console.log('Example 2: Using Different AI Providers');
  console.log('═══════════════════════════════════════════════\n');

  const personas = await loadPersonas(['.wayfinder/personas']);
  const selectedPersonas = Array.from(personas.values()).slice(0, 2);

  let reviewer: ReviewerFunction;

  // Choose provider based on environment
  if (process.env.ANTHROPIC_API_KEY) {
    console.log('Using Anthropic (Claude)...');
    reviewer = createAnthropicReviewer({
      apiKey: process.env.ANTHROPIC_API_KEY,
      model: 'claude-3-5-sonnet-20241022',
    });
  } else if (process.env.VERTEX_PROJECT_ID && process.env.VERTEX_MODEL?.includes('claude')) {
    console.log('Using VertexAI (Claude)...');
    reviewer = createVertexAIClaudeReviewer({
      projectId: process.env.VERTEX_PROJECT_ID,
      location: process.env.VERTEX_LOCATION || 'us-east5',
      model: process.env.VERTEX_MODEL || 'claude-sonnet-4-5@20250929',
    });
  } else if (process.env.VERTEX_PROJECT_ID) {
    console.log('Using VertexAI (Gemini)...');
    reviewer = createVertexAIReviewer({
      projectId: process.env.VERTEX_PROJECT_ID,
      location: process.env.VERTEX_LOCATION || 'us-central1',
      model: process.env.VERTEX_MODEL || 'gemini-2.5-flash',
    });
  } else {
    console.error('No API credentials configured');
    console.error('Set ANTHROPIC_API_KEY or VERTEX_PROJECT_ID');
    return;
  }

  const result = await reviewFiles(
    {
      files: ['/tmp/sample-code.ts'],
      personas: selectedPersonas,
      mode: 'quick',
      fileScanMode: 'full',
    },
    process.cwd(),
    reviewer
  );

  console.log(`✓ Review complete: ${result.findings.length} findings`);
  console.log(`✓ Cost: $${result.cost.toFixed(3)}\n`);
}

/**
 * Example 3: JSON output for integration
 */
async function jsonOutputExample() {
  console.log('═══════════════════════════════════════════════');
  console.log('Example 3: JSON Output for Integration');
  console.log('═══════════════════════════════════════════════\n');

  if (!process.env.ANTHROPIC_API_KEY && !process.env.VERTEX_PROJECT_ID) {
    console.log('Skipping: No API credentials\n');
    return;
  }

  const personas = await loadPersonas(['.wayfinder/personas']);
  const reviewer = process.env.ANTHROPIC_API_KEY
    ? createAnthropicReviewer({ apiKey: process.env.ANTHROPIC_API_KEY })
    : createVertexAIReviewer({ projectId: process.env.VERTEX_PROJECT_ID! });

  const result = await reviewFiles(
    {
      files: ['/tmp/sample-code.ts'],
      personas: Array.from(personas.values()).slice(0, 2),
      mode: 'quick',
      fileScanMode: 'full',
    },
    process.cwd(),
    reviewer
  );

  // Export as JSON
  const jsonOutput = formatReviewResultJSON(result);

  console.log('JSON Output:');
  console.log('─────────────────────────────────────────────');
  console.log(JSON.stringify(JSON.parse(jsonOutput), null, 2).substring(0, 500) + '...');
  console.log('─────────────────────────────────────────────\n');

  // Save to file
  await import('fs/promises').then(fs =>
    fs.writeFile('/tmp/review-results.json', jsonOutput, 'utf-8')
  );

  console.log('✓ Saved to /tmp/review-results.json\n');
}

/**
 * Example 4: GitHub format for PR comments
 */
async function githubFormatExample() {
  console.log('═══════════════════════════════════════════════');
  console.log('Example 4: GitHub PR Comment Format');
  console.log('═══════════════════════════════════════════════\n');

  if (!process.env.ANTHROPIC_API_KEY && !process.env.VERTEX_PROJECT_ID) {
    console.log('Skipping: No API credentials\n');
    return;
  }

  const personas = await loadPersonas(['.wayfinder/personas']);
  const reviewer = process.env.ANTHROPIC_API_KEY
    ? createAnthropicReviewer({ apiKey: process.env.ANTHROPIC_API_KEY })
    : createVertexAIReviewer({ projectId: process.env.VERTEX_PROJECT_ID! });

  const result = await reviewFiles(
    {
      files: ['/tmp/sample-code.ts'],
      personas: Array.from(personas.values()).slice(0, 2),
      mode: 'quick',
      fileScanMode: 'full',
    },
    process.cwd(),
    reviewer
  );

  // Format as GitHub comment
  const githubComment = formatReviewResultGitHub(result);

  console.log('GitHub PR Comment:');
  console.log('─────────────────────────────────────────────');
  console.log(githubComment);
  console.log('─────────────────────────────────────────────\n');
}

/**
 * Example 5: Cost tracking
 */
async function costTrackingExample() {
  console.log('═══════════════════════════════════════════════');
  console.log('Example 5: Cost Tracking');
  console.log('═══════════════════════════════════════════════\n');

  if (!process.env.ANTHROPIC_API_KEY && !process.env.VERTEX_PROJECT_ID) {
    console.log('Skipping: No API credentials\n');
    return;
  }

  const personas = await loadPersonas(['.wayfinder/personas']);
  const reviewer = process.env.ANTHROPIC_API_KEY
    ? createAnthropicReviewer({ apiKey: process.env.ANTHROPIC_API_KEY })
    : createVertexAIReviewer({ projectId: process.env.VERTEX_PROJECT_ID! });

  // Create cost sink to track costs
  const costSink = createCostSink({
    type: 'file',
    filePath: '/tmp/review-costs.jsonl',
  });

  const result = await reviewFiles(
    {
      files: ['/tmp/sample-code.ts'],
      personas: Array.from(personas.values()).slice(0, 2),
      mode: 'quick',
      fileScanMode: 'full',
      costSink,
    },
    process.cwd(),
    reviewer
  );

  console.log(`✓ Review cost: $${result.cost.toFixed(4)}`);
  console.log(`✓ Cost details saved to /tmp/review-costs.jsonl\n`);
}

/**
 * Main function - run all examples
 */
async function main() {
  console.log('\n');
  console.log('╔═══════════════════════════════════════════════╗');
  console.log('║  Multi-Persona Review - Programmatic API     ║');
  console.log('╚═══════════════════════════════════════════════╝');
  console.log('\n');

  try {
    // Run examples
    await basicReviewExample();
    await multiProviderExample();
    await jsonOutputExample();
    await githubFormatExample();
    await costTrackingExample();

    console.log('═══════════════════════════════════════════════');
    console.log('✅ All examples completed successfully!');
    console.log('═══════════════════════════════════════════════\n');

    console.log('Next steps:');
    console.log('  • Integrate into your application');
    console.log('  • Create custom review workflows');
    console.log('  • Build automated quality gates');
    console.log('  • See README.md for more API examples\n');

  } catch (error) {
    console.error('\n❌ Error running examples:');
    console.error(error);
    process.exit(1);
  }
}

// Run if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}

// Export for use as a module
export {
  basicReviewExample,
  multiProviderExample,
  jsonOutputExample,
  githubFormatExample,
  costTrackingExample,
};
