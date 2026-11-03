package project

import (
	"testing"
)

func TestWorkMetadataUpdateCannotLoseConcurrentCheckpoint(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	workGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "progress")
	head := workGit(t, dir, "rev-parse", "HEAD")
	start := make(chan struct{})
	result := make(chan error, 2)
	go func() {
		<-start
		_, err := CheckpointWork(db, dir)
		result <- err
	}()
	go func() {
		<-start
		for range 12 {
			agent := "claude"
			if _, err := UpdateTask(db, first.Task.ID, UpdateTaskParams{AgentType: &agent}); err != nil {
				result <- err
				return
			}
		}
		result <- nil
	}()
	close(start)
	for range 2 {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	task, err := GetTask(db, first.Task.ID)
	if err != nil || task.CommitRange != first.Git.Head+".."+head || task.AgentType != "claude" {
		t.Fatalf("metadata update lost Git progress: %+v %v", task, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); err != nil {
		t.Fatalf("metadata update corrupted recovery: %v", err)
	}
}

func TestCreateTaskInProjectAtomicValidation(t *testing.T) {
	db := workDB(t)
	if _, err := CreateTaskInProject(db, CreateTaskParams{}, "missing"); err == nil {
		t.Fatal("missing project accepted")
	}
	tasks, err := ListTasks(db)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("failed attach orphaned task: %+v %v", tasks, err)
	}
}
