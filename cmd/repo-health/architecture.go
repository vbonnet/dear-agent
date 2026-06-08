package main

import (
	"encoding/json"
	"go/ast"
	"sort"
	"strings"
)

// collectArchitecture fills the architecture section: dependency-graph
// depth, circular dependencies, oversized files and overlong functions.
func collectArchitecture(sc *scanCtx) Architecture {
	a := Architecture{}

	a.DepGraph, a.MaxDepDepth, a.CircularDeps = depGraph(sc)

	// Large files / long functions are pure AST work over production
	// (non-test) sources.
	for _, s := range sc.sources {
		if s.isTest {
			continue
		}
		if s.lines > sc.opts.maxFileLines {
			a.LargeFiles = append(a.LargeFiles, SizedItem{Name: relTo(sc.root, s.path), Lines: s.lines})
		}
		for _, decl := range s.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if span := funcLineSpan(s.fset, fn); span > sc.opts.maxFuncLines {
				a.LongFunctions = append(a.LongFunctions, SizedItem{
					Name:  relTo(sc.root, s.path) + ":" + fn.Name.Name,
					Lines: span,
				})
			}
		}
	}
	// Largest offenders first so the markdown summary leads with the worst.
	sort.Slice(a.LargeFiles, func(i, j int) bool { return a.LargeFiles[i].Lines > a.LargeFiles[j].Lines })
	sort.Slice(a.LongFunctions, func(i, j int) bool { return a.LongFunctions[i].Lines > a.LongFunctions[j].Lines })
	return a
}

// goListPkg is the subset of `go list -json` we consume.
type goListPkg struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
}

// depGraph builds the intra-module import graph and returns its longest
// dependency chain plus any import cycles. Imports outside this module
// (stdlib, third-party) are ignored — we measure *our* layering, not the
// universe. Go itself forbids import cycles, so a non-empty cycle list is a
// real red flag worth surfacing loudly.
func depGraph(sc *scanCtx) (Metric, int, [][]string) {
	if !haveBinary("go") {
		return Metric{Available: false, Note: "go not on PATH"}, 0, nil
	}
	res := run(sc.root, sc.opts.goListTimeout, "go", "list", "-json", "./...")
	if !res.ok() && res.stdout == "" {
		return Metric{Available: false, Note: "go list failed: " + firstLine(res.stderr)}, 0, nil
	}

	graph := map[string][]string{}
	dec := json.NewDecoder(strings.NewReader(res.stdout))
	for dec.More() {
		var p goListPkg
		if err := dec.Decode(&p); err != nil {
			break
		}
		var internal []string
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, sc.module) {
				internal = append(internal, imp)
			}
		}
		graph[p.ImportPath] = internal
	}
	if len(graph) == 0 {
		return Metric{Available: false, Note: "go list returned no packages"}, 0, nil
	}

	depth, cycles := longestChainAndCycles(graph)
	return Metric{Available: true}, depth, cycles
}

// longestChainAndCycles returns the longest dependency chain length (number
// of edges) and every cycle found. It walks each node with a colour-marked
// DFS: grey nodes on the current stack reveal back-edges (cycles); the
// longest acyclic chain is memoised in depthCache.
func longestChainAndCycles(graph map[string][]string) (int, [][]string) {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	colour := map[string]int{}
	depthCache := map[string]int{}
	var stack []string
	var cycles [][]string
	seenCycle := map[string]bool{}

	var visit func(n string) int
	visit = func(n string) int {
		colour[n] = grey
		stack = append(stack, n)
		best := 0
		for _, d := range graph[n] {
			switch colour[d] {
			case grey:
				// Back-edge: extract the cycle from the current stack.
				if cyc := extractCycle(stack, d); cyc != nil {
					key := cycleKey(cyc)
					if !seenCycle[key] {
						seenCycle[key] = true
						cycles = append(cycles, cyc)
					}
				}
			case white:
				if c := visit(d) + 1; c > best {
					best = c
				}
			case black:
				if c := depthCache[d] + 1; c > best {
					best = c
				}
			}
		}
		stack = stack[:len(stack)-1]
		colour[n] = black
		depthCache[n] = best
		return best
	}

	maxDepth := 0
	// Deterministic iteration order so output is stable run-to-run.
	nodes := make([]string, 0, len(graph))
	for n := range graph {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for _, n := range nodes {
		if colour[n] == white {
			if d := visit(n); d > maxDepth {
				maxDepth = d
			}
		}
	}
	return maxDepth, cycles
}

// extractCycle returns the slice of the stack from the first occurrence of
// target to the top, representing the cycle closed by a back-edge to target.
func extractCycle(stack []string, target string) []string {
	for i, n := range stack {
		if n == target {
			cyc := make([]string, len(stack)-i)
			copy(cyc, stack[i:])
			return cyc
		}
	}
	return nil
}

// cycleKey produces a rotation-stable key so the same cycle discovered from
// different entry points is only reported once.
func cycleKey(cyc []string) string {
	lo := 0
	for i := range cyc {
		if cyc[i] < cyc[lo] {
			lo = i
		}
	}
	rotated := append(append([]string{}, cyc[lo:]...), cyc[:lo]...)
	return strings.Join(rotated, "->")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
