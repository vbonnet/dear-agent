package workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)


// ErrOutputMissing is returned when a node finishes but at least one
// declared output does not exist on disk. The runner uses this to
// refuse the succeeded transition.
var ErrOutputMissing = errors.New("workflow: declared output missing")

// OutputWriter materialises declared outputs and (for tiers that
// require it) records them in the node_outputs table. Phase 1 covered
// the local_disk and git_committed tiers; Phase 3 wires
// engram_indexed through pkg/source.Adapter so the artifact is added
// to the knowledge corpus and becomes addressable via FetchSource and
// `dear-agent search`.
//
// The writer is intentionally tiny — paths are resolved as Go
// templates over the run's inputs/outputs/run_id, and the file is
// written if missing. The writer never overwrites; if the node was
// supposed to produce the file and didn't, MustExist returns
// ErrOutputMissing rather than fabricating contents.
type OutputWriter struct {
	// WorkflowDir is the directory of the workflow YAML. Relative
	// paths in OutputSpec.Path are resolved against this.
	WorkflowDir string

	// Recorder, if non-nil, persists a node_outputs row per declared
	// output. The SQLiteState backend implements OutputRecorder; tests
	// can plug in their own.
	Recorder OutputRecorder

	// Git, if non-nil, performs the `git add` + `git commit` for the
	// git_committed durability tier. Defaults to NewGitCommitter using
	// WorkflowDir; nil disables git commit (the file is still written
	// to disk).
	Git GitCommitter

	// SourceIndexer, if non-nil, indexes engram_indexed outputs into
	// the knowledge store. Implemented by anything that satisfies the
	// minimal Add+Name surface — pkg/source.Adapter is the production
	// implementation; tests use a fake. Nil disables indexing (the
	// file is still written and recorded as engram_indexed in
	// node_outputs).
	SourceIndexer SourceIndexer
}

// SourceIndexer is the small surface of pkg/source.Adapter that
// OutputWriter actually uses. Defined as a local interface so
// pkg/workflow does not import pkg/source — keeping the dependency
// graph one-way (workflow → source via composition, never the other
// direction).
type SourceIndexer interface {
	Name() string
	Add(ctx context.Context, s SourceArtifact) error
}

// SourceArtifact is the minimal payload OutputWriter hands to
// SourceIndexer for an engram_indexed output. Mirrors the substantive
// fields of pkg/source.Source without taking the dependency.
type SourceArtifact struct {
	URI         string
	Title       string
	Snippet     string
	Content     []byte
	ContentType string
	WorkItem    string
	Cues        []string
	IndexedAt   time.Time
}

// OutputRecorder is the substrate hook for writing node_outputs rows.
// SQLiteState implements it; tests can substitute a fake.
type OutputRecorder interface {
	RecordOutput(ctx context.Context, rec OutputRecord) error
}

// OutputRecord is the persisted shape of one node_outputs row.
type OutputRecord struct {
	RunID       string
	NodeID      string
	OutputKey   string
	Path        string
	ContentType string
	Durability  OutputDurability
	SizeBytes   int64
	Hash        string
}

// GitCommitter is the surface OutputWriter uses to commit a
// git_committed artifact. Implementations may shell out to `git` (the
// default) or use an in-process git library.
type GitCommitter interface {
	AddAndCommit(ctx context.Context, paths []string, message string) error
}

