# SPEC: cmd/ai-review/SPEC.md
Feature: AI review semantic owner capacity
  Semantic ownership review must admit the build-authority contract without
  relaxing its fixed shard, concurrency, wave, prompt, or index ceilings.

  Scenario: Authority-sized review capacity remains explicit and bounded
    Given AI review package "cmd/ai-review" is configured
    When AGM validates AI review capacity coverage
    Then AI review package "cmd/ai-review" should have a co-located SPEC
    And AI review requirement "AIREV-22" should contain "192-KiB owner candidate"
    And AI review requirement "AIREV-22" should contain "304-KiB owner shard"
    And AI review requirement "AIREV-33" should contain "177,631-byte source shape"
    And AI review requirement "AIREV-33" should contain "eight shards"
    And AI review requirement "AIREV-33" should contain "four concurrent calls"
    And AI review requirement "AIREV-33" should contain "two request waves"
    And AI review requirement "AIREV-33" should contain "one spare shard"
    And AI review requirement "AIREV-33" should contain "640-KiB prompt bound"
