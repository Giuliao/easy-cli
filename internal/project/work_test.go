package project

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func workGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func workRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	workGit(t, dir, "init", "-b", "main")
	workGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "base")
	return dir
}

func workDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustWork(t *testing.T, db *sql.DB, dir string) WorkState {
	t.Helper()
	if _, err := CreateProject(db, CreateProjectParams{Name: "feature"}); err != nil {
		t.Fatal(err)
	}
	s, err := EnsureWork(db, EnsureWorkParams{Directory: dir, ProjectName: "feature", AgentType: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEnsureWorkAcrossSessionsPreservesTaskAndBase(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	base := first.Git.Head
	if !first.Created || first.Task.SessionID != "" || first.Task.CommitRange != base+".."+base {
		t.Fatalf("first = %+v task %+v", first, first.Task)
	}
	workGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "next")
	second, err := EnsureWork(db, EnsureWorkParams{Directory: dir, AgentType: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.Task.ID != first.Task.ID || second.Task.AgentType != "codex" || second.Task.CommitRange != first.Task.CommitRange {
		t.Fatalf("second task = %+v, first %+v", second.Task, first.Task)
	}
	if err := os.WriteFile(filepath.Join(dir, "pending.txt"), []byte("not committed"), 0o600); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := CheckpointWork(db, dir)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Task.CommitRange != base+".."+second.Git.Head || !checkpoint.Checkpoint.Dirty {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
	if head := workGit(t, dir, "rev-parse", "HEAD"); head != second.Git.Head {
		t.Fatal("checkpoint changed HEAD")
	}
	history, err := WorkHistory(db, dir)
	if err != nil || len(history) != 1 || history[0].BaseCommit != base {
		t.Fatalf("history = %+v, %v", history, err)
	}
	p, err := GetProjectByName(db, "feature")
	if err != nil || p.Status != StatusActive {
		t.Fatalf("project unexpectedly ended: %+v %v", p, err)
	}
}

func TestEnsureWorkExplicitNewTaskAndSelection(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	workGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "new segment")
	second, err := EnsureWork(db, EnsureWorkParams{Directory: dir, NewTask: true})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Created || second.Task.ID == first.Task.ID || second.Task.CommitRange != second.Git.Head+".."+second.Git.Head || second.Workspace.ID != first.Workspace.ID {
		t.Fatalf("second = %+v", second)
	}
	selected, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: first.Task.ID})
	if err != nil || selected.Task.ID != first.Task.ID {
		t.Fatalf("select = %+v %v", selected, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: first.Task.ID, NewTask: true}); err == nil {
		t.Fatal("mutually exclusive options accepted")
	}
}

func TestEnsureWorkLegacyContextAndCandidateAmbiguity(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	p, err := CreateProject(db, CreateProjectParams{Name: "legacy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCurrentProject(dir, p.Name); err != nil {
		t.Fatal(err)
	}
	git, err := ReadGitState(dir)
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTask(db, CreateTaskParams{Branch: "main", Directory: dir, SessionID: "old-session", CommitRange: git.Head[:8] + ".." + git.Head[:8]})
	if err != nil {
		t.Fatal(err)
	}
	if err := AttachTaskToProject(db, task.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := EnsureWork(db, EnsureWorkParams{Directory: dir})
	if err != nil || got.Created || got.Task.ID != task.ID || got.Task.SessionID != "old-session" || got.Task.CommitRange != git.Head+".."+git.Head {
		t.Fatalf("legacy = %+v %v", got, err)
	}
	other, err := CreateTask(db, CreateTaskParams{Branch: "main", Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := AttachTaskToProject(db, other.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE workspaces SET current_task_id = NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) || !strings.Contains(err.Error(), "multiple tasks") {
		t.Fatalf("ambiguity error = %v", err)
	}
	got, err = EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: other.ID})
	if err != nil || got.Task.ID != other.ID {
		t.Fatalf("selection = %+v %v", got, err)
	}
}

func TestWorkNoBindingAndConflictDoNotCreateRecords(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkNotBound) {
		t.Fatalf("missing binding error = %v", err)
	}
	first := mustWork(t, db, dir)
	if _, err := CreateProject(db, CreateProjectParams{Name: "other"}); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir, ProjectName: "other"}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("ensure conflict = %v", err)
	}
	if _, err := BindWorkspace(db, dir, "other", false); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("bind conflict = %v", err)
	}
	tasks, _ := ListTasks(db)
	if len(tasks) != 1 {
		t.Fatalf("failed resume created tasks: %+v", tasks)
	}
	w, err := BindWorkspace(db, dir, "other", true)
	if err != nil || w.CurrentTaskID != "" || w.ProjectID == first.Project.ID {
		t.Fatalf("replace = %+v %v", w, err)
	}
}

