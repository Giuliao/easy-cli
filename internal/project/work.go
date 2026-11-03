package project

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrWorkNotBound = errors.New("worktree is not bound; select a project with easy project use <name> or easy work ensure --project <name>")
	ErrWorkConflict = errors.New("workspace conflict")
	ErrProjectEnded = errors.New("project is ended; ask the user whether to reopen it or start a different project")
)

// Workspace is a persistent development location, independent of agent sessions.
type Workspace struct {
	ID                    string    `json:"id"`
	Active                bool      `json:"active"`
	TaskSelectionRequired bool      `json:"task_selection_required"`
	ProjectID             string    `json:"project_id"`
	Directory             string    `json:"directory"`
	GitCommonDir          string    `json:"git_common_dir"`
	Repository            string    `json:"repository"`
	Branch                string    `json:"branch"`
	CurrentTaskID         string    `json:"current_task_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type Checkpoint struct {
	ID          int64     `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	TaskID      string    `json:"task_id"`
	Branch      string    `json:"branch"`
	BaseCommit  string    `json:"base_commit"`
	HeadCommit  string    `json:"head_commit"`
	Dirty       bool      `json:"dirty"`
	CreatedAt   time.Time `json:"created_at"`
}

type WorkState struct {
	Project    Project     `json:"project"`
	Workspace  Workspace   `json:"workspace"`
	Task       *Task       `json:"task,omitempty"`
	Git        GitState    `json:"git"`
	Issues     []string    `json:"issues"`
	Created    bool        `json:"created"`
	Checkpoint *Checkpoint `json:"checkpoint,omitempty"`
}

type EnsureWorkParams struct {
	Directory   string
	ProjectName string
	AgentType   string
	TaskID      string
	NewTask     bool
}

type workQuery interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

const workspaceColumns = `id, project_id, directory, active, task_selection_required, git_common_dir, repository, branch, current_task_id, created_at, updated_at`

func scanWorkspace(s projectScanner) (Workspace, error) {
	var w Workspace
	var task sql.NullString
	var created, updated string
	if err := s.Scan(&w.ID, &w.ProjectID, &w.Directory, &w.Active, &w.TaskSelectionRequired, &w.GitCommonDir, &w.Repository, &w.Branch, &task, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return w, ErrWorkNotBound
		}
		return w, fmt.Errorf("scan workspace: %w", err)
	}
	w.CurrentTaskID = task.String
	var err error
	if w.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return w, err
	}
	w.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return w, err
}

func workspaceAt(q workQuery, directory string) (Workspace, error) {
	return scanWorkspace(q.QueryRow(`SELECT `+workspaceColumns+` FROM workspaces WHERE directory = ? AND active = 1`, directory))
}

