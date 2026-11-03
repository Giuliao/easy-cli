package project

import (
	"errors"
	"sync"
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
	if err != nil || task.CommitRange != first.Git.Head+".."+head {
		t.Fatalf("metadata update lost Git progress: %+v %v", task, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); err != nil {
		t.Fatalf("metadata update corrupted recovery: %v", err)
	}
}

func TestEnsureWorkConcurrentIdempotency(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	if _, err := CreateProject(db, CreateProjectParams{Name: "feature"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := EnsureWork(db, EnsureWorkParams{Directory: dir, ProjectName: "feature"})
			if err != nil {
				failures <- err
				return
			}
			ids <- s.Task.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("duplicate tasks: %s %s", first, id)
		}
	}
	tasks, err := ListTasks(db)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks = %+v %v", tasks, err)
	}
	ctx, err := ReadWorkContext(dir)
	if err != nil || ctx.TaskID != first {
		t.Fatalf("context = %+v %v", ctx, err)
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
	if _, err := CreateTaskInProject(db, CreateTaskParams{}, "missing"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("error = %v", err)
	}
}
