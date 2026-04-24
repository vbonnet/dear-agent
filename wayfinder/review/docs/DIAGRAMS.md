# Architecture Diagrams

This document provides visual representations of the Multi-Persona Review architecture using Mermaid diagrams.

## Table of Contents

- [System Architecture](#system-architecture)
- [Data Flow](#data-flow)
- [Agency-Agents Flow](#agency-agents-flow)
- [CI/CD Pipeline](#cicd-pipeline)
- [Cost Tracking Flow](#cost-tracking-flow)
- [Persona Loading Sequence](#persona-loading-sequence)

---

## System Architecture

```mermaid
graph TB
    subgraph "Entry Points"
        CLI[CLI<br/>cli.ts]
        API[API<br/>index.ts]
    end

    subgraph "Configuration Layer"
        ConfigLoader[Config Loader<br/>config-loader.ts]
        PersonaLoader[Persona Loader<br/>persona-loader.ts]
        FileScanner[File Scanner<br/>file-scanner.ts]
    end

    subgraph "Review Engine"
        ReviewEngine[Review Engine<br/>review-engine.ts]
        VoteAggregator[Vote Aggregator<br/>Agency-Agents]
        Dedup[Deduplicator<br/>similarity-based]
    end

    subgraph "LLM Clients"
        AnthropicClient[Anthropic Client<br/>Claude Direct]
        VertexGemini[Vertex AI<br/>Gemini]
        VertexClaude[Vertex AI<br/>Claude]
    end

    subgraph "Post-Processing"
        CostTracker[Cost Tracker<br/>cost-sink.ts]
        TextFormatter[Text Formatter<br/>ANSI colors]
        JSONFormatter[JSON Formatter<br/>CI/CD]
        GitHubFormatter[GitHub Formatter<br/>PR comments]
    end

    CLI --> ConfigLoader
    API --> ConfigLoader

    ConfigLoader --> PersonaLoader
    ConfigLoader --> FileScanner

    PersonaLoader --> ReviewEngine
    FileScanner --> ReviewEngine

    ReviewEngine --> AnthropicClient
    ReviewEngine --> VertexGemini
    ReviewEngine --> VertexClaude

    AnthropicClient --> VoteAggregator
    VertexGemini --> VoteAggregator
    VertexClaude --> VoteAggregator

    VoteAggregator --> Dedup

    Dedup --> CostTracker

    CostTracker --> TextFormatter
    CostTracker --> JSONFormatter
    CostTracker --> GitHubFormatter

    style CLI fill:#e1f5ff
    style API fill:#e1f5ff
    style ReviewEngine fill:#fff4e6
    style VoteAggregator fill:#f3e5f5
    style AnthropicClient fill:#e8f5e9
    style VertexGemini fill:#e8f5e9
    style VertexClaude fill:#e8f5e9
```

---

## Data Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Config
    participant Personas
    participant ReviewEngine
    participant LLM
    participant AgencyAgents
    participant Formatter

    User->>CLI: multi-persona-review src/
    CLI->>Config: Load configuration
    Config-->>CLI: CrossCheckConfig

    CLI->>Personas: Load personas
    Personas-->>CLI: Map<name, Persona>

    CLI->>ReviewEngine: reviewFiles(config)

    ReviewEngine->>ReviewEngine: Scan files (git-aware)

    loop For each persona
        ReviewEngine->>LLM: Review files
        LLM-->>ReviewEngine: Findings[]
    end

    ReviewEngine->>AgencyAgents: Aggregate votes
    AgencyAgents-->>ReviewEngine: Weighted decision

    ReviewEngine->>AgencyAgents: Filter by confidence
    AgencyAgents-->>ReviewEngine: High-confidence findings

    ReviewEngine->>ReviewEngine: Deduplicate findings
    ReviewEngine->>ReviewEngine: Track costs

    ReviewEngine-->>CLI: ReviewResult

    CLI->>Formatter: Format results
    Formatter-->>User: Display output
```

---

## Agency-Agents Flow

```mermaid
flowchart TD
    Start([Review Start]) --> ScanFiles[Scan Files]
    ScanFiles --> LoadPersonas[Load Personas<br/>with Tiers]

    LoadPersonas --> ParallelReview{Parallel<br/>Execution?}

    ParallelReview -->|Yes| ParallelExec[Execute All<br/>Personas in Parallel]
    ParallelReview -->|No| SequentialExec[Execute Personas<br/>Sequentially]

    ParallelExec --> CollectFindings[Collect All Findings]
    SequentialExec --> CollectFindings

    CollectFindings --> ExtractVotes[Extract Voting<br/>Decisions]

    ExtractVotes --> CalculateWeights[Calculate Tier Weights<br/>Tier 1=3x, Tier 2=1x, Tier 3=0.5x]

    CalculateWeights --> AggregateVotes[Aggregate Votes<br/>weighted_vote = GO_votes / total_votes]

    AggregateVotes --> VoteDecision{weighted_vote ><br/>threshold?}

    VoteDecision -->|Yes| GODecision[Decision: GO]
    VoteDecision -->|No| NOGODecision[Decision: NO-GO]

    GODecision --> FilterConfidence[Filter by Confidence<br/>≥ minConfidence]
    NOGODecision --> FilterConfidence

    FilterConfidence --> ExtractAlternatives[Extract Lateral<br/>Thinking Alternatives]

    ExtractAlternatives --> DetectScope[Detect Out-of-Scope<br/>Findings]

    DetectScope --> RouteFindings[Route to Appropriate<br/>Personas]

    RouteFindings --> Dedup[Deduplicate Similar<br/>Findings]

    Dedup --> FormatOutput[Format Output]

    FormatOutput --> End([Review Complete])

    style GODecision fill:#c8e6c9
    style NOGODecision fill:#ffcdd2
    style AggregateVotes fill:#f3e5f5
    style FilterConfidence fill:#fff9c4
    style ExtractAlternatives fill:#e1bee7
```

---

## CI/CD Pipeline

```mermaid
graph LR
    subgraph "Developer"
        Commit[Git Commit]
        PreCommit[Pre-commit Hooks]
    end

    subgraph "GitHub Actions"
        PR[Pull Request]
        CIWorkflow[CI Workflow]

        subgraph "CI Jobs"
            Lint[Lint Job<br/>ESLint]
            TypeCheck[Type Check<br/>TypeScript]
            Test[Test Job<br/>300 tests]
            Build[Build Job<br/>tsc]
        end

        Coverage[Coverage Upload<br/>Codecov]
        Security[Security Scan<br/>npm audit + CodeQL]
    end

    subgraph "Release Pipeline"
        MergeMain[Merge to Main]
        SemanticRelease[semantic-release]

        subgraph "Release Steps"
            AnalyzeCommits[Analyze Commits]
            BumpVersion[Bump Version]
            GenerateChangelog[Generate CHANGELOG]
            NPMPublish[Publish to npm]
            GitHubRelease[Create GitHub Release]
        end
    end

    subgraph "Dependency Management"
        Dependabot[Dependabot<br/>Weekly Scans]
        DependencyPRs[Automated PRs]
    end

    Commit --> PreCommit
    PreCommit -->|lint-staged<br/>commitlint| PR

    PR --> CIWorkflow
    CIWorkflow --> Lint
    CIWorkflow --> TypeCheck
    CIWorkflow --> Test
    CIWorkflow --> Build

    Test --> Coverage
    CIWorkflow --> Security

    PR -->|Approved| MergeMain
    MergeMain --> SemanticRelease

    SemanticRelease --> AnalyzeCommits
    AnalyzeCommits --> BumpVersion
    BumpVersion --> GenerateChangelog
    GenerateChangelog --> NPMPublish
    NPMPublish --> GitHubRelease

    Dependabot --> DependencyPRs
    DependencyPRs --> CIWorkflow

    style PreCommit fill:#fff9c4
    style Lint fill:#e1f5ff
    style Test fill:#e1f5ff
    style Security fill:#ffcdd2
    style SemanticRelease fill:#c8e6c9
```

---

## Cost Tracking Flow

```mermaid
flowchart TD
    ReviewStart([Review Execution]) --> CollectMetrics[Collect Token Metrics<br/>input, output, cache]

    CollectMetrics --> CalculateCost[Calculate Costs<br/>per-persona]

    CalculateCost --> SelectSink{Cost Sink<br/>Type?}

    SelectSink -->|GCP| GCPSink[GCP Cloud Monitoring<br/>cost-sinks/gcp-sink.ts]
    SelectSink -->|File| FileSink[File Sink<br/>JSONL format]
    SelectSink -->|Stdout| StdoutSink[Stdout Sink<br/>Console output]

    GCPSink --> WriteMetrics[Write Cost Metrics]
    FileSink --> WriteMetrics
    StdoutSink --> WriteMetrics

    WriteMetrics --> EnrichMetadata[Enrich with Metadata<br/>git branch, commit, author]

    EnrichMetadata --> CacheAnalysis[Analyze Cache Performance<br/>hit rate, savings]

    CacheAnalysis --> DisplaySummary[Display Cost Summary<br/>in formatter]

    DisplaySummary --> End([Complete])

    style CalculateCost fill:#fff9c4
    style GCPSink fill:#e8f5e9
    style FileSink fill:#e1f5ff
    style StdoutSink fill:#f3e5f5
    style CacheAnalysis fill:#ffe0b2
```

---

## Persona Loading Sequence

```mermaid
sequenceDiagram
    participant CLI
    participant PersonaLoader
    participant FileSystem
    participant Parser
    participant Validator

    CLI->>PersonaLoader: loadPersonas(searchPaths)

    loop For each search path
        PersonaLoader->>FileSystem: Read directory
        FileSystem-->>PersonaLoader: File list

        loop For each persona file
            PersonaLoader->>FileSystem: Read file (.ai.md or .yaml)
            FileSystem-->>PersonaLoader: File content

            alt .ai.md format
                PersonaLoader->>Parser: Parse frontmatter + markdown
                Parser-->>PersonaLoader: Persona object
            else .yaml format
                PersonaLoader->>Parser: Parse YAML
                Parser-->>PersonaLoader: Persona object
            end

            PersonaLoader->>Validator: Validate persona

            alt Validation passes
                Validator-->>PersonaLoader: Valid persona
                PersonaLoader->>PersonaLoader: Check cache eligibility<br/>(≥1,024 tokens)
            else Validation fails
                Validator-->>PersonaLoader: Error
                PersonaLoader->>PersonaLoader: Skip and log warning
            end
        end
    end

    PersonaLoader->>PersonaLoader: Deduplicate by name<br/>(later paths override)

    PersonaLoader-->>CLI: Map<name, Persona>
```

---

## Component Interaction Matrix

| Component | Depends On | Used By | Purpose |
|-----------|------------|---------|---------|
| **CLI** | Config, Persona, ReviewEngine | User | Entry point for command-line usage |
| **API** | Config, Persona, ReviewEngine | Applications | Entry point for programmatic usage |
| **Config Loader** | FileSystem, YAML parser | CLI, API | Load and validate configuration |
| **Persona Loader** | FileSystem, YAML parser | CLI, API | Load and validate personas |
| **File Scanner** | Git, FileSystem | ReviewEngine | Scan files for review |
| **Review Engine** | Persona, FileScanner, LLM Client | CLI, API | Orchestrate review execution |
| **Vote Aggregator** | ReviewEngine | ReviewEngine | Agency-Agents vote aggregation |
| **LLM Clients** | Anthropic/Vertex AI APIs | ReviewEngine | Execute AI reviews |
| **Deduplicator** | ReviewEngine | ReviewEngine | Merge similar findings |
| **Cost Tracker** | ReviewEngine | Formatters | Track and report costs |
| **Formatters** | ReviewEngine | CLI, API | Format output for display |

---

## Technology Stack

```mermaid
mindmap
  root((Multi-Persona<br/>Review))
    Runtime
      Node.js 18+
      TypeScript 5.0
      ES Modules
    AI Providers
      Anthropic Claude
        claude-sonnet-4.5
        claude-haiku-4.5
        claude-opus-4.6
      Vertex AI
        Gemini 2.5
        Claude via Vertex
    Testing
      Vitest
        300 tests
        Coverage 80%+
      Test Types
        Unit
        Integration
        E2E
    CI/CD
      GitHub Actions
        CI workflow
        Release workflow
        Security workflow
        Coverage workflow
      Automation
        semantic-release
        Dependabot
        Husky hooks
    Quality
      ESLint
      Prettier
      TypeScript strict
      Conventional commits
```

---

## Deployment Architecture

```mermaid
graph TB
    subgraph "Development"
        Dev[Developer Machine]
        LocalGit[Local Git Repo]
    end

    subgraph "CI/CD (GitHub)"
        GitHubRepo[GitHub Repository]
        Actions[GitHub Actions]
        Codecov[Codecov]
        Secrets[GitHub Secrets<br/>API Keys]
    end

    subgraph "Package Registries"
        NPM[npm Registry]
        GitHubPackages[GitHub Packages]
    end

    subgraph "Production Usage"
        UserCLI[User CLI]
        UserApp[User Application]
        CI[CI/CD Pipeline]
    end

    subgraph "AI Providers"
        Anthropic[Anthropic API]
        VertexAI[Google Vertex AI]
    end

    subgraph "Monitoring"
        GCPMonitoring[GCP Cloud Monitoring]
        FileSink[File-based Logs]
    end

    Dev --> LocalGit
    LocalGit --> GitHubRepo
    GitHubRepo --> Actions

    Actions --> Codecov
    Actions --> NPM
    Actions --> GitHubPackages

    Secrets --> Actions

    NPM --> UserCLI
    NPM --> UserApp
    NPM --> CI

    UserCLI --> Anthropic
    UserCLI --> VertexAI
    UserApp --> Anthropic
    UserApp --> VertexAI
    CI --> Anthropic
    CI --> VertexAI

    UserCLI --> GCPMonitoring
    UserApp --> GCPMonitoring
    UserCLI --> FileSink

    style Dev fill:#e1f5ff
    style Actions fill:#fff9c4
    style NPM fill:#c8e6c9
    style Anthropic fill:#f3e5f5
    style VertexAI fill:#f3e5f5
```

---

## Performance Optimization Points

```mermaid
flowchart LR
    Input[Code Files] --> Scan[File Scanning]

    Scan --> Optimize1{Optimization Point 1:<br/>Parallel Execution}

    Optimize1 -->|3x faster| ParallelPersonas[Execute Personas<br/>in Parallel]

    ParallelPersonas --> Optimize2{Optimization Point 2:<br/>Prompt Caching}

    Optimize2 -->|86% cost savings| CachedReview[Use Cached<br/>Persona Prompts]

    CachedReview --> Optimize3{Optimization Point 3:<br/>Deduplication}

    Optimize3 -->|50% noise reduction| DedupFindings[Deduplicate<br/>Similar Findings]

    DedupFindings --> Optimize4{Optimization Point 4:<br/>Confidence Filtering}

    Optimize4 -->|High-quality only| FilteredFindings[Filter Low-Confidence<br/>Findings]

    FilteredFindings --> Output[Final Results]

    style Optimize1 fill:#fff9c4
    style Optimize2 fill:#c8e6c9
    style Optimize3 fill:#e1bee7
    style Optimize4 fill:#ffcdd2
```

---

## See Also

- [ARCHITECTURE.md](../ARCHITECTURE.md) - Detailed architecture documentation
- [SPEC.md](../SPEC.md) - Functional specification
- [README.md](../README.md) - Overview and quick start
