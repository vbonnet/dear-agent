package benchmarks

// init populates DefaultRegistry with the built-in suite scaffolds. Real
// callers replace the loaders/executors via the constructor, but the
// registered suites give CLI commands something to discover out of the box.
func init() {
	mustRegister(DefaultRegistry, NewSWEBenchLite(nil, nil))
	mustRegister(DefaultRegistry, NewSWEBenchVerified(nil, nil))
	mustRegister(DefaultRegistry, NewSWEAtlas(nil, nil))
	mustRegister(DefaultRegistry, NewVibeBench(nil, nil))
}

func mustRegister(r *Registry, b Benchmark) {
	if err := r.Register(b); err != nil {
		// init-time programming error; panic is appropriate.
		panic(err)
	}
}
