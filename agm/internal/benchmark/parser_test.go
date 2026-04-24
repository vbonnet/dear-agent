package benchmark

import (
	"testing"
	"time"
)

func TestParseBenchmarkOutput(t *testing.T) {
	output := `goos: linux
goarch: amd64
pkg: github.com/vbonnet/dear-agent/agm/test
BenchmarkLockAcquireRelease-8       	  300000	      4200 ns/op	     128 B/op	       3 allocs/op
BenchmarkHealthCheckCached-8        	50000000	        15 ns/op	       0 B/op	       0 allocs/op
BenchmarkSearchCached-8             	 5000000	       312 ns/op	      64 B/op	       1 allocs/op
PASS
ok  	github.com/vbonnet/dear-agent/agm/test	4.567s
`

	results, err := ParseBenchmarkOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify first result
	r := results[0]
	if r.Name != "BenchmarkLockAcquireRelease-8" {
		t.Errorf("expected name BenchmarkLockAcquireRelease-8, got %s", r.Name)
	}
	if r.Iterations != 300000 {
		t.Errorf("expected 300000 iterations, got %d", r.Iterations)
	}
	if r.NsPerOp != 4200 {
		t.Errorf("expected 4200 ns/op, got %f", r.NsPerOp)
	}
	if r.BytesPerOp != 128 {
		t.Errorf("expected 128 B/op, got %d", r.BytesPerOp)
	}
	if r.AllocsPerOp != 3 {
		t.Errorf("expected 3 allocs/op, got %d", r.AllocsPerOp)
	}

	// Verify cached result with zero allocations
	r2 := results[1]
	if r2.BytesPerOp != 0 {
		t.Errorf("expected 0 B/op, got %d", r2.BytesPerOp)
	}
}

func TestParseBenchmarkOutput_NoBenchmem(t *testing.T) {
	output := `goos: linux
goarch: amd64
BenchmarkLockAcquireRelease-8       	  300000	      4200 ns/op
BenchmarkHealthCheckCached-8        	50000000	        15 ns/op
PASS
ok  	github.com/vbonnet/dear-agent/agm/test	2.345s
`

	results, err := ParseBenchmarkOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	r := results[0]
	if r.NsPerOp != 4200 {
		t.Errorf("expected 4200 ns/op, got %f", r.NsPerOp)
	}
	if r.BytesPerOp != 0 {
		t.Errorf("expected 0 B/op without benchmem, got %d", r.BytesPerOp)
	}
	if r.AllocsPerOp != 0 {
		t.Errorf("expected 0 allocs/op without benchmem, got %d", r.AllocsPerOp)
	}
}

func TestParseBenchmarkOutput_FailedRun(t *testing.T) {
	output := `--- FAIL: TestSomething (0.00s)
FAIL
FAIL	github.com/vbonnet/dear-agent/agm/test	0.123s
`

	results, err := ParseBenchmarkOutput(output)
	if err == nil {
		t.Fatal("expected error for failed run")
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestEvaluate_PassAndFail(t *testing.T) {
	results := []BenchmarkResult{
		{
			Name:     "BenchmarkLockAcquireRelease-8",
			NsPerOp:  4200,
			Duration: 4200 * time.Nanosecond,
		},
		{
			Name:     "BenchmarkSearchCached-8",
			NsPerOp:  2000000, // 2ms — exceeds 1ms target
			Duration: 2000000 * time.Nanosecond,
		},
		{
			Name:     "BenchmarkHealthCheckCached-8",
			NsPerOp:  15,
			Duration: 15 * time.Nanosecond,
		},
	}

	targets := DefaultTargets()
	report := Evaluate(results, targets)

	if report.Summary.Total != 3 {
		t.Errorf("expected total 3, got %d", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", report.Summary.Passed)
	}
	if report.Summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", report.Summary.Failed)
	}
	if report.Summary.NoTarget != 1 {
		t.Errorf("expected 1 no_target, got %d", report.Summary.NoTarget)
	}
	if report.Summary.AllPassed {
		t.Error("expected AllPassed to be false")
	}

	// Lock should pass (4.2us < 10us target)
	if !report.Evaluations[0].Pass {
		t.Error("expected lock benchmark to pass")
	}
	if report.Evaluations[0].Target == nil {
		t.Error("expected lock benchmark to have a target")
	}

	// Search should fail (2ms > 1ms target)
	if report.Evaluations[1].Pass {
		t.Error("expected search benchmark to fail")
	}

	// HealthCheck has no target
	if report.Evaluations[2].Target != nil {
		t.Error("expected health check benchmark to have no target")
	}
}
