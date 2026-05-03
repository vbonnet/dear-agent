package dev

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// Session is the dev-mode interactive shell. One Session corresponds to
// one workflow file plus its fixtures and accumulates the run history
// in memory so `diff` can compare against prior runs.
//
// Sessions are not safe for concurrent use; the REPL drives one verb at
// a time and the file watcher posts reload events through Run rather
// than mutating state directly.
type Session struct {
	WorkflowPath string
	FixturesPath string

	// LiveAI is the real AIExecutor used when the operator runs `r
	// --live`. Nil rejects --live (mocked-only mode). Most dev sessions
	// run with mocks; --live is the safety valve for verifying a
	// fixture against the real model occasionally.
	LiveAI workflow.AIExecutor

	// Out is where the REPL writes its prompts and run output. Defaults
	// to os.Stdout when zero.
	Out io.Writer

	wf       *workflow.Workflow
	fixtures *FixtureSet
	history  []runRecord
}

// runRecord captures one prior run for `diff` to compare against.
type runRecord struct {
	startedAt time.Time
	live      bool
	report    *workflow.RunReport
	outputs   map[string]string
	err       error
}

// NewSession constructs a Session for the given workflow file. Fixtures
// are loaded from fixturesPath if non-empty; pass "" to use the
// conventional companion file (workflow.yaml → workflow.fixtures.yaml).
func NewSession(workflowPath, fixturesPath string) (*Session, error) {
	w, err := workflow.LoadFile(workflowPath)
	if err != nil {
		return nil, err
	}
	if fixturesPath == "" {
		fixturesPath = FixtureFile(workflowPath)
	}
	fx, err := LoadFixtures(fixturesPath)
	if err != nil {
		return nil, err
	}
	return &Session{
		WorkflowPath: workflowPath,
		FixturesPath: fixturesPath,
		Out:          os.Stdout,
		wf:           w,
		fixtures:     fx,
	}, nil
}

// Workflow returns the currently loaded workflow. Useful for tests and
// for the REPL banner.
func (s *Session) Workflow() *workflow.Workflow { return s.wf }

// Fixtures returns the active fixture set. Read-only access.
func (s *Session) Fixtures() *FixtureSet { return s.fixtures }

// History returns the in-memory run history (most recent last). The
// returned slice aliases internal storage; callers must not mutate.
func (s *Session) History() []runRecord { return s.history }

// Reload re-reads both the workflow YAML and the fixtures file. Returns
// the new node count (workflow) and fixture count.
func (s *Session) Reload() (int, int, error) {
	w, err := workflow.LoadFile(s.WorkflowPath)
	if err != nil {
		return 0, 0, err
	}
	fixCount, err := s.fixtures.Reload()
	if err != nil {
		return 0, 0, err
	}
	s.wf = w
	return len(w.Nodes), fixCount, nil
}

// Run executes the workflow. live=false uses the mock executor with
// the loaded fixtures; live=true uses the operator-supplied LiveAI (or
// returns an error if none is configured).
//
// Each call records a runRecord so `diff` can compare adjacent runs.
func (s *Session) Run(ctx context.Context, live bool) (*workflow.RunReport, error) {
	exec, err := s.executorFor(live)
	if err != nil {
		return nil, err
	}
	r := workflow.NewRunner(exec)
	r.DefaultWorkingDir = filepath.Dir(s.WorkflowPath)
	report, runErr := r.Run(ctx, s.wf, nil)
	rec := runRecord{
		startedAt: time.Now().UTC(),
		live:      live,
		report:    report,
		outputs:   reportOutputs(report),
		err:       runErr,
	}
	s.history = append(s.history, rec)
	return report, runErr
}

// RetryNode re-executes a single node by id. It builds a synthetic
// single-node workflow that depends on the original's outputs, so
// downstream {{ .Outputs.<id> }} references continue to resolve. This
// is the "tweak the prompt, retry just this node" verb from the
// 10-minute walkthrough.
func (s *Session) RetryNode(ctx context.Context, id string, live bool) (*workflow.Result, error) {
	target := s.findNode(id)
	if target == nil {
		return nil, fmt.Errorf("node %q not found in %s", id, s.WorkflowPath)
	}
	last := s.lastSuccessfulOutputs()

	exec, err := s.executorFor(live)
	if err != nil {
		return nil, err
	}
	r := workflow.NewRunner(exec)
	r.DefaultWorkingDir = filepath.Dir(s.WorkflowPath)

	mini := &workflow.Workflow{
		Name:    fmt.Sprintf("%s-retry-%s", s.wf.Name, id),
		Version: "0.0.0",
		Nodes:   []workflow.Node{cloneNodeWithoutDeps(target)},
	}
	report, runErr := r.Run(ctx, mini, last)
	if runErr != nil {
		return nil, runErr
	}
	if len(report.Results) == 0 {
		return nil, errors.New("retry produced no results")
	}
	return &report.Results[0], nil
}

