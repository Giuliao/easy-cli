package project

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	offline, err := WorkHistoryByTask(db, first.Task.ID)
	if err != nil || len(offline) != 1 {
		t.Fatalf("offline history = %+v %v", offline, err)
	}
	// A later ensure must still reuse the task and keep the original base.
	third, err := EnsureWork(db, EnsureWorkParams{Directory: dir})
	if err != nil || third.Task.ID != first.Task.ID || third.Task.CommitRange != checkpoint.Task.CommitRange {
		t.Fatalf("third = %+v %v", third, err)
	}
	p, err := GetProjectByName(db, "feature")
	if err != nil || p.Status != StatusActive {
		t.Fatalf("project unexpectedly ended: %+v %v", p, err)
	}
}

func TestEnsureWorkSelectionAndAmbiguity(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	other, err := CreateTaskInProject(db, CreateTaskParams{Branch: "main", Directory: dir}, first.Project.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The stored task pointer wins while it is valid.
	selected, err := EnsureWork(db, EnsureWorkParams{Directory: dir})
	if err != nil || selected.Created || selected.Task.ID != first.Task.ID {
		t.Fatalf("stored pointer = %+v %v", selected, err)
	}
	// Without a pointer, two matching tasks require explicit selection.
	if err := WriteWorkContext(dir, WorkContext{ProjectID: first.Project.ID, ProjectName: "feature"}); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) || !strings.Contains(err.Error(), "multiple tasks") {
		t.Fatalf("ambiguity error = %v", err)
	}
	picked, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: other.ID})
	if err != nil || picked.Task.ID != other.ID || picked.Created {
		t.Fatalf("selection = %+v %v", picked, err)
	}
	ctx, err := ReadWorkContext(dir)
	if err != nil || ctx.TaskID != other.ID {
		t.Fatalf("context = %+v %v", ctx, err)
	}
	// Selecting a task from another project is drift, not a silent rebind.
	second, err := CreateProject(db, CreateProjectParams{Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := CreateTaskInProject(db, CreateTaskParams{Branch: "main", Directory: dir}, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: foreign.ID}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("foreign task accepted: %v", err)
	}
}

func TestEnsureWorkLegacyContextUpgradesAndNormalizes(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	p, err := CreateProject(db, CreateProjectParams{Name: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	// Legacy plain-name context, plus a task recorded with an abbreviated range.
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
	ctx, err := ReadWorkContext(dir)
	if err != nil || ctx.ProjectID != p.ID || ctx.ProjectName != p.Name || ctx.TaskID != task.ID {
		t.Fatalf("upgraded context = %+v %v", ctx, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".easy-cli", contextFileName))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil || decoded["task_id"] != task.ID {
		t.Fatalf("context JSON = %s %v", data, err)
	}
	// Re-running keeps the same pointer and does not rewrite Git facts.
	again, err := EnsureWork(db, EnsureWorkParams{Directory: dir})
	if err != nil || again.Created || again.Task.ID != task.ID {
		t.Fatalf("rerun = %+v %v", again, err)
	}
}

func TestWorkNoBindingAndConflictDoNotCreateRecords(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkNotBound) {
		t.Fatalf("missing binding error = %v", err)
	}
	if _, err := CurrentWork(db, dir); !errors.Is(err, ErrWorkNotBound) {
		t.Fatalf("status without binding = %v", err)
	}
	first := mustWork(t, db, dir)
	if _, err := CreateProject(db, CreateProjectParams{Name: "other"}); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir, ProjectName: "other"}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("ensure conflict = %v", err)
	}
	if _, err := BindProject(db, dir, "other", false); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("bind conflict = %v", err)
	}
	tasks, _ := ListTasks(db)
	if len(tasks) != 1 {
		t.Fatalf("failed resume created tasks: %+v", tasks)
	}
	replaced, err := BindProject(db, dir, "other", true)
	if err != nil || replaced.Context.ProjectID == first.Project.ID || replaced.Context.TaskID != "" {
		t.Fatalf("replace = %+v %v", replaced, err)
	}
	// Replacement clears the pointer and the new project gets its own record;
	// the previous task is untouched and still readable by ID.
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir, TaskID: first.Task.ID}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("old project's task inherited after replacement: %v", err)
	}
	replacedWork, err := EnsureWork(db, EnsureWorkParams{Directory: dir})
	if err != nil || !replacedWork.Created || replacedWork.Task.ID == first.Task.ID || replacedWork.Project.Name != "other" {
		t.Fatalf("replacement work = %+v %v", replacedWork, err)
	}
	if history, err := WorkHistory(db, dir); err != nil || len(history) != 0 {
		t.Fatalf("old project's history transferred: %+v %v", history, err)
	}
	if history, err := WorkHistoryByTask(db, first.Task.ID); err != nil || len(history) != 0 {
		t.Fatalf("old task lost: %+v %v", history, err)
	}
	if task, err := GetTask(db, first.Task.ID); err != nil || task.ID != first.Task.ID {
		t.Fatalf("old task removed: %+v %v", task, err)
	}
}

