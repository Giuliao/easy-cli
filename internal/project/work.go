package project

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrWorkNotBound = errors.New("worktree is not bound; set the project with easy project use <name> or easy work ensure --project <name>")
	ErrWorkConflict = errors.New("work conflict")
	ErrProjectEnded = errors.New("project is ended; ask the user whether to reopen it or start a different project")
)

// Checkpoint is one saved Git observation of a task. The task remains the
// persistent development record; checkpoints only append progress.
type Checkpoint struct {
	ID         int64     `json:"id"`
	TaskID     string    `json:"task_id"`
	Branch     string    `json:"branch"`
	BaseCommit string    `json:"base_commit"`
	HeadCommit string    `json:"head_commit"`
	Dirty      bool      `json:"dirty"`
	CreatedAt  time.Time `json:"created_at"`
}

// WorkState is the read-only view of the current binding for one worktree.
type WorkState struct {
	Project    Project     `json:"project"`
	Context    WorkContext `json:"context"`
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
}

type workQuery interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
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

// staleTaskMessage explains how to clear a pointer to a task that no longer
// exists without guessing which task should replace it.
func staleTaskMessage(taskID, projectName string) string {
	return fmt.Sprintf("context task %s no longer exists; run easy project use %s --replace to clear the stale pointer, then easy work ensure", taskID, projectName)
}

func attachTask(q workQuery, taskID, projectID string) error {
	if _, err := q.Exec(`INSERT INTO project_tasks (project_id, task_id) VALUES (?, ?)`, projectID, taskID); err != nil {
		return fmt.Errorf("attach task: %w", err)
	}
	return nil
}

func workState(q workQuery, ctx WorkContext, git GitState) (WorkState, error) {
	p, err := resolveContextProject(q, ctx)
	if err != nil {
		return WorkState{}, err
	}
	s := WorkState{Project: p, Context: ctx, Git: git, Issues: []string{}}
	if p.Status != StatusActive {
		s.Issues = append(s.Issues, ErrProjectEnded.Error())
	}
	if ctx.TaskID == "" {
		return s, nil
	}
	t, err := workTask(q, ctx.TaskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			s.Issues = append(s.Issues, staleTaskMessage(ctx.TaskID, p.Name))
			return s, nil
		}
		return s, err
	}
	s.Task = &t
	if err := checkTaskBinding(q, p, t, git); err != nil {
		s.Issues = append(s.Issues, err.Error())
		return s, nil
	}
	if _, _, err := taskProgress(q, t, git); err != nil {
		s.Issues = append(s.Issues, err.Error())
	}
	return s, nil
}

// resolveContextProject verifies the stable project ID and rejects a context
// whose name was reused by a different project.
func resolveContextProject(q workQuery, ctx WorkContext) (Project, error) {
	if ctx.ProjectID == "" {
		if ctx.ProjectName == "" {
			return Project{}, ErrWorkNotBound
		}
		return projectByName(q, ctx.ProjectName)
	}
	p, err := projectByID(q, ctx.ProjectID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return Project{}, fmt.Errorf("%w: context project %s no longer exists; run easy project use to rebind", ErrWorkConflict, ctx.ProjectID)
		}
		return Project{}, fmt.Errorf("context project %s: %w", ctx.ProjectID, err)
	}
	if ctx.ProjectName != "" && ctx.ProjectName != p.Name {
		return Project{}, fmt.Errorf("%w: context project %s is now named %q, not %q; run easy project use to rebind", ErrWorkConflict, p.ID, p.Name, ctx.ProjectName)
	}
	return p, nil
}

// checkTaskBinding verifies that a task still describes this project, worktree
// and branch. Git facts are authoritative; recorded values are expectations.
func checkTaskBinding(q workQuery, p Project, t Task, git GitState) error {
	projectID, err := taskProjectID(q, t.ID)
	if err != nil || projectID != p.ID {
		return fmt.Errorf("%w: task %s is not attached to project %q; inspect easy project show %s", ErrWorkConflict, t.ID, p.Name, p.Name)
	}
	dir, err := ReadGitState(t.Directory)
	if err != nil {
		return fmt.Errorf("%w: task %s directory %q is no longer a usable Git worktree: %v", ErrWorkConflict, t.ID, t.Directory, err)
	}
	if dir.Directory != git.Directory {
		return fmt.Errorf("%w: task %s belongs to worktree %q, not %q", ErrWorkConflict, t.ID, dir.Directory, git.Directory)
	}
	if t.Branch != git.Branch {
		return fmt.Errorf("%w: task %s recorded branch %q but this worktree is on %q; restore the branch or select another task", ErrWorkConflict, t.ID, t.Branch, git.Branch)
	}
	return nil
}

