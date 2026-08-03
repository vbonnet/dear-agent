package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/archive"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/retrospective"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

type fakeRewindCommitter struct {
	isRepo bool
	err    error
	called bool
}

func (f *fakeRewindCommitter) IsGitRepo() bool { return f.isRepo }

func (f *fakeRewindCommitter) CommitRewind(_, _ string, _ archive.ArchiveRef) error {
	f.called = true
	return f.err
}

func TestCommitRewindStateReturnsCommitFailure(t *testing.T) {
	integrator := &fakeRewindCommitter{isRepo: true, err: errors.New("hook rejected commit")}
	var stdout bytes.Buffer

	err := commitRewindState(integrator, status.WaypointV2Build, status.WaypointV2Plan, archive.ArchiveRef{}, &stdout)

	if !integrator.called {
		t.Fatal("CommitRewind was not called")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output", stdout.String())
	}
	if err == nil || !strings.Contains(err.Error(), "hook rejected commit") {
		t.Fatalf("commitRewindState() error = %v, want commit failure", err)
	}
}

func TestResetForRewindResetsTargetAndLaterPhases(t *testing.T) {
	now := time.Now()
	outcome := status.OutcomeSuccess
	st := &status.StatusV2{
		WaypointHistory: []status.WaypointHistory{
			{Name: status.WaypointV2Research, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
			{Name: status.WaypointV2Design, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
			{Name: status.WaypointV2Spec, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
		},
		Roadmap: &status.Roadmap{Phases: []status.RoadmapPhase{
			{ID: status.WaypointV2Research, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
			{ID: status.WaypointV2Design, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
			{ID: status.WaypointV2Spec, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
		}},
	}
	phases := status.AllPhasesV2Schema()
	resetForRewind(st, phases, 3)

	if got := st.WaypointHistory[0].Status; got != status.PhaseStatusV2Completed {
		t.Fatalf("earlier phase status = %q, want completed", got)
	}
	if got := len(st.WaypointHistory); got != 1 {
		t.Fatalf("waypoint history length = %d, want only the earlier completed entry", got)
	}
	for _, phase := range st.Roadmap.Phases[1:] {
		if phase.Status != status.PhaseStatusV2Pending || phase.StartedAt != nil || phase.CompletedAt != nil {
			t.Errorf("rewound roadmap phase = %+v, want clean pending state", phase)
		}
	}
	st.UpdatePhase(status.WaypointV2Design, status.PhaseStatusV2InProgress, "")
	restarted := st.GetPhaseHistory(status.WaypointV2Design)
	if restarted == nil || restarted.StartedAt.IsZero() {
		t.Fatalf("restarted target history = %+v, want a valid start timestamp", restarted)
	}
}

func TestResetLifecycleForRewindReopensTerminalStatus(t *testing.T) {
	completedAt := time.Now().Add(-time.Hour)
	rewoundAt := time.Now()
	st := &status.StatusV2{
		Status:         status.StatusV2Completed,
		LifecycleState: status.LifecycleCompleted,
		CompletionDate: &completedAt,
		BlockedReason:  "stale block",
		BlockedOn:      "stale-dependency",
		ErrorMessage:   "stale error",
		InputNeeded:    "stale input",
	}

	resetLifecycleForRewind(st, rewoundAt)

	if st.Status != status.StatusV2InProgress || st.LifecycleState != status.LifecycleWorking {
		t.Fatalf("rewound lifecycle = %q/%q, want in-progress/working", st.Status, st.LifecycleState)
	}
	if st.CompletionDate != nil || st.BlockedReason != "" || st.BlockedOn != "" || st.ErrorMessage != "" || st.InputNeeded != "" {
		t.Fatalf("rewound lifecycle retained terminal metadata: %+v", st)
	}
	if !st.UpdatedAt.Equal(rewoundAt) {
		t.Fatalf("updated_at = %s, want %s", st.UpdatedAt, rewoundAt)
	}
}

func TestValidateRewindTargetRejectsConfiguredSkip(t *testing.T) {
	now := time.Now()
	st := &status.StatusV2{
		SkipPhases: []string{status.WaypointV2Design},
		WaypointHistory: []status.WaypointHistory{
			{Name: status.WaypointV2Design, Status: status.PhaseStatusV2Skipped, StartedAt: now},
		},
	}

	err := validateRewindTarget(st, status.WaypointV2Design)
	if err == nil || err.Error() != "cannot rewind to phase DESIGN: phase is configured to be skipped" {
		t.Fatalf("validateRewindTarget() error = %v, want configured-skip rejection", err)
	}
}

func TestRunRewindLeavesLegacyHistoryUntouchedWhenTargetIsRejected(t *testing.T) {
	projectDir := t.TempDir()
	cleanupRewindLockFile(t, projectDir)
	previous := projectDirectory
	projectDirectory = projectDir
	t.Cleanup(func() { projectDirectory = previous })

	st := status.NewStatusV2("rewind-guard", "service", "low")
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("seed status file: %v", err)
	}

	legacyPath := filepath.Join(projectDir, history.LegacyHistoryFilename)
	if err := os.WriteFile(legacyPath, []byte("{\"event\":\"seed\"}\n"), 0o600); err != nil {
		t.Fatalf("seed legacy history: %v", err)
	}

	if err := runRewind(nil, []string{"NOT-A-PHASE"}); err == nil {
		t.Fatal("runRewind accepted an invalid target phase")
	}

	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("rejected rewind migrated the legacy history file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, history.HistoryFilename)); !os.IsNotExist(err) {
		t.Fatalf("rejected rewind created %s (stat err: %v)", history.HistoryFilename, err)
	}
}

func setupRewindCommandProject(t *testing.T, projectName, currentPhase string) string {
	t.Helper()

	projectDir := t.TempDir()
	cleanupRewindLockFile(t, projectDir)
	gittest.Run(t, projectDir, "init")
	gittest.HardenRepo(t, projectDir)
	gittest.Run(t, projectDir, "config", "user.name", "Test User")
	gittest.Run(t, projectDir, "config", "user.email", "test@example.com")

	for path, content := range map[string]string{
		".gitignore": ".wayfinder/archives/\n*.jsonl\nRETRO-retrospective.md\n",
		"README.md":  "# Test\n",
	} {
		if err := os.WriteFile(filepath.Join(projectDir, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	gittest.Run(t, projectDir, "add", ".gitignore", "README.md")
	gittest.Run(t, projectDir, "commit", "-m", "Initial")

	now := time.Now().UTC()
	outcome := status.OutcomeSuccess
	st := &status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		ProjectName:     projectName,
		ProjectType:     status.ProjectTypeFeature,
		RiskLevel:       status.RiskLevelS,
		CurrentWaypoint: currentPhase,
		Status:          status.StatusV2InProgress,
		LifecycleState:  status.LifecycleWorking,
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now,
	}
	for _, phase := range status.AllWaypointsV2Schema() {
		st.WaypointHistory = append(st.WaypointHistory, status.WaypointHistory{
			Name:        phase,
			Status:      status.PhaseStatusV2Completed,
			StartedAt:   now.Add(-time.Minute),
			CompletedAt: &now,
			Outcome:     &outcome,
		})
		if phase == currentPhase {
			break
		}
	}
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("write status: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, history.HistoryFilename), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write history: %v", err)
	}
	siblingName := currentPhase + "-sibling"
	if err := os.MkdirAll(filepath.Join(projectDir, ".wayfinder", "archives", siblingName), 0o700); err != nil {
		t.Fatalf("create sibling archive: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".wayfinder", "archives", siblingName, "unrelated.txt"), []byte("private\n"), 0o600); err != nil {
		t.Fatalf("write sibling archive: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "user-notes.md"), []byte("private\n"), 0o600); err != nil {
		t.Fatalf("write user notes: %v", err)
	}
	gittest.Run(t, projectDir, "add", "user-notes.md")
	return projectDir
}

func TestRunRewindSamePhaseCommitsExactTraceAndArchive(t *testing.T) {
	projectDir := setupRewindCommandProject(t, "same-phase-rewind", status.WaypointV2Retro)

	previousDir, previousNoPrompt, previousReason, previousLearnings := projectDirectory, rewindNoPrompt, rewindReason, rewindLearnings
	projectDirectory, rewindNoPrompt, rewindReason, rewindLearnings = projectDir, true, "replay RETRO", "keep the trace complete"
	t.Cleanup(func() {
		projectDirectory, rewindNoPrompt, rewindReason, rewindLearnings = previousDir, previousNoPrompt, previousReason, previousLearnings
	})
	if err := runRewind(nil, []string{status.WaypointV2Retro}); err != nil {
		t.Fatalf("runRewind(RETRO): %v", err)
	}

	historyBytes, err := os.ReadFile(filepath.Join(projectDir, history.HistoryFilename))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if got := strings.Count(string(historyBytes), "rewind.logged"); got != 1 {
		t.Fatalf("rewind events = %d, want 1", got)
	}
	for _, want := range []string{
		`"reason":"replay RETRO"`,
		`"learnings":"keep the trace complete"`,
		`"current_phase":"RETRO"`,
		`"session_id":"same-phase-rewind"`,
	} {
		if !strings.Contains(string(historyBytes), want) {
			t.Errorf("rewind history missing %q:\n%s", want, historyBytes)
		}
	}
	retroBytes, err := os.ReadFile(filepath.Join(projectDir, retrospective.RetroFilename))
	if err != nil {
		t.Fatalf("read RETRO: %v", err)
	}
	if got := strings.Count(string(retroBytes), "## Rewind: RETRO → RETRO (magnitude 0)"); got != 1 {
		t.Fatalf("rewind blocks = %d, want 1", got)
	}
	for _, want := range []string{"**Reason**: replay RETRO", "**Learnings**: keep the trace complete"} {
		if !strings.Contains(string(retroBytes), want) {
			t.Errorf("rewind retrospective missing %q:\n%s", want, retroBytes)
		}
	}
	updated, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		t.Fatalf("parse rewound status: %v", err)
	}
	if updated.CurrentWaypoint != status.WaypointV2Retro || updated.Status != status.StatusV2InProgress {
		t.Errorf("rewound status = phase %s status %s, want active RETRO", updated.CurrentWaypoint, updated.Status)
	}

	committed, err := gittest.Command(t, projectDir, "show", "--name-only", "--format=", "HEAD").Output()
	if err != nil {
		t.Fatalf("show rewind commit: %v", err)
	}
	committedFiles := string(committed)
	if !strings.Contains(committedFiles, ".wayfinder/archives/RETRO-") {
		t.Fatalf("rewind commit missing ignored archive:\n%s", committedFiles)
	}
	for _, unrelated := range []string{"user-notes.md", "RETRO-sibling"} {
		if strings.Contains(committedFiles, unrelated) {
			t.Errorf("rewind commit included unrelated staged content %q:\n%s", unrelated, committedFiles)
		}
	}
	staged, err := gittest.Command(t, projectDir, "diff", "--cached", "--name-only").Output()
	if err != nil {
		t.Fatalf("list retained staged files: %v", err)
	}
	if strings.TrimSpace(string(staged)) != "user-notes.md" {
		t.Errorf("retained staged files = %q, want user-notes.md", staged)
	}
}

func TestRunRewindCrossPhasePreservesTraceAndGitScope(t *testing.T) {
	projectDir := setupRewindCommandProject(t, "cross-phase-rewind", status.WaypointV2Build)
	previousDir, previousNoPrompt, previousReason, previousLearnings := projectDirectory, rewindNoPrompt, rewindReason, rewindLearnings
	projectDirectory, rewindNoPrompt, rewindReason, rewindLearnings = projectDir, true, "rework the plan", "validate the seam first"
	t.Cleanup(func() {
		projectDirectory, rewindNoPrompt, rewindReason, rewindLearnings = previousDir, previousNoPrompt, previousReason, previousLearnings
	})

	if err := runRewind(nil, []string{status.WaypointV2Plan}); err != nil {
		t.Fatalf("runRewind(BUILD -> PLAN): %v", err)
	}

	historyBytes, err := os.ReadFile(filepath.Join(projectDir, history.HistoryFilename))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if got := strings.Count(string(historyBytes), "rewind.logged"); got != 1 {
		t.Fatalf("rewind events = %d, want 1", got)
	}
	for _, want := range []string{
		`"from_phase":"BUILD"`,
		`"to_phase":"PLAN"`,
		`"magnitude":2`,
		`"reason":"rework the plan"`,
		`"learnings":"validate the seam first"`,
		`"current_phase":"PLAN"`,
		`"session_id":"cross-phase-rewind"`,
	} {
		if !strings.Contains(string(historyBytes), want) {
			t.Errorf("cross-phase history missing %q:\n%s", want, historyBytes)
		}
	}

	retroBytes, err := os.ReadFile(filepath.Join(projectDir, retrospective.RetroFilename))
	if err != nil {
		t.Fatalf("read RETRO: %v", err)
	}
	if got := strings.Count(string(retroBytes), "## Rewind: BUILD → PLAN (magnitude 2)"); got != 1 {
		t.Fatalf("cross-phase rewind blocks = %d, want 1", got)
	}

	updated, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		t.Fatalf("parse rewound status: %v", err)
	}
	if updated.CurrentWaypoint != status.WaypointV2Plan || updated.Status != status.StatusV2InProgress {
		t.Errorf("rewound status = phase %s status %s, want active PLAN", updated.CurrentWaypoint, updated.Status)
	}
	if updated.GetPhaseHistory(status.WaypointV2Plan) != nil || updated.GetPhaseHistory(status.WaypointV2Build) != nil {
		t.Errorf("rewound status retained reset phase history: %+v", updated.WaypointHistory)
	}

	archiveRoot := filepath.Join(projectDir, ".wayfinder", "archives")
	archiveEntries, err := os.ReadDir(archiveRoot)
	if err != nil {
		t.Fatalf("read archive root: %v", err)
	}
	generatedArchive := ""
	for _, entry := range archiveEntries {
		if strings.HasPrefix(entry.Name(), "BUILD-") && entry.Name() != "BUILD-sibling" {
			if generatedArchive != "" {
				t.Fatalf("multiple generated BUILD archives: %q and %q", generatedArchive, entry.Name())
			}
			generatedArchive = entry.Name()
		}
	}
	if generatedArchive == "" {
		t.Fatal("cross-phase rewind did not publish a BUILD archive")
	}
	archivedStatus, err := os.ReadFile(filepath.Join(archiveRoot, generatedArchive, status.StatusFilename))
	if err != nil {
		t.Fatalf("read archived status: %v", err)
	}
	if !strings.Contains(string(archivedStatus), "current_waypoint: BUILD") {
		t.Fatalf("archive did not preserve pre-rewind BUILD state:\n%s", archivedStatus)
	}

	committed, err := gittest.Command(t, projectDir, "show", "--name-only", "--format=", "HEAD").Output()
	if err != nil {
		t.Fatalf("show rewind commit: %v", err)
	}
	committedFiles := string(committed)
	wantArchivePath := filepath.ToSlash(filepath.Join(".wayfinder", "archives", generatedArchive, status.StatusFilename))
	if !strings.Contains(committedFiles, wantArchivePath) {
		t.Fatalf("cross-phase commit missing exact archive %q:\n%s", wantArchivePath, committedFiles)
	}
	for _, unrelated := range []string{"user-notes.md", "BUILD-sibling"} {
		if strings.Contains(committedFiles, unrelated) {
			t.Errorf("cross-phase commit included unrelated content %q:\n%s", unrelated, committedFiles)
		}
	}
	staged, err := gittest.Command(t, projectDir, "diff", "--cached", "--name-only").Output()
	if err != nil {
		t.Fatalf("list retained staged files: %v", err)
	}
	if strings.TrimSpace(string(staged)) != "user-notes.md" {
		t.Errorf("retained staged files = %q, want user-notes.md", staged)
	}
}

func TestRunRewindRejectsInvalidRetrospectiveBeforeStatusMutation(t *testing.T) {
	projectDir := t.TempDir()
	cleanupRewindLockFile(t, projectDir)
	previousDir, previousNoPrompt := projectDirectory, rewindNoPrompt
	projectDirectory, rewindNoPrompt = projectDir, true
	t.Cleanup(func() {
		projectDirectory, rewindNoPrompt = previousDir, previousNoPrompt
	})

	now := time.Now().UTC()
	outcome := status.OutcomeSuccess
	st := status.NewStatusV2("rewind-admission", status.ProjectTypeBugfix, status.RiskLevelS)
	st.Status = status.StatusV2InProgress
	st.LifecycleState = status.LifecycleWorking
	st.WaypointHistory = []status.WaypointHistory{{
		Name:        status.WaypointV2Charter,
		Status:      status.PhaseStatusV2Completed,
		StartedAt:   now.Add(-time.Minute),
		CompletedAt: &now,
		Outcome:     &outcome,
	}}
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("write status: %v", err)
	}
	statusPath := filepath.Join(projectDir, status.StatusFilename)
	statusBefore, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status before rewind: %v", err)
	}
	if err := os.Mkdir(filepath.Join(projectDir, retrospective.RetroFilename), 0o700); err != nil {
		t.Fatalf("create invalid RETRO directory: %v", err)
	}

	err = runRewind(nil, []string{status.WaypointV2Charter})
	if err == nil || !strings.Contains(err.Error(), "archive RETRO file") {
		t.Fatalf("runRewind() error = %v, want RETRO admission failure", err)
	}
	statusAfter, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status after rewind: %v", err)
	}
	if !bytes.Equal(statusAfter, statusBefore) {
		t.Fatal("rejected rewind mutated canonical status")
	}
	if _, err := os.Stat(filepath.Join(projectDir, history.HistoryFilename)); !os.IsNotExist(err) {
		t.Fatalf("rejected rewind created history evidence: %v", err)
	}
	archives, err := os.ReadDir(filepath.Join(projectDir, ".wayfinder", "archives"))
	if err != nil {
		t.Fatalf("read archive root: %v", err)
	}
	if len(archives) != 0 {
		t.Fatalf("rejected rewind left archive state: %v", archives)
	}
}
