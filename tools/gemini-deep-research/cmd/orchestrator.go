package cmd

import (
	"fmt"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// LoadAndResolvePrompts loads prompts from config files and CLI flags,
// then resolves @file syntax. Does NOT perform variable substitution.
// Returns resolved prompts ready for variable substitution.
func LoadAndResolvePrompts(flags *types.Flags) (config.ResolvedConfig, error) {
	// Step 1: ConfigParser - Load and merge configs with precedence
	cliPrompts := config.CLIPrompts{
		ExtractPrompt:  flags.ExtractPrompt,
		AnalyzePrompt:  flags.AnalyzePrompt,
		ResearchPrompt: flags.ResearchPrompt,
	}

	// Handle legacy --input and --input-file flags (map to analyze prompt)
	if flags.Input != "" {
		// Legacy --input maps to --analyze-prompt
		cliPrompts.AnalyzePrompt = flags.Input
	} else if flags.InputFile != "" {
		// Legacy --input-file maps to --analyze-prompt @file
		cliPrompts.AnalyzePrompt = "@" + flags.InputFile
	}

	mergedPrompts, err := config.LoadPrompts(cliPrompts)
	if err != nil {
		return config.ResolvedConfig{}, fmt.Errorf("config parser: %w", err)
	}

	// Step 2: FileResolver - Resolve @file syntax
	resolvedPrompts := map[string]string{
		"extract":  mergedPrompts.ExtractPrompt,
		"analyze":  mergedPrompts.AnalyzePrompt,
		"research": mergedPrompts.ResearchPrompt,
	}

	resolvedPrompts, err = config.ResolveFiles(resolvedPrompts)
	if err != nil {
		return config.ResolvedConfig{}, fmt.Errorf("file resolver: %w", err)
	}

	// Return resolved config (ready for variable substitution)
	return config.ResolvedConfig{
		ExtractPrompt:  resolvedPrompts["extract"],
		AnalyzePrompt:  resolvedPrompts["analyze"],
		ResearchPrompt: resolvedPrompts["research"],
	}, nil
}

// SubstituteVariables performs variable substitution on resolved prompts
func SubstituteVariables(resolvedConfig config.ResolvedConfig, variables config.Variables) (config.FinalPrompts, error) {
	substitutor := config.NewVariableSubstitutor()
	finalPrompts, err := substitutor.Substitute(resolvedConfig, variables)
	if err != nil {
		return config.FinalPrompts{}, fmt.Errorf("variable substitutor: %w", err)
	}
	return finalPrompts, nil
}