func TestWorkHistoryDriftRefusesOverwrite(t *testing.T) {
	for _, drift := range []string{"reset", "branch", "range", "directory"} {
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
			case "directory":
				moved := filepath.Join(t.TempDir(), "moved")
				workGit(t, dir, "worktree", "add", "-b", "moved", moved)
				if err := WriteWorkContext(moved, WorkContext{ProjectID: first.Project.ID, ProjectName: "feature", TaskID: first.Task.ID}); err != nil {
					t.Fatal(err)
				}
				dir = moved
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
			if drift != "directory" {
				history, err := WorkHistory(db, dir)
				if err != nil || len(history) != 1 || history[0].HeadCommit != checkpoint.Git.Head {
					t.Fatalf("history overwritten: %+v %v", history, err)
				}
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
	if _, err := BindProject(db, dir, "feature", false); !errors.Is(err, ErrProjectEnded) {
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
	if err != nil || len(s.Issues) == 0 {
		t.Fatalf("detached task invisible: %+v %v", s, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("detached task silently reused: %v", err)
	}
	if err := DeleteTask(db, first.Task.ID); err != nil {
		t.Fatal(err)
	}
	s, err = CurrentWork(db, dir)
	if err != nil || len(s.Issues) == 0 || !strings.Contains(strings.Join(s.Issues, " "), "no longer exists") {
		t.Fatalf("deleted task pointer = %+v %v", s, err)
	}
	if _, err := EnsureWork(db, EnsureWorkParams{Directory: dir}); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("stale pointer repaired silently: %v", err)
	}
	// Clearing the pointer lets the next ensure create the repository record.
	if _, err := BindProject(db, dir, "feature", true); err != nil {
		t.Fatal(err)
	}
	fresh, err := EnsureWork(db, EnsureWorkParams{Directory: dir})
	if err != nil || !fresh.Created || fresh.Task.ID == first.Task.ID {
		t.Fatalf("fresh task = %+v %v", fresh, err)
	}
	if err := DeleteProject(db, "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := CurrentWork(db, dir); !errors.Is(err, ErrWorkConflict) {
		t.Fatalf("deleted project left a usable binding: %v", err)
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
	if err != nil || !second.Created || second.Task.ID == first.Task.ID || second.Git.GitCommonDir != first.Git.GitCommonDir {
		t.Fatalf("linked = %+v %v", second, err)
	}
	if second.Git.Branch != "linked" || second.Task.Branch != "linked" {
		t.Fatalf("linked branch = %+v", second)
	}
	// A subdirectory and a symlink resolve to the same worktree binding.
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(nested, alias); err != nil {
		t.Fatal(err)
	}
	for _, probe := range []string{nested, alias} {
		third, err := EnsureWork(db, EnsureWorkParams{Directory: probe})
		if err != nil || third.Created || third.Task.ID != first.Task.ID {
			t.Fatalf("alias %q = %+v %v", probe, third, err)
		}
	}
	// The linked worktree keeps its own context file.
	if ctx, err := ReadWorkContext(linked); err != nil || ctx.TaskID != second.Task.ID {
		t.Fatalf("linked context = %+v %v", ctx, err)
	}
	if ctx, err := ReadWorkContext(dir); err != nil || ctx.TaskID != first.Task.ID {
		t.Fatalf("main context = %+v %v", ctx, err)
	}
}

func TestWorkContextErrorsAreNotAbsence(t *testing.T) {
	for _, mode := range []string{"conflict", "damaged", "missing-project", "invalid-task", "trailing-data"} {
		t.Run(mode, func(t *testing.T) {
			db, dir := workDB(t), workRepo(t)
			p, err := CreateProject(db, CreateProjectParams{Name: "feature"})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, ".easy-cli"), 0o700); err != nil {
				t.Fatal(err)
			}
			contextFile := filepath.Join(dir, ".easy-cli", contextFileName)
			switch mode {
			case "conflict", "missing-project":
				if err := SetCurrentProject(dir, "missing"); err != nil {
					t.Fatal(err)
				}
			case "damaged":
				if err := os.Mkdir(contextFile, 0o700); err != nil {
					t.Fatal(err)
				}
			case "invalid-task":
				if err := WriteWorkContext(dir, WorkContext{ProjectID: p.ID, ProjectName: p.Name, TaskID: "does-not-exist"}); err != nil {
					t.Fatal(err)
				}
			case "trailing-data":
				if err := os.WriteFile(contextFile, []byte(`{"project_id":"`+p.ID+`"} {"extra":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			params := EnsureWorkParams{Directory: dir}
			if mode == "conflict" {
				params.ProjectName = "feature"
			}
			if _, err := EnsureWork(db, params); err == nil {
				t.Fatal("invalid context silently repaired")
			}
			tasks, err := ListTasksByProject(db, p.ID)
			if err != nil || len(tasks) != 0 {
				t.Fatalf("failed ensure persisted a task: %+v %v", tasks, err)
			}
			if _, err := ReadWorkContext(dir); mode != "damaged" && mode != "trailing-data" && err != nil {
				t.Fatalf("context unreadable: %v", err)
			}
		})
	}
}

func TestWorkContextWriteIsAtomicAndPreservesBytes(t *testing.T) {
	db, dir := workDB(t), workRepo(t)
	first := mustWork(t, db, dir)
	before, err := os.ReadFile(filepath.Join(dir, ".easy-cli", contextFileName))
	if err != nil {
		t.Fatal(err)
	}
	// Rewriting the same binding is a no-op.
	if err := WriteWorkContext(dir, first.Context); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, ".easy-cli", contextFileName))
	if err != nil || string(after) != string(before) {
		t.Fatalf("rewrite changed context: %q %v", after, err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".easy-cli"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary context file leaked: %v", entries)
	}
	// A damaged context is never overwritten.
	if err := os.WriteFile(filepath.Join(dir, ".easy-cli", contextFileName), []byte("{\"project_id\":\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteWorkContext(dir, WorkContext{ProjectID: first.Project.ID, ProjectName: "feature"}); err == nil {
		t.Fatal("damaged context overwritten")
	}
	if data, err := os.ReadFile(filepath.Join(dir, ".easy-cli", contextFileName)); err != nil || string(data) != "{\"project_id\":\n" {
		t.Fatalf("damaged context changed: %q %v", data, err)
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

func TestWorkMigrationCopiesEarlierPRCheckpoints(t *testing.T) {
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
		INSERT INTO tasks VALUES ('kept', 'codex', '', 'main', '/tmp/repo', 'aaa..bbb', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		CREATE TABLE workspaces (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, repository TEXT NOT NULL, git_common_dir TEXT NOT NULL, directory TEXT NOT NULL, branch TEXT NOT NULL, active INTEGER NOT NULL, current_task_id TEXT, task_selection_required INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL);
		INSERT INTO workspaces VALUES ('ws', 'p', '/tmp/repo', '/tmp/repo/.git', '/tmp/repo', 'main', 1, 'kept', 0, '2026-01-01T00:00:00Z');
		CREATE TABLE work_checkpoints (id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id TEXT NOT NULL, task_id TEXT NOT NULL, branch TEXT NOT NULL, base_commit TEXT NOT NULL, head_commit TEXT NOT NULL, dirty INTEGER NOT NULL, created_at TEXT NOT NULL);
		INSERT INTO work_checkpoints (workspace_id, task_id, branch, base_commit, head_commit, dirty, created_at) VALUES ('ws', 'kept', 'main', 'aaa', 'bbb', 1, '2026-01-01T00:00:00Z');
		INSERT INTO work_checkpoints (workspace_id, task_id, branch, base_commit, head_commit, dirty, created_at) VALUES ('ws', 'gone', 'main', 'aaa', 'bbb', 0, '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	history, err := WorkHistoryByTask(db, "kept")
	if err != nil || len(history) != 1 || history[0].HeadCommit != "bbb" || !history[0].Dirty {
		t.Fatalf("migrated history = %+v %v", history, err)
	}
	for _, table := range []string{"workspaces", "work_checkpoints"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s not removed: %d %v", table, count, err)
		}
	}
	if err := migrate(db); err != nil {
		t.Fatal("migration not idempotent", err)
	}
	if history, err := WorkHistoryByTask(db, "kept"); err != nil || len(history) != 1 {
		t.Fatalf("rerun duplicated history: %+v %v", history, err)
	}
}