func CurrentWork(db *sql.DB, directory string) (WorkState, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return WorkState{}, err
	}
	ctx, err := ReadWorkContext(git.Directory)
	if err != nil {
		return WorkState{}, err
	}
	if ctx.empty() {
		return WorkState{}, ErrWorkNotBound
	}
	return workState(db, ctx, git)
}

// taskProgress preserves the original base and verifies both ancestry and the
// saved head, so a manual commit_range edit cannot silently reset the range.
func taskProgress(q workQuery, t Task, git GitState) (string, string, error) {
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
		err = q.QueryRow(`SELECT base_commit, head_commit FROM task_checkpoints WHERE task_id = ? ORDER BY id DESC LIMIT 1`, t.ID).Scan(&savedBase, &savedHead)
	}
	if err == nil && (savedBase != base || savedHead != previous) {
		return "", "", fmt.Errorf("%w: task %s range differs from saved progress; inspect easy work history --task %s instead of overwriting it", ErrWorkConflict, t.ID, t.ID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	if err := ValidateGitProgress(git.Directory, base, previous, git.Head); err != nil {
		return "", "", err
	}
	return base, previous, nil
}

// candidateTasks finds existing tasks attached to the project for this
// worktree and branch. The task is the continuous record for that repository.
func candidateTasks(q workQuery, projectID, directory, branch string) ([]Task, error) {
	rows, err := q.Query(`SELECT t.id, t.agent_type, t.session_id, t.branch, t.directory, t.commit_range, t.created_at, t.updated_at
		FROM tasks t JOIN project_tasks pt ON pt.task_id = t.id WHERE pt.project_id = ? ORDER BY t.created_at, t.id`, projectID)
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
		dir, err := ReadGitState(t.Directory)
		if err == nil && dir.Directory == directory && t.Branch == branch {
			tasks = append(tasks, t)
		}
	}
	return tasks, rows.Err()
}

