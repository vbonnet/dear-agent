package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// collectDrift fills the drift section: chezmoi source-vs-deployed drift,
// git-hook source-vs-deployed drift, and .ai.md/.why.md pairing drift.
func collectDrift(sc *scanCtx) Drift {
	d := Drift{}
	d.Chezmoi, d.ChezmoiDrifted = chezmoiDrift(sc)
	d.Hooks, d.HookDrifted = hookDrift(sc)
	d.DocPairing, d.UnpairedDocs = docPairingDrift(sc)
	return d
}

// chezmoiDrift reports dotfiles whose deployed copy differs from the chezmoi
// source. Each `chezmoi status` line is one drifted entry. Absence of
// chezmoi (CI, fresh host) degrades to unavailable, never a failure.
func chezmoiDrift(sc *scanCtx) (Metric, []string) {
	if !haveBinary("chezmoi") {
		return Metric{Available: false, Note: "chezmoi not on PATH (expected on dev hosts, not CI)"}, nil
	}
	res := run(sc.root, sc.opts.gitTimeout, "chezmoi", "status")
	if !res.ok() && res.stdout == "" {
		return Metric{Available: false, Note: "chezmoi status failed: " + firstLine(res.stderr)}, nil
	}
	var drifted []string
	for line := range strings.SplitSeq(res.stdout, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Format: "<status-code> <path>"; keep the path for the report.
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			drifted = append(drifted, fields[len(fields)-1])
		} else {
			drifted = append(drifted, line)
		}
	}
	return Metric{Available: true}, drifted
}

// gitHookNames are the standard git hook basenames we treat as deployable.
var gitHookNames = map[string]bool{
	"pre-commit": true, "pre-push": true, "post-commit": true,
	"pre-merge-commit": true, "commit-msg": true, "post-merge": true,
	"prepare-commit-msg": true, "post-checkout": true, "pre-rebase": true,
}

// hookDrift compares git-hook source files shipped in the repo (any file
// directly inside a `.githooks/` directory whose name is a standard hook)
// against the deployed hook of the same name in the active hooks directory
// (`core.hooksPath`, falling back to `.git/hooks`). It flags hooks whose
// deployed bytes differ from source, or that are not deployed at all — the
// silent-no-op failure mode where a global core.hooksPath shadows the
// repo's intended hooks.
func hookDrift(sc *scanCtx) (Metric, []string) {
	hooksDir := activeHooksDir(sc)
	if hooksDir == "" {
		return Metric{Available: false, Note: "could not resolve active hooks directory"}, nil
	}

	// Collect source hooks: <root>/**/.githooks/<standard-hook-name>.
	sources := map[string]string{} // basename -> source path
	if err := walkRepoFiles(sc.root, func(path string) {
		if filepath.Base(filepath.Dir(path)) != ".githooks" {
			return
		}
		name := filepath.Base(path)
		if gitHookNames[name] {
			sources[name] = path // last one wins; basenames are unique enough
		}
	}); err != nil {
		return Metric{Available: false, Note: "hook source scan failed: " + err.Error()}, nil
	}
	if len(sources) == 0 {
		return Metric{Available: false, Note: "no .githooks/ source hooks found to compare"}, nil
	}

	var drifted []string
	for name, src := range sources {
		deployed := filepath.Join(hooksDir, name)
		got, err := os.ReadFile(deployed)
		if err != nil {
			drifted = append(drifted, name+" (not deployed at "+shortHome(hooksDir)+")")
			continue
		}
		want, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
			drifted = append(drifted, name+" (deployed copy differs from "+relTo(sc.root, src)+")")
		}
	}
	sort.Strings(drifted)
	return Metric{Available: true}, drifted
}

// activeHooksDir resolves the directory git actually runs hooks from.
func activeHooksDir(sc *scanCtx) string {
	res := run(sc.root, sc.opts.gitTimeout, "git", "config", "--get", "core.hooksPath")
	if res.ok() && res.stdout != "" {
		return expandHome(res.stdout)
	}
	gd := run(sc.root, sc.opts.gitTimeout, "git", "rev-parse", "--git-path", "hooks")
	if gd.ok() && gd.stdout != "" {
		if filepath.IsAbs(gd.stdout) {
			return gd.stdout
		}
		return filepath.Join(sc.root, gd.stdout)
	}
	return ""
}

// docPairingDrift reports rationale docs that have drifted from their
// content. A `*.why.md` should sit beside the content it explains — either
// `<base>.ai.md` or `<base>.md`; an orphaned `*.why.md` is real drift
// (warn). A `*.ai.md` without a `*.why.md` is common and expected, so it is
// reported only as an informational note, not a failure.
func docPairingDrift(sc *scanCtx) (Metric, []string) {
	var why, ai []string
	if err := walkRepoFiles(sc.root, func(path string) {
		switch {
		case strings.HasSuffix(path, ".why.md"):
			why = append(why, path)
		case strings.HasSuffix(path, ".ai.md"):
			ai = append(ai, path)
		}
	}); err != nil {
		return Metric{Available: false, Note: "doc pairing scan failed: " + err.Error()}, nil
	}

	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	var unpaired []string
	for _, w := range why {
		base := strings.TrimSuffix(w, ".why.md")
		if !exists(base+".ai.md") && !exists(base+".md") {
			unpaired = append(unpaired, relTo(sc.root, w)+" (orphaned rationale: no .ai.md/.md content)")
		}
	}
	for _, a := range ai {
		base := strings.TrimSuffix(a, ".ai.md")
		if !exists(base + ".why.md") {
			unpaired = append(unpaired, relTo(sc.root, a)+" (no .why.md rationale) [info]")
		}
	}
	sort.Strings(unpaired)
	return Metric{Available: true}, unpaired
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// shortHome replaces the home prefix with ~ for compact display.
func shortHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
