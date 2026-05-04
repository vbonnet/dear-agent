package harnesseffort

// MergeConfigs deep-merges src into dst and returns the result.
// src wins on conflict. Unknown provider keys in src are preserved.
// Neither dst nor src are mutated.
//nolint:gocyclo // reason: linear field-by-field merger
func MergeConfigs(dst, src HarnessEffortConfig) HarnessEffortConfig {
	result := copyConfig(dst)

	// Merge model_aliases: src keys win
	if src.ModelAliases != nil {
		if result.ModelAliases == nil {
			result.ModelAliases = make(map[string]string)
		}
		for k, v := range src.ModelAliases {
			result.ModelAliases[k] = v
		}
	}

	// Merge subagent_preference: src wins if non-empty
	if src.SubagentPreference != "" {
		result.SubagentPreference = src.SubagentPreference
	}

	// Merge task_types: per task type name
	if src.TaskTypes != nil {
		if result.TaskTypes == nil {
			result.TaskTypes = make(map[string]TaskTypeConfig)
		}
		for name, srcType := range src.TaskTypes {
			dstType := result.TaskTypes[name]
			if len(srcType.HarnessOrder) > 0 {
				// Full list replacement (not append)
				order := make([]string, len(srcType.HarnessOrder))
				copy(order, srcType.HarnessOrder)
				dstType.HarnessOrder = order
			}
			result.TaskTypes[name] = dstType
		}
	}

	// Merge tiers: per tier name, then per provider name
	if src.Tiers != nil {
		if result.Tiers == nil {
			result.Tiers = make(map[string]TierConfig)
		}
		for tierName, srcTier := range src.Tiers {
			dstTier := result.Tiers[tierName]
			if srcTier.Description != "" {
				dstTier.Description = srcTier.Description
			}
			if srcTier.Providers != nil {
				if dstTier.Providers == nil {
					dstTier.Providers = make(map[string]ProviderConfig)
				}
				for provName, srcProv := range srcTier.Providers {
					dstProv := dstTier.Providers[provName]
					if srcProv.Model != "" {
						dstProv.Model = srcProv.Model
					}
					if srcProv.Effort != "" {
						dstProv.Effort = srcProv.Effort
					}
					if srcProv.ReasoningEffort != "" {
						dstProv.ReasoningEffort = srcProv.ReasoningEffort
					}
					dstTier.Providers[provName] = dstProv
				}
			}
			result.Tiers[tierName] = dstTier
		}
	}

	return result
}

// copyConfig returns a deep copy of cfg.
func copyConfig(cfg HarnessEffortConfig) HarnessEffortConfig {
	result := HarnessEffortConfig{
		SubagentPreference: cfg.SubagentPreference,
	}

	if cfg.ModelAliases != nil {
		result.ModelAliases = make(map[string]string, len(cfg.ModelAliases))
		for k, v := range cfg.ModelAliases {
			result.ModelAliases[k] = v
		}
	}

	if cfg.TaskTypes != nil {
		result.TaskTypes = make(map[string]TaskTypeConfig, len(cfg.TaskTypes))
		for name, tt := range cfg.TaskTypes {
			order := make([]string, len(tt.HarnessOrder))
			copy(order, tt.HarnessOrder)
			result.TaskTypes[name] = TaskTypeConfig{HarnessOrder: order}
		}
	}

	if cfg.Tiers != nil {
		result.Tiers = make(map[string]TierConfig, len(cfg.Tiers))
		for tierName, tier := range cfg.Tiers {
			newTier := TierConfig{Description: tier.Description}
			if tier.Providers != nil {
				newTier.Providers = make(map[string]ProviderConfig, len(tier.Providers))
				for provName, prov := range tier.Providers {
					newTier.Providers[provName] = prov
				}
			}
			result.Tiers[tierName] = newTier
		}
	}

	return result
}