// MaterialiseOutputs walks the node's declared outputs in alphabetical
// order (so audit logs are diff-friendly), resolves each path template,
// and:
//
//   - confirms the file exists for local_disk / git_committed /
//     engram_indexed (the node body is responsible for producing it).
//   - records the node_outputs row.
//   - calls the git committer for git_committed.
//   - leaves a TODO breadcrumb for engram_indexed (Phase 3 wires
//     pkg/source.AddSource).
//
// Returns the first error encountered. Subsequent outputs are not
// processed — any failure to satisfy the contract should fail the node.
func (w *OutputWriter) MaterialiseOutputs(ctx context.Context, runID, nodeID string, specs map[string]OutputSpec, nc *nodeContext) error {
	keys := sortedKeys(specs)
	gitCommittedPaths := make([]string, 0, len(keys))
	engramIndexed := make([]engramOutput, 0)
	for _, key := range keys {
		spec := specs[key]
		path, err := w.resolvePath(spec.Path, runID, nc)
		if err != nil {
			return fmt.Errorf("output %q: render path: %w", key, err)
		}
		switch spec.Durability {
		case "", DurabilityEphemeral:
			// No persistence requirement; record what we know.
			if err := w.record(ctx, runID, nodeID, key, path, spec, 0, ""); err != nil {
				return err
			}
		case DurabilityLocalDisk, DurabilityGitCommitted, DurabilityEngramIndexed:
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("output %q: %w: %w", key, ErrOutputMissing, err)
			}
			h, hashErr := hashFile(path)
			if hashErr != nil {
				return fmt.Errorf("output %q: hash: %w", key, hashErr)
			}
			if err := w.record(ctx, runID, nodeID, key, path, spec, info.Size(), h); err != nil {
				return err
			}
			if spec.Durability == DurabilityGitCommitted {
				gitCommittedPaths = append(gitCommittedPaths, path)
			}
			if spec.Durability == DurabilityEngramIndexed {
				engramIndexed = append(engramIndexed, engramOutput{key: key, path: path, spec: spec})
			}
		default:
			return fmt.Errorf("output %q: unknown durability %q", key, spec.Durability)
		}
	}
	if len(gitCommittedPaths) > 0 && w.Git != nil {
		msg := fmt.Sprintf("workflow: outputs for run %s node %s", runID, nodeID)
		if err := w.Git.AddAndCommit(ctx, gitCommittedPaths, msg); err != nil {
			return fmt.Errorf("git_committed: %w", err)
		}
	}
	if len(engramIndexed) > 0 && w.SourceIndexer != nil {
		for _, out := range engramIndexed {
			if err := w.indexOutput(ctx, runID, nodeID, out); err != nil {
				return fmt.Errorf("engram_indexed %q: %w", out.key, err)
			}
		}
	}
	return nil
}

// engramOutput is the per-output bundle MaterialiseOutputs hands to
// the SourceIndexer once all local-disk + git-committed work has
// finished. Indexing happens last so a partial git failure doesn't
// leave the corpus referencing files that didn't make it onto disk.
type engramOutput struct {
	key  string
	path string
	spec OutputSpec
}

// indexOutput reads the materialised file and forwards it to the
// SourceIndexer. The URI is constructed deterministically from
// runID/nodeID/key so re-running a node updates the same Source row
// rather than spawning a duplicate.
func (w *OutputWriter) indexOutput(ctx context.Context, runID, nodeID string, out engramOutput) error {
	content, err := os.ReadFile(out.path) //nolint:gosec // path comes from operator-controlled YAML
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	uri := fmt.Sprintf("workflow://%s/%s/%s", runID, nodeID, out.key)
	title := filepath.Base(out.path)
	art := SourceArtifact{
		URI:         uri,
		Title:       title,
		Snippet:     snippetFor(content),
		Content:     content,
		ContentType: out.spec.ContentType,
		WorkItem:    fmt.Sprintf("%s/%s", runID, nodeID),
		Cues:        []string{nodeID, out.key},
		IndexedAt:   time.Now().UTC(),
	}
	return w.SourceIndexer.Add(ctx, art)
}

// snippetFor returns the first non-blank line of content, capped at
// 200 bytes. Cheap, deterministic, and good enough for a "preview"
// shown by `dear-agent search`.
func snippetFor(b []byte) string {
	const cap = 200
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > cap {
			return line[:cap]
		}
		return line
	}
	return ""
}

// resolvePath renders the path template and joins it against
// WorkflowDir if it is relative. The template scope is the standard
// workflow scope plus a {{ .RunID }} convenience.
func (w *OutputWriter) resolvePath(path, runID string, nc *nodeContext) (string, error) {
	rendered, err := renderOutputPath(path, runID, nc)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(rendered) && w.WorkflowDir != "" {
		rendered = filepath.Join(w.WorkflowDir, rendered)
	}
	return rendered, nil
}

