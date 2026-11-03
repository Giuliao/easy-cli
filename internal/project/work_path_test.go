package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkLegacyTaskSubdirectoryAndSymlinkResume(t *testing.T) {
	for _, alias := range []bool{false, true} {
		t.Run(map[bool]string{false: "subdir", true: "symlink"}[alias], func(t *testing.T) {
			db, dir := workDB(t), workRepo(t)
			p, err := CreateProject(db, CreateProjectParams{Name: "feature"})
			if err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(dir, "nested")
			if err := os.Mkdir(nested, 0o700); err != nil {
				t.Fatal(err)
			}
			if alias {
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(nested, link); err != nil {
					t.Fatal(err)
				}
				nested = link
			}
			task, err := CreateTask(db, CreateTaskParams{Directory: nested, Branch: "main"})
			if err != nil {
				t.Fatal(err)
			}
			if err := AttachTaskToProject(db, task.ID, p.ID); err != nil {
				t.Fatal(err)
			}
			s, err := EnsureWork(db, EnsureWorkParams{Directory: nested, ProjectName: "feature"})
			if err != nil || s.Created || s.Task.ID != task.ID {
				t.Fatalf("legacy subdir resume = %+v %v", s, err)
			}
		})
	}
}

func TestWorkLegacyReplacementRequiresExplicitSelection(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	if _, err := CreateProject(db, CreateProjectParams{Name: "feature"}); err != nil {
		t.Fatal(err)
	}
	if err := SetCurrentProject(dir, "other"); err != nil {
		t.Fatal(err)
	}
	w, err := BindWorkspace(db, dir, "feature", true)
	if err != nil || !w.TaskSelectionRequired {
		t.Fatalf("legacy replace = %+v %v", w, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("legacy replacement auto-created task: %v", err)
	}
}
