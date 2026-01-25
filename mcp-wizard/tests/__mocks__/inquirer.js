// Mock for inquirer (interactive CLI prompts)
const inquirer = {
  prompt: jest.fn(),
};

module.exports = inquirer;
module.exports.default = inquirer;
