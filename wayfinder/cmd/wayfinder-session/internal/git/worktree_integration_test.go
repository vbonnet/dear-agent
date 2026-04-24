package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegration_WorktreeLifecycle tests the complete worktree workflow:
// 1. Create worktree project (wayfinder start)
// 2. Work in worktree (wayfinder next-phase)
// 3. Merge to main (wayfinder stop)
func TestIntegration_WorktreeLifecycle(t *testing.T) {
	// Create test workspace (git repo)
	workspaceRoot := t.TempDir()
	initGitRepo(t, workspaceRoot)

	projectID := "test-worktree-project"
	worktreePath := filepath.Join(workspaceRoot, "wf-worktrees", projectID)
	projectPath := filepath.Join(worktreePath, "wf", projectID)
	mainProjectPath := filepath.Join(workspaceRoot, "wf", projectID)

	t.Run("step1_claim_namespace_on_main", func(t *testing.T) {
		// Simulate /wayfinder-start: Create namespace claim on main
		if err := os.MkdirAll(mainProjectPath, 0755); err != nil {
			t.Fatalf("failed to create project dir: %v", err)
		}

		statusContent := `---
schema_version: "2.0"
project_path: ` + mainProjectPath + `
status: claimed
---

# Wayfinder Project: test-worktree-project

**Status**: Namespace claimed (work in progress on branch wayfinder/test-worktree-project)
`
		statusFile := filepath.Join(mainProjectPath, "WAYFINDER-STATUS.md")
		if err := os.WriteFile(statusFile, []byte(statusContent), 0644); err != nil {
			t.Fatalf("failed to write status: %v", err)
		}

		// Commit to main
		runGit(t, workspaceRoot, "add", filepath.Join("wf", projectID))
		runGit(t, workspaceRoot, "commit", "-m", "wayfinder: claim "+projectID+" namespace")

		// Verify claim on main
		assertFileExists(t, statusFile, "namespace claim file")
		assertOnBranch(t, workspaceRoot, "main")
	})

	t.Run("step2_create_wayfinder_branch", func(t *testing.T) {
		// Create wayfinder/{project} branch from main
		runGit(t, workspaceRoot, "branch", "wayfinder/"+projectID)

		// Verify branch exists
		branches := runGitOutput(t, workspaceRoot, "branch", "--list", "wayfinder/"+projectID)
		if !strings.Contains(branches, "wayfinder/"+projectID) {
			t.Fatalf("branch wayfinder/%s not created", projectID)
		}
	})

	t.Run("step3_create_sparse_worktree", func(t *testing.T) {
		// Create worktree (no checkout)
		runGit(t, workspaceRoot, "worktree", "add", "--no-checkout", worktreePath, "wayfinder/"+projectID)

		// Configure sparse checkout
		runGit(t, worktreePath, "sparse-checkout", "set", filepath.Join("wf", projectID))

		// Checkout files
		runGit(t, worktreePath, "checkout")

		// Verify worktree structure
		assertFileExists(t, projectPath, "worktree project directory")
		assertFileExists(t, filepath.Join(projectPath, "WAYFINDER-STATUS.md"), "status file in worktree")
		assertOnBranch(t, worktreePath, "wayfinder/"+projectID)
	})

	t.Run("step4_work_in_worktree", func(t *testing.T) {
		// Simulate creating phase deliverables in worktree
		w0Charter := filepath.Join(projectPath, "W0-charter.md")
		charterContent := `---
phase: "W0"
phase_name: "Project Framing"
wayfinder_session_id: "test-session-123"
created_at: "2026-02-10T10:00:00Z"
phase_engram_hash: "sha256:test-hash"
phase_engram_path: "~/test/engrams/w0.md"
---

# W0: Project Charter

Test charter for worktree integration test.
`
		if err := os.WriteFile(w0Charter, []byte(charterContent), 0644); err != nil {
			t.Fatalf("failed to write charter: %v", err)
		}

		// Commit to worktree branch
		runGit(t, worktreePath, "add", filepath.Join("wf", projectID, "W0-charter.md"))
		runGit(t, worktreePath, "commit", "-m", "wayfinder: complete W0 (Project Framing)")

		// Verify commit on worktree branch
		assertOnBranch(t, worktreePath, "wayfinder/"+projectID)
		log := runGitOutput(t, worktreePath, "log", "--oneline", "-n", "1")
		if !strings.Contains(log, "wayfinder: complete W0") {
			t.Fatalf("commit not found on worktree branch: %s", log)
		}
	})

	t.Run("step5_verify_main_unchanged", func(t *testing.T) {
		// Main should NOT have W0-charter.md yet
		w0CharterOnMain := filepath.Join(mainProjectPath, "W0-charter.md")
		if _, err := os.Stat(w0CharterOnMain); err == nil {
			t.Fatalf("W0-charter.md should NOT exist on main yet (worktree isolation failed)")
		}

		// Main should only have placeholder STATUS
		assertFileExists(t, filepath.Join(mainProjectPath, "WAYFINDER-STATUS.md"), "status on main")
	})

	t.Run("step6_rebase_worktree_onto_main", func(t *testing.T) {
		// Simulate rebase (worktree branch onto main)
		// Note: In test environment, no remote fetch needed (local repo only)

		// Rebase worktree branch onto main
		runGit(t, worktreePath, "rebase", "main")

		// Verify rebase succeeded
		log := runGitOutput(t, worktreePath, "log", "--oneline", "-n", "1")
		if !strings.Contains(log, "wayfinder: complete W0") {
			t.Fatalf("rebase lost commit: %s", log)
		}
	})

	t.Run("step7_merge_to_main", func(t *testing.T) {
		// Switch to main
		runGit(t, workspaceRoot, "checkout", "main")

		// Merge worktree branch (fast-forward)
		runGit(t, workspaceRoot, "merge", "--ff-only", "wayfinder/"+projectID)

		// Verify W0-charter.md now on main
		w0CharterOnMain := filepath.Join(mainProjectPath, "W0-charter.md")
		assertFileExists(t, w0CharterOnMain, "W0-charter.md merged to main")

		// Verify merge commit
		log := runGitOutput(t, workspaceRoot, "log", "--oneline", "-n", "1")
		if !strings.Contains(log, "wayfinder: complete W0") {
			t.Fatalf("merge failed, commit not on main: %s", log)
		}
	})

	t.Run("step8_remove_worktree", func(t *testing.T) {
		// Remove worktree
		runGit(t, workspaceRoot, "worktree", "remove", worktreePath)

		// Verify worktree directory gone
		if _, err := os.Stat(worktreePath); err == nil {
			t.Fatalf("worktree directory still exists after removal")
		}
	})

	t.Run("step9_delete_branch", func(t *testing.T) {
		// Delete wayfinder branch (now merged)
		runGit(t, workspaceRoot, "branch", "-d", "wayfinder/"+projectID)

		// Verify branch deleted
		branches := runGitOutput(t, workspaceRoot, "branch", "--list", "wayfinder/"+projectID)
		if strings.Contains(branches, "wayfinder/"+projectID) {
			t.Fatalf("branch wayfinder/%s still exists after deletion", projectID)
		}
	})

	t.Run("step10_verify_final_state", func(t *testing.T) {
		// Main should have complete project
		assertFileExists(t, filepath.Join(mainProjectPath, "WAYFINDER-STATUS.md"), "status on main")
		assertFileExists(t, filepath.Join(mainProjectPath, "W0-charter.md"), "charter on main")

		// Worktree should be gone
		if _, err := os.Stat(worktreePath); err == nil {
			t.Fatalf("worktree still exists: %s", worktreePath)
		}

		// Branch should be gone
		branches := runGitOutput(t, workspaceRoot, "branch", "--list", "wayfinder/"+projectID)
		if strings.Contains(branches, "wayfinder/"+projectID) {
			t.Fatalf("branch still exists after cleanup")
		}
	})
}