func ListWorkspacesByProject(db *sql.DB, projectID string) ([]Workspace, error) {
	rows, err := db.Query(`SELECT `+workspaceColumns+` FROM workspaces WHERE project_id = ? ORDER BY directory`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Workspace{}
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func projectByName(q workQuery, name string) (Project, error) {
	return scanProject(q.QueryRow(`SELECT id, name, description, created_at, status, ended_at FROM projects WHERE name = ?`, name))
}

func projectByID(q workQuery, id string) (Project, error) {
	return scanProject(q.QueryRow(`SELECT id, name, description, created_at, status, ended_at FROM projects WHERE id = ?`, id))
}

func workTask(q workQuery, id string) (Task, error) {
	return scanTask(q.QueryRow(`SELECT id, agent_type, session_id, branch, directory, commit_range, created_at, updated_at FROM tasks WHERE id = ?`, id))
}

func taskProjectID(q workQuery, id string) (string, error) {
	var projectID string
	err := q.QueryRow(`SELECT project_id FROM project_tasks WHERE task_id = ?`, id).Scan(&projectID)
	return projectID, err
}

// legacyContext is only consulted before first binding. A damaged context is not absence.
func legacyContext(directory string) (string, error) {
	data, err := os.ReadFile(filepath.Join(directory, ".easy-cli", contextFileName))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read legacy project context: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func insertWorkspace(q workQuery, p Project, git GitState) (Workspace, error) {
	now := time.Now().UTC()
	w := Workspace{ID: "wsp_" + NewProjectID(), Active: true, ProjectID: p.ID, Directory: git.Directory,
		GitCommonDir: git.GitCommonDir, Repository: git.Repository, Branch: git.Branch, CreatedAt: now, UpdatedAt: now}
	_, err := q.Exec(`INSERT INTO workspaces (`+workspaceColumns+`) VALUES (?, ?, ?, 1, 0, ?, ?, ?, NULL, ?, ?)`,
		w.ID, w.ProjectID, w.Directory, w.GitCommonDir, w.Repository, w.Branch, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return w, err
}

// BindWorkspace never changes Git. Replacement is explicit, and clears the current task.
func BindWorkspace(db *sql.DB, directory, name string, replace bool) (Workspace, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return Workspace{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback()
	p, err := projectByName(tx, name)
	if err != nil {
		return Workspace{}, err
	}
	if p.Status != StatusActive {
		return Workspace{}, ErrProjectEnded
	}
	w, err := workspaceAt(tx, git.Directory)
	if errors.Is(err, ErrWorkNotBound) {
		legacy, contextErr := legacyContext(git.Directory)
		if contextErr != nil {
			return Workspace{}, contextErr
		}
		if legacy != "" && legacy != name && !replace {
			return Workspace{}, fmt.Errorf("%w: legacy context is %q, requested %q; confirm replacement with --replace", ErrWorkConflict, legacy, name)
		}
		w, err = insertWorkspace(tx, p, git)
		if err == nil && replace {
			w.TaskSelectionRequired = true
			_, err = tx.Exec(`UPDATE workspaces SET task_selection_required = 1 WHERE id = ?`, w.ID)
		}
	} else if err == nil {
		changed := w.ProjectID != p.ID || w.Branch != git.Branch || w.GitCommonDir != git.GitCommonDir
		if changed && !replace {
			return Workspace{}, fmt.Errorf("%w: project, branch or repository differs; inspect easy work status and confirm replacement with --replace", ErrWorkConflict)
		}
		if changed {
			// Preserve old ownership and checkpoints instead of moving them to a new project.
			if _, err = tx.Exec(`UPDATE workspaces SET active = 0, updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), w.ID); err == nil {
				w, err = insertWorkspace(tx, p, git)
			}
		}
		if err == nil && replace {
			w.CurrentTaskID = ""
			w.TaskSelectionRequired = true
			w.UpdatedAt = time.Now().UTC()
			_, err = tx.Exec(`UPDATE workspaces SET current_task_id = NULL, task_selection_required = 1, updated_at = ? WHERE id = ?`, w.UpdatedAt.Format(time.RFC3339Nano), w.ID)
		}
	}
	if err != nil {
		return Workspace{}, err
	}
	if err := tx.Commit(); err != nil {
		return Workspace{}, err
	}
	if err := SetCurrentProject(git.Directory, p.Name); err != nil {
		return w, fmt.Errorf("workspace binding saved, but legacy context could not be written: %w", err)
	}
	return w, nil
}

func canonicalDirectory(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	canonical, err := canonicalGitDirectory(absolute)
	if err != nil {
		return "", err
	}
	root, err := runProjectGit(canonical, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return canonicalGitDirectory(gitOutputLine(root))
}

func checkTaskLocation(q workQuery, w Workspace, t Task) error {
	projectID, err := taskProjectID(q, t.ID)
	if err != nil || projectID != w.ProjectID {
		return fmt.Errorf("%w: task %s is not attached to this project", ErrWorkConflict, t.ID)
	}
	dir, err := canonicalDirectory(t.Directory)
	if err != nil || dir != w.Directory || t.Branch != w.Branch {
		return fmt.Errorf("%w: task %s directory or branch differs from workspace", ErrWorkConflict, t.ID)
	}
	return nil
}

func workState(q workQuery, w Workspace, git GitState) (WorkState, error) {
	p, err := projectByID(q, w.ProjectID)
	if err != nil {
		return WorkState{}, err
	}
	s := WorkState{Project: p, Workspace: w, Git: git, Issues: []string{}}
	if p.Status != StatusActive {
		s.Issues = append(s.Issues, ErrProjectEnded.Error())
	}
	if w.Directory != git.Directory || w.Branch != git.Branch || w.GitCommonDir != git.GitCommonDir {
		s.Issues = append(s.Issues, "workspace directory, branch or Git common directory differs from actual Git state")
	}
	if w.CurrentTaskID != "" {
		t, err := workTask(q, w.CurrentTaskID)
		if err != nil {
			return s, err
		}
		s.Task = &t
		if err := checkTaskLocation(q, w, t); err != nil {
			s.Issues = append(s.Issues, err.Error())
		} else if _, _, err := taskProgress(q, w, t, git); err != nil {
			s.Issues = append(s.Issues, err.Error())
		}
	}
	return s, nil
}

func CurrentWork(db *sql.DB, directory string) (WorkState, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return WorkState{}, err
	}
	w, err := workspaceAt(db, git.Directory)
	if err != nil {
		return WorkState{}, err
	}
	return workState(db, w, git)
}

// taskProgress preserves the original base and verifies both ancestry and saved history.
func taskProgress(q workQuery, w Workspace, t Task, git GitState) (string, string, error) {
	base, previous := git.Head, git.Head
	var err error
	if t.CommitRange != "" {
		base, previous, err = ValidateCommitRange(git.Directory, t.CommitRange)
		if err != nil {
			return "", "", fmt.Errorf("task %s commit range: %w", t.ID, err)
		}
	}
	var savedBase, savedHead string
	err = q.QueryRow(`SELECT base_commit, head_commit FROM work_task_progress WHERE task_id = ?`, t.ID).Scan(&savedBase, &savedHead)
	if errors.Is(err, sql.ErrNoRows) {
		err = q.QueryRow(`SELECT base_commit, head_commit FROM work_checkpoints WHERE task_id = ? ORDER BY id DESC LIMIT 1`, t.ID).Scan(&savedBase, &savedHead)
	}
	if err == nil && (savedBase != base || savedHead != previous) {
		return "", "", fmt.Errorf("%w: task range differs from saved progress; inspect history instead of overwriting it", ErrWorkConflict)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	if err := ValidateGitProgress(git.Directory, base, previous, git.Head); err != nil {
		return "", "", err
	}
	return base, previous, nil
}

func candidateTasks(q workQuery, w Workspace) ([]Task, error) {
	rows, err := q.Query(`SELECT t.id, t.agent_type, t.session_id, t.branch, t.directory, t.commit_range, t.created_at, t.updated_at
		FROM tasks t JOIN project_tasks pt ON pt.task_id = t.id WHERE pt.project_id = ? AND t.branch = ? ORDER BY t.id`, w.ProjectID, w.Branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		directory, err := canonicalDirectory(t.Directory)
		if err == nil && directory == w.Directory {
			tasks = append(tasks, t)
		}
	}
	return tasks, rows.Err()
}

func EnsureWork(db *sql.DB, params EnsureWorkParams) (WorkState, error) {
	if params.NewTask && params.TaskID != "" {
		return WorkState{}, fmt.Errorf("--task and --new-task are mutually exclusive")
	}
	git, err := ReadGitState(params.Directory)
	if err != nil {
		return WorkState{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return WorkState{}, err
	}
	defer tx.Rollback()
	w, err := workspaceAt(tx, git.Directory)
	if errors.Is(err, ErrWorkNotBound) {
		legacy, err := legacyContext(git.Directory)
		if err != nil {
			return WorkState{}, err
		}
		name := params.ProjectName
		if name == "" {
			name = legacy
		}
		if name == "" {
			return WorkState{}, ErrWorkNotBound
		}
		if legacy != "" && legacy != name {
			return WorkState{}, fmt.Errorf("%w: legacy context is %q, requested %q; use project use --replace after confirmation", ErrWorkConflict, legacy, name)
		}
		p, err := projectByName(tx, name)
		if err != nil {
			return WorkState{}, err
		}
		if p.Status != StatusActive {
			return WorkState{}, ErrProjectEnded
		}
		w, err = insertWorkspace(tx, p, git)
		if err != nil {
			return WorkState{}, err
		}
	} else if err != nil {
		return WorkState{}, err
	}
	p, err := projectByID(tx, w.ProjectID)
	if err != nil {
		return WorkState{}, err
	}
	if params.ProjectName != "" && params.ProjectName != p.Name {
		return WorkState{}, fmt.Errorf("%w: worktree belongs to %q, requested %q; choose the correct worktree or explicitly replace its binding", ErrWorkConflict, p.Name, params.ProjectName)
	}
	if p.Status != StatusActive {
		return WorkState{}, ErrProjectEnded
	}
	if w.Branch != git.Branch || w.GitCommonDir != git.GitCommonDir {
		return WorkState{}, fmt.Errorf("%w: actual Git branch or repository differs; restore the recorded workspace or explicitly rebind with project use --replace", ErrWorkConflict)
	}
	id := params.TaskID
	if id == "" && !params.NewTask {
		id = w.CurrentTaskID
	}
	if id == "" && !params.NewTask {
		if w.TaskSelectionRequired {
			return WorkState{}, fmt.Errorf("%w: current task was cleared explicitly or removed; choose --task <id> or --new-task instead of inheriting an older task", ErrWorkConflict)
		}
		candidates, err := candidateTasks(tx, w)
		if err != nil {
			return WorkState{}, err
		}
		if len(candidates) > 1 {
			ids := make([]string, len(candidates))
			for i, t := range candidates {
				ids[i] = t.ID
			}
			return WorkState{}, fmt.Errorf("%w: multiple tasks match: %s; choose --task <id> or explicitly --new-task", ErrWorkConflict, strings.Join(ids, ", "))
		}
		if len(candidates) == 1 {
			id = candidates[0].ID
		}
	}
	created := id == ""
	var t Task
	if created {
		now := time.Now().UTC()
		t = Task{ID: NewTaskID(), AgentType: params.AgentType, Branch: git.Branch, Directory: git.Directory,
			CommitRange: git.Head + ".." + git.Head, CreatedAt: now, UpdatedAt: now}
		_, err = tx.Exec(`INSERT INTO tasks (id, agent_type, session_id, branch, directory, commit_range, created_at, updated_at) VALUES (?, ?, '', ?, ?, ?, ?, ?)`,
			t.ID, t.AgentType, t.Branch, t.Directory, t.CommitRange, now.Format(time.RFC3339), now.Format(time.RFC3339))
		if err == nil {
			_, err = tx.Exec(`INSERT INTO project_tasks (project_id, task_id) VALUES (?, ?)`, p.ID, t.ID)
		}
		if err != nil {
			return WorkState{}, err
		}
	} else {
		t, err = workTask(tx, id)
		if err != nil {
			return WorkState{}, err
		}
		if err := checkTaskLocation(tx, w, t); err != nil {
			return WorkState{}, err
		}
		base, previous, err := taskProgress(tx, w, t, git)
		if err != nil {
			return WorkState{}, err
		}
		// Normalize legacy ranges once; never reset base or advance head on ensure.
		if t.CommitRange != base+".."+previous {
			t.CommitRange = base + ".." + previous
			t.UpdatedAt = time.Now().UTC()
			if _, err := tx.Exec(`UPDATE tasks SET commit_range = ?, updated_at = ? WHERE id = ?`, t.CommitRange, t.UpdatedAt.Format(time.RFC3339), t.ID); err != nil {
				return WorkState{}, err
			}
		}
	}
	base, previous, err := ValidateCommitRange(git.Directory, t.CommitRange)
	if err != nil {
		return WorkState{}, err
	}
	if _, err := tx.Exec(`INSERT INTO work_task_progress (task_id, base_commit, head_commit) VALUES (?, ?, ?) ON CONFLICT(task_id) DO NOTHING`, t.ID, base, previous); err != nil {
		return WorkState{}, err
	}
	if w.CurrentTaskID != t.ID {
		w.CurrentTaskID = t.ID
		w.TaskSelectionRequired = false
		w.UpdatedAt = time.Now().UTC()
		if _, err := tx.Exec(`UPDATE workspaces SET current_task_id = ?, task_selection_required = 0, updated_at = ? WHERE id = ?`, t.ID, w.UpdatedAt.Format(time.RFC3339Nano), w.ID); err != nil {
			return WorkState{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return WorkState{}, err
	}
	return WorkState{Project: p, Workspace: w, Task: &t, Git: git, Issues: []string{}, Created: created}, nil
}

func CheckpointWork(db *sql.DB, directory string) (WorkState, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return WorkState{}, err
	}
	tx, err := db.Begin()
	if err != nil {
		return WorkState{}, err
	}
	defer tx.Rollback()
	w, err := workspaceAt(tx, git.Directory)
	if err != nil {
		return WorkState{}, err
	}
	s, err := workState(tx, w, git)
	if err != nil {
		return s, err
	}
	if len(s.Issues) > 0 {
		return s, fmt.Errorf("%w: %s", ErrWorkConflict, strings.Join(s.Issues, "; "))
	}
	if s.Task == nil {
		return s, fmt.Errorf("no current task; run easy work ensure first")
	}
	base, _, err := taskProgress(tx, w, *s.Task, git)
	if err != nil {
		return s, err
	}
	cp := Checkpoint{WorkspaceID: w.ID, TaskID: s.Task.ID, Branch: git.Branch, BaseCommit: base, HeadCommit: git.Head, Dirty: git.Dirty, CreatedAt: time.Now().UTC()}
	result, err := tx.Exec(`INSERT INTO work_checkpoints (workspace_id, task_id, branch, base_commit, head_commit, dirty, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cp.WorkspaceID, cp.TaskID, cp.Branch, cp.BaseCommit, cp.HeadCommit, cp.Dirty, cp.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return s, err
	}
	cp.ID, err = result.LastInsertId()
	if err != nil {
		return s, err
	}
	if _, err := tx.Exec(`INSERT INTO work_task_progress (task_id, base_commit, head_commit) VALUES (?, ?, ?) ON CONFLICT(task_id) DO UPDATE SET head_commit = excluded.head_commit`, s.Task.ID, base, git.Head); err != nil {
		return s, err
	}
	s.Task.CommitRange = base + ".." + git.Head
	s.Task.UpdatedAt = cp.CreatedAt
	if _, err := tx.Exec(`UPDATE tasks SET commit_range = ?, updated_at = ? WHERE id = ?`, s.Task.CommitRange, cp.CreatedAt.Format(time.RFC3339), s.Task.ID); err != nil {
		return s, err
	}
	if _, err := tx.Exec(`UPDATE workspaces SET updated_at = ? WHERE id = ?`, cp.CreatedAt.Format(time.RFC3339Nano), w.ID); err != nil {
		return s, err
	}
	if err := tx.Commit(); err != nil {
		return s, err
	}
	s.Workspace.UpdatedAt = cp.CreatedAt
	s.Checkpoint = &cp
	return s, nil
}

func WorkHistory(db *sql.DB, directory string) ([]Checkpoint, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return nil, err
	}
	w, err := workspaceAt(db, git.Directory)
	if err != nil {
		return nil, err
	}
	return WorkHistoryByWorkspace(db, w.ID)
}

// WorkHistoryByWorkspace allows read-only access to archived records without an online worktree.
func WorkHistoryByWorkspace(db *sql.DB, workspaceID string) ([]Checkpoint, error) {
	if _, err := scanWorkspace(db.QueryRow(`SELECT `+workspaceColumns+` FROM workspaces WHERE id = ?`, workspaceID)); err != nil {
		return nil, fmt.Errorf("workspace %q: %w", workspaceID, err)
	}
	rows, err := db.Query(`SELECT id, workspace_id, task_id, branch, base_commit, head_commit, dirty, created_at FROM work_checkpoints WHERE workspace_id = ? ORDER BY id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Checkpoint{}
	for rows.Next() {
		var cp Checkpoint
		var created string
		if err := rows.Scan(&cp.ID, &cp.WorkspaceID, &cp.TaskID, &cp.Branch, &cp.BaseCommit, &cp.HeadCommit, &cp.Dirty, &created); err != nil {
			return nil, err
		}
		cp.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		result = append(result, cp)
	}
	return result, rows.Err()
}
