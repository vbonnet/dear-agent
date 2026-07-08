# SPEC: pkg/llm/provider/SPEC.md
# RELATED-SPEC: agm/internal/llm/SPEC.md
# RELATED-SPEC: internal/pricing/SPEC.md
Feature: Model family provider parity
  Dear-agent should route supported model families through one provider
  resolver/factory contract. AGM owns the family defaults, and pkg/llm/provider
  owns the concrete provider routing that makes those defaults executable.

  Scenario Outline: OpenRouter-hosted supported model families resolve through OpenRouter
    Given LLM model identifier "<model>" for model family "<family>"
    When dear-agent resolves the LLM provider family
    Then the resolved provider family should be "openrouter"
    And the resolved provider model should be "<model>"

    Examples:
      | family   | model                    |
      | glm      | z-ai/glm-5.2             |
      | deepseek | deepseek/deepseek-v4-pro |
      | nemotron | nvidia/nemotron-3-ultra  |
      | qwen     | qwen/qwen3.6-max         |

  Scenario Outline: AGM model-family defaults resolve through the provider resolver
    Given AGM model family "<family>" has a default route
    When dear-agent resolves the default route through the LLM provider resolver
    Then the resolved provider family should be "openrouter"
    And the resolved provider model should not be empty

    Examples:
      | family   |
      | glm      |
      | deepseek |
      | nemotron |
      | qwen     |

  Scenario: OpenRouter factory constructs the provider for GLM routing
    Given OpenRouter API key authentication is configured
    When dear-agent creates provider family "openrouter" with model "z-ai/glm-5.2"
    Then the created provider should be named "openrouter"

  Scenario Outline: OpenRouter capabilities advertise priority model-family defaults
    Given OpenRouter API key authentication is configured
    When dear-agent reads OpenRouter provider capabilities
    Then OpenRouter capabilities should include model "<model>"

    Examples:
      | model                    |
      | z-ai/glm-5.2             |
      | deepseek/deepseek-v4-pro |
      | nvidia/nemotron-3-ultra  |
      | qwen/qwen3.6-max         |
