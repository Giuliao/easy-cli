package project

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCreateAndGetProject(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	p, err := CreateProject(db, CreateProjectParams{Name: "test", Description: "desc"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Error("expected non-empty project ID")
	}
	if p.Name != "test" {
		t.Errorf("Name = %q, want %q", p.Name, "test")
	}
	if p.Status != StatusActive {
		t.Errorf("Status = %q, want %q", p.Status, StatusActive)
	}
	if p.EndedAt != nil {
		t.Error("expected EndedAt to be nil for active project")
	}

	got, err := GetProjectByName(db, "test")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("ID = %q, want %q", got.ID, p.ID)
	}
}

func TestCreateProjectDuplicateName(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := CreateProject(db, CreateProjectParams{Name: "dup"}); err != nil {
		t.Fatalf("first CreateProject: %v", err)
	}
	_, err := CreateProject(db, CreateProjectParams{Name: "dup"})
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestCloseProject(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := CreateProject(db, CreateProjectParams{Name: "close"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	p, err := CloseProject(db, "close")
	if err != nil {
		t.Fatalf("CloseProject: %v", err)
	}
	if p.Status != StatusEnded {
		t.Errorf("Status = %q, want %q", p.Status, StatusEnded)
	}
	if p.EndedAt == nil {
		t.Error("expected EndedAt to be set after close")
	}
}

func TestDeleteProject(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := CreateProject(db, CreateProjectParams{Name: "del"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := DeleteProject(db, "del"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	_, err := GetProjectByName(db, "del")
	if err != ErrProjectNotFound {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestProjectNotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, err := GetProjectByName(db, "nope")
	if err != ErrProjectNotFound {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestCreateAndGetTask(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	task, err := CreateTask(db, CreateTaskParams{
		AgentType:   "claude",
		SessionID:   "sess-1",
		Branch:      "main",
		Directory:   "/tmp/repo",
		CommitRange: "abc..def",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ID == "" {
		t.Error("expected non-empty task ID")
	}

	got, err := GetTask(db, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.AgentType != "claude" {
		t.Errorf("AgentType = %q, want %q", got.AgentType, "claude")
	}
}

func TestUpdateTask(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	task, err := CreateTask(db, CreateTaskParams{
		AgentType: "claude",
		SessionID: "sess-1",
		Branch:    "main",
		Directory: "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	newRange := "new..range"
	updated, err := UpdateTask(db, task.ID, UpdateTaskParams{CommitRange: &newRange})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.CommitRange != "new..range" {
		t.Errorf("CommitRange = %q, want %q", updated.CommitRange, "new..range")
	}
	if !updated.UpdatedAt.After(task.UpdatedAt) {
		t.Error("expected UpdatedAt to advance")
	}
}

func TestAttachAndDetachTask(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	p, err := CreateProject(db, CreateProjectParams{Name: "proj"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task, err := CreateTask(db, CreateTaskParams{
		AgentType: "claude",
		SessionID: "sess-1",
		Branch:    "main",
		Directory: "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := AttachTaskToProject(db, task.ID, p.ID); err != nil {
		t.Fatalf("AttachTaskToProject: %v", err)
	}

	tasks, err := ListTasksByProject(db, p.ID)
	if err != nil {
		t.Fatalf("ListTasksByProject: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}

	if err := AttachTaskToProject(db, task.ID, p.ID); err == nil {
		t.Error("expected error when attaching already-attached task")
	}

	if err := DetachTaskFromProject(db, task.ID, p.ID); err != nil {
		t.Fatalf("DetachTaskFromProject: %v", err)
	}

	tasks, err = ListTasksByProject(db, p.ID)
	if err != nil {
		t.Fatalf("ListTasksByProject: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("len(tasks) = %d, want 0 after detach", len(tasks))
	}
}

func TestDeleteProjectKeepsTasks(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	p, err := CreateProject(db, CreateProjectParams{Name: "proj"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task, err := CreateTask(db, CreateTaskParams{
		AgentType: "claude",
		SessionID: "sess-1",
		Branch:    "main",
		Directory: "/tmp/repo",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := AttachTaskToProject(db, task.ID, p.ID); err != nil {
		t.Fatalf("AttachTaskToProject: %v", err)
	}

	if err := DeleteProject(db, "proj"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if _, err := GetTask(db, task.ID); err != nil {
		t.Errorf("task should still exist after project deletion: %v", err)
	}
}
