package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/cmd"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

const version = "2.0.0"

var (
	// Custom usage function for better help text
	usageFunc = func() {
		fmt.Fprintf(os.Stderr, `gemini-deep-research - Research tool using Gemini Deep Research API

Usage:
  gemini-deep-research [FLAGS] <URL>

Flags:
  Per-Stage Prompts:
  --extract-prompt <text|@file>   Custom prompt for extraction stage
  --analyze-prompt <text|@file>   Custom prompt for topic analysis stage
  --research-prompt <text|@file>  Custom prompt for deep research stage

  Legacy (Deprecated):
  --input <text>          DEPRECATED: Use --analyze-prompt instead
  --input-file <path>     DEPRECATED: Use --analyze-prompt @file.txt instead

  Other Options:
  --type <content-type>   Override content type auto-detection (video|article|arxiv|huggingface)
  --mode <general|competitive>  Analysis mode (auto-detected from query if not specified)
  --output-dir <path>     Output directory for results (default: ./output)
  --timeout <minutes>     Deep Research timeout in minutes (default: 60)
  --project <id>          GCP project ID (fallback: GOOGLE_CLOUD_PROJECT env var)
  --force                 Force refresh of existing research (ignore cache)
  --help, -h              Show this help message
  --version, -v           Show version information

  Competitive Mode Options:
  --no-discovery          Skip URL discovery in competitive mode (use provided URL directly)
  --discovery-limit <n>   Maximum URLs to discover (default: 5, max: 20)

Positional Arguments:
  <URL>                   Content URL to analyze (required)

Examples:
  # Basic usage (YouTube video)
  gemini-deep-research https://www.youtube.com/watch?v=VIDEO_ID

  # Custom prompt for analysis stage
  gemini-deep-research --analyze-prompt "Focus on security topics" https://example.com

  # Custom prompt from file (using @file syntax)
  gemini-deep-research --analyze-prompt @prompts/security.txt https://example.com

  # Multi-stage prompts
  gemini-deep-research \
    --extract-prompt "Extract security vulnerabilities" \
    --analyze-prompt "Analyze threat patterns" \
    --research-prompt "Research mitigation strategies" \
    https://example.com

  # Override content type detection
  gemini-deep-research --type article https://youtu.be/VIDEO_ID

  # Force competitive analysis mode
  gemini-deep-research --mode competitive https://github.com/features/copilot

  # Skip URL discovery in competitive mode (use provided URL directly)
  gemini-deep-research --mode competitive --no-discovery https://github.com/features/copilot

  # Limit discovery to 3 URLs
  gemini-deep-research --mode competitive --discovery-limit 3 https://example.com

  # Custom output directory
  gemini-deep-research --output-dir /tmp/research https://arxiv.org/abs/2601.20802

  # Specify GCP project
  gemini-deep-research --project my-project-id https://example.com

Environment Variables:
  GEMINI_API_KEY          API key for Deep Research (required)
  GOOGLE_CLOUD_PROJECT    GCP project ID (fallback if --project not specified)
  GEMINI_DR_OUTPUT_DIR    Default output directory (fallback if --output-dir not specified)
  GEMINI_DR_TIMEOUT       Research timeout in minutes (default: 60)

Exit Codes:
  0  Success
  1  Invalid arguments or configuration
  2  Content extraction failed
  3  Topic analysis failed
  4  Deep Research failed
  5  Output writing failed
  6  Unexpected error

For more information, visit: https://github.com/vbonnet/gemini-deep-research
`)
	}
)

func main() {
	// Create flags
	var flags types.Flags

	// New per-stage prompt flags
	flag.StringVar(&flags.ExtractPrompt, "extract-prompt", "", "Custom prompt for extraction stage (supports @file syntax)")
	flag.StringVar(&flags.AnalyzePrompt, "analyze-prompt", "", "Custom prompt for topic analysis stage (supports @file syntax)")
	flag.StringVar(&flags.ResearchPrompt, "research-prompt", "", "Custom prompt for deep research stage (supports @file syntax)")

	// Legacy flags (deprecated)
	flag.StringVar(&flags.Input, "input", "", "DEPRECATED: Use --analyze-prompt instead")
	flag.StringVar(&flags.InputFile, "input-file", "", "DEPRECATED: Use --analyze-prompt @file.txt instead")

	// Other flags
	flag.StringVar(&flags.Type, "type", "", "Override content type auto-detection (video|article|arxiv|huggingface)")
	flag.StringVar(&flags.Mode, "mode", "", "Analysis mode: general or competitive (auto-detected from query by default)")
	flag.StringVar(&flags.OutputDir, "output-dir", "", "Output directory for results")
	flag.IntVar(&flags.Timeout, "timeout", 0, "Deep Research timeout in minutes")
	flag.StringVar(&flags.Project, "project", "", "GCP project ID")
	flag.BoolVar(&flags.Force, "force", false, "Force refresh of existing research")

	// Competitive mode discovery flags
	flag.BoolVar(&flags.NoDiscovery, "no-discovery", false, "Skip URL discovery in competitive mode (use provided URL directly)")
	flag.IntVar(&flags.DiscoveryLimit, "discovery-limit", 5, "Maximum URLs to discover in competitive mode (default: 5, max: 20)")

	var showHelp bool
	var showVersion bool
	flag.BoolVar(&showHelp, "help", false, "Show help message")
	flag.BoolVar(&showHelp, "h", false, "Show help message (shorthand)")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showVersion, "v", false, "Show version information (shorthand)")

	// Set custom usage
	flag.Usage = usageFunc

	// Parse flags
	flag.Parse()

	// Handle version flag
	if showVersion {
		fmt.Printf("gemini-deep-research version %s\n", version)
		os.Exit(0)
	}

	// Handle help flag
	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// Validate positional arguments
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Error: URL argument required")
		fmt.Fprintln(os.Stderr, "")
		flag.Usage()
		os.Exit(1)
	}

	// Get URL from positional argument
	url := flag.Arg(0)

	// Validate flags
	if err := cmd.Validate(&flags); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "")
		flag.Usage()
		os.Exit(1)
	}

	// Show deprecation warning for --input or --input-file
	if flags.Input != "" || flags.InputFile != "" {
		fmt.Fprintln(os.Stderr, "WARNING: --input and --input-file are deprecated.")
		fmt.Fprintln(os.Stderr, "         Use --analyze-prompt instead. These flags will be removed in v3.0.0.")
		fmt.Fprintln(os.Stderr, "         Migration: --input 'text' → --analyze-prompt 'text'")
		fmt.Fprintln(os.Stderr, "                   --input-file file.txt → --analyze-prompt @file.txt")
		fmt.Fprintln(os.Stderr, "")
	}

	// Load configuration
	cfg, err := config.Load(&flags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Run the command
	exitCode := cmd.Run(url, &flags, cfg)
	os.Exit(exitCode)
}