// DiffNode reports the difference between the most recent run's output
// for a node and the prior run's output. Returns ("", nil) when there
// is no diff (or no prior run). Empty histories return an explicit
// error so the REPL can render "no prior run".
func (s *Session) DiffNode(id string) (string, error) {
	if len(s.history) < 2 {
		return "", errors.New("need at least two runs to diff")
	}
	curr := s.history[len(s.history)-1].outputs[id]
	prev := s.history[len(s.history)-2].outputs[id]
	if curr == prev {
		return "", nil
	}
	return formatLineDiff(prev, curr), nil
}

// Approve writes a placeholder approval message. Real HITL approval
// requires a SQLite-backed runner with the approval poll loop wired
// up; in dev mode the runner is the mock executor and never enters
// awaiting_hitl. Surfacing this explicitly keeps the verb discoverable
// while signalling that --live mode is required for true HITL flows.
func (s *Session) Approve(approvalID string) error {
	if approvalID == "" {
		return errors.New("approve requires an approval id")
	}
	fmt.Fprintf(s.out(), "(dev) approve %s — dev mode runs without HITL backend; use workflow-approve against a SQLite db for real approvals\n", approvalID)
	return nil
}

func (s *Session) out() io.Writer {
	if s.Out == nil {
		return os.Stdout
	}
	return s.Out
}

func (s *Session) executorFor(live bool) (workflow.AIExecutor, error) {
	if !live {
		return &MockAIExecutor{Fixtures: s.fixtures}, nil
	}
	if s.LiveAI == nil {
		return nil, errors.New("live AI executor not configured; pass --live-ai or run without --live")
	}
	return s.LiveAI, nil
}

func (s *Session) findNode(id string) *workflow.Node {
	for i := range s.wf.Nodes {
		n := &s.wf.Nodes[i]
		if n.ID == id {
			return n
		}
		if n.Kind == workflow.KindLoop && n.Loop != nil {
			for j := range n.Loop.Nodes {
				if n.Loop.Nodes[j].ID == id {
					return &n.Loop.Nodes[j]
				}
			}
		}
	}
	return nil
}

func (s *Session) lastSuccessfulOutputs() map[string]string {
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].err == nil && len(s.history[i].outputs) > 0 {
			return cloneStringMap(s.history[i].outputs)
		}
	}
	return map[string]string{}
}

// cloneNodeWithoutDeps copies a Node minus its Depends slice so the
// retry mini-workflow is a single root.
func cloneNodeWithoutDeps(n *workflow.Node) workflow.Node {
	c := *n
	c.Depends = nil
	return c
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func reportOutputs(r *workflow.RunReport) map[string]string {
	out := map[string]string{}
	if r == nil {
		return out
	}
	for _, res := range r.Results {
		out[res.NodeID] = res.Output
	}
	return out
}

// formatLineDiff is a minimal line-level diff renderer — enough to
// show the operator what changed between two runs without pulling in
// a full diff library. Lines unique to prev are prefixed with "-",
// lines unique to curr with "+", common lines with " ".
//
// The output is order-preserving on the curr side: every curr line is
// emitted in order, with a "+" if it was added and " " if it appeared
// in prev. After all curr lines, any leftover prev lines are emitted
// with "-". Sufficient for the "did the prompt change move the needle"
// inner-loop check; not a replacement for `diff -u`.
func formatLineDiff(prev, curr string) string {
	prevLines := strings.Split(prev, "\n")
	currLines := strings.Split(curr, "\n")

	prevSet := make(map[string]int, len(prevLines))
	for _, l := range prevLines {
		prevSet[l]++
	}
	var out strings.Builder
	for _, l := range currLines {
		if prevSet[l] > 0 {
			out.WriteString(" ")
			prevSet[l]--
		} else {
			out.WriteString("+")
		}
		out.WriteString(l)
		out.WriteByte('\n')
	}
	// Anything still in prevSet is a deletion.
	for _, l := range prevLines {
		if prevSet[l] > 0 {
			out.WriteString("-")
			out.WriteString(l)
			out.WriteByte('\n')
			prevSet[l]--
		}
	}
	return out.String()
}

// REPL drives the interactive shell. Reads commands from r line by line
// until EOF or `exit`. Each verb is documented inline so the user can
// run `help` and see the same list.
//
//nolint:gocyclo // command dispatch is naturally a switch; splitting hurts readability
func REPL(ctx context.Context, sess *Session, in io.Reader, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	sess.Out = out
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024*64), 1024*1024)
	fmt.Fprintf(out, "workflow-dev: %s (%d node%s, %d fixture%s)\n",
		sess.WorkflowPath, len(sess.wf.Nodes), pluralS(len(sess.wf.Nodes)),
		len(sess.fixtures.Responses), pluralS(len(sess.fixtures.Responses)))
	fmt.Fprintln(out, `verbs: r [--live]  retry <node>  diff <node>  approve <id>  reload  fixtures  nodes  history  help  exit`)
	for {
		fmt.Fprint(out, "dev> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "exit", "quit":
			return nil
		case "help":
			printHelp(out)
		case "nodes":
			printNodes(out, sess.wf)
		case "fixtures":
			printFixtures(out, sess.fixtures)
		case "history":
			printHistory(out, sess.history)
		case "reload":
			n, f, err := sess.Reload()
			if err != nil {
				fmt.Fprintf(out, "reload failed: %v\n", err)
				break
			}
			fmt.Fprintf(out, "reloaded: %d node%s, %d fixture%s\n", n, pluralS(n), f, pluralS(f))
		case "r", "run":
			live := false
			for _, f := range fields[1:] {
				if f == "--live" {
					live = true
				}
			}
			started := time.Now()
			report, err := sess.Run(ctx, live)
			if err != nil {
				fmt.Fprintf(out, "run failed in %s: %v\n", time.Since(started), err)
				break
			}
			printRun(out, report, time.Since(started), live)
		case "retry":
			if len(fields) < 2 {
				fmt.Fprintln(out, "usage: retry <node-id>")
				break
			}
			res, err := sess.RetryNode(ctx, fields[1], false)
			if err != nil {
				fmt.Fprintf(out, "retry failed: %v\n", err)
				break
			}
			fmt.Fprintf(out, "retry %s: ok in %s\n", fields[1], res.Finished.Sub(res.Started))
			fmt.Fprintln(out, indent(res.Output, "  "))
		case "diff":
			if len(fields) < 2 {
				fmt.Fprintln(out, "usage: diff <node-id>")
				break
			}
			d, err := sess.DiffNode(fields[1])
			if err != nil {
				fmt.Fprintln(out, err)
				break
			}
			if d == "" {
				fmt.Fprintln(out, "no change")
			} else {
				fmt.Fprint(out, d)
			}
		case "approve":
			if len(fields) < 2 {
				fmt.Fprintln(out, "usage: approve <approval-id>")
				break
			}
			if err := sess.Approve(fields[1]); err != nil {
				fmt.Fprintf(out, "approve failed: %v\n", err)
			}
		default:
			fmt.Fprintf(out, "unknown verb %q (try `help`)\n", fields[0])
		}
	}
	return scanner.Err()
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, `verbs:
  r [--live]        run the workflow (mock by default; --live uses the configured LiveAI)
  retry <node>      re-run a single node, replaying upstream outputs
  diff <node>       diff this run's output against the prior run
  approve <id>      placeholder; real approvals require workflow-approve against a SQLite db
  reload            re-read the workflow YAML and fixture file from disk
  fixtures          list fixture node ids
  nodes             list workflow node ids
  history           print run history (mock vs live, success vs error)
  help              this listing
  exit              leave the dev session`)
}