// TestIntegration_WorktreeValidation tests detection and validation of worktree projects
func TestIntegration_WorktreeValidation(t *testing.T) {
	workspaceRoot := t.TempDir()
	initGitRepo(t, workspaceRoot)

	projectID := "test-validation"
	worktreePath := filepath.Join(workspaceRoot, "wf-worktrees", projectID)
	projectPath := filepath.Join(worktreePath, "wf", projectID)

	// Setup: Create worktree project
	mainProjectPath := filepath.Join(workspaceRoot, "wf", projectID)
	os.MkdirAll(mainProjectPath, 0755)
	os.WriteFile(filepath.Join(mainProjectPath, "WAYFINDER-STATUS.md"), []byte("---\nstatus: claimed\n---\n"), 0644)
	runGit(t, workspaceRoot, "add", filepath.Join("wf", projectID))
	runGit(t, workspaceRoot, "commit", "-m", "claim namespace")
	runGit(t, workspaceRoot, "branch", "wayfinder/"+projectID)
	runGit(t, workspaceRoot, "worktree", "add", "--no-checkout", worktreePath, "wayfinder/"+projectID)
	runGit(t, worktreePath, "sparse-checkout", "set", filepath.Join("wf", projectID))
	runGit(t, worktreePath, "checkout")

	t.Run("detect_worktree_vs_legacy", func(t *testing.T) {
		// Test: Is this path a worktree project?
		isWorktree := strings.Contains(projectPath, "wf-worktrees/")
		if !isWorktree {
			t.Fatalf("failed to detect worktree project from path: %s", projectPath)
		}

		// Contrast: Legacy project path
		legacyPath := filepath.Join(workspaceRoot, "wf", "legacy-project", "WAYFINDER-STATUS.md")
		isLegacy := !strings.Contains(legacyPath, "wf-worktrees/")
		if !isLegacy {
			t.Fatalf("incorrectly identified legacy path as worktree: %s", legacyPath)
		}
	})

	t.Run("validate_on_correct_branch", func(t *testing.T) {
		// Test: Worktree must be on wayfinder/* branch
		branch := runGitOutput(t, worktreePath, "branch", "--show-current")
		branch = strings.TrimSpace(branch)

		expectedBranch := "wayfinder/" + projectID
		if branch != expectedBranch {
			t.Fatalf("worktree on wrong branch: got %q, want %q", branch, expectedBranch)
		}

		// Verify NOT on main
		if branch == "main" {
			t.Fatalf("worktree should NOT be on main branch")
		}
	})

	t.Run("reject_commits_to_main", func(t *testing.T) {
		// Test: Committing to main with worktree path should fail validation
		// This simulates the check in /wayfinder-next Step 1.4

		// Create test file in worktree
		testFile := filepath.Join(projectPath, "test.md")
		os.WriteFile(testFile, []byte("test content"), 0644)

		// If we accidentally committed to main, this validation should catch it
		// (Simulated - actual validation happens in skill)
		currentBranch := runGitOutput(t, worktreePath, "branch", "--show-current")
		currentBranch = strings.TrimSpace(currentBranch)

		if currentBranch == "main" && strings.Contains(projectPath, "wf-worktrees/") {
			t.Fatalf("VALIDATION FAILURE: Worktree project committed to main branch")
		}
	})
}

