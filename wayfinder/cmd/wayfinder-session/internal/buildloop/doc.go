// Package buildloop implements the BUILD Loop State Machine for Wayfinder v2 S8 phase.
//
// # Overview
//
// The BUILD loop replaces linear S8→S9→S10 execution with tight TDD feedback cycles.
// Each task follows test-first development with continuous validation and risk-adaptive reviews.
//
// # State Machine
//
// The BUILD loop implements 8 primary states with 3 error/recovery states:
//
//	TEST_FIRST → CODING → GREEN → REFACTOR → VALIDATION → DEPLOY → MONITORING → COMPLETE
//	     ↓         ↓              ↓           ↓
//	  TIMEOUT  TIMEOUT    REVIEW_FAILED  INTEGRATE_FAIL
//
// # States
//
//   - TEST_FIRST: Red phase - tests must fail as expected (TDD enforcement)
//   - CODING: Write minimal code to make failing tests pass
//   - GREEN: Tests pass, run quality gates (assertion density, coverage)
//   - REFACTOR: Improve code quality without breaking tests
//   - VALIDATION: Multi-persona review (L/XL risk tasks only)
//   - DEPLOY: Integration testing
//   - MONITORING: Observe in production/staging
//   - COMPLETE: Task successfully completed
//
// Error/recovery states:
//
//   - TIMEOUT: Test execution timeout
//   - REVIEW_FAILED: P0/P1 blocking issues found
//   - INTEGRATE_FAIL: Integration tests failed
//
// # Risk-Adaptive Review
//
// Tasks are routed based on risk level:
//
//   - XS/S/M (< 500 LOC): No per-task review, defer to batch review
//   - L/XL (≥ 500 LOC): Immediate per-task review required
//
// # Quality Gates
//
//   - Assertion Density: Minimum 0.5 assertions per test
//   - Coverage: Minimum 80% for changed files
//   - Complexity: Maximum cyclomatic complexity 10
//   - Test Quality: No empty tests or commented assertions
//
// # Usage
//
//	task := &buildloop.Task{
//	    ID:          "T1",
//	    Description: "Add authentication",
//	    RiskLevel:   buildloop.RiskM,
//	}
//
//	bl := buildloop.NewBuildLoop(task, nil) // Use default config
//	result, err := bl.Execute()
//
//	if err != nil {
//	    log.Fatalf("BUILD loop failed: %v", err)
//	}
//
//	if result.Success {
//	    fmt.Printf("Task completed in %v\n", result.Metrics.Duration)
//	}
//
// # Integration with Wayfinder
//
// The BUILD loop integrates at the S8 (build.implement) phase:
//
//	discovery.problem → ... → build.implement (S8) → ... → deploy.release
//	                              ↓
//	                         BUILD Loop
//	                              ↓
//	                    Task → TEST_FIRST → ... → COMPLETE
//
// # Configuration
//
// Default configuration values:
//
//   - MaxRetries: 3
//   - MinAssertionDensity: 0.5
//   - MinCoveragePercent: 80.0
//   - TestTimeoutSeconds: 300 (5 minutes)
//   - ReviewTimeoutSeconds: 600 (10 minutes)
//   - EnableTDDEnforcement: true
//
// # Iteration Tracking
//
// The IterationTracker records metrics per task:
//
//   - Iteration count and success rate
//   - State visits per iteration
//   - Test run count
//   - Duration metrics
//
// # References
//
// Design documentation:
//   - ~/src/ws/oss/repos/engram-research/swarm/projects/wayfinder-v2-consolidation/build-loop-state-machine.md
//   - ~/src/ws/oss/repos/engram-research/swarm/projects/wayfinder-v2-consolidation/task-iteration-algorithm.md
//
// Implementation documentation:
//   - buildloop-implementation.md in this package
package buildloop
