export class SetupError extends Error {
  constructor(
    public problem: string,
    public fix: string,
    public helpLink: string = 'https://github.com/your-org/mcp-wizard/issues'
  ) {
    super(`${problem}\n\nFix: ${fix}\nHelp: ${helpLink}`);
    this.name = 'SetupError';
  }
}
