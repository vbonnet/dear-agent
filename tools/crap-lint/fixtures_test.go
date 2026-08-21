package main

import "github.com/vbonnet/dear-agent/internal/craplens"

// cleanReport is a diff with nothing worth saying.
func cleanReport() craplens.Report {
	return craplens.Report{Threshold: craplens.DefaultThreshold, Scored: 3, WithinAgentTarget: 3}
}

// flaggedReport is a diff with a high-scoring changed function.
func flaggedReport() craplens.Report {
	return craplens.Report{
		Threshold: craplens.DefaultThreshold,
		Scored:    2,
		Over: []craplens.Function{
			{File: "pkg/x/y.go", Line: 12, Name: "Classify", Complexity: 20, Coverage: 0},
		},
	}
}
