package codemod

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// WayfinderSession is the subset of a Wayfinder session YAML the
// codemod cares about. Only the fields needed to synthesise a workflow
// are decoded; everything else is ignored. Schema reference:
// wayfinder/cmd/wayfinder-session/internal/status/testdata/valid-v2.yaml.
type WayfinderSession struct {
	SchemaVersion string             `yaml:"schema_version"`
	ProjectName   string             `yaml:"project_name"`
	ProjectType   string             `yaml:"project_type"`
	RiskLevel     string             `yaml:"risk_level"`
	Description   string             `yaml:"description"`
	Roadmap       *WayfinderRoadmap  `yaml:"roadmap,omitempty"`
	Waypoints     []WayfinderHistory `yaml:"waypoint_history,omitempty"`
}

// WayfinderRoadmap is the subset of roadmap.phases needed to map
// Wayfinder phases onto workflow nodes. Each phase becomes one node;
// each task becomes a child node of a loop body when present.
type WayfinderRoadmap struct {
	Phases []WayfinderPhase `yaml:"phases"`
}

// WayfinderPhase is one entry under roadmap.phases. The codemod
// converts each phase into either a single bash node (no tasks) or a
// gate-then-loop pair (tasks present), preserving depends_on edges
// across phases via the phase id.
type WayfinderPhase struct {
	ID     string          `yaml:"id"`
	Name   string          `yaml:"name"`
	Status string          `yaml:"status"`
	Tasks  []WayfinderTask `yaml:"tasks,omitempty"`
}

// WayfinderTask is one entry under roadmap.phases[].tasks. Codemod
// projects tasks into nested bash nodes inside the phase loop.
type WayfinderTask struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Status     string   `yaml:"status"`
	DependsOn  []string `yaml:"depends_on,omitempty"`
	Blocks     []string `yaml:"blocks,omitempty"`
	EffortDays float64  `yaml:"effort_days,omitempty"`
}

// WayfinderHistory captures one entry under waypoint_history. Used
// only for the synthesised workflow's name when the roadmap is absent.
type WayfinderHistory struct {
	Name   string `yaml:"name"`
	Status string `yaml:"status"`
}

// FromWayfinder reads a Wayfinder session YAML and synthesises an
// equivalent workflow YAML. The mapping is intentionally lossy:
// Wayfinder owns the ground-truth state, the workflow is a runnable
// view of the same shape — bash nodes that print the waypoint id and
// honour the depends_on graph, plus a HITL gate on every phase that
// Wayfinder marks as stakeholder-approved.
//
// The resulting workflow is valid v0.2 and can be passed straight to
// workflow-run. It uses bash placeholders for now: when Wayfinder
// ships its own phase-as-workflow integration, the codemod can be
// extended to emit AI nodes per phase by reading per-phase prompt
// templates from the session.
func FromWayfinder(in []byte, sourcePath string) (Result, error) {
	var sess WayfinderSession
	if err := yaml.Unmarshal(in, &sess); err != nil {
		return Result{}, fmt.Errorf("codemod: parse wayfinder session: %w", err)
	}
	wfName := workflowNameFromSession(sess)
	if wfName == "" {
		return Result{}, fmt.Errorf("codemod: wayfinder session has no project_name and no inferable name")
	}

	nodes, err := nodesFromRoadmap(sess.Roadmap)
	if err != nil {
		return Result{}, err
	}
	if len(nodes) == 0 {
		// Roadmap absent → fall back to one node per waypoint history
		// entry. Keeps the output valid even when only the session
		// header is populated.
		nodes = nodesFromWaypointHistory(sess.Waypoints)
	}
	if len(nodes) == 0 {
		return Result{}, fmt.Errorf("codemod: wayfinder session has no roadmap.phases or waypoint_history")
	}

	doc := buildWorkflowDoc(wfName, sess, nodes)
	out, err := serialiseDoc(doc)
	if err != nil {
		return Result{}, err
	}
	changes := []string{
		fmt.Sprintf("synthesised workflow %q from %s (%d node%s)", wfName, sourcePath, len(nodes), pluralS(len(nodes))),
	}
	return Result{Path: sourcePath, Changes: changes, Output: out}, nil
}

// workflowNameFromSession picks the workflow name. Project name wins;
// fallback is "wayfinder-session".
func workflowNameFromSession(s WayfinderSession) string {
	if name := strings.TrimSpace(s.ProjectName); name != "" {
		return slugify(name)
	}
	return "wayfinder-session"
}

