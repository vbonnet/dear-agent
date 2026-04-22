package harnesseffort

// ResolveAliases substitutes model alias references in all tier provider configs.
// For each provider, if provider.Model is a key in cfg.ModelAliases, it is
// replaced with the alias value. Unknown alias keys pass through unchanged.
// Returns a new config; does not mutate the input.
func ResolveAliases(cfg HarnessEffortConfig) HarnessEffortConfig {
	result := copyConfig(cfg)

	for tierName, tier := range result.Tiers {
		for provName, prov := range tier.Providers {
			if resolved, ok := result.ModelAliases[prov.Model]; ok {
				prov.Model = resolved
				tier.Providers[provName] = prov
			}
		}
		result.Tiers[tierName] = tier
	}

	return result
}