func printNodes(out io.Writer, w *workflow.Workflow) {
	for _, n := range w.Nodes {
		fmt.Fprintf(out, "  %-20s %s\n", n.ID, n.Kind)
	}
}

func printFixtures(out io.Writer, fs *FixtureSet) {
	keys := fs.Keys()
	if len(keys) == 0 {
		fmt.Fprintln(out, "(no fixtures)")
		return
	}
	for _, k := range keys {
		v, _ := fs.Get(k)
		fmt.Fprintf(out, "  %-20s %s\n", k, oneLineSummary(v))
	}
}

func printHistory(out io.Writer, hist []runRecord) {
	if len(hist) == 0 {
		fmt.Fprintln(out, "(no runs yet)")
		return
	}
	for i, r := range hist {
		mode := "mock"
		if r.live {
			mode = "live"
		}
		status := "ok"
		if r.err != nil {
			status = fmt.Sprintf("err: %v", r.err)
		}
		ids := make([]string, 0, len(r.outputs))
		for k := range r.outputs {
			ids = append(ids, k)
		}
		sort.Strings(ids)
		fmt.Fprintf(out, "  [%d] %s %-4s %s nodes=%v\n",
			i+1, r.startedAt.Format(time.RFC3339), mode, status, ids)
	}
}

func printRun(out io.Writer, report *workflow.RunReport, elapsed time.Duration, live bool) {
	mode := "mock"
	if live {
		mode = "live"
	}
	if report == nil {
		fmt.Fprintf(out, "run (%s) in %s — empty report\n", mode, elapsed)
		return
	}
	status := "ok"
	if !report.Succeeded {
		status = "failed"
	}
	fmt.Fprintf(out, "run (%s) in %s — %s (%d node%s)\n", mode, elapsed, status, len(report.Results), pluralS(len(report.Results)))
	for _, res := range report.Results {
		s := "ok"
		if res.Error != nil {
			s = fmt.Sprintf("err: %v", res.Error)
		}
		fmt.Fprintf(out, "  %-20s %s\n", res.NodeID, s)
	}
}

func oneLineSummary(s string) string {
	line := strings.SplitN(strings.TrimSpace(s), "\n", 2)[0]
	if len(line) > 60 {
		return line[:60] + "…"
	}
	return line
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" && i == len(lines)-1 {
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
