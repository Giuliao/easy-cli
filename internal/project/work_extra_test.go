package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkReplacementArchivesHistoryWithoutTransfer(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	if _, err := CheckpointWork(db, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateProject(db, CreateProjectParams{Name: "other"}); err != nil {
		t.Fatal(err)
	}
	w, err := BindWorkspace(db, dir, "other", true)
	if err != nil || w.ID == first.Workspace.ID || w.CurrentTaskID != "" {
		t.Fatalf("replacement = %+v %v", w, err)
	}
	archived, err := ListWorkspacesByProject(db, first.Project.ID)
	if err != nil || len(archived) != 1 || archived[0].Active || archived[0].CurrentTaskID != first.Task.ID {
		t.Fatalf("archived = %+v %v", archived, err)
	}
	history, err := WorkHistory(db, dir)
	if err != nil || len(history) != 0 {
		t.Fatalf("old project's history transferred: %+v %v", history, err)
	}
	archivedHistory, err := WorkHistoryByWorkspace(db, first.Workspace.ID)
	if err != nil || len(archivedHistory) != 1 || archivedHistory[0].TaskID != first.Task.ID {
		t.Fatalf("archived history not readable: %+v %v", archivedHistory, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM work_checkpoints WHERE workspace_id = ?`, first.Workspace.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("archive lost checkpoint: %d %v", count, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("replace requires explicit task selection: %v", err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: first.Task.ID}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("old project task inherited: %v", err)
	}
}

func TestWorkBindingIdempotentAndExplicitClear(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	w, err := BindWorkspace(db, dir, "feature", false)
	if err != nil || w.ID != first.Workspace.ID || w.CurrentTaskID != first.Task.ID {
		t.Fatalf("same binding changed task: %+v %v", w, err)
	}
	w, err = BindWorkspace(db, dir, "feature", true)
	if err != nil || w.CurrentTaskID != "" || !w.TaskSelectionRequired {
		t.Fatalf("clear = %+v %v", w, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("cleared task silently inherited: %v", err)
	}
	s, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: first.Task.ID})
	if err != nil || s.Task.ID != first.Task.ID {
		t.Fatalf("explicit resume = %+v %v", s, err)
	}
}

func TestWorkSavedBaseProtectedBeforeFirstCheckpoint(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	workGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "next")
	head := workGit(t, dir, "rev-parse", "HEAD")
	rangeValue := head + ".." + head
	if _, err := UpdateTask(db, first.Task.ID, UpdateTaskParams{CommitRange: &rangeValue}); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("manual range change reset base: %v", err)
	}
	if _, err := CheckpointWork(db, dir); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("manual range change checkpointed: %v", err)
	}
}

func TestWorkLegacyContextErrorsAreNotAbsence(t *testing.T) {
	for _, mode := range []string{"conflict", "damaged", "missing-project", "invalid-task"} {
		t.Run(mode, func(t *testing.T) {
			db, dir := workDB(t), workRepo(t)
			p, err := CreateProject(db, CreateProjectParams{Name: "feature"})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, ".easy-cli"), 0o700); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "conflict", "missing-project":
				if err := SetCurrentProject(dir, "missing"); err != nil {
					t.Fatal(err)
				}
			case "damaged":
				if err := os.Mkdir(filepath.Join(dir, ".easy-cli", "context"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "invalid-task":
				if err := SetCurrentProject(dir, "feature"); err != nil {
					t.Fatal(err)
				}
				task, err := CreateTask(db, CreateTaskParams{Branch: "main", Directory: dir, CommitRange: "main..HEAD"})
				if err != nil {
					t.Fatal(err)
				}
				if err := AttachTaskToProject(db, task.ID, p.ID); err != nil {
					t.Fatal(err)
				}
			}
			params := EnsureWorkParams{Directory: dir}
			if mode == "conflict" {
				params.ProjectName = "feature"
			}
			if _, err := EnsureWork(db, params); err == nil {
				t.Fatal("invalid legacy context/task silently repaired")
			}
			workspaces, err := ListWorkspacesByProject(db, p.ID)
			if err != nil || len(workspaces) != 0 {
				t.Fatalf("failed ensure persisted partial workspace: %+v %v", workspaces, err)
			}
		})
	}
}

func TestWorkDeletedTaskDoesNotInheritOlderTask(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	second, err := EnsureWork(db, EnsureWorkParams{Directory: dir, NewTask: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteTask(db, second.Task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("deleted task inherited older one: %v", err)
	}
	if s, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: first.Task.ID}); err != nil || s.Task.ID != first.Task.ID {
		t.Fatalf("explicit old task selection %+v %v", s, err)
	}
}