// EnsureWork resumes the task that already represents this requirement in this
// worktree. It creates a task only when the project has no record for this
// directory and branch, never because a session or agent changed.
func EnsureWork(db *sql.DB, params EnsureWorkParams) (WorkState, error) {
	git, err := ReadGitState(params.Directory)
	if err != nil {
		return WorkState{}, err
	}
	ctx, err := ReadWorkContext(git.Directory)
	if err != nil {
		return WorkState{}, err
	}
	desired := ctx
	if params.ProjectName != "" {
		p, err := GetProjectByName(db, params.ProjectName)
		if err != nil {
			return WorkState{}, err
		}
		if p.Status != StatusActive {
			return WorkState{}, ErrProjectEnded
		}
		if ctx.ProjectID != "" && ctx.ProjectID != p.ID {
			return WorkState{}, fmt.Errorf("%w: worktree is bound to %q; confirm replacement with easy project use --replace", ErrWorkConflict, ctx.ProjectName)
		}
		if ctx.ProjectName != "" && ctx.ProjectName != p.Name {
			return WorkState{}, fmt.Errorf("%w: worktree is bound to %q; confirm replacement with easy project use --replace", ErrWorkConflict, ctx.ProjectName)
		}
		desired = WorkContext{ProjectID: p.ID, ProjectName: p.Name, TaskID: ctx.TaskID}
	}
	if desired.empty() {
		return WorkState{}, ErrWorkNotBound
	}
	if params.TaskID != "" {
		if ctx.TaskID != "" && ctx.TaskID != params.TaskID {
			return WorkState{}, fmt.Errorf("%w: worktree already resumes task %s; start a different binding only after confirming intent", ErrWorkConflict, ctx.TaskID)
		}
		desired.TaskID = params.TaskID
	}

	// Serialize selection and creation; concurrent sessions must not duplicate
	// the development record.
	tx, err := db.Begin()
	if err != nil {
		return WorkState{}, err
	}
	defer tx.Rollback()
	p, err := resolveContextProject(tx, desired)
	if err != nil {
		return WorkState{}, err
	}
	if p.Status != StatusActive {
		return WorkState{}, ErrProjectEnded
	}
	if desired.ProjectID == "" {
		desired.ProjectID = p.ID
	}
	s := WorkState{Project: p, Context: desired, Git: git, Issues: []string{}}
	var task Task
	switch {
	case desired.TaskID != "":
		task, err = workTask(tx, desired.TaskID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				return WorkState{}, fmt.Errorf("%w: %s", ErrWorkConflict, staleTaskMessage(desired.TaskID, p.Name))
			}
			return WorkState{}, err
		}
		if err := checkTaskBinding(tx, p, task, git); err != nil {
			return WorkState{}, err
		}
	default:
		candidates, err := candidateTasks(tx, p.ID, git.Directory, git.Branch)
		if err != nil {
			return WorkState{}, err
		}
		switch len(candidates) {
		case 0:
			task, err = createTask(tx, CreateTaskParams{AgentType: params.AgentType, Branch: git.Branch, Directory: git.Directory})
			if err != nil {
				return WorkState{}, err
			}
			if err := attachTask(tx, task.ID, p.ID); err != nil {
				return WorkState{}, err
			}
			if _, err := tx.Exec(`INSERT INTO work_task_progress (task_id, base_commit, head_commit) VALUES (?, ?, ?)`, task.ID, git.Head, git.Head); err != nil {
				return WorkState{}, err
			}
			task.CommitRange = git.Head + ".." + git.Head
			if _, err := tx.Exec(`UPDATE tasks SET commit_range = ? WHERE id = ?`, task.CommitRange, task.ID); err != nil {
				return WorkState{}, err
			}
			s.Created = true
		case 1:
			task = candidates[0]
		default:
			return WorkState{}, fmt.Errorf("%w: multiple tasks for this project and worktree; select one with --task (e.g. easy work ensure --task %s)", ErrWorkConflict, candidates[0].ID)
		}
		desired.TaskID = task.ID
	}
	base, previous, err := taskProgress(tx, task, git)
	if err != nil {
		return WorkState{}, err
	}
	if task.CommitRange != base+".."+previous {
		task.CommitRange = base + ".." + previous
		if _, err := tx.Exec(`UPDATE tasks SET commit_range = ? WHERE id = ?`, task.CommitRange, task.ID); err != nil {
			return WorkState{}, err
		}
	}
	snapshot, err := snapshotContext(git.Directory)
	if err != nil {
		return WorkState{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkState{}, err
	}
	if err := WriteWorkContext(git.Directory, desired); err != nil {
		_ = snapshot.restore()
		return WorkState{}, fmt.Errorf("save work context: %w", err)
	}
	s.Context = desired
	s.Task = &task
	return s, nil
}

