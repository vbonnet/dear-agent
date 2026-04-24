package ecphory

// applyTriggerBoosting boosts relevance for engrams with active triggers.
// Called from the Query pipeline after failure boosting, when triggerPaths
// is configured via WithTriggerPaths.
func applyTriggerBoosting(ranked []RankingResult, triggerPaths map[string]bool) {
	for i := range ranked {
		if triggerPaths[ranked[i].Path] {
			ranked[i].Relevance += 20.0
			if ranked[i].Relevance > 100.0 {
				ranked[i].Relevance = 100.0
			}
		}
	}
}