func TestWorkHistoryDriftRefusesOverwrite(t *testing.T) {
	for _, drift := range []string{"reset", "branch", "range"} {
		t.Run(drift, func(t *testing.T) {
			db, dir := workDB(t), workRepo(t)
			first := mustWork(t, db, dir)
			workGit(t, dir, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "next")
			checkpoint, err := CheckpointWork(db, dir)
			if err != nil {
				t.Fatal(err)
			}
			switch drift {
			case "reset":
				workGit(t, dir, "reset", "--hard", first.Git.Head)
			case "branch":
				workGit(t, dir, "switch", "-c", "different")
			case "range":
				rangeValue := first.Git.Head + ".." + first.Git.Head
				if _, err := UpdateTask(db, first.Task.ID, UpdateTaskParams{CommitRange: &rangeValue}); err != nil {
					t.Fatal(err)
				}
			}
			s, err := CurrentWork(db, dir)
			if err != nil || len(s.Issues) == 0 {
				t.Fatalf("drift invisible: %+v %v", s, err)
			}
			if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); err == nil {
				t.Fatal("ensure accepted drift")
			}
			if _, err := CheckpointWork(db, dir); err == nil {
				t.Fatal("checkpoint accepted drift")
			}
			history, err := WorkHistory(db, dir)
			if err != nil || len(history) != 1 || history[0].HeadCommit != checkpoint.Git.Head {
				t.Fatalf("history overwritten: %+v %v", history, err)
			}
		})
	}
}

func TestWorkTaskAssociationAndProjectLifecycle(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	if _, err := CloseProject(db, "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrProjectEnded) {
		t.Fatalf("ended ensure = %v", err)
	}
	if _, err := BindWorkspace(db, dir, "feature", false); !errors.Is(err, ErrProjectEnded) {
		t.Fatalf("ended bind = %v", err)
	}
	if _, err := CheckpointWork(db, dir); err == nil {
		t.Fatal("ended project checkpoint accepted")
	}
	p, err := ReopenProject(db, "feature")
	if err != nil || p.Status != StatusActive || p.EndedAt != nil {
		t.Fatalf("reopen = %+v %v", p, err)
	}
	if err := DetachTaskFromProject(db, first.Task.ID, first.Project.ID); err != nil {
		t.Fatal(err)
	}
	s, err := CurrentWork(db, dir)
	if err != nil || s.Task != nil {
		t.Fatalf("detached task still current: %+v %v", s, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: first.Task.ID}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("foreign task accepted: %v", err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("removed task silently inherited: %v", err)
	}
	second, err := EnsureWork(db, EnsureWorkParams{Directory: dir, NewTask: true})
	if err != nil {
		t.Fatal(err)
	}
	branch := "other"
	if _, err := UpdateTask(db, second.Task.ID, UpdateTaskParams{Branch: &branch}); err == nil {
		t.Fatal("bound task branch update accepted")
	}
	if err := DeleteTask(db, second.Task.ID); err != nil {
		t.Fatal(err)
	}
	s, err = CurrentWork(db, dir)
	if err != nil || s.Task != nil {
		t.Fatalf("deleted task still current: %+v %v", s, err)
	}
	if err := DeleteProject(db, "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := CurrentWork(db, dir); !errors.Is(err, ErrWorkNotBound) {
		t.Fatalf("project deletion left binding: %v", err)
	}
}

func TestWorkLinkedWorktreeIsolationAndCanonicalPaths(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	linked := filepath.Join(t.TempDir(), "linked")
	workGit(t, dir, "worktree", "add", "-b", "linked", linked)
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: linked}); !errors.Is(err, ErrWorkNotBound) {
		t.Fatalf("linked inherited binding: %v", err)
	}
	second, err := EnsureWork(db, EnsureWorkParams{Directory: linked, ProjectName: "feature"})
	if err != nil || second.Workspace.ID == first.Workspace.ID || second.Workspace.GitCommonDir != first.Workspace.GitCommonDir {
		t.Fatalf("linked = %+v %v", second, err)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(nested, alias); err != nil {
		t.Fatal(err)
	}
	third, err := EnsureWork(db, EnsureWorkParams{Directory: alias})
	if err != nil || third.Task.ID != first.Task.ID {
		t.Fatalf("alias = %+v %v", third, err)
	}
}

func TestEnsureWorkConcurrentIdempotency(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	if _, err := CreateProject(db, CreateProjectParams{Name: "feature"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errors := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := EnsureWork(db, EnsureWorkParams{Directory: dir, ProjectName: "feature"})
			if err != nil {
				errors <- err
				return
			}
			ids <- s.Task.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errors)
	for err := range errors {
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
}

func TestWorkMigrationPreservesOldData(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "easy-cli")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(path, dbFileName))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, agent_type TEXT NOT NULL, session_id TEXT NOT NULL, branch TEXT NOT NULL, directory TEXT NOT NULL, commit_range TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO tasks VALUES ('old', 'codex', 'session-before-upgrade', 'main', '/tmp/repo', 'abc..def', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old, err := GetTask(db, "old")
	if err != nil || old.SessionID != "session-before-upgrade" {
		t.Fatalf("old task = %+v %v", old, err)
	}
	if err := migrate(db); err != nil {
		t.Fatal("migration not idempotent", err)
	}
}