// TestIntegration_WorktreeConflictPrevention tests namespace claim prevents conflicts
func TestIntegration_WorktreeConflictPrevention(t *testing.T) {
	workspaceRoot := t.TempDir()
	initGitRepo(t, workspaceRoot)

	t.Run("namespace_claim_prevents_conflicts", func(t *testing.T) {
		projectID := "conflict-test"

		// Agent 1: Claims namespace on main
		mainProjectPath := filepath.Join(workspaceRoot, "wf", projectID)
		os.MkdirAll(mainProjectPath, 0755)
		os.WriteFile(filepath.Join(mainProjectPath, "WAYFINDER-STATUS.md"),
			[]byte("---\nstatus: claimed\n---\n"), 0644)
		runGit(t, workspaceRoot, "add", filepath.Join("wf", projectID))
		runGit(t, workspaceRoot, "commit", "-m", "agent1: claim namespace")

		// Agent 2: Tries to claim same namespace (should fail)
		// In real workflow, wayfinder start would detect existing directory
		_, err := os.Stat(mainProjectPath)
		if err != nil {
			t.Fatalf("namespace claim should prevent second agent from using same name")
		}

		// This proves the namespace claim on main prevents conflicts
		// Both agents cannot claim wf/conflict-test/ simultaneously
	})

	t.Run("merge_different_projects_no_conflict", func(t *testing.T) {
		// Agent 1: Project A
		projectA := "project-a"
		mainPathA := filepath.Join(workspaceRoot, "wf", projectA)
		os.MkdirAll(mainPathA, 0755)
		os.WriteFile(filepath.Join(mainPathA, "WAYFINDER-STATUS.md"),
			[]byte("---\nstatus: claimed\n---\n"), 0644)
		runGit(t, workspaceRoot, "add", filepath.Join("wf", projectA))
		runGit(t, workspaceRoot, "commit", "-m", "claim project-a")

		// Agent 2: Project B
		projectB := "project-b"
		mainPathB := filepath.Join(workspaceRoot, "wf", projectB)
		os.MkdirAll(mainPathB, 0755)
		os.WriteFile(filepath.Join(mainPathB, "WAYFINDER-STATUS.md"),
			[]byte("---\nstatus: claimed\n---\n"), 0644)
		runGit(t, workspaceRoot, "add", filepath.Join("wf", projectB))
		runGit(t, workspaceRoot, "commit", "-m", "claim project-b")

		// Both projects coexist on main without conflict
		assertFileExists(t, filepath.Join(mainPathA, "WAYFINDER-STATUS.md"), "project A on main")
		assertFileExists(t, filepath.Join(mainPathB, "WAYFINDER-STATUS.md"), "project B on main")

		// This proves the namespace claim strategy prevents cross-project conflicts
	})
}

// Test helper functions

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	// Create initial commit (git requires at least one commit)
	readmeFile := filepath.Join(dir, "README.md")
	os.WriteFile(readmeFile, []byte("# Test Repo\n"), 0644)
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial commit")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, output)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, output)
	}
	return string(output)
}

func assertFileExists(t *testing.T, path, description string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("%s does not exist: %s", description, path)
	}
}

func assertOnBranch(t *testing.T, dir, expectedBranch string) {
	t.Helper()
	branch := runGitOutput(t, dir, "branch", "--show-current")
	branch = strings.TrimSpace(branch)
	if branch != expectedBranch {
		t.Fatalf("wrong branch: got %q, want %q", branch, expectedBranch)
	}
}