// CheckpointWork records the current HEAD and dirty state for the bound task,
// advancing its head while keeping the original base. It never calls ensure.
func CheckpointWork(db *sql.DB, directory string) (WorkState, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return WorkState{}, err
	}
	ctx, err := ReadWorkContext(git.Directory)
	if err != nil {
		return WorkState{}, err
	}
	if ctx.empty() {
		return WorkState{}, ErrWorkNotBound
	}
	if ctx.TaskID == "" {
		return WorkState{}, fmt.Errorf("%w: worktree has no current task; run easy work ensure first", ErrWorkConflict)
	}
	tx, err := db.Begin()
	if err != nil {
		return WorkState{}, err
	}
	defer tx.Rollback()
	p, err := resolveContextProject(tx, ctx)
	if err != nil {
		return WorkState{}, err
	}
	if p.Status != StatusActive {
		return WorkState{}, ErrProjectEnded
	}
	t, err := workTask(tx, ctx.TaskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return WorkState{}, fmt.Errorf("%w: %s", ErrWorkConflict, staleTaskMessage(ctx.TaskID, p.Name))
		}
		return WorkState{}, err
	}
	if err := checkTaskBinding(tx, p, t, git); err != nil {
		return WorkState{}, err
	}
	base, previous, err := taskProgress(tx, t, git)
	if err != nil {
		return WorkState{}, err
	}
	if previous != git.Head {
		if err := ValidateGitProgress(git.Directory, base, previous, git.Head); err != nil {
			return WorkState{}, err
		}
	}
	s := WorkState{Project: p, Context: ctx, Git: git, Issues: []string{}}
	cp := Checkpoint{TaskID: t.ID, Branch: git.Branch, BaseCommit: base, HeadCommit: git.Head, Dirty: git.Dirty, CreatedAt: time.Now().UTC()}
	result, err := tx.Exec(`INSERT INTO task_checkpoints (task_id, branch, base_commit, head_commit, dirty, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		cp.TaskID, cp.Branch, cp.BaseCommit, cp.HeadCommit, cp.Dirty, cp.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return s, err
	}
	if cp.ID, err = result.LastInsertId(); err != nil {
		return s, err
	}
	if _, err := tx.Exec(`INSERT INTO work_task_progress (task_id, base_commit, head_commit) VALUES (?, ?, ?) ON CONFLICT(task_id) DO UPDATE SET head_commit = excluded.head_commit`, t.ID, base, git.Head); err != nil {
		return s, err
	}
	t.CommitRange = base + ".." + git.Head
	t.UpdatedAt = cp.CreatedAt
	if _, err := tx.Exec(`UPDATE tasks SET commit_range = ?, updated_at = ? WHERE id = ?`, t.CommitRange, cp.CreatedAt.Format(time.RFC3339), t.ID); err != nil {
		return s, err
	}
	if err := tx.Commit(); err != nil {
		return s, err
	}
	s.Task = &t
	s.Checkpoint = &cp
	return s, nil
}

// WorkHistory returns the saved checkpoints of the task bound to this worktree.
func WorkHistory(db *sql.DB, directory string) ([]Checkpoint, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return nil, err
	}
	ctx, err := ReadWorkContext(git.Directory)
	if err != nil {
		return nil, err
	}
	if ctx.TaskID == "" {
		return nil, fmt.Errorf("%w: worktree has no current task; run easy work ensure first", ErrWorkNotBound)
	}
	return WorkHistoryByTask(db, ctx.TaskID)
}

// WorkHistoryByTask reads a task's history without requiring its worktree to
// be present, which is how another session can inspect a repository it does
// not have checked out.
func WorkHistoryByTask(db *sql.DB, taskID string) ([]Checkpoint, error) {
	if _, err := workTask(db, taskID); err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil, fmt.Errorf("task %q: %w", taskID, err)
		}
		return nil, err
	}
	rows, err := db.Query(`SELECT id, task_id, branch, base_commit, head_commit, dirty, created_at FROM task_checkpoints WHERE task_id = ? ORDER BY id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Checkpoint{}
	for rows.Next() {
		var cp Checkpoint
		var created string
		if err := rows.Scan(&cp.ID, &cp.TaskID, &cp.Branch, &cp.BaseCommit, &cp.HeadCommit, &cp.Dirty, &created); err != nil {
			return nil, err
		}
		if cp.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, err
		}
		result = append(result, cp)
	}
	return result, rows.Err()
}

// BindProject sets the project for this worktree. Rebinding the same project
// keeps the current task; replacing a different binding requires confirmation
// and clears the task pointer, leaving records intact for explicit selection.
func BindProject(db *sql.DB, directory, name string, replace bool) (WorkState, error) {
	git, err := ReadGitState(directory)
	if err != nil {
		return WorkState{}, err
	}
	ctx, err := ReadWorkContext(git.Directory)
	if err != nil {
		return WorkState{}, err
	}
	p, err := GetProjectByName(db, name)
	if err != nil {
		return WorkState{}, err
	}
	if p.Status != StatusActive {
		return WorkState{}, fmt.Errorf("%w: project %q is %s", ErrProjectEnded, p.Name, p.Status)
	}
	desired := WorkContext{ProjectID: p.ID, ProjectName: p.Name}
	different := !ctx.empty() && (ctx.ProjectID != p.ID || (ctx.ProjectID == "" && ctx.ProjectName != p.Name))
	if different && !replace {
		return WorkState{}, fmt.Errorf("%w: worktree is bound to %q; confirm replacement with --replace, which clears the current task pointer", ErrWorkConflict, ctx.ProjectName)
	}
	if !different && !replace {
		desired.TaskID = ctx.TaskID
	}
	if err := WriteWorkContext(git.Directory, desired); err != nil {
		return WorkState{}, fmt.Errorf("save work context: %w", err)
	}
	s := WorkState{Project: p, Context: desired, Git: git, Issues: []string{}}
	if desired.TaskID != "" {
		t, err := workTask(db, desired.TaskID)
		if err == nil {
			s.Task = &t
		}
	}
	return s, nil
}
