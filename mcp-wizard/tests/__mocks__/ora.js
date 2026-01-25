// Mock for ora (terminal spinner)
const ora = jest.fn(() => ({
  start: jest.fn().mockReturnThis(),
  stop: jest.fn().mockReturnThis(),
  succeed: jest.fn(),
  fail: jest.fn(),
  text: '',
}));

module.exports = ora;
module.exports.default = ora;