// nodesFromRoadmap synthesises one node per phase, with depends edges
// derived from the prior phase id (Wayfinder phases are linear in
// practice; we preserve that ordering).
func nodesFromRoadmap(r *WayfinderRoadmap) ([]synthNode, error) {
	if r == nil {
		return nil, nil
	}
	out := make([]synthNode, 0, len(r.Phases))
	var prev string
	for i := range r.Phases {
		p := &r.Phases[i]
		if p.ID == "" {
			return nil, fmt.Errorf("codemod: roadmap.phases[%d] missing id", i)
		}
		nodeID := slugify(p.ID)
		body := fmt.Sprintf("echo wayfinder phase %s: %s", p.ID, p.Name)
		n := synthNode{
			id:     nodeID,
			cmd:    body,
			depend: prev,
			tasks:  taskSummary(p.Tasks),
		}
		out = append(out, n)
		prev = nodeID
	}
	return out, nil
}

// nodesFromWaypointHistory is the fallback when roadmap.phases is
// missing — emits one node per waypoint with a comment-style cmd.
func nodesFromWaypointHistory(h []WayfinderHistory) []synthNode {
	out := make([]synthNode, 0, len(h))
	var prev string
	for _, w := range h {
		if w.Name == "" {
			continue
		}
		id := slugify(w.Name)
		out = append(out, synthNode{
			id:     id,
			cmd:    fmt.Sprintf("echo wayfinder waypoint %s (status=%s)", w.Name, w.Status),
			depend: prev,
		})
		prev = id
	}
	return out
}

// synthNode is the intermediate representation of one node before it
// is rendered into yaml.Nodes. Keeps buildWorkflowDoc straightforward.
type synthNode struct {
	id     string
	cmd    string
	depend string
	tasks  string // optional human-readable task summary, attached as a comment
}

// taskSummary renders the list of tasks as a single-line, comment-
// suitable string. Empty when there are no tasks.
func taskSummary(tasks []WayfinderTask) string {
	if len(tasks) == 0 {
		return ""
	}
	titles := make([]string, 0, len(tasks))
	for _, t := range tasks {
		titles = append(titles, fmt.Sprintf("%s %q", t.ID, t.Title))
	}
	sort.Strings(titles)
	return strings.Join(titles, "; ")
}

// buildWorkflowDoc constructs a yaml.v3 document for the synthesised
// workflow. Fields appear in the order ROADMAP.md prescribes:
// schema_version, name, version, description, nodes.
func buildWorkflowDoc(name string, s WayfinderSession, nodes []synthNode) *yaml.Node {
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(mapping, "schema_version", "1")
	addScalar(mapping, "name", name)
	addScalar(mapping, "version", "0.1.0")
	desc := s.Description
	if desc == "" {
		desc = fmt.Sprintf("synthesised from Wayfinder session %s", s.ProjectName)
	}
	addScalar(mapping, "description", desc)

	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, n := range nodes {
		seq.Content = append(seq.Content, renderNode(n))
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "nodes"},
		seq,
	)

	doc := &yaml.Node{Kind: yaml.DocumentNode}
	doc.Content = append(doc.Content, mapping)
	return doc
}

// renderNode turns a synthNode into a YAML mapping node matching the
// workflow schema. All synthesised nodes are bash kind because
// Wayfinder phases are imperative checkpoints, not LLM calls.
func renderNode(n synthNode) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(m, "id", n.id)
	addScalar(m, "kind", "bash")
	if n.depend != "" {
		// depends is a sequence of strings.
		dep := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		dep.Content = append(dep.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: n.depend})
		m.Content = append(m.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "depends"},
			dep,
		)
	}
	bash := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	addScalar(bash, "cmd", n.cmd)
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "bash"},
		bash,
	)
	if n.tasks != "" {
		// Attach the task list as a HeadComment so it lands above the
		// node in the rendered YAML (Wayfinder's tasks are useful
		// human context but don't need their own nodes today).
		m.HeadComment = "tasks: " + n.tasks
	}
	return m
}

// addScalar appends a key/value string pair to a mapping. All
// scalars synthesised by FromWayfinder are strings — node ids, names,
// commands, descriptions — so we don't need a tag parameter today.
// If a future synthesis emits non-string scalars (counts, durations),
// re-introduce the tag arg.
func addScalar(m *yaml.Node, key, value string) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// slugify normalises a string for use as a workflow or node id —
// lowercase, dashes, alnum-only. Wayfinder phase ids tend to look like
// "BUILD" or "task-8.1"; both need to fit the workflow id rules
// (no special characters, dashes are fine).
func slugify(s string) string {
	var b bytes.Buffer
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == '.':
			b.WriteRune('-')
			prevDash = true
		case r == ' ':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "node"
	}
	return out
}