// record forwards to the recorder when configured.
func (w *OutputWriter) record(ctx context.Context, runID, nodeID, key, path string, spec OutputSpec, size int64, hash string) error {
	if w.Recorder == nil {
		return nil
	}
	dur := spec.Durability
	if dur == "" {
		dur = DurabilityEphemeral
	}
	return w.Recorder.RecordOutput(ctx, OutputRecord{
		RunID:       runID,
		NodeID:      nodeID,
		OutputKey:   key,
		Path:        path,
		ContentType: spec.ContentType,
		Durability:  dur,
		SizeBytes:   size,
		Hash:        hash,
	})
}

// renderOutputPath is renderTemplate's shape but exposes RunID at the
// top-level scope. Kept separate from the runner's renderTemplate so
// output paths can use {{ .RunID }} without polluting the bash/ai
// template scope.
func renderOutputPath(t, runID string, nc *nodeContext) (string, error) {
	if !strings.Contains(t, "{{") {
		return t, nil
	}
	if nc == nil {
		nc = &nodeContext{outputs: map[string]string{}}
	}
	tpl, err := template.New("output_path").Option("missingkey=zero").Parse(t)
	if err != nil {
		return "", err
	}
	data := map[string]any{
		"Inputs":  nc.inputs,
		"Outputs": nc.outputs,
		"Env":     nc.env,
		"RunID":   runID,
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// hashFile returns the hex-encoded sha256 of the file at path.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from operator-controlled YAML
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// sortedKeys returns the keys of m in alphabetical order.
func sortedKeys(m map[string]OutputSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// ShellGitCommitter shells out to /usr/bin/env git for the commit. The
// default committer when WorkflowDir is a git repo.
type ShellGitCommitter struct {
	Dir string
}

// AddAndCommit runs `git add <paths>` then `git commit -m <message>`.
// Failures bubble up so the runner can audit the path that didn't
// commit (e.g. paths outside the repo).
func (g *ShellGitCommitter) AddAndCommit(ctx context.Context, paths []string, message string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	add := exec.CommandContext(ctx, "git", args...)
	add.Dir = g.Dir
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	commit := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commit.Dir = g.Dir
	if out, err := commit.CombinedOutput(); err != nil {
		// "nothing to commit" is not an error — the artifact already
		// matched a previously committed version.
		s := strings.TrimSpace(string(out))
		if strings.Contains(s, "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %w: %s", err, s)
	}
	return nil
}

// RecordOutput inserts (or updates) one row in node_outputs.
// SQLiteState satisfies OutputRecorder via this method — defined here
// rather than in state_sqlite.go so the contract sits next to the
// type that consumes it.
func (s *SQLiteState) RecordOutput(ctx context.Context, rec OutputRecord) error {
	if rec.RunID == "" || rec.NodeID == "" || rec.OutputKey == "" {
		return fmt.Errorf("workflow: RecordOutput: RunID, NodeID, OutputKey required")
	}
	dur := rec.Durability
	if dur == "" {
		dur = DurabilityEphemeral
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_outputs (run_id, node_id, output_key, path, content_type, durability, size_bytes, hash, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, node_id, output_key) DO UPDATE SET
		    path         = excluded.path,
		    content_type = excluded.content_type,
		    durability   = excluded.durability,
		    size_bytes   = excluded.size_bytes,
		    hash         = excluded.hash,
		    indexed_at   = excluded.indexed_at
	`, rec.RunID, rec.NodeID, rec.OutputKey, rec.Path, nullableString(rec.ContentType), string(dur), rec.SizeBytes, nullableString(rec.Hash), s.now())
	if err != nil {
		return fmt.Errorf("workflow: RecordOutput: %w", err)
	}
	return nil
}

// queryOutput is exported only for tests within the package; CLI tools
// query the table directly via DB().
func (s *SQLiteState) queryOutput(ctx context.Context, runID, nodeID, key string) (OutputRecord, error) {
	var rec OutputRecord
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, node_id, output_key, path, COALESCE(content_type, ''), durability, COALESCE(size_bytes, 0), COALESCE(hash, '')
		FROM node_outputs
		WHERE run_id = ? AND node_id = ? AND output_key = ?
	`, runID, nodeID, key)
	var dur string
	if err := row.Scan(&rec.RunID, &rec.NodeID, &rec.OutputKey, &rec.Path, &rec.ContentType, &dur, &rec.SizeBytes, &rec.Hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rec, fmt.Errorf("output not found")
		}
		return rec, err
	}
	rec.Durability = OutputDurability(dur)
	return rec, nil
}
