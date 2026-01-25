/**
 * Mock for chalk (terminal colors)
 *
 * Simplifies chalk for testing - just returns the text without colors
 */

const createChalkMock = (text) => text;

// Create chainable mock
const chalk = new Proxy(createChalkMock, {
  get(target, prop) {
    if (prop === 'default') {
      return chalk;
    }
    // Return a function that returns the text (for chaining)
    return (...args) => args.join(' ');
  },
  apply(target, thisArg, args) {
    return args.join(' ');
  }
});

// Add common color methods
chalk.red = (text) => text;
chalk.green = (text) => text;
chalk.yellow = (text) => text;
chalk.blue = (text) => text;
chalk.cyan = (text) => text;
chalk.dim = (text) => text;
chalk.bold = (text) => text;

module.exports = chalk;
module.exports.default = chalk;
