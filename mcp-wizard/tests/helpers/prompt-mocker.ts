/**
 * Inquirer prompt mocking utilities
 *
 * Provides helpers for mocking multi-step inquirer wizard flows in tests.
 *
 * @example
 * ```typescript
 * mockPromptSequence([
 *   { shouldResume: false },                          // Step 1
 *   { selectedAgents: ['claude-code'] },              // Step 2
 *   { selectedMcps: ['GoogleDocs', 'Atlassian'] },    // Step 3
 *   { confirmSetup: true },                           // Step 4
 * ]);
 *
 * await setupCommand({});
 *
 * expect(inquirer.prompt).toHaveBeenCalledTimes(4);
 * ```
 */

import * as inquirer from 'inquirer';

/**
 * Mock a sequence of inquirer prompts
 *
 * Configures inquirer.prompt to return pre-defined responses in sequence.
 * Each call to prompt() will return the next response in the sequence.
 *
 * @param responses - Array of responses, one per prompt call
 *
 * @example
 * ```typescript
 * // Mock a 3-step wizard
 * mockPromptSequence([
 *   { name: 'Alice' },           // First prompt() call returns this
 *   { age: 30 },                 // Second prompt() call returns this
 *   { confirmed: true },         // Third prompt() call returns this
 * ]);
 *
 * const r1 = await inquirer.prompt([{ name: 'name', message: 'Name?' }]);
 * expect(r1.name).toBe('Alice');
 *
 * const r2 = await inquirer.prompt([{ name: 'age', message: 'Age?' }]);
 * expect(r2.age).toBe(30);
 *
 * const r3 = await inquirer.prompt([{ name: 'confirmed', message: 'OK?' }]);
 * expect(r3.confirmed).toBe(true);
 * ```
 */
export function mockPromptSequence(
  responses: Record<string, any>[]
): jest.Mock {
  const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

  // Clear any previous mocks
  mockPrompt.mockClear();

  // Chain mockResolvedValueOnce for each response
  responses.forEach((response, index) => {
    mockPrompt.mockResolvedValueOnce(response);
  });

  return mockPrompt;
}

/**
 * Mock a single inquirer prompt
 *
 * Convenience function for tests with a single prompt.
 *
 * @param response - The response to return
 *
 * @example
 * ```typescript
 * mockPrompt({ confirmed: true });
 *
 * const result = await inquirer.prompt([...]);
 * expect(result.confirmed).toBe(true);
 * ```
 */
export function mockPrompt(response: Record<string, any>): jest.Mock {
  return mockPromptSequence([response]);
}

/**
 * Mock inquirer prompts with conditional responses
 *
 * Useful for testing branching wizard flows where later prompts depend on
 * earlier answers.
 *
 * @param responseFn - Function that returns response based on call count
 *
 * @example
 * ```typescript
 * let callCount = 0;
 * mockPromptConditional(() => {
 *   callCount++;
 *   if (callCount === 1) return { setupType: 'advanced' };
 *   if (callCount === 2) return { advancedOption: 'custom' };
 *   return { confirmed: true };
 * });
 * ```
 */
export function mockPromptConditional(
  responseFn: (callCount: number) => Record<string, any>
): jest.Mock {
  const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

  mockPrompt.mockClear();

  let callCount = 0;
  mockPrompt.mockImplementation(async () => {
    callCount++;
    return responseFn(callCount);
  });

  return mockPrompt;
}

/**
 * Verify prompt was called with expected questions
 *
 * Helper for asserting that inquirer.prompt was called with specific question names.
 *
 * @param expectedNames - Array of question names in order
 *
 * @example
 * ```typescript
 * mockPromptSequence([{ name: 'Alice' }, { age: 30 }]);
 *
 * await setupWizard();
 *
 * verifyPromptQuestions(['name', 'age']);  // Passes
 * verifyPromptQuestions(['name']);         // Fails - missing age question
 * ```
 */
export function verifyPromptQuestions(expectedNames: string[]): void {
  const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

  expect(mockPrompt).toHaveBeenCalledTimes(expectedNames.length);

  expectedNames.forEach((name, index) => {
    const call = mockPrompt.mock.calls[index];
    const questions = call[0];

    // questions can be a single object or an array
    const questionArray = Array.isArray(questions) ? questions : [questions];

    const hasQuestion = questionArray.some((q: any) => q.name === name);
    expect(hasQuestion).toBe(true);
  });
}

/**
 * Create a mock for inquirer.prompt that throws an error
 *
 * Useful for testing user cancellation (Ctrl+C) scenarios.
 *
 * @param errorMessage - Optional custom error message
 *
 * @example
 * ```typescript
 * mockPromptError('User cancelled');
 *
 * await expect(setupCommand({})).rejects.toThrow('User cancelled');
 * ```
 */
export function mockPromptError(errorMessage: string = 'User cancelled'): jest.Mock {
  const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

  mockPrompt.mockClear();
  mockPrompt.mockRejectedValue(new Error(errorMessage));

  return mockPrompt;
}

/**
 * Reset inquirer.prompt mock to default behavior
 *
 * Clears all mock implementations and call history.
 *
 * @example
 * ```typescript
 * afterEach(() => {
 *   resetPromptMock();
 * });
 * ```
 */
export function resetPromptMock(): void {
  const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;
  mockPrompt.mockReset();
}
